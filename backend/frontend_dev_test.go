//go:build dev
// +build dev

package main

import (
	"testing"
)

func TestGetFrontendFS(t *testing.T) {
	fsys, err := GetFrontendFS()
	if err != nil {
		t.Fatalf("GetFrontendFS returned error: %v", err)
	}
	if fsys == nil {
		t.Fatalf("expected non-nil fs.FS")
	}

	if _, err := fsys.Open("index.html"); err != nil {
		t.Errorf("expected to open index.html from frontend/build, got error: %v", err)
	}
}

func TestIsFrontendEmbedded(t *testing.T) {
	if IsFrontendEmbedded() {
		t.Errorf("expected IsFrontendEmbedded() to be false in dev build")
	}
}
