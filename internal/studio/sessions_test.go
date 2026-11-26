package studio

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteSessionsRemovesOriginalsAndPreservesOtherData(t *testing.T) {
	app, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	configureTestModel(t, app, "https://example.com/images/generations")
	original := originalPNG(t)
	var sessions []*Session
	for range 4 {
		session, err := app.store.createSession()
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"upload", "generated"} {
			asset, err := app.store.writeAsset(session.ID, kind+".png", kind, original)
			if err != nil {
				t.Fatal(err)
			}
			session.Assets = append(session.Assets, asset)
		}
		if err := app.store.saveSession(session); err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, session)
	}
	for _, ids := range [][]string{{sessions[0].ID}, {sessions[1].ID, sessions[2].ID}} {
		response := localRequest(t, app, "DELETE", "/api/sessions", map[string]any{"ids": ids})
		var result sessionDeletion
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || len(result.Deleted) != len(ids) || len(result.Failed) != 0 {
			t.Fatalf("delete failed: %s", response.Body.String())
		}
		for _, id := range ids {
			directory, _ := app.store.sessionPath(id)
			if _, err := os.Stat(directory); !os.IsNotExist(err) {
				t.Fatalf("session directory remains: %v", err)
			}
			if response := localRequest(t, app, "GET", "/api/sessions/"+id, nil); response.Code != http.StatusNotFound {
				t.Fatal("deleted session is still readable")
			}
		}
	}
	keeper := sessions[3]
	for _, asset := range keeper.Assets {
		response := localRequest(t, app, "GET", "/api/sessions/"+keeper.ID+"/assets/"+asset.ID, nil)
		if response.Code != http.StatusOK || response.Body.Len() != len(original) {
			t.Fatal("unselected session lost an image")
		}
	}
	config, err := app.store.config()
	if err != nil || len(config.Models) != 1 || config.Models[0].ID != "test" || config.Models[0].APIKey != "test-secret" {
		t.Fatal("model configuration changed")
	}
	listed, err := app.store.sessions()
	if err != nil || len(listed) != 1 || listed[0].ID != keeper.ID {
		t.Fatalf("history did not remove selected sessions: %v", err)
	}
}

func TestDeleteSessionsPreservesRunningGeneration(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	original := originalPNG(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-finish:
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(original)}}})
		case <-r.Context().Done():
		}
	}))
	defer provider.Close()
	app, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	configureTestModel(t, app, provider.URL)
	running, _ := app.store.createSession()
	completed, _ := app.store.createSession()
	response := localRequest(t, app, "POST", "/api/sessions/"+running.ID+"/turns", GenerationInput{ModelID: "test", Prompt: "保留正在生成的任务", Options: Options{Count: 1}})
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	<-started
	response = localRequest(t, app, "DELETE", "/api/sessions", map[string]any{"ids": []string{completed.ID, running.ID}})
	var result sessionDeletion
	json.Unmarshal(response.Body.Bytes(), &result)
	if len(result.Deleted) != 1 || result.Deleted[0] != completed.ID || len(result.Failed) != 1 || result.Failed[0].ID != running.ID {
		t.Fatalf("incorrect partial deletion: %s", response.Body.String())
	}
	close(finish)
	app.wg.Wait()
	stored, err := app.store.session(running.ID)
	if err != nil || len(stored.Turns) != 1 || stored.Turns[0].Status != "completed" || len(stored.Assets) != 1 {
		t.Fatalf("deletion disturbed the running generation: %v", err)
	}
	response = localRequest(t, app, "DELETE", "/api/sessions", map[string]any{"ids": []string{running.ID}})
	json.Unmarshal(response.Body.Bytes(), &result)
	if len(result.Deleted) != 1 || len(result.Failed) != 0 {
		t.Fatalf("completed session cannot be deleted: %s", response.Body.String())
	}
}

func TestDeleteSessionsKeepsPathsInsideSessionDirectory(t *testing.T) {
	app, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	configureTestModel(t, app, "https://example.com/images/generations")
	session, err := app.store.createSession()
	if err != nil {
		t.Fatal(err)
	}
	invalid := []string{"", ".", "..", "../config.json", app.Directory(), filepath.Join("..", "config.json")}
	ids := append(invalid, session.ID)
	response := localRequest(t, app, "DELETE", "/api/sessions", map[string]any{"ids": ids})
	var result sessionDeletion
	json.Unmarshal(response.Body.Bytes(), &result)
	if len(result.Failed) != len(invalid) || len(result.Deleted) != 1 || result.Deleted[0] != session.ID {
		t.Fatalf("invalid paths must fail without blocking valid IDs: %s", response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(app.Directory(), "config.json")); err != nil {
		t.Fatalf("configuration outside session directory was removed: %v", err)
	}
	// Repeating a completed deletion is safe when another browser tab has already removed it.
	response = localRequest(t, app, "DELETE", "/api/sessions", map[string]any{"ids": []string{session.ID}})
	json.Unmarshal(response.Body.Bytes(), &result)
	if len(result.Deleted) != 1 || len(result.Failed) != 0 {
		t.Fatal("an already absent session should count as deleted")
	}
}
