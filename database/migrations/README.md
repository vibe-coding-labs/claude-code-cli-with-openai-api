# Database Migrations

This directory contains database migration scripts for the load balancer enhancements feature.

## Migration Files

Migrations are numbered sequentially and should be applied in order:

1. `001_create_health_statuses_table.sql` - Creates the health_statuses table
2. `002_create_circuit_breaker_states_table.sql` - Creates the circuit_breaker_states table
3. `003_create_lb_request_logs_table.sql` - Creates the load_balancer_request_logs table
4. `004_create_lb_stats_tables.sql` - Creates load_balancer_stats and node_stats tables
5. `005_create_alerts_table.sql` - Creates the alerts table
6. `006_add_lb_enhancement_config_fields.sql` - Adds enhancement configuration fields to load_balancers table
7. `007_create_tenants_table.sql` - Creates the tenants table for multi-tenancy support
8. `008_create_tenant_configs_table.sql` - Creates the tenant_configs table for tenant-specific configurations
9. `009_create_api_keys_table.sql` - Creates the api_keys table for API key management
10. `010_create_quotas_table.sql` - Creates the quotas table for tenant quota management
11. `011_create_rate_limits_table.sql` - Creates the rate_limits table for rate limiting configuration
12. `012_create_ip_rules_table.sql` - Creates the ip_rules table for IP access control
13. `013_create_usage_records_table.sql` - Creates the usage_records table for tracking API usage
14. `014_create_audit_logs_table.sql` - Creates the audit_logs table for security audit logging

## How to Apply Migrations

### Automatic (Recommended)

Migrations are automatically applied when the application starts via the `database.InitDB()` function.

### Manual

You can also apply migrations manually using the SQLite CLI:

```bash
sqlite3 data/proxy.db < database/migrations/001_create_health_statuses_table.sql
```

## Rollback

Each migration file includes a rollback section at the bottom (commented out) that can be used to undo the migration if needed.

## Creating New Migrations

When creating new migrations:

1. Use the next sequential number
2. Use a descriptive name
3. Include both UP and DOWN (rollback) sections
4. Test the migration on a copy of the database first
5. Document any data transformations or special considerations

## Migration Status

The application tracks which migrations have been applied using the `schema_migrations` table.
