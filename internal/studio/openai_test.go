package studio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAIImagesGenerationAndEditsPreserveOriginals(t *testing.T) {
	for _, references := range []int{0, 3, 16} {
		t.Run(strconv.Itoa(references)+" references", func(t *testing.T) {
			testOpenAIImages(t, references)
		})
	}
}

func testOpenAIImages(t *testing.T, references int) {
	original := originalPNG(t)
	markup := originalPNG(t)
	prompt := "  修改红框里的颜色\n保留原画面的其他细节  "
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fields, files := readOpenAIRequest(t, r)
		path := "/v1/images/generations"
		if references > 0 {
			path = "/v1/images/edits"
		}
		if r.URL.Path != path || r.Header.Get("Authorization") != "Bearer openai-fixture-key" {
			t.Errorf("incorrect endpoint or authorization: %s", r.URL.Path)
		}
		if fields["model"] != "gpt-image-2.6-flare" || fields["prompt"] != prompt || fields["size"] != "auto" || fields["quality"] != "xhigh" || fields["complexity"] != "low" {
			t.Errorf("OpenAI parameters were changed: %+v", fields)
		}
		for _, name := range []string{"response_format", "watermark", "layer_decomposition", "input_fidelity", "system", "messages"} {
			if _, exists := fields[name]; exists {
				t.Errorf("unsupported field reached OpenAI: %s", name)
			}
		}
		if len(files["image[]"]) != references {
			t.Errorf("expected %d originals, got %d", references, len(files["image[]"]))
		}
		for _, image := range files["image[]"] {
			if !bytes.Equal(image, original) {
				t.Error("reference original was altered")
			}
		}
		if references > 0 && (len(files["markup"]) != 1 || !bytes.Equal(files["markup"][0], markup)) {
			t.Error("separate markup file was not forwarded intact")
		}
		w.Header().Set("X-Request-Id", "openai-request")
		json.NewEncoder(w).Encode(map[string]any{
			"data":          []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(original)}},
			"output_format": "png", "size": "1280x720", "background": "transparent", "usage": map[string]int{"total_tokens": 10},
		})
	}))
	defer provider.Close()
	app, _ := New(t.TempDir())
	defer app.Close()
	configureOpenAI(t, app, provider.URL+"/v1/images/generations")
	session, input := openAIInput(t, app, references, original, markup)
	input.Prompt, input.Options.Count = prompt, 2
	response := localRequest(t, app, "POST", "/api/sessions/"+session.ID+"/turns", input)
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	app.wg.Wait()
	saved, _ := app.store.session(session.ID)
	turn := saved.Turns[0]
	if turn.Status != "completed" || calls.Load() != 2 || turn.Completed != 2 || len(turn.Outputs) != 2 || len(turn.Usage) != 2 {
		t.Fatalf("incorrect result: %+v, requests=%d", turn, calls.Load())
	}
	for _, output := range turn.Outputs {
		response := localRequest(t, app, "GET", "/api/sessions/"+session.ID+"/assets/"+output.AssetID+"?download=1", nil)
		if !bytes.Equal(response.Body.Bytes(), original) || output.Size != "1280x720" || output.Background != "transparent" {
			t.Fatal("output original or OpenAI response metadata was lost")
		}
	}
	encoded, _ := os.ReadFile(filepath.Join(app.Directory(), "sessions", session.ID, "session.json"))
	if bytes.Contains(encoded, []byte("base64")) || bytes.Contains(encoded, []byte("openai-fixture-key")) || !bytes.Contains(encoded, []byte("sources/")) {
		t.Fatal("session should reference originals by path and exclude encoded bytes and API keys")
	}
}

func readOpenAIRequest(t *testing.T, r *http.Request) (map[string]string, map[string][][]byte) {
	t.Helper()
	fields, files := map[string]string{}, map[string][][]byte{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			t.Error(err)
		}
		return fields, files
	}
	reader, err := r.MultipartReader()
	if err != nil {
		t.Error(err)
		return fields, files
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Error(err)
			break
		}
		content, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			t.Error(err)
		}
		if part.FileName() == "" {
			fields[part.FormName()] = string(content)
		} else {
			if part.Header.Get("Content-Type") != "image/png" {
				t.Error("multipart image MIME was lost")
			}
			files[part.FormName()] = append(files[part.FormName()], content)
		}
	}
	return fields, files
}

func configureOpenAI(t *testing.T, app *App, endpoint string) {
	t.Helper()
	response := localRequest(t, app, "PUT", "/api/config", Config{Models: []Model{{
		ID: "openai", Provider: "openai", Name: "GPT Image", ModelName: "gpt-image-2.6-flare",
		APIKey: "openai-fixture-key", URL: endpoint,
	}}})
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
}

func openAIInput(t *testing.T, app *App, references int, original, markup []byte) (*Session, GenerationInput) {
	t.Helper()
	session, _ := app.store.createSession()
	input := GenerationInput{ModelID: "openai", Options: Options{Count: 1, Quality: "xhigh", Complexity: "low", Background: "transparent"}}
	for range references {
		asset, err := app.store.writeAsset(session.ID, "original.png", "upload", original)
		if err != nil {
			t.Fatal(err)
		}
		session.Assets = append(session.Assets, asset)
		input.ReferenceIDs = append(input.ReferenceIDs, asset.ID)
	}
	if references > 0 {
		asset, err := app.store.writeAsset(session.ID, "markup.png", "upload", markup)
		if err != nil {
			t.Fatal(err)
		}
		session.Assets = append(session.Assets, asset)
		input.MarkupID = asset.ID
	}
	if err := app.store.saveSession(session); err != nil {
		t.Fatal(err)
	}
	return session, input
}

func TestOpenAIEndpointKeepsCustomPrefixAndQuery(t *testing.T) {
	for _, endpoint := range []string{
		"https://proxy.example/team/v1", "https://proxy.example/team/v1/", "https://proxy.example/team/v1/images",
		"https://proxy.example/team/v1/images/generations", "https://proxy.example/team/v1/images/edits",
	} {
		for _, editing := range []bool{false, true} {
			suffix := "/images/generations?account=test"
			if editing {
				suffix = "/images/edits?account=test"
			}
			actual, err := openAIEndpoint(endpoint+"?account=test", editing)
			if err != nil || actual != "https://proxy.example/team/v1"+suffix {
				t.Fatalf("endpoint %s became %s (%v)", endpoint, actual, err)
			}
		}
	}
	actual, _ := openAIEndpoint("https://api.openai.com", false)
	if actual != "https://api.openai.com/v1/images/generations" {
		t.Fatal(actual)
	}
}

func TestOpenAIDocumentedSizesAndFormats(t *testing.T) {
	for _, size := range []string{"auto", "1024x1024", "1536x1024", "1024x1536", "1536x864", "864x1536", "2048x2048", "2560x1440", "1440x2560", "3840x2160", "2160x3840", "480x1440", "1024x640"} {
		for _, format := range []string{"png", "jpeg", "webp"} {
			input := GenerationInput{Prompt: "有效的图像请求", Options: Options{Size: size, Format: format}}
			if err := validateOpenAIInput(&input); err != nil {
				t.Fatalf("valid request %s %s rejected: %v", size, format, err)
			}
		}
	}
	for _, size := range []string{"1025x1024", "3840x3840", "4000x2000", "1024x256", "0x1024", "512x512"} {
		if validateOpenAISize(size) == nil {
			t.Errorf("unsupported size accepted: %s", size)
		}
	}
	session := &Session{Assets: []Asset{{ID: "large", MIME: "image/webp", Bytes: 40 * 1024 * 1024}}}
	if err := validateOpenAIReferences(session, GenerationInput{ReferenceIDs: []string{"large"}}); err != nil {
		t.Fatal("a valid OpenAI reference was rejected by the Seedream limit:", err)
	}
}

func TestOpenAIErrorsAndProviderKeySeparation(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "request-openai-error")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"invalid_size","message":"invalid size; openai-fixture-key"}}`)
	}))
	defer provider.Close()
	app, _ := New(t.TempDir())
	defer app.Close()
	configureOpenAI(t, app, provider.URL+"/v1")
	session, _ := app.store.createSession()
	response := localRequest(t, app, "POST", "/api/sessions/"+session.ID+"/turns", GenerationInput{ModelID: "openai", Prompt: "测试错误"})
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	app.wg.Wait()
	saved, _ := app.store.session(session.ID)
	failure := saved.Turns[0].Error
	if failure == nil || failure.Status != 400 || failure.Code != "invalid_size" || failure.RequestID != "request-openai-error" || strings.Contains(failure.Message, "openai-fixture-key") {
		t.Fatalf("OpenAI error details or key redaction lost: %+v", failure)
	}
	before, _ := app.store.config()
	changed := Config{Models: append([]Model(nil), before.Models...)}
	changed.Models[0].Provider, changed.Models[0].APIKey = "seedream", ""
	response = localRequest(t, app, "PUT", "/api/config", changed)
	after, _ := app.store.config()
	if response.Code != http.StatusBadRequest || !reflect.DeepEqual(before, after) {
		t.Fatal("changing provider reused or discarded a credential without a new key")
	}
}
