---
sidebar_position: 2
title: Backup and Restore (v0.x)
sidebar_label: Backup and Restore (v0.x)
slug: /backup_and_restore_v0
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
docker compose -p docker -f docker/docker-compose.yml ps --all
```

Display the mounts of every container in the Compose project:

```bash
for container_id in $(docker compose -p docker -f docker/docker-compose.yml ps --all -q); do
  docker inspect "$container_id" --format '{{.Name}}{{range .Mounts}}{{println "" .Type .Source .Name .Destination}}{{end}}'
done
```

The `docker` project commonly uses these volumes:

| Data | Typical volume |
|------|----------------|
| Metadata database | `docker_mysql_data` |
| Uploaded objects in bundled MinIO | `docker_minio_data` |
| Redis data | `docker_redis_data` |
| Elasticsearch index | `docker_esdata01` |

Use these four default volumes as the starting point. If the deployment uses Infinity instead of Elasticsearch, include its persistent data. Add every named volume and persistent host directory shown by the mount check.

If any RAGFlow data is stored outside the Docker volumes listed above, back it up at the same time.

For a non-standard storage layout, identify the required data by following the Docker documentation for [volumes](https://docs.docker.com/engine/storage/volumes/) and [bind mounts](https://docs.docker.com/engine/storage/bind-mounts/).

Also retain `docker/.env`, the configuration template, custom certificates, and custom-mounted files. Protect these files because they can contain credentials.

Before stopping the deployment, run `df -h` and `docker system df`. Make sure the host has enough free space for the volume archives and temporary backup files.

For an offline host, cache the image required by the chosen method before stopping RAGFlow: `alpine` for `docker/migration.sh`, or `alpine:3.20` for the manual commands below.

## 2. Stop RAGFlow

Stop RAGFlow and its bundled services. This keeps the database, object storage, and search index at the same point in time while the backup is created:

```bash
docker compose -p docker -f docker/docker-compose.yml down
```

Wait until the command finishes before creating the archives.

## 3. Back up the persistent volumes

Create a timestamped backup directory:

```bash
backup_dir="backup-ragflow-v0-$(date +%Y%m%d-%H%M%S)"
mkdir "$backup_dir"
```

The `date` command adds the current time to the directory name so that files from different backups are not mixed.

Choose one backup method:

| Deployment | Backup method |
|------------|---------------|
| `v0.20.1` or later with the four default MySQL, MinIO, Redis, and Elasticsearch volumes | Use the bundled script. |
| `v0.20.0`, Infinity, or any non-default volume set | Use the manual procedure for every recorded volume. |

For a deployment with the four default volumes, create the archives with:

```bash
bash docker/migration.sh backup "$backup_dir"
```

Otherwise, do not use the script. Repeat the following commands for **every exact volume name recorded in step 1**:

```bash
volume_name=docker_mysql_data
docker volume inspect "$volume_name" >/dev/null &&
  docker run --rm -v "$volume_name":/source:ro -v "$PWD/$backup_dir":/backup alpine:3.20 \
    tar czf "/backup/$volume_name.tar.gz" -C /source .
```

After all required volumes have been processed, list its contents:

```bash
for archive in "$backup_dir"/*.tar.gz; do tar tzf "$archive" > /dev/null || exit 1; done
ls -lh "$backup_dir"
```

Confirm that every recorded volume has a readable archive and keep all files in the same backup set.

## 4. Transfer the backup

Copy the complete backup directory and retained configuration files to the target host. Keep archive filenames unchanged. Use a transfer method appropriate for your environment.

On the target host, list the transferred directory and confirm that its files and sizes match the source. Keep the source deployment and the backup available until the restored deployment passes the checks in step 6.

## 5. Restore on the target host

Install the same RAGFlow release on the target host and place the backup directory in the repository root. Keep all target services stopped.

Set `backup_dir` to the transferred directory name:

```bash
backup_dir="<backup-directory-name>"
```

Restore only into target volumes that do not yet exist. Both the bundled script and the manual procedure extract files without removing files already present in a volume. Restoring into a used volume can therefore mix old and restored data.

Use the bundled restore command only when the backup was created by a compatible version of the script and contains all four default archives:

```bash
bash docker/migration.sh restore "$backup_dir"
```

If the backup was created manually, set `volume_name` to each saved volume and repeat the following procedure:

```bash
volume_name=docker_mysql_data
if ! test -f "$backup_dir/$volume_name.tar.gz"; then
  echo "Missing archive: $backup_dir/$volume_name.tar.gz"
elif docker volume inspect "$volume_name" > /dev/null 2>&1; then
  echo "Target volume already exists; stop and inspect it: $volume_name"
else
  docker volume create "$volume_name" &&
    docker run --rm -v "$volume_name":/target -v "$PWD/$backup_dir":/backup:ro alpine:3.20 \
      tar xzf "/backup/$volume_name.tar.gz" -C /target
fi
```

If the deployment uses Infinity, restore it from its saved volume or with its storage-specific procedure. Restore any additional volumes and external services from the same backup set before continuing.

If a manual restore fails, do not use the incomplete target volume. After confirming that it contains no data that must be retained, replace it with a new empty volume before retrying the restore.

If the restore script reports that a target volume already exists, enter `n` and stop. Restore only after replacing it with a confirmed empty volume.

## 6. Start and verify the restored release

Start the same RAGFlow version that created the backup. The first command starts the containers; the second displays their current state:

```bash
docker compose -p docker -f docker/docker-compose.yml up -d
docker compose -p docker -f docker/docker-compose.yml ps
```

Wait until the services report a running or healthy state. Then confirm that:

- Existing users can sign in.
- Knowledge bases and files are present.
- Existing files can be opened.
- Retrieval returns content from previously parsed documents.
- A new document can be uploaded, parsed, and retrieved.
- The configured model providers and default models are available.
- Agents and enabled data-source synchronization still work.

When this backup is part of a version upgrade, first verify the restored source version. Then follow [Upgrade from v0.x to v1.x](./upgrade_from_v0_to_v1.md), starting and validating every intermediate version in the standard route.

After all checks pass, the backup and restore are complete.
