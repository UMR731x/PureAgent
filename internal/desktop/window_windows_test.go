package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/egoist/mygo"
)

func TestMain(tests *testing.M) {
	// MyGo's event loop must run on the main goroutine, including in this subprocess.
	if os.Getenv("PUREAGENT_TEST_WINDOW_STATE") == "1" {
		if err := checkWorkspaceWindowState(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(tests.Run())
}

func TestWorkspaceActivationPreservesWindowState(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestWorkspaceActivationPreservesWindowState$")
	command.Env = append(os.Environ(), "PUREAGENT_TEST_WINDOW_STATE=1", "WEBVIEW2_USER_DATA_FOLDER="+t.TempDir())
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("workspace activation: %v\n%s", err, output)
	}
}

func checkWorkspaceWindowState() error {
	mygo.App.SetName("PureAgent window state test")
	var failure error
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{Hidden: true, Width: 1024, Height: 720})
		defer mygo.App.Quit()
		defer window.Destroy()
		window.ShowInactive()
		window.Focus()
		normalBounds := window.Bounds()
		cases := []struct {
			name                       string
			maximized, minimized, hide bool
		}{
			{name: "maximized then minimized", maximized: true, minimized: true},
			{name: "already maximized", maximized: true},
			{name: "maximized then hidden", maximized: true, hide: true},
			{name: "normal then minimized", minimized: true},
			{name: "normal then hidden", hide: true},
			{name: "already normal"},
		}
		for _, state := range cases {
			window.Restore()
			window.Unmaximize()
			window.Show()
			window.SetBounds(normalBounds)
			if state.maximized {
				window.Maximize()
			}
			if state.minimized {
				window.Minimize()
			}
			if state.hide {
				window.Hide()
			}
			showWorkspace(window)
			if !window.IsVisible() || window.IsMinimized() || window.IsMaximized() != state.maximized {
				failure = fmt.Errorf("%s: visible=%v minimized=%v maximized=%v", state.name,
					window.IsVisible(), window.IsMinimized(), window.IsMaximized())
				return
			}
			if !state.maximized && window.Bounds() != normalBounds {
				failure = fmt.Errorf("%s: bounds changed from %+v to %+v", state.name, normalBounds, window.Bounds())
				return
			}
		}
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return failure
}
