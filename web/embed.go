package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticFiles embed.FS

// GetFS returns the embedded web static files as an fs.FS.
func GetFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("rawth: failed to load embedded web files: " + err.Error())
	}
	return sub
}
