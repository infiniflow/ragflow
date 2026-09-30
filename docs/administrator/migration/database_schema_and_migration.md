---
sidebar_position: 1
title: Database Schema and Migration
sidebar_label: Database Schema and Migration
slug: /database_schema_and_migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Database Schema and Migration

For a manually started Go backend, run database migration as a standalone step before starting any server process:

```bash
./bin/ragflow_server --migrate
```

The command loads the same database configuration as the Go server, applies schema and data changes, then exits. Use `-f` or `--config` to select a configuration file when needed. Do not combine `--migrate` with `--admin`, `--api`, `--ingestor`, or `--syncer`. Allow enough time for data backfills and schema changes on large databases.

The migration process does not start a server mode or initialize the document engine, Kvrocks cache, object storage, or NATS JetStream message queue. It needs access to the configured metadata database and local configuration files. Run it where `conf/models` is available: database initialization loads the model provider definitions from that relative path and fails if the directory cannot be read. The model data migration also tries to read `conf/llm_factories.json`; if the file is absent or unreadable, that input is skipped. Keep the matching configuration files with the new binary.

## Where migrations are defined

Go migration logic lives in `internal/dao/`. `database.go` runs the migration sequence: manual changes in `migration.go` and related files, GORM `AutoMigrate` for the Go entities in `internal/entity/`, and the conversation history backfill. Model provider data migration is in `model_migration.go`; its version handling is in `migration_version.go`. These are Go source files, not a directory of generated, numbered SQL migration files.

The standalone command runs the full sequence. Ordinary server startup connects to the database and may create or update a limited set of runtime tables, but does not run the full manual and data migration sequence. Do not rely on server startup to perform an upgrade.

## Database version and startup check

The Go migration marker is stored in `system_settings` under the fixed key `mysql_migration.database.version`. The key name is retained for database compatibility and does not select another migration implementation. The Go implementation writes `v0.26.0` after the base tenant-model step and `v0.27.2` after the follow-up model-data step. When the stored version is below `v1.0.0-rc1.dev1`, the conversation-history migration writes that value after it completes, even if the old history columns or rows are absent and no data needs copying. A step already covered by the stored version is skipped. The last completed step therefore determines the value that remains in the row. This marker is **not** a complete version number for every table or schema change. A missing or unparseable marker does not prove that the database is up to date.

Each Go server process compares its code version with this marker after database initialization. That initialization may already have updated runtime tables, indexes, or built-in templates before the check runs. The comparison uses the release-number portion of each version; development suffixes are not used to establish ordering. If the database version is newer, startup fails with `Refusing to start: database was migrated by a newer version`. If the marker is absent or the versions cannot be compared, this downgrade check does not block startup. The check does not apply pending migrations, and the standalone `--migrate` command does not run the downgrade check. Verify the binary and target database before invoking it.

## Upgrade procedure

1. Back up the metadata database and verify that you can restore it. See [Backup and Migration](./backup_and_migration.md).
2. Stop or drain all Go server processes that use the database.
3. Deploy the new Go binary and its matching configuration. For a manually started deployment, run `./bin/ragflow_server --migrate` against the target database and wait for it to exit before starting servers. In a deployment with multiple replicas, coordinate a single migration job and wait for it to finish before starting the replicas. The Go-only Docker entrypoint runs this command before its enabled Go server modes; account for that behavior when planning a separate migration job.
4. Start `--admin` first, then `--api` and `--ingestor`; start `--syncer` if file synchronization is enabled.

Check the migration logs as well as the exit status before starting services. Some schema conflicts are logged and skipped, and built-in template seeding failures are logged as warnings; a successful exit alone does not verify every schema change or template row. The migration command can be rerun after a failure, but do not assume every database DDL operation is transactional. Check the error and database state before retrying. The Go backend has no supported automatic rollback command or generated reverse migration. To return to an older release, restore a compatible database backup along with the older binary.

## Development mode

`RAGFLOW_DEV_MODE=true` disables only the code-versus-database downgrade check for Go server processes. It does not run migrations, reverse schema changes, or make an older binary compatible with a newer database. This is especially relevant to development builds: the conversation-history migration records `v1.0.0-rc1.dev1` even when the checkout still reports a `v0.27.x` release. Use it only for a development database in that situation. Set it for each affected server process; keep it unset in production.

This migration procedure applies to the default MySQL metadata database.
