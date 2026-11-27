package desktop

import (
	"net/http"
	"sync"
)

// workspaceResources drains in-process page requests before the data store closes.
type workspaceResources struct {
	handler http.Handler
	mu      sync.RWMutex
	closed  bool
}

func (resources *workspaceResources) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resources.mu.RLock()
	defer resources.mu.RUnlock()
	if resources.closed {
		http.Error(w, "工作台正在退出", http.StatusServiceUnavailable)
		return
	}
	resources.handler.ServeHTTP(w, r)
}

func (resources *workspaceResources) close() {
	resources.mu.Lock()
	defer resources.mu.Unlock()
	resources.closed = true
}
