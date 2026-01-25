package database

import (
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migration represents a database migration
type Migration struct {
	Version     int
	Name        string
	Filename    string
	SQL         string
	AppliedAt   *time.Time
}

// createMigrationsTable creates the schema_migrations table if it doesn't exist
func createMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`
	_, err := DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}
	return nil
}

// getAppliedMigrations returns a map of applied migration versions
func getAppliedMigrations() (map[int]time.Time, error) {
	query := `SELECT version, applied_at FROM schema_migrations ORDER BY version`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]time.Time)
	for rows.Next() {
		var version int
		var appliedAt time.Time
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration: %w", err)
		}
		applied[version] = appliedAt
	}

	return applied, nil
}

// recordMigration records a migration as applied
func recordMigration(version int, name string) error {
	query := `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, datetime('now'))`
	_, err := DB.Exec(query, version, name)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}
	return nil
}

// loadMigrations loads all migration files from the embedded filesystem
func loadMigrations() ([]*Migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []*Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Skip README
		if entry.Name() == "README.md" {
			continue
		}

		// Parse version from filename (e.g., "001_create_table.sql" -> 1)
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			log.Printf("⚠️  Skipping migration file with invalid name: %s", entry.Name())
			continue
		}

		var version int
		_, err := fmt.Sscanf(parts[0], "%d", &version)
		if err != nil {
			log.Printf("⚠️  Skipping migration file with invalid version: %s", entry.Name())
			continue
		}

		// Read migration SQL
		content, err := migrationsFS.ReadFile(filepath.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}

		// Extract migration name (remove version prefix and .sql suffix)
		name := strings.TrimSuffix(parts[1], ".sql")

		migrations = append(migrations, &Migration{
			Version:  version,
			Name:     name,
			Filename: entry.Name(),
			SQL:      string(content),
		})
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// extractUpMigration extracts the UP migration SQL from the full migration content
func extractUpMigration(sql string) string {
	// Find the UP section
	upStart := strings.Index(sql, "-- UP Migration")
	if upStart == -1 {
		// If no UP marker found, use the entire SQL
		return sql
	}

	// Find the DOWN section
	downStart := strings.Index(sql, "-- DOWN Migration")
	if downStart == -1 {
		// If no DOWN marker found, use everything after UP marker
		return sql[upStart:]
	}

	// Extract only the UP section
	return sql[upStart:downStart]
}

// RunMigrations runs all pending database migrations
func RunMigrations() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	// Create migrations table if it doesn't exist
	if err := createMigrationsTable(); err != nil {
		return err
	}

	// Get applied migrations
	applied, err := getAppliedMigrations()
	if err != nil {
		return err
	}

	// Load all migrations
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	if len(migrations) == 0 {
		log.Println("ℹ️  No migration files found")
		return nil
	}

	// Run pending migrations
	pendingCount := 0
	for _, migration := range migrations {
		if _, exists := applied[migration.Version]; exists {
			log.Printf("✓ Migration %03d already applied: %s", migration.Version, migration.Name)
			continue
		}

		log.Printf("⏳ Applying migration %03d: %s", migration.Version, migration.Name)

		// Extract UP migration SQL
		upSQL := extractUpMigration(migration.SQL)

		// Execute migration in a transaction
		tx, err := DB.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.Version, err)
		}

		// Execute the migration SQL
		if _, err := tx.Exec(upSQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %d (%s): %w", migration.Version, migration.Name, err)
		}

		// Record the migration
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, datetime('now'))`,
			migration.Version, migration.Name); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}

		log.Printf("✅ Applied migration %03d: %s", migration.Version, migration.Name)
		pendingCount++
	}

	if pendingCount == 0 {
		log.Println("✅ All migrations are up to date")
	} else {
		log.Printf("✅ Applied %d pending migration(s)", pendingCount)
	}

	return nil
}

// GetMigrationStatus returns the status of all migrations
func GetMigrationStatus() ([]*Migration, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Create migrations table if it doesn't exist
	if err := createMigrationsTable(); err != nil {
		return nil, err
	}

	// Get applied migrations
	applied, err := getAppliedMigrations()
	if err != nil {
		return nil, err
	}

	// Load all migrations
	migrations, err := loadMigrations()
	if err != nil {
		return nil, err
	}

	// Mark which migrations have been applied
	for _, migration := range migrations {
		if appliedAt, exists := applied[migration.Version]; exists {
			migration.AppliedAt = &appliedAt
		}
	}

	return migrations, nil
}

// RollbackMigration rolls back a specific migration (if possible)
// Note: This is a placeholder - actual rollback implementation would need
// to parse and execute the DOWN section of migration files
func RollbackMigration(version int) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	// Check if migration is applied
	applied, err := getAppliedMigrations()
	if err != nil {
		return err
	}

	if _, exists := applied[version]; !exists {
		return fmt.Errorf("migration %d is not applied", version)
	}

	// Load the migration
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	var targetMigration *Migration
	for _, m := range migrations {
		if m.Version == version {
			targetMigration = m
			break
		}
	}

	if targetMigration == nil {
		return fmt.Errorf("migration %d not found", version)
	}

	log.Printf("⚠️  Rollback for migration %d (%s) must be performed manually", version, targetMigration.Name)
	log.Printf("Please refer to the DOWN section in: migrations/%s", targetMigration.Filename)

	return fmt.Errorf("automatic rollback not implemented - please rollback manually")
}
