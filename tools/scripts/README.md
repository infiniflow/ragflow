# Database Scripts

This directory contains database-related utility scripts for RAGFlow.

- **mysql_migration.py**: Data migration between tables with stage-based execution
- **db_schema_sync.py**: Database schema synchronization using peewee-migrate

---

# mysql_migration.py

A flexible MySQL data migration tool for migrating data between tables with stage-based execution.

## Overview

This script provides stage-based data migration between MySQL tables. Currently supports:
- `tenant_model_provider`
- `tenant_model_instance`
- `tenant_model`

### Migration Stages

| Stage | Source Table | Target Table | Description |
|-------|-------------|--------------|-------------|
| `tenant_model_provider` | `tenant_llm` | `tenant_model_provider` | Extracts distinct `(tenant_id, llm_factory)` pairs |
| `tenant_model_instance` | `tenant_llm` + `tenant_model_provider` | `tenant_model_instance` | Creates instances with distinct `(tenant_id, llm_factory, api_key)` |
| `tenant_model` | `tenant_llm` + `tenant_model_provider` + `tenant_model_instance` | `tenant_model` | Migrates model configurations (only `status='0'` records) |

### Stage Dependencies

```
tenant_model_provider (no dependencies)
        ↓
tenant_model_instance (depends on tenant_model_provider)
        ↓
tenant_model (depends on tenant_model_provider and tenant_model_instance)
```

### Field Mapping Rules

#### tenant_model_provider

| Target Field | Source | Rule |
|--------------|--------|------|
| `id` | - | Random 32-character UUID1 |
| `provider_name` | `tenant_llm.llm_factory` | Direct mapping |
| `tenant_id` | `tenant_llm.tenant_id` | Direct mapping |

- **Deduplication**: Groups by `(tenant_id, llm_factory)` and takes distinct pairs

#### tenant_model_instance

| Target Field | Source | Rule |
|--------------|--------|------|
| `id` | - | Random 32-character UUID1 |
| `instance_name` | `tenant_llm.llm_factory` | Direct mapping |
| `provider_id` | `tenant_model_provider.id` | JOIN on `tenant_id` and `provider_name=llm_factory` |
| `api_key` | `tenant_llm.api_key` | Direct mapping |
| `status` | `tenant_llm.status` | Direct mapping |

- **Deduplication**: Groups by `(tenant_id, llm_factory, api_key)` and takes distinct records

#### tenant_model

| Target Field | Source | Rule |
|--------------|--------|------|
| `id` | - | Random 32-character UUID1 |
| `model_name` | `tenant_llm.llm_name` | Direct mapping |
| `provider_id` | `tenant_model_provider.id` | JOIN on `tenant_id` and `provider_name=llm_factory` |
| `instance_id` | `tenant_model_instance.id` | JOIN on `provider_id` and `api_key` |
| `model_type` | `tenant_llm.model_type` | Direct mapping |
| `status` | `tenant_llm.status` | Direct mapping |

- **Filter**: Only migrates records where `tenant_llm.status='0'`

## Usage

### Command Line Arguments

```
python mysql_migration.py [OPTIONS]
```

| Option | Short | Description | Default |
|--------|-------|-------------|---------|
| `--host` | - | MySQL host | `localhost` |
| `--port` | - | MySQL port | `3306` |
| `--user` | - | MySQL user | `root` |
| `--password` | - | MySQL password | (empty) |
| `--database` | - | MySQL database name | `rag_flow` |
| `--config` | `-c` | Path to YAML config file | - |
| `--stages` | `-s` | Comma-separated list of stages to run | - |
| `--list-stages` | `-l` | List available stages and exit | - |
| `--execute` | `-e` | Execute full migration (create tables and migrate data) | `False` |
| `--create-table-only` | - | Only create target tables, skip data migration | `False` |

> **Note**: MySQL connection can be configured via command line arguments (`--host`, `--port`, `--user`, `--password`, `--database`) or via a YAML config file (`--config`). Command line arguments take precedence over config file values.

### Execution Modes

The script has three mutually exclusive modes:

1. **Dry-Run Mode** (default): Check only, no database writes
   ```bash
   # Using config file
   python mysql_migration.py --stages tenant_model_provider --config config.yaml
   
   # Using command line MySQL connection
   python mysql_migration.py --stages tenant_model_provider --host localhost --port 3306 --user root
   ```

2. **Create Table Only Mode**: Create target tables without migrating data
   ```bash
   python mysql_migration.py --stages tenant_model_provider --config config.yaml --create-table-only
   ```

3. **Execute Mode**: Create tables and migrate data
   ```bash
   python mysql_migration.py --stages tenant_model_provider --config config.yaml --execute
   ```

### Configuration File

Create a YAML configuration file with MySQL connection settings:

```yaml
database:
  host: localhost
  port: 3306
  user: root
  password: your_password
  name: rag_flow
```

Alternative keys are also supported:

```yaml
mysql:
  host: localhost
  port: 3306
  user: root
  password: your_password
  database: rag_flow
```

### Examples

```bash
# List all available stages
python mysql_migration.py --list-stages

# Dry run single stage using command line MySQL connection
python mysql_migration.py --stages tenant_model_provider --host localhost --port 3306 --user root --password secret

# Dry run single stage using config file
python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml

# Create tables only for multiple stages
python mysql_migration.py --stages tenant_model_provider,tenant_model_instance --config /path/to/config.yaml --create-table-only

# Execute full migration for all stages (in dependency order)
python mysql_migration.py --stages tenant_model_provider,tenant_model_instance,tenant_model --config /path/to/config.yaml --execute

# Use config file with command line password override
python mysql_migration.py --stages tenant_model_provider --config /path/to/config.yaml --password mypassword --execute
```

## Output Interpretation

### Stage Execution Log

Each stage displays a header showing progress:

```
============================================================
Stage [1/3]: tenant_model_provider
============================================================
```

The stage then performs:
1. Check phase: Verifies source/target tables exist and counts records to migrate
2. Execute phase: Creates tables (if needed) and migrates data in batches

### Dry-Run Output

In dry-run mode, the script outputs what it would do without writing:

```
[DRY RUN] Would insert 150 records
  instance_name=OpenAI, provider_id=abc123, api_key=***
  ... and 145 more records
```

### Migration Summary

After all stages complete, a summary is printed:

```
============================================================
Migration Summary
============================================================
Total Duration: 2.45s
Total Rows Processed: 350
Tables Operated: tenant_model_provider, tenant_model_instance
------------------------------------------------------------
Stage Details:
  [tenant_model_provider] Tables: tenant_model_provider, Rows: 50, Duration: 0.82s
  [tenant_model_instance] Tables: tenant_model_instance, Rows: 300, Duration: 1.63s
============================================================
```

### Common Messages

| Message | Meaning                                                                 |
|---------|-------------------------------------------------------------------------|
| `No new data to migrate` | All records already exist in target table                               |
| `[DRY RUN] Target table does not exist` | Target table missing, use `--execute` or `--create-table-only`to create |
| `Dependency table does not exist` | Required table from previous stage missing                              |
| `Inserted batch X: Y records` | Successfully inserted batch of records                                  |

---

# db_schema_sync.py

A database schema synchronization tool that uses peewee-migrate to detect and manage schema changes.

## Overview

This script:
1. Reads model definitions from `api/db/db_models.py`
2. Compares with existing database tables specified via command line
3. Generates migration files in `tools/migrate/{version}/`

## Prerequisites

Install peewee-migrate:
```bash
pip install peewee-migrate
```

## Usage

### Command Line Arguments

```
python db_schema_sync.py [OPTIONS]
```

| Option | Short | Description |
|--------|-------|-------------|
| `--host` | - | MySQL host (required) |
| `--port` | - | MySQL port (default: 3306) |
| `--user` | - | MySQL user (required) |
| `--password` | - | MySQL password (required) |
| `--database` | - | MySQL database name (required) |
| `--version` | `-v` | Version number in format `vxx.xx.xx` (required) |
| `--list` | `-l` | List all migrations |
| `--create` | - | Create a new migration (auto-detect changes) |
| `--migrate` | `-m` | Run pending migrations |
| `--diff` | `-d` | Show schema differences |
| `--name` | `-n` | Migration name (default: auto) |

### Version Format

Version must be in format `vxx.xx.xx` where `xx` are digits:
- Valid: `v0.24.0`, `v1.0.0`, `v10.20.30`
- Invalid: `0.24.0`, `v0.24`, `v0.24.0.1`

### Migration File Location

Migration files are stored in:
```
tools/migrate/{version_dir}/
```

Where `{version_dir}` is the version with `.` replaced by `_`.

Example: Version `v0.24.0` → Directory `tools/migrate/v0_24_0/`

### Examples

```bash
# List all migrations
python db_schema_sync.py --list \
    --host localhost --port 3306 --user root --password xxx --database rag_flow \
    --version v0.24.0

# Create a new auto-detected migration
python db_schema_sync.py --create \
    --host localhost --port 3306 --user root --password xxx --database rag_flow \
    --version v0.24.0

# Create a named migration
python db_schema_sync.py --create --name add_user_table \
    --host localhost --port 3306 --user root --password xxx --database rag_flow \
    --version v0.24.0

# Run all pending migrations
python db_schema_sync.py --migrate \
    --host localhost --port 3306 --user root --password xxx --database rag_flow \
    --version v0.24.0

# Show schema differences
python db_schema_sync.py --diff \
    --host localhost --port 3306 --user root --password xxx --database rag_flow \
    --version v0.24.0
```

## How It Works

1. **Load Models**: Imports all model classes from `api/db/db_models.py`
2. **Connect Database**: Creates MySQL connection from command line arguments
3. **Detect Changes**: peewee-migrate compares model definitions with actual database schema
4. **Generate Migration**: Creates Python migration file with `upgrade()` and `downgrade()` functions
