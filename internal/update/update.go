package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
)

type State struct {
	Current    string `json:"current"`
	Latest     string `json:"latest,omitempty"`
	Phase      string `json:"phase"`
	Message    string `json:"message,omitempty"`
	ReleaseURL string `json:"release_url,omitempty"`
}

// Lifecycle belongs to the running application; the updater never opens its data directory.
type Lifecycle struct {
	WaitForJobs func()
	ResumeJobs  func()
	Restart     func()
}

type Updater struct {
	mu        sync.Mutex
	state     State
	busy      bool
	ctx       context.Context
	client    *http.Client
	lifecycle Lifecycle
	arguments []string
	parent    io.WriteCloser // Kept open until this process exits, releasing the installer.
}

func New(ctx context.Context, version string, arguments []string, lifecycle Lifecycle) *Updater {
	return &Updater{
		state: State{Current: version, Phase: "idle"},
		ctx:   ctx, client: &http.Client{}, arguments: arguments, lifecycle: lifecycle,
	}
}

func (u *Updater) Failure(message string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.Phase, u.state.Message, u.busy = "error", message, false
}

func (u *Updater) setPhase(phase, message string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.Phase, u.state.Message = phase, message
}

func (u *Updater) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		defer u.mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(u.state)
	})
	mux.HandleFunc("POST /api/update", func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		defer u.mu.Unlock()
		if !u.busy {
			u.busy = true
			u.state.Phase, u.state.Message = "checking", "正在检查 GitHub Release…"
			go u.run()
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(u.state)
	})
	return mux
}

func (u *Updater) run() {
	latest, err := fetchRelease(u.ctx, u.client)
	if err != nil {
		u.Failure(err.Error())
		return
	}
	u.mu.Lock()
	u.state.Latest, u.state.ReleaseURL = latest.Tag, latest.URL
	current := u.state.Current
	u.mu.Unlock()
	newer, err := newerVersion(latest.Tag, current)
	if err != nil {
		u.Failure(err.Error())
		return
	}
	if !newer {
		u.mu.Lock()
		u.state.Phase, u.state.Message, u.busy = "latest", "已是最新版本", false
		u.mu.Unlock()
		return
	}
	if current == "dev" {
		u.Failure("当前为源码开发版，请先下载并运行 Release 客户端，之后即可一键更新。")
		return
	}
	asset, err := latest.platformAsset()
	if err != nil {
		u.Failure(err.Error())
		return
	}
	u.setPhase("downloading", fmt.Sprintf("正在下载 %s…", latest.Tag))
	stage, err := prepare(u.ctx, u.client, asset, u.arguments)
	if err != nil {
		u.Failure(err.Error())
		return
	}
	u.activate(stage)
}

func (u *Updater) activate(stage *installation) {
	u.setPhase("waiting", "等待正在生成的任务完成，随后自动重启…")
	u.lifecycle.WaitForJobs()
	parent, err := stage.launch(u.ctx)
	if err != nil {
		u.lifecycle.ResumeJobs()
		os.RemoveAll(stage.directory)
		u.Failure(err.Error())
		return
	}
	u.parent = parent
	u.setPhase("restarting", "正在重启客户端，配置、任务和原图将保留…")
	u.lifecycle.Restart()
}
