---
sidebar_position: 1
title: Upgrade from v0.x to v1.x
sidebar_label: Upgrade from v0.x to v1.x
slug: /upgrade_guide
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Upgrade from v0.x to v1.x

This guide provides the step-by-step procedure for upgrading a MySQL-based Docker deployment from RAGFlow `v0.20.0` or later to the current latest version.

Before continuing, use [Migration Guide](./migration_overview.md) to choose an upgrade route and complete the backup and preparation checklist.

:::danger Do not skip the v0.x to v1.x boundary
Every `v0.x` deployment must successfully start and pass verification on `v0.27.2`, and must then be upgraded to `v1.0.0-rc1`. Do not upgrade directly from an earlier `v0.x` release to `v1.0.0-rc1` or a later `v1.x` release.

The `v0.27.2` to `v1.0.0-rc1` migration is irreversible. Create and verify a complete `v0.27.2` recovery point before starting it. If migration fails, restore that recovery point before trying again.
:::

## Quick navigation

- [Upgrade between v0.x releases](#1-upgrade-between-v0x-releases)
- [Special instructions for each v0.x checkpoint](#2-handle-each-key-release)
- [Upgrade from v0.27.2 to v1.0.0-rc1](#3-upgrade-from-v0272-to-v100-rc1)
- [Upgrade from v1.0.0-rc1 to a later v1.x release](#4-upgrade-from-v100-rc1-to-a-later-v1x-release)

For every v0.x checkpoint in the routes below, first follow the [v0.x-to-v0.x upgrade steps](#1-upgrade-between-v0x-releases), and then complete the linked version-specific instructions.

**Standard route:**

[`v0.22.1`](#22-v0221) → [`v0.25.6`](#23-v0256) → [`v0.26.4`](#24-v0264) → [`v0.27.2`](#25-v0272) → [`v1.0.0-rc1`](#3-upgrade-from-v0272-to-v100-rc1) → [later v1.x release, if needed](#4-upgrade-from-v100-rc1-to-a-later-v1x-release)

**Conservative route:**

[`v0.21.1`](#21-v0211) → [`v0.22.1`](#22-v0221) → [`v0.25.6`](#23-v0256) → [`v0.26.4`](#24-v0264) → [`v0.27.2`](#25-v0272) → [`v1.0.0-rc1`](#3-upgrade-from-v0272-to-v100-rc1) → [later v1.x release, if needed](#4-upgrade-from-v100-rc1-to-a-later-v1x-release)

After the final upgrade, complete [Verify the upgraded deployment](#5-verify-the-upgraded-deployment). If any step fails, follow [Recover from a failed upgrade](#6-recover-from-a-failed-upgrade).

## 1. Upgrade between v0.x releases

Treat each arrow in the selected route as a separate upgrade. You **must start and verify every target release in the route**. Changing the image tag repeatedly, or copying volumes without starting the intermediate release, does not execute that release's database and data migrations.

### 1.1. Stop writers and create a checkpoint

From the current release directory, stop the deployment. Use the same project name throughout the upgrade:

```bash
project_name=docker
docker compose -p "$project_name" -f docker/docker-compose.yml down
```

For critical deployments, create a new backup before each version change so that every checkpoint has its own recovery point.

### 1.2. Deploy the target release files

Create a separate working directory for the next checkpoint. Set `target_version` to the next version in the selected route:

```bash
target_version=v0.22.1
git fetch --tags
git worktree add "../ragflow-$target_version" "$target_version"
cd "../ragflow-$target_version"
```

Use the Docker Compose files and environment template from this directory. Copy the required values from the previous deployment into the new `docker/.env`; do not replace the new file with the old one. Confirm that its image tag matches `target_version`:

```bash
grep '^RAGFLOW_IMAGE=' docker/.env
```

### 1.3. Start one deployment and monitor migration

Start the target release with the same Compose project name used by the existing deployment:

```bash
project_name=docker
docker compose -p "$project_name" -f docker/docker-compose.yml up -d
docker compose -p "$project_name" -f docker/docker-compose.yml ps --all
```

From the `SERVICE` column, copy the RAGFlow application service name, then follow its logs:

```bash
application_service="<service-name>"
docker compose -p "$project_name" -f docker/docker-compose.yml logs -f "$application_service"
```

Starting the release is required because it applies that release's database and data changes. Keep only one application replica running until this work finishes.

Do not use container health alone as the success criterion. Review migration errors and warnings before enabling users, workers, or synchronization jobs.

### 1.4. Verify the checkpoint

At every checkpoint, confirm that:

- Existing users can sign in and see their knowledge bases.
- Existing files can be listed and opened.
- Retrieval returns content from previously parsed documents.
- A new document can be uploaded, parsed, and retrieved.
- Model providers, credentials, model types, API endpoints, and tenant default models are correct.
- Existing Agents open and complete a representative run.
- The selected search engine and object storage are healthy.

Do not continue to the next checkpoint until failures at the current release have been resolved.

## 2. Handle each key release

These sections contain only the additional checks for each release. Use them together with the [v0.x-to-v0.x upgrade steps](#1-upgrade-between-v0x-releases).

### 2.1. v0.21.1

This checkpoint is optional. Start `v0.21.1`, complete the standard verification, and continue if all checks pass.

### 2.2. v0.22.1

Use `v0.22.1` rather than `v0.22.0`. If a knowledge base used the embedding model from the former `full` image, select an available replacement model. If the compatibility check fails, use another compatible model or re-parse the existing chunks with the new model.

### 2.3. v0.25.6

This checkpoint is required. Start `v0.25.6`, complete the standard verification, and continue only after all checks pass.

### 2.4. v0.26.4

Keep `--init-model-provider-tables` enabled on the first start. After startup, check the migration logs and verify the providers, credentials, models, and tenant default models in the UI. Test each model type used by the deployment before continuing.

### 2.5. v0.27.2

Keep `--init-model-provider-tables` enabled on the first start. Confirm that the migration logs report success, then test model configuration, existing retrieval, and a new document upload and parse.

Do not continue until `v0.27.2` passes all checks. The older GraphRAG and RAPTOR configuration pages are no longer available, but previously generated content remains searchable.

## 3. Upgrade from v0.27.2 to v1.0.0-rc1

This mandatory step is different from the earlier checkpoints. It introduces the v1 runtime and performs an irreversible data migration.

### 3.1. Stop and back up v0.27.2

Make sure that document parsing, synchronization, and Agent tasks have finished. From the `v0.27.2` repository root, set the Compose project name used by the running deployment, check the containers, and stop the deployment:

```bash
project_name=docker
docker compose -p "$project_name" -f docker/docker-compose.yml ps
docker compose -p "$project_name" -f docker/docker-compose.yml down
```

Create a new backup directory and back up the default MySQL, MinIO, Redis, and Elasticsearch volumes:

```bash
backup_dir="backup-v0.27.2-$(date +%Y%m%d-%H%M%S)"
bash docker/migration.sh -p "$project_name" backup "$backup_dir"
ls -lh "$backup_dir"
```

The timestamp gives each recovery point its own directory and prevents files from an earlier backup from being overwritten.

Also save `docker/.env`, custom certificates, mounted configuration files, and the data of any enabled storage service not covered by the command. Confirm that the backup contains the expected data before continuing. Keep the `v0.27.2` Redis or Valkey backup for rollback; it is not restored into a v1 service. See [Backup and Restore (v0.x)](./backup_and_restore_v0.md) for deployments that use another document engine or additional volumes.

### 3.2. Prepare the v1.0.0-rc1 deployment

`v1.0.0-rc1` includes several changes outside the database:

- Deprecated HTTP API aliases are removed.
- NATS replaces Redis as the message queue.
- Kvrocks stores cache and checkpoints.
- The local sandbox and the earlier Team/Me permission behavior are not supported in this release.

Redis or Valkey data files are not compatible with Kvrocks or NATS storage. Restore or reuse MySQL, object storage, and the same type of search engine, but start Kvrocks, NATS, and ClickHouse with new empty volumes. Do not mount the `v0.27.2` Redis or Valkey volume into any of these services. Finish or stop queued work before the backup, and submit any incomplete jobs again after the upgrade.

Prepare a separate repository directory at the `v1.0.0-rc1` tag. Use its Compose files and `.env` template, then copy the required values from the old configuration into the new template. Set the release image to:

```dotenv
RAGFLOW_IMAGE=infiniflow/ragflow:v1.0.0-rc1
```

Use the same `project_name` when reusing compatible volumes on the same host. When restoring into a new project or host, restore the MySQL, MinIO, and same-type search-engine archives to the volume names expected by that project. Let Compose create new Kvrocks, NATS, and ClickHouse volumes.

Keep parallel application replicas stopped until one migration attempt has completed. Large databases need enough startup time for schema changes and conversation-history backfills.

### 3.3. Start v1.0.0-rc1 and wait for migration

From the `v1.0.0-rc1` repository root, start the deployment with the same Compose project name:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml up -d
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps --all
application_service="<application-service-name-shown-above>"
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml logs -f "$application_service"
```

Set `application_service` to the RAGFlow application service name from the `SERVICE` column.

The RAGFlow startup process runs the data migration before starting the application. Keep the log command open until migration finishes and the application starts. Exit the log view with `Ctrl+C`; this does not stop the containers.

Check the container state and the HTTP health endpoint:

```bash
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps
curl -f http://127.0.0.1/api/v1/system/healthz
```

The health-check command uses the default web port `80`. If `SVR_WEB_HTTP_PORT` has been changed in `docker/.env`, use that port in the URL.

If the logs report a migration error, stop the v1 deployment and preserve the logs. Do not retry against the partially migrated data. Restore the complete `v0.27.2` recovery point, resolve the cause in a separate test environment, and retry the upgrade only from the restored recovery point.

The migration is irreversible. Never start `v0.27.2` against a database that has been partially or fully migrated by `v1.0.0-rc1`.

## 4. Upgrade from v1.0.0-rc1 to a later v1.x release

First verify that `v1.0.0-rc1` started successfully and that its migration completed. If the target version is newer, create a new v1 backup and then follow [Database Migration for v1.x](./database_migration_v1.md). Do not skip directly from `v0.27.2` to the later release.

## 5. Verify the upgraded deployment

Sign in to RAGFlow and confirm that existing data and configuration are present, then test document retrieval, a new document upload, one Chat, and one Agent run.

Keep the `v0.27.2` backup until the upgraded deployment has completed acceptance testing and operated successfully under normal workload.

## 6. Recover from a failed upgrade

Stop the target release and preserve its logs. Do not continue to the next checkpoint, rerun migrations blindly, or start an older application image against a database already modified by a newer release.

Restore the application and all persistent stores from the same checkpoint backup:

- The metadata database.
- Object storage and search engine data.
- Cache, queue, analytics, and other enabled persistent volumes.
- Configuration files, certificates, and custom-mounted files.
- The matching RAGFlow image and Compose files.

RAGFlow does not provide an automatic reverse migration. Mixing an older application with newer database or service data is not a rollback.
