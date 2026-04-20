package agent

import (
	"embed"
	"io/fs"
)

// Global variable containing the embedded frontend assets.
// These are captured from core/www/dist during build.

//go:embed www/dist/*
var staticEmbedFS embed.FS

// GetStaticFS returns the sub-filesystem for the dist folder.
func GetStaticFS() fs.FS {
	f, _ := fs.Sub(staticEmbedFS, "www/dist")
	return f
}
