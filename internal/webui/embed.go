package webui

import (
	"embed"
	"io/fs"
)

// content contains the complete Overmynd frontend.
//
//go:embed web/index.html web/css web/js
var content embed.FS

func Files() (fs.FS, error) {
	return fs.Sub(content, "web")
}
