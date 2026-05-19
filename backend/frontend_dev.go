//go:build dev
// +build dev

package main

import (
	"io/fs"
	"os"
)

// GetFrontendFS returns the filesystem for development
// This uses the actual filesystem instead of embedded files
func GetFrontendFS() (fs.FS, error) {
	return os.DirFS("frontend/build"), nil
}

// IsFrontendEmbedded returns false in development mode
func IsFrontendEmbedded() bool {
	return false
}
