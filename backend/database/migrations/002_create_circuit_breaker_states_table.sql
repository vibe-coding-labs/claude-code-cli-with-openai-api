-- Migration: 002_create_circuit_breaker_states_table
-- Description: Creates the circuit_breaker_states table for tracking circuit breaker state
-- Date: 2024-01-15

-- ============================================================================
-- UP Migration
-- ============================================================================

CREATE TABLE IF NOT EXISTS circuit_breaker_states (
    config_id TEXT PRIMARY KEY,
    state TEXT NOT NULL CHECK(state IN ('closed', 'open', 'half_open')),
    failure_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    last_state_change DATETIME NOT NULL,
    next_retry_time DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (config_id) REFERENCES api_configs(id) ON DELETE CASCADE
);

-- Create index for faster queries
CREATE INDEX IF NOT EXISTS idx_circuit_breaker_state ON circuit_breaker_states(state);
CREATE INDEX IF NOT EXISTS idx_circuit_breaker_next_retry ON circuit_breaker_states(next_retry_time);

-- ============================================================================
-- DOWN Migration (Rollback)
-- ============================================================================
-- To rollback this migration, uncomment and run the following:
--
-- DROP INDEX IF EXISTS idx_circuit_breaker_next_retry;
-- DROP INDEX IF EXISTS idx_circuit_breaker_state;
-- DROP TABLE IF EXISTS circuit_breaker_states;
