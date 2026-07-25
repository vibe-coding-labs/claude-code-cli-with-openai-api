package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DatabaseSizeInfo holds information about the database file size.
type DatabaseSizeInfo struct {
	DBPath        string `json:"db_path"`
	DBSizeBytes   int64  `json:"db_size_bytes"`
	WALSizeBytes  int64  `json:"wal_size_bytes"`
	TotalBytes    int64  `json:"total_bytes"`
	FreePages     int64  `json:"free_pages"`
	TotalPages    int64  `json:"total_pages"`
	PageSize      int64  `json:"page_size"`
	BodySizeBytes int64  `json:"body_size_bytes"`
}

// GetDatabaseSizeInfo returns size information about the database files.
func GetDatabaseSizeInfo() (*DatabaseSizeInfo, error) {
	info := &DatabaseSizeInfo{}

	// Get DB path from the current connection
	var dbPath string
	err := DB.QueryRow("PRAGMA database_list").Scan(nil, nil, &dbPath)
	if err != nil || dbPath == "" {
		// Fallback: use PRAGMA for size estimation
		var pageSize, pageCount, freeList int64
		DB.QueryRow("PRAGMA page_size").Scan(&pageSize)
		DB.QueryRow("PRAGMA page_count").Scan(&pageCount)
		DB.QueryRow("PRAGMA freelist_count").Scan(&freeList)
		info.PageSize = pageSize
		info.TotalPages = pageCount
		info.FreePages = freeList
		info.DBSizeBytes = pageSize * pageCount
		info.TotalBytes = info.DBSizeBytes
		return info, nil
	}

	info.DBPath = dbPath

	// Check main DB file
	if stat, err := os.Stat(dbPath); err == nil {
		info.DBSizeBytes = stat.Size()
	}

	// Check WAL file
	walPath := dbPath + "-wal"
	if stat, err := os.Stat(walPath); err == nil {
		info.WALSizeBytes = stat.Size()
	}

	// Check SHM file
	shmPath := dbPath + "-shm"
	if stat, err := os.Stat(shmPath); err == nil {
		info.WALSizeBytes += stat.Size()
	}

	info.TotalBytes = info.DBSizeBytes + info.WALSizeBytes

	// Get page info
	DB.QueryRow("PRAGMA page_size").Scan(&info.PageSize)
	DB.QueryRow("PRAGMA page_count").Scan(&info.TotalPages)
	DB.QueryRow("PRAGMA freelist_count").Scan(&info.FreePages)

	// Get body storage size
	storage := GetLogStorage()
	bodySize, _ := storage.GetStorageSize()
	info.BodySizeBytes = bodySize

	return info, nil
}

// VacuumInto performs a safe VACUUM INTO operation.
func VacuumInto(dbPath string) (int64, error) {
	cleanPath := dbPath + ".clean"

	var dbSize int64
	if stat, err := os.Stat(dbPath); err == nil {
		dbSize = stat.Size()
	}

	freeSpace := getAvailableDiskSpace(filepath.Dir(dbPath))
	if freeSpace >= 0 && freeSpace < int64(float64(dbSize)*1.1) {
		return 0, fmt.Errorf("insufficient disk space: need ~%.1f GB, have %.1f GB free",
			float64(dbSize)*1.1/1e9, float64(freeSpace)/1e9)
	}

	os.Remove(cleanPath)

	_, err := DB.Exec(fmt.Sprintf("VACUUM INTO '%s'", cleanPath))
	if err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("VACUUM INTO failed: %w", err)
	}

	var cleanSize int64
	if stat, err := os.Stat(cleanPath); err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("clean copy not found after VACUUM INTO: %w", err)
	} else {
		cleanSize = stat.Size()
	}

	// Integrity check on the clean copy
	cleanDB, err := sql.Open("sqlite3", cleanPath+"?mode=ro")
	if err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("failed to open clean copy for verification: %w", err)
	}

	var ok string
	err = cleanDB.QueryRow("PRAGMA integrity_check").Scan(&ok)
	cleanDB.Close()

	if err != nil || ok != "ok" {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("integrity check failed on clean copy: %v", ok)
	}

	// Atomically replace
	backupPath := dbPath + ".old"
	os.Remove(backupPath)

	if err := os.Rename(dbPath, backupPath); err != nil {
		os.Remove(cleanPath)
		return 0, fmt.Errorf("failed to rename original DB: %w", err)
	}

	if err := os.Rename(cleanPath, dbPath); err != nil {
		os.Rename(backupPath, dbPath)
		os.Remove(cleanPath)
		return 0, fmt.Errorf("failed to rename clean copy: %w", err)
	}

	os.Remove(backupPath)

	log.Printf("VACUUM INTO completed: %.1f MB → %.1f MB (saved %.1f MB)",
		float64(dbSize)/1e6, float64(cleanSize)/1e6, float64(dbSize-cleanSize)/1e6)

	return cleanSize, nil
}

// getAvailableDiskSpace returns available disk space in bytes for the given directory.
// Returns -1 if unable to determine.
func getAvailableDiskSpace(dir string) int64 {
	var stat syscallStatfs
	if err := statfs(dir, &stat); err != nil {
		return -1
	}
	return int64(stat.Bavail) * int64(stat.Bsize)
}

// DeleteOldestRequestLogs deletes the oldest N request logs to free space.
func DeleteOldestRequestLogs(count int) (int64, error) {
	query := `
		DELETE FROM request_logs
		WHERE id IN (
			SELECT id FROM request_logs
			ORDER BY created_at ASC
			LIMIT ?
		)
	`
	result, err := DB.Exec(query, count)
	if err != nil {
		return 0, fmt.Errorf("failed to delete oldest request logs: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// GetRequestLogCountByAge returns the count of request logs older than N days.
func GetRequestLogCountByAge(days int) (int64, error) {
	var count int64
	err := DB.QueryRow(
		"SELECT COUNT(*) FROM request_logs WHERE created_at < datetime('now', '-' || ? || ' days')",
		days,
	).Scan(&count)
	return count, err
}

// MigrateInlineBodiesToFiles migrates a batch of inline request_body/response_body
// to file storage. Returns the number of rows migrated.
func MigrateInlineBodiesToFiles(batchSize int) (int, error) {
	query := `
		SELECT id, request_body, response_body, created_at
		FROM request_logs
		WHERE (request_body IS NOT NULL AND request_body_path IS NULL)
		   OR (response_body IS NOT NULL AND response_body_path IS NULL)
		ORDER BY id DESC
		LIMIT ?
	`

	rows, err := DB.Query(query, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	storage := GetLogStorage()
	migrated := 0

	for rows.Next() {
		var id int64
		var requestBody, responseBody sql.NullString
		var createdAt time.Time

		if err := rows.Scan(&id, &requestBody, &responseBody, &createdAt); err != nil {
			continue
		}

		var reqPath, respPath string

		if requestBody.Valid && requestBody.String != "" {
			path, err := storage.StoreBody(id, requestBody.String, "req", createdAt)
			if err == nil {
				reqPath = path
			}
		}

		if responseBody.Valid && responseBody.String != "" {
			path, err := storage.StoreBody(id, responseBody.String, "resp", createdAt)
			if err == nil {
				respPath = path
			}
		}

		if reqPath != "" || respPath != "" {
			DB.Exec(
				"UPDATE request_logs SET request_body_path = COALESCE(?, request_body_path), response_body_path = COALESCE(?, response_body_path), request_body = CASE WHEN ? != '' THEN NULL ELSE request_body END, response_body = CASE WHEN ? != '' THEN NULL ELSE response_body END WHERE id = ?",
				reqPath, respPath, reqPath, respPath, id,
			)
			migrated++
		}
	}

	return migrated, nil
}
