package database

import (
	"database/sql"
	"fmt"
	"time"
)

// GetSetting retrieves a system setting by key
func GetSetting(key string) (string, error) {
	var value string
	err := DB.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get setting: %w", err)
	}
	return value, nil
}

// SetSetting updates or creates a system setting
func SetSetting(key, value, updatedBy string) error {
	_, err := DB.Exec(`
		INSERT INTO system_settings (key, value, updated_at, updated_by)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at,
			updated_by = excluded.updated_by
	`, key, value, time.Now().Format("2006-01-02 15:04:05"), updatedBy)
	if err != nil {
		return fmt.Errorf("failed to set setting: %w", err)
	}
	return nil
}

// GetAllSettings retrieves all system settings
func GetAllSettings() (map[string]string, error) {
	rows, err := DB.Query("SELECT key, value FROM system_settings")
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settings[key] = value
	}
	return settings, nil
}
