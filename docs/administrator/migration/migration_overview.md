---
sidebar_position: 0
title: Migration Guide
sidebar_label: Start Here
slug: /migration_overview
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Migration Guide

Use this page to identify the correct path, prepare a recoverable backup, and then open the procedure for your task.

:::danger Mandatory and irreversible v0.x to v1.x upgrade
Every `v0.x` deployment must successfully start and pass verification on `v0.27.2`, and must then be upgraded to `v1.0.0-rc1`. Do not skip either version or upgrade directly from `v0.x` to a later `v1.x` release.

The `v0.27.2` to `v1.0.0-rc1` migration is irreversible. Before starting it, stop all data-writing tasks and create a complete, verified `v0.27.2` recovery point. If migration fails, do not retry against the partially migrated data; restore the complete recovery point before trying again.
:::

## 1. Document map: choose your task

| Current version | Goal | Follow these guides |
|-----------------|------|---------------------|
| `v0.x` | Back up, restore, or move the deployment to another host | [Backup and Restore (v0.x)](./backup_and_restore_v0.md) |
| `v0.x` | Upgrade to the current latest version | Create and verify an upgrade recovery point by following [Backup and Restore (v0.x)](./backup_and_restore_v0.md), then follow [Upgrade from v0.x to v1.x](./upgrade_from_v0_to_v1.md) |
| `v1.0.0-rc1` or later | Back up, restore, or move the deployment to another host | [Backup and Restore (v1.x)](./backup_and_restore_v1.md) |
| `v1.0.0-rc1` or later | Upgrade to another release | First complete [Backup and Restore (v1.x)](./backup_and_restore_v1.md), then follow [Database Migration for v1.x](./database_migration_v1.md) |

## 2. Choose an upgrade route from v0.x

For most deployments, use the standard route:

```text
v0.20.0 -> v0.22.1 -> v0.25.6 -> v0.26.4 -> v0.27.2 -> v1.0.0-rc1
                                                               -> later v1.x release (if needed)
```

For a business-critical deployment or a large database, use the conservative route:

```text
v0.20.0 -> v0.21.1 -> v0.22.1 -> v0.25.6 -> v0.26.4
          -> v0.27.2 -> v1.0.0-rc1 -> later v1.x release (if needed)
```

The key checkpoints are:

| Checkpoint | Why it is included |
|------------|--------------------|
| [`v0.22.1`](./upgrade_from_v0_to_v1.md#22-v0221) | Handles the bundled embedding model transition. |
| [`v0.25.6`](./upgrade_from_v0_to_v1.md#23-v0256) | Required compatibility checkpoint before the model-provider migration. |
| [`v0.26.4`](./upgrade_from_v0_to_v1.md#24-v0264) | Runs and verifies the base model-provider migration. |
| [`v0.27.2`](./upgrade_from_v0_to_v1.md#25-v0272) | Completes the required v0.x migration work. |
| [`v1.0.0-rc1`](./upgrade_from_v0_to_v1.md#3-upgrade-from-v0272-to-v100-rc1) | Mandatory, irreversible boundary between v0.x and v1.x. |

If the current deployment is already on one of these routes, begin with the next checkpoint. Never start an older RAGFlow version against a database that has already been used by a newer version.

If the current deployment is older than `v0.20.0`, first upgrade it to `v0.20.0` using the files supplied with that release. Agents created before `v0.20.0` are incompatible with `v0.20.0` and later; retain their definitions only as a reference, then rebuild and test them after reaching `v0.20.0`.

## 3. Prepare before changing versions

1. Record the current RAGFlow version, Compose project name, enabled services, search engine, storage backend, and custom-mounted files.
2. Stop document imports, parsing jobs, Agents, synchronization, and other data-writing tasks.
3. Back up the configuration and every persistent volume from the same stopped deployment.
4. Restore the backup in a separate test environment and verify that the restored version can start and retrieve existing documents.
5. Keep the original backup unchanged until that version has passed acceptance testing.

When testing on the same host, use a different Compose project name, host ports, and volumes. Do not connect the test deployment to production data stores.

## 4. Complete the migration safely

For a version upgrade, start and verify every checkpoint in the selected route. Merely changing image tags or copying volumes does not run the checkpoint's data migration.

At each checkpoint, verify sign-in, existing files, retrieval, a new document upload and parse, model configuration, and a representative Agent run. Continue only after the current checkpoint passes these checks.

The `v0.27.2` to `v1.0.0-rc1` migration is irreversible. If it fails, stop `v1.0.0-rc1` and restore the application version, configuration, and all persistent data from the same `v0.27.2` recovery point. Do not retry against partially migrated data.
