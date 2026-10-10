---
sidebar_position: 2
title: Backup and Restore (v0.x)
sidebar_label: Backup and Restore (v0.x)
slug: /backup_and_migration_v0
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Backup and Restore (v0.x)

:::info Version scope
This document applies to RAGFlow `v0.x` Docker deployments. For `v1.0.0-rc1` and later, see [Backup and Restore (v1.x)](./backup_and_restore_v1.md).
:::

:::warning This guide does not upgrade RAGFlow
This guide creates or restores a same-version recovery point and can move a deployment to another host. To upgrade from `v0.x` to `v1.x`, complete this backup first and then follow [Upgrade from v0.x to v1.x](./upgrade_from_v0_to_v1.md). The upgrade must pass through `v0.27.2` and then `v1.0.0-rc1`; that step is irreversible.
:::

Choose the steps for your task:

- **Create an upgrade recovery point:** Complete [steps 1–3](#1-find-the-data-to-back-up), verify every archive, and keep the backup unchanged.
- **Move to another host:** Complete all six steps, including transfer, restore, and verification.

## 1. Find the data to back up

From the repository root, first list the containers in the RAGFlow Compose project. This shows which services are enabled:

```bash
docker compose -f docker/docker-compose.yml ps --all
```

Next, list the Docker volumes on the host. You will compare this inventory with the services shown by the first command:

```bash
docker volume ls
```

Display the mounts of every container in the Compose project. The output shows whether each mount is a Docker volume or a host directory, its source, and its location inside the container:

```bash
project_name=docker
for container_id in $(docker compose -p "$project_name" -f docker/docker-compose.yml ps --all -q); do
  docker inspect "$container_id" --format '{{.Name}}{{range .Mounts}}{{println "" .Type .Source .Name .Destination}}{{end}}'
done
```

If the deployment was started with a project name, include the same `-p` value in every Compose command. The migration script supports `-p` only in `v0.25.x` and later; step 3 provides the safe alternative for earlier releases. For example:

```bash
docker compose -p ragflow -f docker/docker-compose.yml ps --all
```

The project name determines the volume-name prefix. A default project named `docker` commonly uses these volumes:

| Data | Typical volume |
|------|----------------|
| Metadata database | `docker_mysql_data` |
| Uploaded objects in bundled MinIO | `docker_minio_data` |
| Redis data | `docker_redis_data` |
| Elasticsearch index | `docker_esdata01` |

Use these four default volumes as the starting point. Add every named volume and persistent host directory shown by the mount check. Back up external MySQL, object storage, and search services with the corresponding provider's procedure.

Also retain `docker/.env`, the configuration template, custom certificates, and custom-mounted files. Protect these files because they can contain credentials.

Before stopping the deployment, run `df -h` and `docker system df`. Make sure the host has enough free space for the volume archives and temporary backup files.

For an offline host, cache the image required by the chosen method before stopping RAGFlow: `alpine` for `docker/migration.sh`, or `alpine:3.20` for the manual commands below.

## 2. Stop RAGFlow

Stop RAGFlow and its bundled services. This keeps the database, object storage, and search index at the same point in time while the backup is created:

```bash
docker compose -f docker/docker-compose.yml down
```

For a custom Compose project name:

```bash
docker compose -p ragflow -f docker/docker-compose.yml down
```

The command above stops and removes the containers while preserving their Docker volumes. Wait until it finishes before continuing, and create every archive from this stopped deployment.

## 3. Back up the persistent volumes

Create a timestamped backup directory:

```bash
backup_dir="backup-ragflow-v0-$(date +%Y%m%d-%H%M%S)"
mkdir "$backup_dir"
```

The `date` command adds the current time to the directory name so that files from different backups are not mixed.

The available backup command depends on the source release and its Compose project name:

| Source release | Default Compose project named `docker` | Custom Compose project name |
|----------------|----------------------------------------|-----------------------------|
| `v0.20.0` | Use the manual volume procedure below. This release does not contain `docker/migration.sh`. | Use the manual volume procedure below. |
| `v0.20.1` through `v0.24.x` | The bundled script can back up the four default volumes. | Use the manual volume procedure below. The script in these releases does not support `-p` and always reads `docker_*` volumes. |
| `v0.25.x` and later `v0.x` | The bundled script can back up the four default volumes. | Pass the project name with `-p`. |

For a supported script-based backup, first display the syntax of the script in the checked-out release:

```bash
bash docker/migration.sh help
```

For the default `docker` project, create the four archives in a new backup directory:

```bash
bash docker/migration.sh backup "$backup_dir"
```

On `v0.25.x` or later, a deployment with a custom Compose project name can use:

```bash
bash docker/migration.sh -p ragflow backup "$backup_dir"
```

Do not pass `-p` to the script from `v0.20.1` through `v0.24.x`. For `v0.20.0`, for a custom project on an earlier release, or whenever a recorded volume is not covered by the script, repeat the following commands for **each exact volume name recorded in step 1**:

```bash
volume_name=ragflow_mysql_data
docker volume inspect "$volume_name" > /dev/null
docker run --rm -v "$volume_name":/source:ro -v "$PWD/$backup_dir":/backup alpine:3.20 \
  tar czf "/backup/$volume_name.tar.gz" -C /source .
tar tzf "$backup_dir/$volume_name.tar.gz" > /dev/null
```

After all required volumes have been processed, list its contents:

```bash
for archive in "$backup_dir"/*.tar.gz; do tar tzf "$archive" > /dev/null || exit 1; done
ls -lh "$backup_dir"
```

Confirm that every volume recorded in step 1 has a corresponding readable archive. A script-based backup is complete only for a deployment using all four default MySQL, MinIO, Redis, and Elasticsearch volumes. If the deployment uses another document engine, external storage, or additional persistent services, back up those volumes or services now. Keep all of them together as one backup set.

## 4. Transfer the backup

Copy the complete backup directory and retained configuration files to the target host. Keep archive filenames unchanged. You can transfer them with `scp`, `rsync`, removable storage, or another file-transfer method appropriate for your environment.

On the target host, list the transferred directory and confirm that its files and sizes match the source. Keep the source deployment and the backup available until the restored deployment passes the checks in step 6.

## 5. Restore on the target host

Install the same RAGFlow release on the target host and place the backup directory in the repository root. Keep all target services stopped.

Set `backup_dir` to the transferred directory name:

```bash
backup_dir="<backup-directory-name>"
```

Restore only into target volumes that do not yet exist. Both the bundled script and the manual procedure extract files without removing files already present in a volume. Restoring into a used volume can therefore mix old and restored data.

Use the bundled restore command only when the backup was created by a compatible version of the script and contains all four default archives. For a default `docker` project, run:

```bash
bash docker/migration.sh restore "$backup_dir"
```

On `v0.25.x` or later, a target with a custom Compose project name can use:

```bash
bash docker/migration.sh -p ragflow restore "$backup_dir"
```

Do not use `-p` with the script from `v0.20.1` through `v0.24.x`. Use the following manual procedure for a `v0.20.0` backup, a custom project on an earlier release, a backup whose archive names are the recorded volume names, or a deployment that does not have all four script archives. Repeat it for every saved volume, changing `source_volume` and `target_volume` when the target project uses a different prefix:

```bash
source_volume=ragflow_mysql_data
target_volume=ragflow_mysql_data
if docker volume inspect "$target_volume" > /dev/null 2>&1; then
  echo "Target volume already exists; stop and inspect it: $target_volume"
else
  docker volume create "$target_volume"
  docker run --rm -v "$target_volume":/target -v "$PWD/$backup_dir":/backup:ro alpine:3.20 \
    tar xzf "/backup/$source_volume.tar.gz" -C /target
fi
```

Restore Infinity, OpenSearch, SereneDB, or another document engine from its saved volume or with its storage-specific procedure. Restore any additional volumes and external services from the same backup set before continuing.

If the restore script reports that any target volume already exists, enter `n` and stop. Check the Compose project name and the contents of the listed volumes. Continue only after choosing a new project name or, after separately confirming that the existing volumes contain no data that must be retained, replacing them with new empty volumes. Do not use the script's confirmation prompt to restore over a previously used volume. When the script finishes, confirm that it reports a successful restore for all four archives.

## 6. Start and verify the restored release

Start the same RAGFlow version that created the backup. The first command starts the containers; the second displays their current state:

```bash
docker compose -f docker/docker-compose.yml up -d
docker compose -f docker/docker-compose.yml ps
```

For a custom Compose project name:

```bash
docker compose -p ragflow -f docker/docker-compose.yml up -d
docker compose -p ragflow -f docker/docker-compose.yml ps
```

Wait until the services report a running or healthy state. Then confirm that:

- Existing users can sign in.
- Knowledge bases and files are present.
- Existing files can be opened.
- Retrieval returns content from previously parsed documents.
- A new document can be uploaded, parsed, and retrieved.
- The configured model providers and default models are available.
- Agents and enabled data-source synchronization still work.

When this backup is part of a version upgrade, first verify the restored source version. Then follow [Upgrade from v0.x to v1.x](./upgrade_from_v0_to_v1.md), starting and validating every intermediate version in the selected route.

After all checks pass, the backup and restore are complete. Keep the application version, Compose files, configuration, database, object data, and search index together so that the same recovery point can be used again when needed.
