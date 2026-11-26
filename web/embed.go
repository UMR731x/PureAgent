package web

import "embed"

//go:embed *.html *.css *.js *.png *.ico
var Files embed.FS
