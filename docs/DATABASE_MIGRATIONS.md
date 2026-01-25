# Database Migrations Guide

This document describes the database migration system for the load balancer enhancements feature.

## Overview

The migration system provides a structured way to manage database schema changes. Migrations are:

- **Version-controlled**: Each migration has a unique version number
- **Idempotent**: Can be run multiple times safely
- **Tracked**: The system tracks which migrations have been applied
- **Reversible**: Each migration includes rollback instructions (manual)

## Migration Files

Migration files are located in `database/migrations/` and follow this naming convention:

```
XXX_description.sql
```

Where:
- `XXX` is a 3-digit version number (e.g., 001, 002, 003)
- `description` is a brief description of the migration

### Current Migrations

1. **001_create_health_statuses_table.sql**
   - Creates the `health_statuses` table for tracking configuration node health
   - Adds indexes for performance

2. **002_create_circuit_breaker_states_table.sql**
   - Creates the `circuit_breaker_states` table for circuit breaker state management
   - Adds indexes for performance

3. **003_create_lb_request_logs_table.sql**
   - Creates the `load_balancer_request_logs` table for request logging
   - Adds indexes for efficient querying

4. **004_create_lb_stats_tables.sql**
   - Creates `load_balancer_stats` and `node_stats` tables for aggregated statistics
   - Adds indexes for time-based queries

5. **005_create_alerts_table.sql**
   - Creates the `alerts` table for the alerting system
   - Adds indexes for filtering and sorting

6. **006_add_lb_enhancement_config_fields.sql**
   - Adds enhancement configuration fields to the `load_balancers` table
   - Includes health check, retry, circuit breaker, and logging configuration

## Running Migrations

### Automatic (Recommended)

Migrations are automatically applied when the application starts:

```bash
./claude-with-openai-api server
```

The application will:
1. Check which migrations have been applied
2. Apply any pending migrations in order
3. Log the results

### Manual via CLI

You can also run migrations manually using the CLI:

```bash
# Run all pending migrations
./claude-with-openai-api migrate up

# Check migration status
./claude-with-openai-api migrate status

# Rollback a migration (manual process)
./claude-with-openai-api migrate rollback 6
```

### Manual via SQL

You can apply migrations directly using the SQLite CLI:

```bash
# Apply a specific migration
sqlite3 data/proxy.db < database/migrations/001_create_health_statuses_table.sql

# Check applied migrations
sqlite3 data/proxy.db "SELECT * FROM schema_migrations ORDER BY version;"
```

## Migration Status

The system tracks applied migrations in the `schema_migrations` table:

```sql
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

To check which migrations have been applied:

```bash
./claude-with-openai-api migrate status
```

Output example:
```
VERSION   NAME                                    STATUS    APPLIED AT
-------   ----                                    ------    ----------
001       create_health_statuses_table            Applied   2024-01-15 10:30:00
002       create_circuit_breaker_states_table     Applied   2024-01-15 10:30:01
003       create_lb_request_logs_table            Applied   2024-01-15 10:30:02
004       create_lb_stats_tables                  Applied   2024-01-15 10:30:03
005       create_alerts_table                     Applied   2024-01-15 10:30:04
006       add_lb_enhancement_config_fields        Pending   -
```

## Rolling Back Migrations

### Important Notes

- SQLite has limited support for schema changes (no DROP COLUMN)
- Rollbacks must be performed manually
- Always backup your database before rolling back

### Rollback Process

1. **Backup the database**:
   ```bash
   cp data/proxy.db data/proxy.db.backup
   ```

2. **Review the rollback SQL**:
   Each migration file includes a commented-out DOWN section with rollback instructions.

3. **Execute the rollback**:
   ```bash
   # Extract and run the DOWN section manually
   sqlite3 data/proxy.db
   ```

4. **Remove the migration record**:
   ```sql
   DELETE FROM schema_migrations WHERE version = X;
   ```

### Example Rollback

To rollback migration 005 (alerts table):

```bash
# Backup first
cp data/proxy.db data/proxy.db.backup

# Connect to database
sqlite3 data/proxy.db

# Execute rollback commands
DROP INDEX IF EXISTS idx_alerts_type;
DROP INDEX IF EXISTS idx_alerts_level;
DROP INDEX IF EXISTS idx_alerts_created_at;
DROP INDEX IF EXISTS idx_alerts_acknowledged;
DROP INDEX IF EXISTS idx_alerts_lb_id;
DROP TABLE IF EXISTS alerts;

# Remove migration record
DELETE FROM schema_migrations WHERE version = 5;

# Exit
.quit
```

## Creating New Migrations

When you need to create a new migration:

1. **Determine the next version number**:
   ```bash
   ./claude-with-openai-api migrate status
   ```

2. **Create the migration file**:
   ```bash
   touch database/migrations/007_your_migration_name.sql
   ```

3. **Write the migration**:
   ```sql
   -- Migration: 007_your_migration_name
   -- Description: Brief description of what this migration does
   -- Date: YYYY-MM-DD

   -- ============================================================================
   -- UP Migration
   -- ============================================================================

   -- Your SQL statements here
   CREATE TABLE IF NOT EXISTS your_table (
       id TEXT PRIMARY KEY,
       -- columns...
   );

   -- ============================================================================
   -- DOWN Migration (Rollback)
   -- ============================================================================
   -- To rollback this migration, uncomment and run the following:
   --
   -- DROP TABLE IF EXISTS your_table;
   ```

4. **Test the migration**:
   ```bash
   # On a copy of the database
   cp data/proxy.db data/proxy.db.test
   sqlite3 data/proxy.db.test < database/migrations/007_your_migration_name.sql
   ```

5. **Run the migration**:
   ```bash
   ./claude-with-openai-api migrate up
   ```

## Best Practices

1. **Always backup before migrations**: Especially in production
2. **Test migrations on a copy first**: Never test on production data
3. **Keep migrations small**: One logical change per migration
4. **Make migrations idempotent**: Use `IF NOT EXISTS` and `IF EXISTS`
5. **Document complex migrations**: Add comments explaining the purpose
6. **Never modify applied migrations**: Create a new migration instead
7. **Include rollback instructions**: Even if automatic rollback isn't supported

## Troubleshooting

### Migration Failed

If a migration fails:

1. Check the error message
2. Review the migration SQL
3. Check database constraints and foreign keys
4. Restore from backup if needed

### Migration Already Applied

If you see "migration already applied" but need to re-run it:

```sql
-- Remove the migration record
DELETE FROM schema_migrations WHERE version = X;

-- Then run the migration again
./claude-with-openai-api migrate up
```

### Duplicate Column Error

If you see "duplicate column name" errors:

- The column already exists (migration was partially applied)
- Check the database schema: `sqlite3 data/proxy.db ".schema table_name"`
- Either skip the migration or manually fix the schema

## Production Deployment

When deploying to production:

1. **Backup the database**:
   ```bash
   cp data/proxy.db data/proxy.db.backup.$(date +%Y%m%d_%H%M%S)
   ```

2. **Stop the application**:
   ```bash
   systemctl stop claude-proxy
   ```

3. **Run migrations**:
   ```bash
   ./claude-with-openai-api migrate up
   ```

4. **Verify migrations**:
   ```bash
   ./claude-with-openai-api migrate status
   ```

5. **Start the application**:
   ```bash
   systemctl start claude-proxy
   ```

6. **Monitor logs**:
   ```bash
   journalctl -u claude-proxy -f
   ```

## Schema Migrations Table

The `schema_migrations` table structure:

```sql
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Query examples:

```sql
-- List all applied migrations
SELECT * FROM schema_migrations ORDER BY version;

-- Check if a specific migration is applied
SELECT * FROM schema_migrations WHERE version = 5;

-- Get the latest migration version
SELECT MAX(version) FROM schema_migrations;
```

## Support

For issues or questions about migrations:

1. Check this documentation
2. Review the migration files in `database/migrations/`
3. Check the application logs
4. Consult the development team

## References

- [SQLite ALTER TABLE Documentation](https://www.sqlite.org/lang_altertable.html)
- [SQLite Constraints](https://www.sqlite.org/lang_createtable.html#constraints)
- [Load Balancer Enhancements Design](./LOAD_BALANCER_ENHANCEMENTS.md)
