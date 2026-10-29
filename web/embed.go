package web

import (
	"embed"
	"net/http"
)

//go:embed index.html app.js styles.css assets
var files embed.FS

func Handler() http.Handler { return http.FileServer(http.FS(files)) }
