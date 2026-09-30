---
sidebar_position: 1
title: Upgrade to v1.0.0-rc1
sidebar_label: Upgrade to v1.0.0-rc1
slug: /upgrade_guide
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Upgrade to v1.0.0-rc1

This guide describes how to upgrade a Docker deployment from RAGFlow `v0.20.x` to `v1.0.0-rc1`. It identifies safe intermediate releases, the compatibility changes that require manual action, and the checks to complete before continuing to the next release.

The only intermediate version explicitly required by the `v1.0.0-rc1` release is `v0.27.2`. For older deployments, however, upgrading through additional checkpoints makes migration problems easier to isolate and reduces the amount of change introduced at one time.

This guide assumes the default MySQL-based Docker deployment. For a manually started deployment, also follow [Database Schema and Migration](./database_schema_and_migration.md).

## Choose an upgrade route

Use the standard route for most deployments:

```text
v0.20.x
  -> v0.22.1
  -> latest v0.25.x patch release
  -> v0.27.2
  -> v1.0.0-rc1
```

This route stops at the main compatibility boundaries:

- `v0.22.1` resolves the bundled embedding model transition introduced in `v0.22.0`.
- `v0.25.x` introduces the newer model configuration and database migration tooling.
- `v0.27.2` is the required starting point for the `v1.0.0-rc1` data upgrade.

For a business-critical deployment or a large database, use the conservative route:

```text
v0.20.x
  -> v0.21.1
  -> v0.22.1
  -> v0.23.1
  -> v0.24.0
  -> latest v0.25.x patch release
  -> latest v0.26.x patch release
  -> v0.27.2
  -> v1.0.0-rc1
```

The conservative route is an engineering risk-control recommendation. RAGFlow does not officially require every listed minor release. Its purpose is to reduce the size of each change, create more recovery points, and make it easier to identify the release that introduced a migration or compatibility problem.

If your current deployment is already partway through a route, begin with the next checkpoint. Do not install an older release over a newer database.

The following rules apply regardless of which route you choose:

| Upgrade scenario | Backup and recovery requirement | What completes the upgrade |
|------------------|---------------------------------|----------------------------|
| Between any two checkpoints | Preserve all persistent volumes and configuration used by the running deployment. For critical deployments, create a separate recovery point before each step. | Deploy **and start** the target release, allow its database work to finish, and verify the application before continuing. |
| An older release to `v0.27.2` | Copying or restoring volumes is only preparation; it does not prove that the upgrade succeeded. | Start `v0.27.2`, wait for the database and model configuration migrations to complete, and pass the `v0.27.2` checks. |
| `v0.27.2` to `v1.0.0-rc1` | Back up the stopped, fully verified `v0.27.2` deployment, including every persistent volume it uses. | Start `v1.0.0-rc1` and allow its irreversible data migration to complete before accepting traffic. |

## Before the first upgrade

1. Record the exact RAGFlow version, Docker Compose project name, enabled services, search engine, storage backend, and custom-mounted files.
2. Stop or drain document imports, parsing jobs, Agents, data-source synchronization, and every other process that can write data.
3. Back up all persistent volumes and configuration files from the same stopped deployment. The legacy migration script covers only four stores and is not a complete backup when the deployment uses another document engine or additional persistent services. Follow [Backup and Migration (v0.x)](./backup_and_migration_v0.md).
4. Restore the backup in a test environment and confirm that the restored release can start and retrieve existing documents.
5. Export any Agent configuration that can be retained. Agents created before `v0.20.0` are incompatible with `v0.20.0` and later and must be rebuilt.

Keep the original backup unchanged until the final release has passed acceptance testing.

## Repeat this process at every checkpoint

Treat each arrow in the selected route as a separate upgrade. You **must start and verify every target release in the route**. Changing the image tag repeatedly, or copying volumes without starting the intermediate release, does not execute that release's database and data migrations.

### 1. Stop writers and create a checkpoint

Stop the current deployment cleanly. For critical deployments, create a new backup before each version change so that every checkpoint has its own recovery point.

### 2. Deploy the target release files

Use the Docker Compose files, environment template, and image shipped with the target release. Merge your settings into that release's template instead of carrying an old Compose file forward unchanged. Services, command-line options, environment variables, and persistent volumes can change between releases.

### 3. Start one deployment and monitor migration

Start the target release without allowing parallel application replicas to race through initialization. Starting the release is a required part of the upgrade: its initialization path applies the schema and data changes owned by that release. Follow the RAGFlow container logs until this work has finished.

Do not use container health alone as the success criterion. Review migration errors and warnings before enabling users, workers, or synchronization jobs.

### 4. Verify the checkpoint

At every checkpoint, confirm that:

- Existing users can sign in and see their knowledge bases.
- Existing files can be listed and opened.
- Retrieval returns content from previously parsed documents.
- A new document can be uploaded, parsed, and retrieved.
- Model providers, credentials, model types, API endpoints, and tenant default models are correct.
- Existing Agents open and complete a representative run.
- The selected search engine and object storage are healthy.

Do not continue to the next checkpoint until failures at the current release have been resolved.

## What to handle at each key release

### v0.21.1

This is an optional checkpoint in the conservative route. Use it to verify that the existing `v0.20.x` deployment can complete a normal minor-version upgrade before the bundled embedding model is removed in `v0.22.x`.

No additional data rebuild is required specifically for this checkpoint. Complete the standard checkpoint verification before continuing.

### v0.22.1

Starting with `v0.22.0`, RAGFlow no longer ships the `full` image containing a bundled embedding model. If a knowledge base depends on that model, the model can be missing after the upgrade, preventing new documents from being parsed or causing retrieval to fail.

Use `v0.22.1`, rather than `v0.22.0`, as the checkpoint. `v0.22.1` fixes the upgrade path for knowledge bases that contain chunks created with a model from the former `full` image.

Configure an available replacement embedding model. When the knowledge base already contains chunks, RAGFlow samples existing content, re-embeds it with the candidate model, and calculates the average cosine similarity between the old and new vectors.

- If the average similarity is greater than `0.9`, the model can be switched without rebuilding the knowledge base.
- If the check fails, try another compatible model. To use an incompatible model, delete and re-parse the existing chunks.

This compatibility check does not create a general requirement to delete or recreate knowledge bases.

### v0.23.1 and v0.24.0

These are optional checkpoints in the conservative route. They reduce the version gap before the model configuration changes introduced in `v0.25.x`.

Run the standard verification at each release. Pay particular attention to document parsing, retrieval, Agent execution, and external model calls. No general data rebuild is required solely because the deployment passes through these versions.

### Latest v0.25.x patch release

The `v0.25.x` line introduces database migration tooling for the newer model configuration structure. Verify that existing model settings are represented correctly as Provider, Instance, and Model data.

After startup, inspect both the migration logs and the model configuration UI. Verify every provider's API endpoint and credentials, each model's type, and the tenant default models. Do not proceed while model records are missing, duplicated, or assigned the wrong type.

Use the latest available `v0.25.x` patch release instead of the initial `v0.25.0` release so that the checkpoint includes subsequent fixes from the same release line.

### Latest v0.26.x patch release

This is an optional checkpoint in the conservative route. Use the latest available `v0.26.x` patch release.

Recheck all migrated model settings and run representative embedding, chat, rerank, vision, speech, and OCR calls for the model types your deployment uses. This checkpoint gives you an additional recovery point before the final `v0.x` release required by `v1.0.0-rc1`.

### v0.27.2

Every route must reach and successfully run `v0.27.2` before continuing to `v1.0.0-rc1`. Restoring the old volumes into a `v0.27.2` deployment directory, without starting and validating `v0.27.2`, does not satisfy this prerequisite.

Use the standard `v0.27.2` Docker Compose configuration and keep `--init-model-provider-tables` enabled for the first start. The corresponding migration runs the model-provider stages, normalizes stored model identifiers, completes the Provider, Instance, and Model migration, and records the database at `v0.27.2`.

Start `v0.27.2` and confirm that the container logs report completion of the database and model-provider migrations. Then perform the full checkpoint verification, including a new document ingestion and representative calls to every configured model type.

The legacy GraphRAG and RAPTOR configuration interfaces were removed in `v0.27.0` and replaced by Graph and Tree in Knowledge Compilation. Previously generated GraphRAG and RAPTOR content remains searchable; do not rebuild it only because the old interface is no longer present.

Resolve all migration and configuration problems on `v0.27.2`. Do not use `v1.0.0-rc1` to repair an incomplete `v0.27.2` upgrade.

## Upgrade from v0.27.2 to v1.0.0-rc1

This final step is different from the earlier checkpoints. It introduces the `v1.0.0-rc1` runtime and performs an irreversible data migration.

### 1. Create a verified v0.27.2 rollback point

After `v0.27.2` has been started, migrated, and fully verified, stop all writers and create a fresh backup. Keep this backup separate from every earlier checkpoint.

The backup must include the metadata database, object storage, search engine data, configuration, certificates, custom-mounted files, and every persistent volume used by the deployment. Follow [Backup and Migration (v0.x)](./backup_and_migration_v0.md). This is the only supported way to return to `v0.27.2` after the final migration.

### 2. Review application compatibility

`v1.0.0-rc1` includes several changes outside the database:

- RAGFlow CLI commands and backend behavior change in `v1.0.0-rc1`.
- Deprecated HTTP API aliases are removed.
- NATS replaces Redis as the message queue.
- Kvrocks stores cache and checkpoints.
- The local sandbox and the earlier Team/Me permission behavior are not supported in this release.

Update dependent scripts and integrations before switching production traffic.

### 3. Deploy the v1.0.0-rc1 configuration

Use the Compose files, environment template, and image shipped with `v1.0.0-rc1`. Merge the verified settings from `v0.27.2` into the new template. Do not reuse the older Compose definition unchanged, because the `v1.0.0-rc1` deployment requires different supporting services and volumes.

Keep parallel application replicas stopped until one migration attempt has completed. Large databases need enough startup time for schema changes and conversation-history backfills.

### 4. Start and monitor the migration

In the standard Docker deployment, the entrypoint runs `ragflow_server --migrate` before starting the enabled server modes. The migration applies manual data changes, converges the schema, updates model data, and moves conversation messages and references into their new tables.

Watch the logs until the migration completes. Inspect warnings as well as errors; a running container or successful process exit does not by itself prove that every schema change and built-in data update succeeded.

The migration is irreversible. Never start `v0.27.2` against a database that has been partially or fully migrated by `v1.0.0-rc1`.

### 5. Accept the upgraded deployment

Repeat the standard checkpoint verification, then also confirm that:

- Existing conversation history is visible.
- API and CLI integrations use interfaces supported by `v1.0.0-rc1`.
- Ingestion workers can receive and complete new jobs.
- NATS and Kvrocks are healthy and use the intended persistent volumes.
- File synchronization and other optional services work when enabled.

Keep the `v0.27.2` backup until the upgraded deployment has completed acceptance testing and operated successfully under normal workload.

## If an upgrade step fails

Stop the target release and preserve its logs. Do not continue to the next checkpoint, rerun migrations blindly, or start an older application image against a database already modified by a newer release.

Restore the application and all persistent stores from the same checkpoint backup:

- The metadata database.
- Object storage and search engine data.
- Cache, queue, analytics, and other enabled persistent volumes.
- Configuration files, certificates, and custom-mounted files.
- The matching RAGFlow image and Compose files.

RAGFlow does not provide an automatic reverse migration. Mixing an older application with newer database or service data is not a rollback.
