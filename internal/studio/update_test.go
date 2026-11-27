package studio

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"
)

func TestUpdateWaitsForGenerationAndCanResume(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	original := originalPNG(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-finish
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"b64_json": base64.StdEncoding.EncodeToString(original),
		}}})
	}))
	defer provider.Close()
	app, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	configureTestModel(t, app, provider.URL)
	session, _ := app.store.createSession()
	input := GenerationInput{ModelID: "test", Prompt: "保留原任务", Options: Options{Count: 1}}
	path := "/api/sessions/" + session.ID + "/turns"
	if response := localRequest(t, app, "POST", path, input); response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	<-started
	drained := make(chan struct{})
	go func() { app.WaitForUpdate(); close(drained) }()
	for {
		app.store.mu.Lock()
		updating := app.updating
		app.store.mu.Unlock()
		if updating {
			break
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-drained:
		t.Fatal("update did not wait for the running generation")
	default:
	}
	if response := localRequest(t, app, "POST", path, input); response.Code != http.StatusConflict {
		t.Fatal("a new generation started while draining")
	}
	close(finish)
	<-drained
	saved, _ := app.store.session(session.ID)
	if saved.Turns[0].Status != "completed" || len(saved.Turns[0].Outputs) != 1 {
		t.Fatal("updating interrupted the generation or lost its output")
	}
	app.ResumeAfterUpdate()
	if response := localRequest(t, app, "POST", path, input); response.Code != http.StatusAccepted {
		t.Fatal("failed update left generation disabled")
	}
	app.wg.Wait()
}

func TestUpdateEndpointUsesLocalOriginProtection(t *testing.T) {
	app, _ := New(t.TempDir())
	defer app.Close()
	calls := 0
	handler := app.Handler(fstest.MapFS{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusAccepted)
	}), "http://mygo.localhost")
	for _, origin := range []string{"https://unrelated.example", "http://mygo.localhost"} {
		request := httptest.NewRequest("POST", "mygo://localhost/api/update", nil)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if origin == "https://unrelated.example" && response.Code != http.StatusForbidden {
			t.Fatal("another website could trigger a client update")
		}
		if origin == "http://mygo.localhost" && response.Code != http.StatusAccepted {
			t.Fatal("the local UI could not trigger an update")
		}
	}
	if calls != 1 {
		t.Fatalf("unexpected number of authorized update requests: %d", calls)
	}
}
