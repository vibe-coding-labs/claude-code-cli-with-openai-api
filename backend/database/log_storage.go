package database

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogStorage manages file-based storage for request/response bodies.
// Bodies are stored as gzip-compressed JSON files under a configurable
// base directory, organized by date (YYYY/MM/DD/) for easy cleanup.
type LogStorage struct {
	baseDir string
	mu      sync.Once
}

var (
	logStorage     *LogStorage
	logStorageOnce sync.Once
)

// GetLogStorage returns the global LogStorage instance.
// baseDir defaults to ~/.claude-proxy/logs/bodies/
func GetLogStorage() *LogStorage {
	logStorageOnce.Do(func() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "."
		}
		baseDir := filepath.Join(homeDir, ".claude-proxy", "logs", "bodies")
		logStorage = &LogStorage{baseDir: baseDir}
	})
	return logStorage
}

// SetLogStorageDir overrides the base directory (for testing).
func SetLogStorageDir(dir string) {
	logStorage = &LogStorage{baseDir: dir}
}

// dateDir returns the date-based subdirectory for the given time.
func (ls *LogStorage) dateDir(t time.Time) string {
	return filepath.Join(ls.baseDir, t.Format("2006"), t.Format("01"), t.Format("02"))
}

// StoreBody writes a body string to a gzip-compressed file and returns the relative path.
// The relative path is relative to baseDir, e.g., "2026/07/26/123-req.json.gz"
func (ls *LogStorage) StoreBody(id int64, body string, kind string, createdAt time.Time) (string, error) {
	if body == "" {
		return "", nil
	}

	dir := ls.dateDir(createdAt)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create body directory %s: %w", dir, err)
	}

	filename := fmt.Sprintf("%d-%s.json.gz", id, kind)
	fullPath := filepath.Join(dir, filename)

	f, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create body file %s: %w", fullPath, err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	if _, err := io.WriteString(gw, body); err != nil {
		return "", fmt.Errorf("failed to write body to gzip: %w", err)
	}
	if err := gw.Close(); err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Return relative path from baseDir
	relPath, err := filepath.Rel(ls.baseDir, fullPath)
	if err != nil {
		return fullPath, nil // fallback to absolute path
	}
	return relPath, nil
}

// LoadBody reads a body from a gzip-compressed file.
// path can be relative (to baseDir) or absolute.
func (ls *LogStorage) LoadBody(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	fullPath := path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(ls.baseDir, fullPath)
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to open body file %s: %w", fullPath, err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		// Maybe it's not gzipped (legacy), try reading raw
		f.Seek(0, 0)
		data, err := io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("failed to read body file: %w", err)
		}
		return string(data), nil
	}
	defer gr.Close()

	data, err := io.ReadAll(gr)
	if err != nil {
		return "", fmt.Errorf("failed to read gzipped body: %w", err)
	}
	return string(data), nil
}

// DeleteBodiesForDate removes all body files for a given date directory.
// datePath is relative, e.g., "2026/07/26"
func (ls *LogStorage) DeleteBodiesForDate(datePath string) error {
	fullPath := filepath.Join(ls.baseDir, datePath)
	return os.RemoveAll(fullPath)
}

// DeleteBodyFile removes a single body file.
func (ls *LogStorage) DeleteBodyFile(path string) error {
	if path == "" {
		return nil
	}
	fullPath := path
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(ls.baseDir, fullPath)
	}
	return os.Remove(fullPath)
}

// GetBaseDir returns the base directory for body storage.
func (ls *LogStorage) GetBaseDir() string {
	return ls.baseDir
}

// GetStorageSize returns the total size of the body storage directory in bytes.
func (ls *LogStorage) GetStorageSize() (int64, error) {
	var totalSize int64
	err := filepath.Walk(ls.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})
	return totalSize, err
}

// CleanOldDirectories removes date directories older than the specified number of days.
func (ls *LogStorage) CleanOldDirectories(days int) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0

	entries, err := os.ReadDir(ls.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	for _, yearEntry := range entries {
		if !yearEntry.IsDir() {
			continue
		}
		yearDir := filepath.Join(ls.baseDir, yearEntry.Name())
		monthEntries, err := os.ReadDir(yearDir)
		if err != nil {
			continue
		}

		for _, monthEntry := range monthEntries {
			if !monthEntry.IsDir() {
				continue
			}
			monthDir := filepath.Join(yearDir, monthEntry.Name())
			dayEntries, err := os.ReadDir(monthDir)
			if err != nil {
				continue
			}

			for _, dayEntry := range dayEntries {
				if !dayEntry.IsDir() {
					continue
				}
				// Parse date from directory structure: YYYY/MM/DD
				dateStr := fmt.Sprintf("%s-%s-%s", yearEntry.Name(), monthEntry.Name(), dayEntry.Name())
				dirDate, err := time.Parse("2006-01-02", dateStr)
				if err != nil {
					continue
				}

				if dirDate.Before(cutoff) {
					dayDir := filepath.Join(monthDir, dayEntry.Name())
					if err := os.RemoveAll(dayDir); err == nil {
						removed++
					}
				}
			}

			// Remove empty month directories
			remaining, _ := os.ReadDir(monthDir)
			if len(remaining) == 0 {
				os.Remove(monthDir)
			}
		}

		// Remove empty year directories
		remaining, _ := os.ReadDir(yearDir)
		if len(remaining) == 0 {
			os.Remove(yearDir)
		}
	}

	return removed, nil
}

// shouldStoreBodyToFile checks the system setting to determine storage mode.
// Returns true if bodies should be stored in files (default), false for inline.
func shouldStoreBodyToFile() bool {
	setting, err := GetSetting("log_body_storage")
	if err != nil {
		return true // default to file storage
	}
	return strings.ToLower(setting) != "inline"
}
