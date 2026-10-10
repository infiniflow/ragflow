---
sidebar_position: 3
title: Database Migration for v1.x Docker Deployments
sidebar_label: Database Migration (v1.x)
slug: /database_schema_and_migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Database Migration for v1.x Docker Deployments

:::info Version scope
This document applies to RAGFlow `v1.0.0-rc1` and later Docker deployments. To upgrade from an earlier release, first follow [Upgrade from v0.x to v1.x](./upgrade_from_v0_to_v1.md).
:::

:::warning GaussDB deployments
The automatic database migration described below does not run when `DB_TYPE` is `gaussdb` or `gauss`. Before upgrading such a deployment, obtain a GaussDB-compatible migration procedure and verify it in a separate test environment.
:::

The commands below start one application instance. For a custom multi-replica deployment, reduce it to one application instance before migration and restore the remaining instances only after migration succeeds.

## Before upgrading

1. Stop document imports, parsing jobs, data-source synchronization, and other processes that can write data.
2. Back up the metadata database and all other persistent data from the same stopped deployment. Verify that the backup can be restored. See [Backup and Restore (v1.x)](./backup_and_restore_v1.md).
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

This starts the target release. For supported metadata databases, the RAGFlow startup process runs the required database migration before starting the application.

### 4. Monitor the migration

Display all service states. Copy the RAGFlow application service name from the `SERVICE` column and set `application_service` to that value:

```bash
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps --all
application_service="<application-service-name-shown-above>"
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml logs -f "$application_service"
```

Keep the log view open until the migration finishes and the application starts. Press `Ctrl+C` to leave the log view; the containers continue running. Review warnings as well as errors before allowing users, ingestion workers, or synchronization jobs to write data.

If the logs report a migration failure, stop the deployment and preserve the logs. Treat the database and the other persistent services as partially migrated: do not retry against them and do not start the previous release against them. Restore the complete pre-upgrade backup, including the matching application release and configuration, resolve the cause in a separate test environment, and then retry the upgrade from the restored recovery point.

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
