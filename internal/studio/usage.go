package studio

import (
	"fmt"
	"net/http"
	"path/filepath"

	"pureagent/internal/usage"
)

func (a *App) getModelUsage(w http.ResponseWriter, r *http.Request) {
	a.store.mu.Lock()
	config, err := a.store.config()
	a.store.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, model := range config.Models {
		if model.ID == r.PathValue("model") {
			credentials := usage.Credentials{}
			if model.Usage != nil {
				credentials = *model.Usage
			}
			report := usage.Query(r.Context(), a.client, model.Provider, model.ModelName, credentials)
			writeResponse(w, http.StatusOK, report)
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("找不到模型配置"))
}

func (a *App) putUsageCredentials(w http.ResponseWriter, r *http.Request) {
	var input usage.Credentials
	if !decodeRequest(w, r, &input) {
		return
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	config, err := a.store.config()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for i := range config.Models {
		model := &config.Models[i]
		if model.ID != r.PathValue("model") {
			continue
		}
		previous := usage.Credentials{}
		if model.Usage != nil {
			previous = *model.Usage
		}
		credentials := input.Merge(previous)
		model.Usage = &credentials
		if err := writeJSON(filepath.Join(a.store.directory, "config.json"), config); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeResponse(w, http.StatusOK, credentials.Redacted())
		return
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("找不到模型配置"))
}
