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

This guide describes how to upgrade a Docker deployment from RAGFlow `v0.20.0` to `v1.0.0-rc1`. It identifies safe intermediate releases, the compatibility changes that require manual action, and the checks to complete before continuing to the next release.

The only intermediate version explicitly required by the `v1.0.0-rc1` release is `v0.27.2`. For older deployments, however, upgrading through additional checkpoints makes migration problems easier to isolate and reduces the amount of change introduced at one time.

This guide applies to MySQL-based Docker deployments.

If the current deployment is older than `v0.20.0`, first upgrade it to `v0.20.0` using the files and instructions supplied with that release, start it, and verify its data before following this guide. Agents created before `v0.20.0` are incompatible with `v0.20.0` and later. Their definitions may be retained as a reference, but the Agents themselves must be rebuilt and tested after reaching `v0.20.0`.

## Choose an upgrade route

Use the standard route for most deployments:

```text
v0.20.0
  -> v0.22.1
  -> v0.25.6
  -> v0.26.4
  -> v0.27.2
  -> v1.0.0-rc1
```

This route stops at the main compatibility boundaries:

- `v0.22.1` resolves the bundled embedding model transition introduced in `v0.22.0`.
- `v0.25.6` is the tested checkpoint for migrating and verifying the newer model configuration structure.
- `v0.26.4` provides a checkpoint for verifying the migrated model configuration before the final `v0.x` release.
- `v0.27.2` is the required starting point for the `v1.0.0-rc1` data upgrade.

For a business-critical deployment or a large database, use the conservative route:

```text
v0.20.0
  -> v0.21.1
  -> v0.22.1
  -> v0.23.1
  -> v0.24.0
  -> v0.25.6
  -> v0.26.4
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
2. Run `df -h` and `docker system df`. Make sure the host has enough free space for the backup, the target release image and volumes, and migration work before starting the upgrade.
3. Stop or drain document imports, parsing jobs, Agents, data-source synchronization, and every other process that can write data.
4. Back up all persistent volumes and configuration files from the same stopped deployment. The legacy migration script covers only four stores and is not a complete backup when the deployment uses another document engine or additional persistent services. Follow [Backup and Migration (v0.x)](./backup_and_migration_v0.md).
5. Restore the backup in a test environment and confirm that the restored release can start and retrieve existing documents.
6. Export the configuration of any Agent that must be retained. If the original deployment was older than `v0.20.0`, use this export only as a reference: rebuild and test every such Agent after reaching `v0.20.0`.

:::warning Isolate same-host tests
Use a separate Compose project name, host ports, and Docker volumes. Do not connect the test deployment to production data stores.
:::

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

### v0.25.6

`v0.25.6` is the tested checkpoint for migrating and verifying the newer model configuration structure. Verify that existing model settings are represented correctly as Provider, Instance, and Model data.

After startup, inspect both the migration logs and the model configuration UI. Verify every provider's API endpoint and credentials, each model's type, and the tenant default models. Do not proceed while model records are missing, duplicated, or assigned the wrong type.

Use `v0.25.6` for this checkpoint. This is the version covered by the tested upgrade route.

### v0.26.4

Use `v0.26.4` to verify the migrated model configuration before continuing to `v0.27.2`. This is the version covered by the tested upgrade route.

Recheck all migrated model settings and run representative embedding, chat, rerank, vision, speech, and OCR calls for the model types your deployment uses. This checkpoint gives you an additional recovery point before the final `v0.x` release required by `v1.0.0-rc1`.

### v0.27.2

Every route must reach and successfully run `v0.27.2` before continuing to `v1.0.0-rc1`. Restoring the old volumes into a `v0.27.2` deployment directory, without starting and validating `v0.27.2`, does not satisfy this prerequisite.

Use the standard `v0.27.2` Docker Compose configuration and keep `--init-model-provider-tables` enabled for the first start. The corresponding migration runs the model-provider stages, normalizes stored model identifiers, completes the Provider, Instance, and Model migration, and records the database at `v0.27.2`.

Start `v0.27.2` and confirm that the container logs report completion of the database and model-provider migrations. Then perform the full checkpoint verification, including a new document ingestion and representative calls to every configured model type.

The legacy GraphRAG and RAPTOR configuration interfaces were removed in `v0.27.0` and replaced by Graph and Tree in Knowledge Compilation. Previously generated GraphRAG and RAPTOR content remains searchable; do not rebuild it only because the old interface is no longer present.

Resolve all migration and configuration problems on `v0.27.2`. Do not use `v1.0.0-rc1` to repair an incomplete `v0.27.2` upgrade.

## Upgrade from v0.27.2 to v1.0.0-rc1

This final step is different from the earlier checkpoints. It introduces the `v1.0.0-rc1` runtime and performs an irreversible data migration.

### 1. Stop and back up v0.27.2

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

Also save `docker/.env`, custom certificates, mounted configuration files, and the data of any enabled storage service not covered by the command. Confirm that the backup contains the expected data before continuing. Keep the `v0.27.2` Redis or Valkey backup for rollback; it is not restored into a v1 service. See [Backup and Migration (v0.x)](./backup_and_migration_v0.md) for deployments that use another document engine or additional volumes.

### 2. Prepare the v1.0.0-rc1 deployment

`v1.0.0-rc1` includes several changes outside the database:

- RAGFlow CLI commands and backend behavior change in `v1.0.0-rc1`.
- Deprecated HTTP API aliases are removed.
- NATS replaces Redis as the message queue.
- Kvrocks stores cache and checkpoints.
- The local sandbox and the earlier Team/Me permission behavior are not supported in this release.

Redis or Valkey data files are not compatible with Kvrocks or NATS storage. Restore or reuse MySQL, object storage, and the same type of search engine, but start Kvrocks, NATS, and ClickHouse with new empty volumes. Do not mount the `v0.27.2` Redis or Valkey volume into any of these services. Finish or stop queued work before the backup, and submit any incomplete jobs again after the upgrade.

Prepare a separate repository directory at the `v1.0.0-rc1` tag. Use its Compose files and `.env` template, then copy the required values from the old configuration into the new template. Set the release image in the new `.env` file:

```dotenv
RAGFLOW_IMAGE=infiniflow/ragflow:v1.0.0-rc1
```

Use the same `project_name` when reusing compatible volumes on the same host. When restoring into a new project or host, restore the MySQL, MinIO, and same-type search-engine archives to the volume names expected by that project. Let Compose create new Kvrocks, NATS, and ClickHouse volumes.

Keep parallel application replicas stopped until one migration attempt has completed. Large databases need enough startup time for schema changes and conversation-history backfills.

### 3. Start v1.0.0-rc1 and wait for migration

From the `v1.0.0-rc1` repository root, start the deployment with the same Compose project name:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml up -d
application_service="$(docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps --all --services | sed -n '/^ragflow-\(cpu\|gpu\)$/p' | head -n 1)"
if test -z "$application_service"; then
  echo "No RAGFlow application service was found"
else
  docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml logs -f "$application_service"
fi
```

The container entrypoint runs the data migration before starting the application. Keep the log command open until migration finishes and the application starts. Exit the log view with `Ctrl+C`; this does not stop the containers.

Check the container state and the HTTP health endpoint:

```bash
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps
curl -f http://127.0.0.1/api/v1/system/healthz
```

The health-check command uses the default web port `80`. If `SVR_WEB_HTTP_PORT` has been changed in `docker/.env`, use that port in the URL.

If the logs report a migration error, stop the v1 deployment and preserve the logs. Do not retry against the partially migrated data. Restore the complete `v0.27.2` recovery point, resolve the cause in a separate test environment, and retry the upgrade only from the restored recovery point.

The migration is irreversible. Never start `v0.27.2` against a database that has been partially or fully migrated by `v1.0.0-rc1`.

### 4. Verify the upgraded deployment

Sign in to RAGFlow and confirm that existing data and configuration are present, then test document retrieval, a new document upload, one Chat, and one Agent run.

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
