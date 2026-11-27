package desktop

import "net/http"

// WebView2 serves MyGo's embedded scheme at this virtual origin, without a socket.
const WorkspaceOrigin = "http://mygo.localhost"

type Options struct {
	Directory    string
	Version      string
	LegacyOrigin string
	Resources    http.Handler
	Ready        func() error
	Restart      <-chan struct{}
}
