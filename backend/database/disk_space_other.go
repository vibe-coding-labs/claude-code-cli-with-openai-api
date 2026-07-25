//go:build !linux

package database

import (
	"fmt"
)

type syscallStatfs struct {
	Bavail uint64
	Bsize  uint64
}

func statfs(path string, stat *syscallStatfs) error {
	return fmt.Errorf("statfs not implemented on this platform")
}
