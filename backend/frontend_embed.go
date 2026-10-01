//go:build !dev
// +build !dev

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend_build
var frontendFS embed.FS

// GetFrontendFS returns the embedded frontend filesystem
// This is used when building for production
func GetFrontendFS() (fs.FS, error) {
	return fs.Sub(frontendFS, "frontend_build")
}

// IsFrontendEmbedded returns true when frontend is embedded
func IsFrontendEmbedded() bool {
	return true
}
