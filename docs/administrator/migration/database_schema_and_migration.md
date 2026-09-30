---
sidebar_position: 3
title: Database Migration for Docker Deployments (v1.0.0-rc1 and Later)
sidebar_label: Docker Database Migration (v1.0.0-rc1+)
slug: /database_schema_and_migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Database Migration for Docker Deployments (v1.0.0-rc1 and Later)

:::info Version scope
This document applies to RAGFlow `v1.0.0-rc1` and later Docker deployments. To upgrade from an earlier release, first follow [Upgrade to v1.0.0-rc1](./upgrade_guide.md).
:::

The standard Docker startup process runs the required database migration before starting the RAGFlow services. Start one application replica first and wait for its migration to finish before starting additional replicas.

## Before upgrading

1. Stop document imports, parsing jobs, data-source synchronization, and other processes that can write data.
2. Back up the metadata database and all other persistent data from the same stopped deployment. Verify that the backup can be restored. See [Backup and Migration (v1.0.0-rc1 and Later)](./backup_and_migration.md).
3. Use the Docker Compose files, environment template, and image shipped with the target release. Copy the required settings from the existing deployment into the new template.
4. Keep enough free time in the maintenance window for schema changes and data backfills. Large databases can take longer to migrate.

## Run the migration

### 1. Set the Compose project name

Set `project_name` to the name used by the existing deployment. The default project name is usually `docker`. If the deployment was started with `docker compose -p ragflow`, use `ragflow` instead:

```bash
project_name=docker
```

Use the same value in every command below so that Docker Compose operates on the existing containers and volumes.

### 2. Stop the current deployment

Stop the existing deployment before installing the target release files:

```bash
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml down
```

This command stops and removes the containers while retaining the Compose project's named volumes.

### 3. Start the target release

Install the target release files, merge the required settings into its `docker/.env` template, and start the deployment:

```bash
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml up -d
```

This starts the target release. The container entrypoint runs the required database migration before starting the application. In a deployment with multiple replicas, allow one migration attempt to finish before starting the remaining replicas.

### 4. Monitor the migration

Follow the application logs while the deployment starts:

```bash
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml logs -f ragflow-cpu
```

Keep the log view open until the migration finishes and the application starts. Press `Ctrl+C` to leave the log view; the containers continue running. Review warnings as well as errors before allowing users, ingestion workers, or synchronization jobs to write data.

If the logs report a migration failure, stop the deployment, inspect the reported error and current database state, resolve the cause, and then retry the migration.

### 5. Check the deployment status

Display the state of the containers:

```bash
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps
```

Wait until the expected services report a running or healthy state, then complete the checks below.

## Verify the upgraded deployment

After migration, confirm that:

- All expected containers are running and healthy.
- Existing users can sign in.
- Existing knowledge bases and files are visible.
- Existing files can be opened and retrieved.
- A new document can be uploaded, parsed, and retrieved.
- Existing conversation history is available.
- Model providers and tenant default models are configured correctly.
- Enabled ingestion and file-synchronization services can complete new jobs.

## Roll back after a failed migration

Rollback is performed by restoring the complete pre-upgrade backup together with its matching RAGFlow release and configuration.

To return to the previous release:

1. Stop the failed deployment.
2. Restore the metadata database, object storage, search index, cache, queue, and other persistent data from the same pre-upgrade backup point.
3. Restore the matching Docker Compose files and configuration.
4. Start the previous RAGFlow release and repeat the verification checks.
