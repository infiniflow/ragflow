---
sidebar_position: 1
title: Upgrade from v0.x to v1.x
sidebar_label: Upgrade from v0.x to v1.x
slug: /upgrade_from_v0_to_v1
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Upgrade from v0.x to v1.x

This guide provides the step-by-step procedure for upgrading a MySQL-based Docker deployment from RAGFlow `v0.20.0` or later to the current latest version.

Before continuing, review the standard route in [Migration Guide](./migration_overview.md) and complete the backup and preparation checklist.

:::danger Do not skip the v0.x to v1.x boundary
Every `v0.x` deployment must successfully start and pass verification on `v0.27.2`, and must then be upgraded to `v1.0.0-rc1`. Do not upgrade directly from an earlier `v0.x` release to `v1.0.0-rc1` or a later `v1.x` release.

The `v0.27.2` to `v1.0.0-rc1` migration is irreversible. Create and verify a complete `v0.27.2` recovery point before starting it. If migration fails, restore that recovery point before trying again.
:::

## Quick navigation

- [Upgrade between v0.x releases](#1-upgrade-between-v0x-releases)
- [Special instructions for each v0.x checkpoint](#2-handle-each-key-release)
- [Upgrade from v0.27.2 to v1.0.0-rc1](#3-upgrade-from-v0272-to-v100-rc1)
- [Upgrade from v1.0.0-rc1 to a later v1.x release](#4-upgrade-from-v100-rc1-to-a-later-v1x-release)

For every v0.x checkpoint below, first follow the [v0.x-to-v0.x upgrade steps](#1-upgrade-between-v0x-releases), and then complete the linked version-specific instructions.

**Upgrade route:**

[`v0.22.1`](#21-v0221) → [`v0.25.6`](#22-v0256) → [`v0.26.4`](#23-v0264) → [`v0.27.2`](#24-v0272) → [`v1.0.0-rc1`](#3-upgrade-from-v0272-to-v100-rc1) → [latest v1.x release](#4-upgrade-from-v100-rc1-to-a-later-v1x-release)

Start with the first checkpoint that is newer than your current version. Never downgrade to an earlier checkpoint.

After the final upgrade, complete [Verify the upgraded deployment](#5-verify-the-upgraded-deployment). If any step fails, follow [Recover from a failed upgrade](#6-recover-from-a-failed-upgrade).

## 1. Upgrade between v0.x releases

Treat each arrow in the route as a separate upgrade. You **must start and verify every target release in the route**. Changing the image tag repeatedly, or copying volumes without starting the intermediate release, does not execute that release's database and data migrations.

### 1.1. Stop writers and create a checkpoint

From the current release directory, stop the deployment:

```bash
docker compose -p docker -f docker/docker-compose.yml down
```

For critical deployments, create a new backup before each version change so that every checkpoint has its own recovery point.

### 1.2. Deploy the target release files

Create a separate working directory for the next checkpoint. Set `target_version` to the next version in the route:

```bash
target_version=v0.22.1
git fetch --tags
git worktree add "../ragflow-$target_version" "$target_version"
cd "../ragflow-$target_version"
```

Change `target_version=v0.22.1` to the version required for the current checkpoint.

If the worktree command fails because the directory or tag already exists, see the [Git worktree documentation](https://git-scm.com/docs/git-worktree.html).

Copy the required settings into the new `docker/.env` without replacing the file, then confirm that `RAGFLOW_IMAGE` uses the target version:

```bash
grep '^RAGFLOW_IMAGE=' docker/.env
```

### 1.3. Start one deployment and monitor migration

Start the target release:

```bash
docker compose -p docker -f docker/docker-compose.yml up -d
docker compose -p docker -f docker/docker-compose.yml ps --all
```

From the `SERVICE` column, copy the RAGFlow application service name. Replace `<service-name>` below with that value:

```bash
docker compose -p docker -f docker/docker-compose.yml logs -f "<service-name>"
```

Keep one application replica running and do not resume data-writing tasks until the migration logs show no errors or unresolved warnings.

### 1.4. Verify the checkpoint

At every checkpoint, confirm that:

- Existing users can sign in and see their knowledge bases.
- Existing files can be listed and opened.
- Retrieval returns content from previously parsed documents.
- A new document can be uploaded, parsed, and retrieved.
- Model providers, credentials, model types, API endpoints, and tenant default models are correct.
- Existing Agents open and complete a representative run.
- The selected Elasticsearch or Infinity service and object storage are healthy.

Do not continue to the next checkpoint until failures at the current release have been resolved.

## 2. Handle each key release

These sections contain only the additional checks for each release. Use them together with the [v0.x-to-v0.x upgrade steps](#1-upgrade-between-v0x-releases).

### 2.1. v0.22.1

Use `v0.22.1` rather than `v0.22.0`. If a knowledge base used the embedding model from the former `full` image, select an available replacement model. If the compatibility check fails, use another compatible model or re-parse the existing chunks with the new model.

### 2.2. v0.25.6

This checkpoint is required. Start `v0.25.6`, complete the standard verification, and continue only after all checks pass.

### 2.3. v0.26.4

Keep `--init-model-provider-tables` enabled on the first start. After startup, check the migration logs and verify the providers, credentials, models, and tenant default models in the UI. Test each model type used by the deployment before continuing.

### 2.4. v0.27.2

Keep `--init-model-provider-tables` enabled on the first start. Confirm that the migration logs report success, then test model configuration, existing retrieval, and a new document upload and parse.

Do not continue until `v0.27.2` passes all checks. The older GraphRAG and RAPTOR configuration pages are no longer available, but previously generated content remains searchable.

## 3. Upgrade from v0.27.2 to v1.0.0-rc1

This mandatory step is different from the earlier checkpoints. It introduces the v1 runtime and performs an irreversible data migration.

### 3.1. Stop and back up v0.27.2

Make sure that document parsing, synchronization, and Agent tasks have finished. From the `v0.27.2` repository root, stop the deployment:

```bash
docker compose -p docker -f docker/docker-compose.yml down
```

If the deployment uses the four default MySQL, MinIO, Redis, and Elasticsearch volumes, create and verify the backup with:

```bash
backup_dir="backup-v0.27.2-$(date +%Y%m%d-%H%M%S)"
bash docker/migration.sh backup "$backup_dir"
for archive in mysql_backup.tar.gz minio_backup.tar.gz redis_backup.tar.gz es_backup.tar.gz; do
  tar tzf "$backup_dir/$archive" >/dev/null || exit 1
done
```

If the deployment uses Infinity or any non-default volume set, use the manual method in [Backup and Restore (v0.x)](./backup_and_restore_v0.md) for every recorded volume. Do not combine script-created and manually created volume archives.

- Continue only if all required archives pass the checks, then keep the backup unchanged for rollback.
- Also save `docker/.env`, custom certificates, mounted configuration files, and any additional volumes.
- Keep the Redis or Valkey backup for rollback only; do not restore it to v1.

### 3.2. Prepare the v1.0.0-rc1 deployment

`v1.0.0-rc1` includes several changes outside the database:

- Deprecated HTTP API aliases are removed.
- NATS replaces Redis as the message queue.
- Kvrocks stores cache and checkpoints.
- The local sandbox and the earlier Team/Me permission behavior are not supported in this release.

Redis or Valkey data files are not compatible with Kvrocks or NATS storage. Restore or reuse MySQL, object storage, and the same Elasticsearch or Infinity data, but start Kvrocks, NATS, and ClickHouse with new empty volumes. Do not mount the `v0.27.2` Redis or Valkey volume into any of these services. Finish or stop queued work before the backup, and submit any incomplete jobs again after the upgrade.

Prepare a separate repository directory at the `v1.0.0-rc1` tag. Use its Compose files and `.env` template, then copy the required values from the old configuration into the new template. Set the release image to:

```dotenv
RAGFLOW_IMAGE=infiniflow/ragflow:v1.0.0-rc1
```

When reusing compatible volumes on the same host, keep the `docker` volume-name prefix. On a new host, restore the MySQL, MinIO, and Elasticsearch or Infinity archives with their original names. Let Compose create new Kvrocks, NATS, and ClickHouse volumes.

Keep parallel application replicas stopped until one migration attempt has completed. For a custom orchestrated deployment, use that platform's documentation to reduce and restore the replica count. Large databases need enough startup time for schema changes and conversation-history backfills.

### 3.3. Start v1.0.0-rc1 and wait for migration

From the `v1.0.0-rc1` repository root, start the deployment:

```bash
docker compose -p docker -f docker/docker-compose.yml up -d
docker compose -p docker -f docker/docker-compose.yml logs -f ragflow-cpu
```

Keep the logs open until the migration finishes and RAGFlow starts. Press `Ctrl+C` to exit the log view without stopping the containers.

Check the container state and the HTTP health endpoint:

```bash
docker compose -p docker -f docker/docker-compose.yml ps
curl -f http://127.0.0.1/api/v1/system/healthz
```

The health-check command uses the default web port `80`. If `SVR_WEB_HTTP_PORT` has been changed in `docker/.env`, use that port in the URL.

If the logs report a migration error, stop the v1 deployment and preserve the logs. Do not retry against the partially migrated data. Restore the complete `v0.27.2` recovery point, resolve the cause in a separate test environment, and retry the upgrade only from the restored recovery point.

The migration is irreversible. Never start `v0.27.2` against a database that has been partially or fully migrated by `v1.0.0-rc1`.

## 4. Upgrade from v1.0.0-rc1 to a later v1.x release

First verify that `v1.0.0-rc1` started successfully and that its migration completed. If the target version is newer, create a new v1 backup and then follow [Upgrade between v1.x releases](./upgrade_between_v1_releases.md). Do not skip directly from `v0.27.2` to the later release.

## 5. Verify the upgraded deployment

Sign in to RAGFlow and confirm that existing data and configuration are present, then test document retrieval, a new document upload, one Chat, and one Agent run.

Keep the `v0.27.2` backup until the upgraded deployment has completed acceptance testing and operated successfully under normal workload.

## 6. Recover from a failed upgrade

Stop the target release and preserve its logs. Do not continue to the next checkpoint, rerun migrations blindly, or start an older application image against a database already modified by a newer release.

Restore the application and all persistent stores from the same checkpoint backup:

- The metadata database.
- Object storage and Elasticsearch or Infinity data.
- Cache, queue, analytics, and other enabled persistent volumes.
- Configuration files, certificates, and custom-mounted files.
- The matching RAGFlow image and Compose files.

RAGFlow does not provide an automatic reverse migration. Mixing an older application with newer database or service data is not a rollback.
