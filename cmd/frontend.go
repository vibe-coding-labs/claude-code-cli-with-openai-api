package cmd

import (
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// ServeFrontend sets up frontend static file serving
// Supports both embedded FS and regular filesystem
func ServeFrontend(router *gin.Engine, frontendFS fs.FS, isEmbedded bool) {
	// Serve static files using embedded or regular FS
	router.GET("/ui", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	router.GET("/ui/*filepath", func(c *gin.Context) {
		path := c.Param("filepath")

		// Handle root path
		if path == "/" || path == "" {
			serveFile(c, frontendFS, "index.html")
			return
		}

		// Remove leading slash
		if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}

		// Try to serve the file
		if serveFile(c, frontendFS, path) {
			return
		}

		// For all other paths (React Router routes), serve index.html
		serveFile(c, frontendFS, "index.html")
	})
}

// serveFile serves a file from the filesystem
func serveFile(c *gin.Context, fsys fs.FS, filename string) bool {
	// Clean the path
	filename = filepath.ToSlash(filepath.Clean(filename))
	if strings.HasPrefix(filename, "../") || strings.Contains(filename, "/../") {
		c.Status(http.StatusForbidden)
		return true
	}

	// Try to open the file
	file, err := fsys.Open(filename)
	if err != nil {
		// Debug: log error
		// fmt.Printf("Failed to open file %s: %v\n", filename, err)
		return false
	}
	defer file.Close()

	// Get file info
	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	// Serve the file using http.FS wrapper
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, filename, stat.ModTime(), seeker)
	} else {
		// Fallback: read all and serve
		data, err := io.ReadAll(file)
		if err != nil {
			return false
		}
		c.Data(http.StatusOK, http.DetectContentType(data), data)
	}
	return true
}
