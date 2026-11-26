package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fixtureStatus struct {
	PID       int      `json:"pid"`
	URL       string   `json:"url"`
	Arguments []string `json:"arguments"`
	Directory string   `json:"directory"`
	Stage     string   `json:"stage"`
}

type releaseTransport struct{ endpoint string }

func (transport releaseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.String() == releaseEndpoint {
		request, _ = http.NewRequestWithContext(request.Context(), request.Method, transport.endpoint, nil)
	}
	return http.DefaultTransport.RoundTrip(request)
}

// The test binary also acts as a real installed client, so Windows executable locks are exercised.
func TestMain(tests *testing.M) {
	arguments := os.Args[1:]
	if len(arguments) == 2 && arguments[0] == HelperFlag {
		if err := RunHelper(arguments[1]); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	var restarted *Restart
	if len(arguments) == 2 && arguments[0] == RestartFlag {
		var err error
		restarted, err = ReadRestart(arguments[1])
		if err != nil {
			os.Exit(1)
		}
		arguments = restarted.Arguments
	}
	if len(arguments) > 0 && arguments[0] == "--update-fixture" {
		if err := runFixture(arguments, restarted); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(tests.Run())
}

func runFixture(arguments []string, restarted *Restart) error {
	if restarted != nil && restarted.Failure == "" && arguments[7] == "client exits on startup" {
		return fmt.Errorf("fixture startup failed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	stop := make(chan struct{})
	updater := New(context.Background(), "v0.1.0", arguments, Lifecycle{
		WaitForJobs: func() {}, ResumeJobs: func() {}, Restart: func() { close(stop) },
	})
	updater.client.Transport = releaseTransport{endpoint: arguments[2]}
	if restarted != nil && restarted.Failure != "" {
		updater.Failure(restarted.Failure)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/update", updater.Handler())
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) { close(stop) })
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	status := fixtureStatus{PID: os.Getpid(), URL: "http://" + listener.Addr().String(), Arguments: arguments}
	status.Directory, err = os.Getwd()
	if err != nil {
		return err
	}
	if restarted != nil {
		status.Stage = restarted.stage.directory
		if err := restarted.Ready(updater.Failure); err != nil {
			return err
		}
	}
	encoded, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(arguments[1], "client.json"), encoded, 0600); err != nil {
		return err
	}
	<-stop
	err = server.Shutdown(context.Background())
	runtime.KeepAlive(updater)
	return err
}

func TestVersionOrdering(t *testing.T) {
	for _, test := range []struct {
		latest, current string
		want            bool
	}{{"v0.10.0", "v0.9.9", true}, {"v0.1.0", "v0.1.0", false}, {"v0.1.0", "v0.2.0", false}, {"v1.0.0", "dev", true}} {
		actual, err := newerVersion(test.latest, test.current)
		if err != nil || actual != test.want {
			t.Fatalf("%s vs %s: newer=%v, error=%v", test.latest, test.current, actual, err)
		}
	}
}

func TestInstalledClientUpdate(t *testing.T) {
	for _, scenario := range []string{"successive updates", "unstartable download", "client exits on startup", "corrupt download"} {
		t.Run(scenario, func(t *testing.T) { testInstalledClient(t, scenario) })
	}
}

func testInstalledClient(t *testing.T, scenario string) {
	root := t.TempDir()
	clientDirectory := filepath.Join(root, "client with spaces")
	workingDirectory := filepath.Join(root, "working directory")
	dataDirectory := filepath.Join(workingDirectory, "my images")
	for _, directory := range []string{clientDirectory, dataDirectory} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	originals := seedUserFiles(t, dataDirectory)
	clientPath := filepath.Join(clientDirectory, executableName("PureAgent"))
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(testBinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clientPath, binary, 0755); err != nil {
		t.Fatal(err)
	}
	downloaded := binary
	if scenario == "unstartable download" {
		downloaded = []byte("this is not an executable")
	}
	server := releaseServer(t, downloaded, scenario == "corrupt download")
	arguments := []string{"--update-fixture", root, server.URL, "-data", "my images", "-port", "57135", scenario}
	command := exec.Command(clientPath, arguments...)
	command.Dir = workingDirectory
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Wait()
	defer command.Process.Kill()
	t.Cleanup(func() {
		status := readFixtureStatus(root)
		if status.URL != "" {
			wait, _ := waitForParent(status.PID)
			response, err := http.Post(status.URL+"/stop", "application/json", nil)
			if err == nil {
				response.Body.Close()
			}
			if wait != nil {
				wait()
			}
		}
	})
	waitUntil(t, func() bool { return readFixtureStatus(root).PID != 0 })
	status := readFixtureStatus(root)
	cycles := 1
	if scenario == "successive updates" {
		cycles = 2 // A restarted client must keep the original arguments for its next update.
	}
	for range cycles {
		status = updateFixture(t, root, status, scenario)
	}
	if status.Directory != workingDirectory || strings.Join(status.Arguments, "|") != strings.Join(arguments, "|") {
		t.Fatalf("restart changed the working directory or arguments: %+v", status)
	}
	for path, expected := range originals {
		actual, err := os.ReadFile(path)
		if err != nil || string(actual) != expected {
			t.Fatalf("existing configuration, session or original was changed: %s (%v)", path, err)
		}
	}
}

func seedUserFiles(t *testing.T, directory string) map[string]string {
	t.Helper()
	files := map[string]string{
		filepath.Join(directory, "config.json"):                                  `{"models":[{"api_key":"fixture-key"}]}`,
		filepath.Join(directory, "sessions", "saved", "session.json"):            `{"title":"已有创作","assets":[{"path":"sources/original.png"}]}`,
		filepath.Join(directory, "sessions", "saved", "sources", "original.png"): "original-image-bytes\x00\xff",
	}
	for path, contents := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func releaseServer(t *testing.T, binary []byte, corrupt bool) *httptest.Server {
	t.Helper()
	digest := sha256.Sum256(binary) // Integrity is required for executing a downloaded release.
	asset := releaseAsset{Name: "PureAgent-" + runtime.GOOS + "-" + runtime.GOARCH, Digest: "sha256:" + hex.EncodeToString(digest[:])}
	if runtime.GOOS == "windows" {
		asset.Name += ".exe"
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/binary" {
			if corrupt {
				io.WriteString(w, "truncated download")
			} else {
				w.Write(binary)
			}
			return
		}
		available := asset
		available.URL = server.URL + "/binary"
		json.NewEncoder(w).Encode(release{Tag: "v0.2.0", URL: "https://github.com/UMR731x/PureAgent/releases/tag/v0.2.0", Assets: []releaseAsset{available}})
	}))
	t.Cleanup(server.Close)
	return server
}

func updateFixture(t *testing.T, root string, previous fixtureStatus, scenario string) fixtureStatus {
	t.Helper()
	response, err := http.Post(previous.URL+"/api/update", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if scenario == "corrupt download" {
		waitUntil(t, func() bool { return fixtureUpdateState(previous.URL).Phase == "error" })
		if !strings.Contains(fixtureUpdateState(previous.URL).Message, "校验") {
			t.Fatal("corrupt download was not rejected before replacing the executable")
		}
		return readFixtureStatus(root)
	}
	waitUntil(t, func() bool {
		status := readFixtureStatus(root)
		return status.PID != 0 && status.PID != previous.PID
	})
	status := readFixtureStatus(root)
	waitUntil(t, func() bool { _, err := os.Stat(status.Stage); return os.IsNotExist(err) })
	state := fixtureUpdateState(status.URL)
	if scenario == "unstartable download" || scenario == "client exits on startup" {
		if state.Phase != "error" || !strings.Contains(state.Message, "已恢复原版本") {
			t.Fatalf("failed startup did not restore the original client: %+v", state)
		}
	} else if state.Phase != "idle" {
		t.Fatalf("new client is not usable: %+v", state)
	}
	return status
}

func readFixtureStatus(directory string) fixtureStatus {
	encoded, _ := os.ReadFile(filepath.Join(directory, "client.json"))
	var status fixtureStatus
	json.Unmarshal(encoded, &status)
	return status
}

func fixtureUpdateState(url string) State {
	response, err := http.Get(url + "/api/update")
	if err != nil {
		return State{}
	}
	defer response.Body.Close()
	var state State
	json.NewDecoder(response.Body).Decode(&state)
	return state
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	for !condition() {
		if t.Context().Err() != nil {
			t.Fatal(t.Context().Err())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
