package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
)

type App struct {
	store    *Store
	client   *http.Client
	jobs     map[string]context.CancelFunc // Protected by store.mu.
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	updating bool // Protected by store.mu; prevents new jobs while draining for an update.
}

func New(directory string) (*App, error) {
	store, err := newStore(directory)
	if err != nil {
		return nil, err
	}
	if err := store.recoverInterrupted(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		store: store, client: &http.Client{}, jobs: make(map[string]context.CancelFunc),
		ctx: ctx, cancel: cancel,
	}, nil
}

func (a *App) Directory() string { return a.store.directory }

func (a *App) Close() {
	a.store.mu.Lock()
	a.cancel()
	a.store.mu.Unlock()
	a.wg.Wait()
}

func (a *App) WaitForUpdate() {
	a.store.mu.Lock()
	a.updating = true
	a.store.mu.Unlock()
	a.wg.Wait()
}

func (a *App) ResumeAfterUpdate() {
	a.store.mu.Lock()
	a.updating = false
	a.store.mu.Unlock()
}

func (a *App) Handler(frontend fs.FS, updates http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", a.getConfig)
	mux.HandleFunc("PUT /api/config", a.putConfig)
	mux.HandleFunc("GET /api/models/{model}/usage", a.getModelUsage)
	mux.HandleFunc("PUT /api/models/{model}/usage", a.putUsageCredentials)
	mux.HandleFunc("GET /api/sessions", a.listSessions)
	mux.HandleFunc("POST /api/sessions", a.createSession)
	mux.HandleFunc("GET /api/sessions/{session}", a.getSession)
	mux.HandleFunc("POST /api/sessions/{session}/assets", a.upload)
	mux.HandleFunc("GET /api/sessions/{session}/assets/{asset}", a.serveAsset)
	mux.HandleFunc("POST /api/sessions/{session}/turns", a.startGeneration)
	mux.HandleFunc("POST /api/sessions/{session}/cancel", a.cancelGeneration)
	if updates != nil {
		mux.Handle("/api/update", updates)
	}
	mux.Handle("/", http.FileServer(http.FS(frontend)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		host := r.Host
		if hostname, _, err := net.SplitHostPort(host); err == nil {
			host = hostname
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			writeError(w, http.StatusForbidden, fmt.Errorf("PureAgent 仅接受本机访问"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			writeError(w, http.StatusForbidden, fmt.Errorf("不允许其他网站访问本地任务或配置"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func writeResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeResponse(w, status, map[string]any{"error": publicError(err, "")})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, value any) bool {
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请求 JSON 无法读取：%w", err))
		return false
	}
	return true
}

func redactConfig(config Config) Config {
	models := make([]Model, len(config.Models))
	for i, model := range config.Models {
		model.HasKey, model.APIKey = model.APIKey != "", ""
		if model.Usage != nil {
			credentials := model.Usage.Redacted()
			model.Usage = &credentials
		}
		models[i] = model
	}
	return Config{Models: models}
}

func (a *App) getConfig(w http.ResponseWriter, r *http.Request) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	config, err := a.store.config()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeResponse(w, http.StatusOK, redactConfig(config))
}

func prepareConfig(config *Config, previous Config) error {
	seen := map[string]bool{}
	for i := range config.Models {
		model := &config.Models[i]
		if model.ID == "" {
			model.ID = newID()
		}
		if seen[model.ID] {
			return fmt.Errorf("模型配置 ID 重复")
		}
		seen[model.ID] = true
		model.Name = strings.TrimSpace(model.Name)
		model.ModelName = strings.TrimSpace(model.ModelName)
		model.URL = strings.TrimSpace(model.URL)
		model.APIKey = strings.TrimSpace(model.APIKey)
		if model.Provider != "" && model.Provider != "seedream" && model.Provider != "openai" {
			return fmt.Errorf("接口类型应为 Seedream 或 OpenAI Images")
		}
		endpoint, err := url.Parse(model.URL)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil {
			return fmt.Errorf("模型 URL 需要是完整的 HTTP(S) 接口地址")
		}
		if model.ModelName == "" {
			return fmt.Errorf("请填写 model_name")
		}
		if model.Name == "" {
			model.Name = model.ModelName
		}
		if model.APIKey == "" {
			for _, old := range previous.Models {
				if old.ID == model.ID {
					if (old.Provider == "openai") != (model.Provider == "openai") && old.APIKey != "" {
						return fmt.Errorf("切换接口类型时请重新填写 API Key，避免把原密钥发送到另一个接口；原配置尚未修改")
					}
					model.APIKey = old.APIKey
				}
			}
		}
		model.HasKey = false
		// Usage credentials are updated separately; ordinary model edits retain them.
		model.Usage = nil
		for _, old := range previous.Models {
			if old.ID == model.ID {
				model.Usage = old.Usage
			}
		}
	}
	return nil
}

func (a *App) putConfig(w http.ResponseWriter, r *http.Request) {
	var config Config
	if !decodeRequest(w, r, &config) {
		return
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	previous, err := a.store.config()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := prepareConfig(&config, previous); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := writeJSON(filepath.Join(a.store.directory, "config.json"), config); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeResponse(w, http.StatusOK, redactConfig(config))
}

func (a *App) listSessions(w http.ResponseWriter, r *http.Request) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	sessions, err := a.store.sessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeResponse(w, http.StatusOK, sessions)
}

func (a *App) createSession(w http.ResponseWriter, r *http.Request) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	session, err := a.store.createSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeResponse(w, http.StatusCreated, session)
}

func (a *App) getSession(w http.ResponseWriter, r *http.Request) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	session, err := a.store.session(r.PathValue("session"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeResponse(w, http.StatusOK, session)
}
