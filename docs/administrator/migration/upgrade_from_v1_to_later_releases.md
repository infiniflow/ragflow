---
sidebar_position: 4
title: Upgrade from v1.x to later releases
sidebar_label: Upgrade from v1.x to later releases
slug: /upgrade_from_v1_to_later_releases
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Upgrade from v1.x to later releases

:::info Version scope
This document applies to MySQL-based RAGFlow `v1.0.0-rc1` and later Docker deployments. To upgrade from an earlier release, first follow [Upgrade from v0.x to v1.x](./upgrade_from_v0_to_v1.md).
:::

The commands below start one application instance. For a custom multi-replica deployment, use the orchestration platform's documentation to reduce it to one application instance before migration and restore the remaining instances only after migration succeeds.

## Before upgrading

1. Stop document imports, parsing jobs, data-source synchronization, and other processes that can write data.
2. Back up the metadata database and all other persistent data from the same stopped deployment. Verify that the backup can be restored. See [Backup and Restore (v1.x)](./backup_and_restore_v1.md).
3. Use the Docker Compose files, environment template, and image shipped with the target release. Copy the required settings from the existing deployment into the new template.
4. Keep enough free time in the maintenance window for schema changes and data backfills. Large databases can take longer to migrate.

## Run the migration

### 1. Stop the current deployment

Stop the existing deployment before installing the target release files:

```bash
docker compose -p docker -f docker/docker-compose.yml down
```

### 2. Start the target release

Install the target release files, merge the required settings into its `docker/.env` template, and start the deployment:

```bash
docker compose -p docker -f docker/docker-compose.yml up -d
```

For a MySQL metadata database, RAGFlow runs the required migration during startup.

### 3. Monitor the migration

Follow the RAGFlow application logs:

```bash
docker compose -p docker -f docker/docker-compose.yml logs -f ragflow-cpu
```

Keep the log view open until the migration finishes and the application starts. Press `Ctrl+C` to leave the log view; the containers continue running. Review warnings as well as errors before allowing users, ingestion workers, or synchronization jobs to write data.

If the logs report a migration failure, stop the deployment, preserve the logs, and follow [Roll back after a failed migration](#roll-back-after-a-failed-migration).

### 4. Check the deployment status

Display the state of the containers:

```bash
docker compose -p docker -f docker/docker-compose.yml ps
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
