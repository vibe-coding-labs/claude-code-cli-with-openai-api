//go:build !dev
// +build !dev

package main

import (
	"embed"
	"io/fs"
)

//go:embed frontend/build/*
var frontendFS embed.FS

// GetFrontendFS returns the embedded frontend filesystem
// This is used when building for production
func GetFrontendFS() (fs.FS, error) {
	return fs.Sub(frontendFS, "frontend/build")
}

// IsFrontendEmbedded returns true when frontend is embedded
func IsFrontendEmbedded() bool {
	return true
}
