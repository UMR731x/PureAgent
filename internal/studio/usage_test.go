package studio

import (
	"bytes"
	"encoding/json"
	"testing"

	"pureagent/internal/usage"
)

func TestUsageCredentialsSurviveModelEditsAndStayPrivate(t *testing.T) {
	app, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	configureTestModel(t, app, "https://example.com/images/generations")
	credentials := usage.Credentials{AccessKey: "private-ak", SecretKey: "private-sk", AdminKey: "private-admin", SeatID: "seat-1"}
	response := localRequest(t, app, "PUT", "/api/models/test/usage", credentials)
	if response.Code != 200 || bytes.Contains(response.Body.Bytes(), []byte("private-")) {
		t.Fatal("usage credentials exposed or save failed")
	}
	response = localRequest(t, app, "GET", "/api/config", nil)
	if bytes.Contains(response.Body.Bytes(), []byte("private-")) {
		t.Fatal("config exposed usage credentials")
	}
	var config Config
	json.Unmarshal(response.Body.Bytes(), &config)
	if !config.Models[0].Usage.HasAccessKey || !config.Models[0].Usage.HasSecretKey || !config.Models[0].Usage.HasAdminKey {
		t.Fatal("missing saved-credential indicators")
	}
	// The existing model editor sends only its own fields.
	config.Models[0].Usage = nil
	config.Models[0].Name = "Renamed model"
	response = localRequest(t, app, "PUT", "/api/config", config)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	response = localRequest(t, app, "PUT", "/api/models/test/usage", usage.Credentials{SeatID: ""})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	saved, err := app.store.config()
	if err != nil {
		t.Fatal(err)
	}
	actual := saved.Models[0].Usage
	if actual.AccessKey != credentials.AccessKey || actual.SecretKey != credentials.SecretKey || actual.AdminKey != credentials.AdminKey || actual.SeatID != "" {
		t.Fatal("credentials lost on an unrelated edit or blank secret submission")
	}
	if saved.Models[0].APIKey != "test-secret" {
		t.Fatal("usage configuration changed image API key")
	}
}
