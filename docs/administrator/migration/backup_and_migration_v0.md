---
sidebar_position: 2
title: Backup and Migration (v0.x)
sidebar_label: Backup and Migration (v0.x)
slug: /backup_and_migration_v0
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Backup and Migration (v0.x)

:::info Version scope
This document applies to RAGFlow `v0.x` Docker deployments. For `v1.0.0-rc1` and later, see [Backup and Migration (v1.0.0-rc1 and Later)](./backup_and_migration.md).
:::

Use this guide to move a `v0.x` deployment to another host or create a recovery point before upgrading it. Complete the six steps in order. At the end, you will have a backup of the metadata database, uploaded objects, search index, cache data, and deployment configuration, together with a verified restored deployment.

## 1. Find the data to back up

From the repository root, first list the containers in the RAGFlow Compose project. This shows which services are enabled:

```bash
docker compose -f docker/docker-compose.yml ps
```

Next, list the Docker volumes on the host. You will compare this inventory with the services shown by the first command:

```bash
docker volume ls
```

If the deployment was started with a project name, include the same `-p` value in every Compose command. The migration script supports `-p` only in `v0.25.x` and later; step 3 provides the safe alternative for earlier releases. For example:

```bash
docker compose -p ragflow -f docker/docker-compose.yml ps
```

The project name determines the volume-name prefix. A default project named `docker` commonly uses these volumes:

| Data | Typical volume |
|------|----------------|
| Metadata database | `docker_mysql_data` |
| Uploaded objects in bundled MinIO | `docker_minio_data` |
| Redis data | `docker_redis_data` |
| Elasticsearch index | `docker_esdata01` |

Use these four default volumes as the starting point. Check the mounts of every active container and add the volumes for Infinity, OpenSearch, SereneDB, OceanBase, SeekDB, NATS, or any other enabled service. Back up external MySQL, object storage, and search services with the corresponding provider's procedure.

Also retain `docker/.env`, the configuration template, custom certificates, and custom-mounted files. Protect these files because they can contain credentials.

Before stopping the deployment, run `df -h` and `docker system df`. Make sure the host has enough free space for the volume archives and temporary backup files.

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

The available backup command depends on the source release and its Compose project name:

| Source release | Default Compose project named `docker` | Custom Compose project name |
|----------------|----------------------------------------|-----------------------------|
| `v0.20.x` | Use the manual volume procedure below. This release does not contain `docker/migration.sh`. | Use the manual volume procedure below. |
| `v0.21.x` through `v0.24.x` | The bundled script can back up the four default volumes. | Use the manual volume procedure below. The script in these releases does not support `-p` and always reads `docker_*` volumes. |
| `v0.25.x` and later `v0.x` | The bundled script can back up the four default volumes. | Pass the project name with `-p`. |

For a supported script-based backup, first display the syntax of the script in the checked-out release:

```bash
bash docker/migration.sh help
```

For the default `docker` project, create the four archives in a new backup directory:

```bash
bash docker/migration.sh backup my_ragflow_backup
```

On `v0.25.x` or later, a deployment with a custom Compose project name can use:

```bash
bash docker/migration.sh -p ragflow backup my_ragflow_backup
```

Do not pass `-p` to the script from `v0.21.x` through `v0.24.x`. For `v0.20.x`, for a custom project on an earlier release, or whenever a recorded volume is not covered by the script, create a new directory and repeat the following commands for **each exact volume name recorded in step 1**:

```bash
mkdir my_ragflow_backup
volume_name=ragflow_mysql_data
docker volume inspect "$volume_name" > /dev/null
docker run --rm -v "$volume_name":/source:ro -v "$PWD/my_ragflow_backup":/backup alpine:3.20 \
  tar czf "/backup/$volume_name.tar.gz" -C /source .
tar tzf "my_ragflow_backup/$volume_name.tar.gz" > /dev/null
```

Use a new directory for each backup. After all required volumes have been processed, list its contents:

```bash
ls -lh my_ragflow_backup
```

Confirm that every volume recorded in step 1 has a corresponding readable archive. A script-based backup is complete only for a deployment using all four default MySQL, MinIO, Redis, and Elasticsearch volumes. If the deployment uses another document engine, external storage, or additional persistent services, back up those volumes or services now. Keep all of them together as one backup set.

## 4. Transfer the backup

Copy the complete backup directory and retained configuration files to the target host. Keep archive filenames unchanged. You can transfer them with `scp`, `rsync`, removable storage, or another file-transfer method appropriate for your environment.

On the target host, list the transferred directory and confirm that its files and sizes match the source. Keep the source deployment and the backup available until the restored deployment passes the checks in step 6.

## 5. Restore on the target host

Install the same RAGFlow release on the target host and place the backup directory in the repository root. Keep all target services stopped.

Restore only into target volumes that do not yet exist. Both the bundled script and the manual procedure extract files without removing files already present in a volume. Restoring into a used volume can therefore mix old and restored data.

Use the bundled restore command only when the backup was created by a compatible version of the script and contains all four default archives. For a default `docker` project, run:

```bash
bash docker/migration.sh restore my_ragflow_backup
```

On `v0.25.x` or later, a target with a custom Compose project name can use:

```bash
bash docker/migration.sh -p ragflow restore my_ragflow_backup
```

Do not use `-p` with the script from `v0.21.x` through `v0.24.x`. Use the following manual procedure for a `v0.20.x` backup, a custom project on an earlier release, a backup whose archive names are the recorded volume names, or a deployment that does not have all four script archives. Repeat it for every saved volume, changing `source_volume` and `target_volume` when the target project uses a different prefix:

```bash
source_volume=ragflow_mysql_data
target_volume=ragflow_mysql_data
if docker volume inspect "$target_volume" > /dev/null 2>&1; then
  echo "Target volume already exists; stop and inspect it: $target_volume"
else
  docker volume create "$target_volume"
  docker run --rm -v "$target_volume":/target -v "$PWD/my_ragflow_backup":/backup:ro alpine:3.20 \
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

When this backup is part of a version upgrade, first verify the restored source version. Then follow [Upgrade to v1.0.0-rc1](./upgrade_guide.md), starting and validating every intermediate version in the selected route.

After all checks pass, the backup and restore are complete. Keep the application version, Compose files, configuration, database, object data, and search index together so that the same recovery point can be used again when needed.
