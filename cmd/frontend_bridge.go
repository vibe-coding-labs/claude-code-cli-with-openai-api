package cmd

import (
	"io/fs"
)

// SetFrontendFunctions sets the frontend filesystem functions
// This is called from main package to inject the appropriate implementation
func SetFrontendFunctions(getFS func() (fs.FS, error), isEmbedded func() bool) {
	getFrontendFS = getFS
	isFrontendEmbedded = isEmbedded
}
