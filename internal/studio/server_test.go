package studio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

func originalPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, 1280, 720))
	picture.SetNRGBA(10, 10, color.NRGBA{R: 70, G: 110, B: 42, A: 140})
	var output bytes.Buffer
	if err := png.Encode(&output, picture); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func localRequest(t *testing.T, app *App, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, "mygo://localhost"+path, &encoded)
	response := httptest.NewRecorder()
	app.Handler(fstest.MapFS{}, nil, "http://mygo.localhost").ServeHTTP(response, request)
	return response
}

func configureTestModel(t *testing.T, app *App, endpoint string) {
	t.Helper()
	response := localRequest(t, app, "PUT", "/api/config", Config{Models: []Model{{
		ID: "test", Name: "Test", ModelName: "doubao-seedream-5.5", APIKey: "test-secret", URL: endpoint,
	}}})
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("test-secret")) {
		t.Fatal("configuration response exposed the API key")
	}
}

func TestOriginalImageUploadReferenceOutputAndDownload(t *testing.T) {
	original := originalPNG(t)
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		expected := "data:image/png;base64," + base64.StdEncoding.EncodeToString(original)
		if body["image"] != expected || body["prompt"] != "把图 1 的绿色改成蓝色" {
			t.Error("reference original or prompt was changed")
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"b64_json": base64.StdEncoding.EncodeToString(original), "size": "1280x720", "background": "transparent",
		}}})
	}))
	defer provider.Close()
	app, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	configureTestModel(t, app, provider.URL)
	created := localRequest(t, app, "POST", "/api/sessions", nil)
	var session Session
	json.Unmarshal(created.Body.Bytes(), &session)
	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	part, _ := writer.CreateFormFile("file", "参考原图.png")
	part.Write(original)
	writer.Close()
	request := httptest.NewRequest("POST", "mygo://localhost/api/sessions/"+session.ID+"/assets", &multipartBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://mygo.localhost")
	response := httptest.NewRecorder()
	app.Handler(fstest.MapFS{}, nil, "http://mygo.localhost").ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatal(response.Body.String())
	}
	var reference Asset
	json.Unmarshal(response.Body.Bytes(), &reference)
	input := GenerationInput{ModelID: "test", Prompt: "把图 1 的绿色改成蓝色", ReferenceIDs: []string{reference.ID}, Options: Options{Count: 2}}
	response = localRequest(t, app, "POST", "/api/sessions/"+session.ID+"/turns", input)
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	app.wg.Wait()
	stored, _ := app.store.session(session.ID)
	turn := stored.Turns[0]
	if calls.Load() != 2 || turn.Status != "completed" || len(turn.Outputs) != 2 {
		t.Fatalf("unexpected batch result: calls=%d, turn=%+v", calls.Load(), turn)
	}
	for _, output := range turn.Outputs {
		response = localRequest(t, app, "GET", "/api/sessions/"+session.ID+"/assets/"+output.AssetID+"?download=1", nil)
		if !bytes.Equal(response.Body.Bytes(), original) {
			t.Fatal("downloaded original bytes changed")
		}
	}
	encoded, _ := os.ReadFile(filepath.Join(app.Directory(), "sessions", session.ID, "session.json"))
	if bytes.Contains(encoded, []byte("base64")) || bytes.Contains(encoded, []byte("test-secret")) || !bytes.Contains(encoded, []byte("sources/")) {
		t.Fatal("session must store paths, never encoded images or API keys")
	}
}

func TestBatchKeepsSuccessAndDisplaysProviderErrorWithoutRetry(t *testing.T) {
	original := originalPNG(t)
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(original)}}})
			return
		}
		w.Header().Set("X-Request-Id", "request-123")
		w.WriteHeader(429)
		io.WriteString(w, `{"error":{"code":"RateLimitExceeded","message":"额度不足 test-secret"}}`)
	}))
	defer provider.Close()
	app, _ := New(t.TempDir())
	defer app.Close()
	configureTestModel(t, app, provider.URL)
	session, _ := app.store.createSession()
	response := localRequest(t, app, "POST", "/api/sessions/"+session.ID+"/turns", GenerationInput{ModelID: "test", Prompt: "山间日出", Options: Options{Count: 3}})
	if response.Code != 202 {
		t.Fatal(response.Body.String())
	}
	app.wg.Wait()
	session, _ = app.store.session(session.ID)
	turn := session.Turns[0]
	if calls.Load() != 2 || len(turn.Outputs) != 1 || turn.Completed != 1 || turn.Status != "failed" {
		t.Fatalf("partial result lost or request retried: %+v", turn)
	}
	if turn.Error.Code != "RateLimitExceeded" || turn.Error.Status != 429 || turn.Error.RequestID != "request-123" || bytes.Contains([]byte(turn.Error.Message), []byte("test-secret")) {
		t.Fatalf("incorrect public error: %+v", turn.Error)
	}
}

func TestRestartPreservesResultsAndMarksUnfinishedTurn(t *testing.T) {
	directory := t.TempDir()
	store, _ := newStore(directory)
	session, _ := store.createSession()
	session.Turns = []Turn{{ID: "unfinished", Status: "running", Outputs: []Output{{AssetID: "saved-original"}}}}
	if err := store.saveSession(session); err != nil {
		t.Fatal(err)
	}
	app, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	recovered, _ := app.store.session(session.ID)
	if recovered.Turns[0].Status != "interrupted" || recovered.Turns[0].Outputs[0].AssetID != "saved-original" {
		t.Fatalf("incorrect restart state: %+v", recovered)
	}
}

func TestLocalOriginAndConfigKeyRetention(t *testing.T) {
	app, _ := New(t.TempDir())
	defer app.Close()
	configureTestModel(t, app, "https://ark.cn-beijing.volces.com/api/plan/v3/images/generations")
	response := localRequest(t, app, "GET", "/api/config", nil)
	var config Config
	json.Unmarshal(response.Body.Bytes(), &config)
	if !config.Models[0].HasKey || config.Models[0].APIKey != "" {
		t.Fatal("public config should expose key presence only")
	}
	config.Models[0].Name = "Updated"
	response = localRequest(t, app, "PUT", "/api/config", config)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	saved, _ := app.store.config()
	if saved.Models[0].APIKey != "test-secret" {
		t.Fatal("empty key update erased the stored key")
	}
	request := httptest.NewRequest("POST", "mygo://localhost/api/sessions", nil)
	request.Header.Set("Origin", "https://unrelated.example")
	response = httptest.NewRecorder()
	app.Handler(fstest.MapFS{}, nil, "http://mygo.localhost").ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatal("cross-origin local data modification was accepted")
	}
}
