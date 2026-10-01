package cmd

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
)

// nonSeekableFile implements fs.File (Read/Stat/Close) but deliberately not
// io.Seeker, to exercise serveFile's io.ReadAll fallback branch for fs.FS
// implementations whose files don't support seeking.
type nonSeekableFile struct {
	*bytes.Buffer
	name string
}

func (f *nonSeekableFile) Stat() (fs.FileInfo, error) {
	return nonSeekableFileInfo{f.name, f.Buffer.Len()}, nil
}
func (f *nonSeekableFile) Close() error { return nil }

type nonSeekableFileInfo struct {
	name string
	size int
}

func (i nonSeekableFileInfo) Name() string       { return i.name }
func (i nonSeekableFileInfo) Size() int64        { return int64(i.size) }
func (i nonSeekableFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i nonSeekableFileInfo) ModTime() time.Time { return time.Time{} }
func (i nonSeekableFileInfo) IsDir() bool        { return false }
func (i nonSeekableFileInfo) Sys() any           { return nil }

type nonSeekableFS struct {
	name string
	data []byte
}

func (fsys nonSeekableFS) Open(name string) (fs.File, error) {
	if name != fsys.name {
		return nil, fs.ErrNotExist
	}
	return &nonSeekableFile{Buffer: bytes.NewBuffer(fsys.data), name: name}, nil
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func testFrontendFS() fs.FS {
	return fstest.MapFS{
		"index.html":          {Data: []byte("<html>index</html>")},
		"static/app.js":       {Data: []byte("console.log('app')")},
		"nested/dir/file.txt": {Data: []byte("nested file")},
	}
}

func TestServeFrontendIndex(t *testing.T) {
	router := newTestRouter()
	ServeFrontend(router, testFrontendFS(), false)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "<html>index</html>" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestServeFrontendRedirect(t *testing.T) {
	router := newTestRouter()
	ServeFrontend(router, testFrontendFS(), false)

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/" {
		t.Errorf("expected redirect to /ui/, got %q", loc)
	}
}

func TestServeFrontendStaticFile(t *testing.T) {
	router := newTestRouter()
	ServeFrontend(router, testFrontendFS(), false)

	req := httptest.NewRequest(http.MethodGet, "/ui/static/app.js", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "console.log('app')" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestServeFrontendNestedFile(t *testing.T) {
	router := newTestRouter()
	ServeFrontend(router, testFrontendFS(), false)

	req := httptest.NewRequest(http.MethodGet, "/ui/nested/dir/file.txt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "nested file" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestServeFrontendSPAFallback(t *testing.T) {
	router := newTestRouter()
	ServeFrontend(router, testFrontendFS(), false)

	req := httptest.NewRequest(http.MethodGet, "/ui/some/react-router/route", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (index.html fallback), got %d", w.Code)
	}
	if w.Body.String() != "<html>index</html>" {
		t.Errorf("expected index.html fallback body, got: %s", w.Body.String())
	}
}

func TestServeFrontendPathTraversalBlocked(t *testing.T) {
	router := newTestRouter()
	ServeFrontend(router, testFrontendFS(), false)

	req := httptest.NewRequest(http.MethodGet, "/ui/..%2f..%2fetc%2fpasswd", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Either blocked with 403 (serveFile's own traversal guard) or falls back
	// to index.html (gin/filepath.Clean normalizes the path before we see
	// it) — either is an acceptable safe outcome, but it must never be a
	// literal escape (500 or a passwd-looking body).
	if w.Code != http.StatusForbidden && w.Code != http.StatusOK {
		t.Fatalf("unexpected status for traversal attempt: %d", w.Code)
	}
	if w.Body.String() == "root:" {
		t.Fatal("path traversal was not blocked")
	}
}

func TestServeFileDirectlyBlocksTraversal(t *testing.T) {
	router := newTestRouter()
	gin.SetMode(gin.TestMode)

	fsys := testFrontendFS()
	router.GET("/direct", func(c *gin.Context) {
		if serveFile(c, fsys, "../../etc/passwd") {
			return
		}
		c.Status(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/direct", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for direct traversal path, got %d", w.Code)
	}
}

func TestServeFileMissingReturnsFalse(t *testing.T) {
	router := newTestRouter()
	fsys := testFrontendFS()
	router.GET("/direct", func(c *gin.Context) {
		if serveFile(c, fsys, "does-not-exist.txt") {
			return
		}
		c.Status(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/direct", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing file, got %d", w.Code)
	}
}

func TestServeFileDirectoryReturnsFalse(t *testing.T) {
	router := newTestRouter()
	fsys := testFrontendFS()
	router.GET("/direct", func(c *gin.Context) {
		if serveFile(c, fsys, "nested/dir") {
			return
		}
		c.Status(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/direct", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when trying to serve a directory, got %d", w.Code)
	}
}

func TestServeFileNonSeekableFallback(t *testing.T) {
	router := newTestRouter()
	fsys := nonSeekableFS{name: "blob.bin", data: []byte("fallback-content")}
	router.GET("/direct", func(c *gin.Context) {
		if serveFile(c, fsys, "blob.bin") {
			return
		}
		c.Status(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/direct", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "fallback-content" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestSetFrontendFunctions(t *testing.T) {
	origGet, origIsEmbedded := getFrontendFS, isFrontendEmbedded
	defer func() { getFrontendFS, isFrontendEmbedded = origGet, origIsEmbedded }()

	called := false
	SetFrontendFunctions(func() (fs.FS, error) {
		called = true
		return testFrontendFS(), nil
	}, func() bool { return true })

	if getFrontendFS == nil || isFrontendEmbedded == nil {
		t.Fatal("SetFrontendFunctions did not set the package-level funcs")
	}

	fsys, err := getFrontendFS()
	if err != nil {
		t.Fatalf("getFrontendFS() error = %v", err)
	}
	if !called {
		t.Error("expected injected getFS to be invoked")
	}
	if fsys == nil {
		t.Error("expected non-nil fs.FS")
	}
	if !isFrontendEmbedded() {
		t.Error("expected isFrontendEmbedded() to return true")
	}
}
