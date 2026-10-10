---
sidebar_position: 3
title: "Backup and Restore (v1.x)"
sidebar_label: "Backup and Restore (v1.x)"
slug: /backup_and_restore_v1
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Backup and Restore (v1.x)

:::info Backup and host migration only
This guide creates or restores a same-version recovery point and can move a deployment to another host. To upgrade an existing v1 deployment to a later release, create this backup first and then follow [Upgrade between v1.x releases](./upgrade_between_v1_releases.md).
:::

Choose the steps for your task:

- **Create an upgrade recovery point:** Complete [steps 1–2](#1-find-the-data-to-back-up), verify every archive, and keep the backup unchanged.
- **Move to another host:** Complete all four steps, including restore and verification.

## 1. Find the data to back up

List all containers in the `docker` Compose project to identify the services that are enabled:

```bash
docker compose -p docker -f docker/docker-compose.yml ps --all
```

Display the mounts of every container in the Compose project:

```bash
for container_id in $(docker compose -p docker -f docker/docker-compose.yml ps --all -q); do
  docker inspect "$container_id" --format '{{.Name}}{{range .Mounts}}{{println "" .Type .Source .Name .Destination}}{{end}}'
done
```

Write down every RAGFlow volume name and every persistent host directory shown in the output. Use the container mounts as the backup list. RAGFlow volume names normally start with `docker_`.

Use the following table to check the list you recorded. Include each service enabled in your deployment, together with any additional or externally managed storage used in your environment.

| Service or data | Persistent location |
|-----------------|---------------------|
| Metadata database in bundled MySQL | `docker_mysql_data` |
| Uploaded objects in bundled MinIO | `docker_minio_data` |
| Elasticsearch index | `docker_esdata01` |
| Infinity index | `docker_infinity_data` |
| Kvrocks cache and checkpoint data | `docker_kvrocks_data` |
| NATS JetStream data | `docker_nats_data` |
| ClickHouse analytics data | `docker_clickhouse_data` |
| Kibana data, when Kibana is enabled | `docker_kibana_data` |
| RAGFlow logs | `docker/ragflow-logs` bind mount |

This table is a checklist. Always use the mounts reported by `docker inspect` as the final backup scope.

For a non-standard storage layout, identify the required data by following the Docker documentation for [volumes](https://docs.docker.com/engine/storage/volumes/) and [bind mounts](https://docs.docker.com/engine/storage/bind-mounts/).

RAGFlow logs are retained for troubleshooting. Missing logs do not affect the restoration of business data.
If you need the logs for troubleshooting, copy `docker/ragflow-logs` with the backup.

If any RAGFlow data is stored outside the Docker volumes listed above, back it up at the same time.

Also retain `docker/.env`, the configuration template, and any custom certificates or mounted files. Protect the backup because these files may contain credentials.

Before stopping the deployment, run `df -h` and `docker system df`. Make sure the host has enough free space for the volume archives and temporary backup files.

For an offline host, make sure that the `alpine:3.20` image is available before stopping RAGFlow.

## 2. Stop RAGFlow and create the backup

First, stop RAGFlow and its bundled services. This prevents the database, object storage, and search index from changing while their archives are being created:

```bash
docker compose -p docker -f docker/docker-compose.yml down
```

After all containers have stopped, create a new directory for this backup. The volume archives created below will be stored in this directory:

```bash
backup_dir="backup-ragflow-v1-$(date +%Y%m%d-%H%M%S)"
mkdir "$backup_dir"
```

The `date` command adds the current time to the directory name so that files from different backups are not mixed. Then repeat the following commands for **each volume you recorded**. Set `volume_name` to the exact Docker volume name:

```bash
volume_name=docker_mysql_data
docker volume inspect "$volume_name" >/dev/null &&
  docker run --rm -v "$volume_name":/source:ro -v "$PWD/$backup_dir":/backup alpine:3.20 \
    tar czf "/backup/$volume_name.tar.gz" -C /source . &&
  tar tzf "$backup_dir/$volume_name.tar.gz" >/dev/null
```

A successful command returns without an error and creates a readable archive.

After processing all volumes, list the archives:

```bash
ls -lh "$backup_dir"
```

Confirm that every recorded volume has a corresponding `.tar.gz` file. Keep the entire backup directory together with `docker/.env`, the configuration template, certificates, and other custom-mounted files.

## 3. Restore the backup on the target host

Copy the complete backup to the target host and install the same RAGFlow release. Restore the saved configuration, put the backup directory in the repository root, and keep all services stopped. Set `backup_dir` to the transferred directory name:

```bash
backup_dir="<backup-directory-name>"
```

For each archive, set `volume_name` to the volume name contained in the archive filename and run:

```bash
volume_name=docker_mysql_data
if ! test -f "$backup_dir/$volume_name.tar.gz"; then
  echo "Missing archive: $backup_dir/$volume_name.tar.gz"
elif docker volume inspect "$volume_name" >/dev/null 2>&1; then
  echo "Target volume already exists; inspect it before restoring: $volume_name"
else
  docker volume create "$volume_name" &&
    docker run --rm -v "$volume_name":/target -v "$PWD/$backup_dir":/backup:ro alpine:3.20 \
      tar xzf "/backup/$volume_name.tar.gz" -C /target
fi
```

If extraction fails, do not use the incomplete target volume. After confirming that it contains no data that must be retained, replace it with a new empty volume before retrying the restore.

Repeat this operation until every archive has been restored. Restore the metadata database, object data, search index, and other service data from the same backup set.

Restore every saved bind-mounted directory to the same location used on the source host.

## 4. Start and verify

Start RAGFlow with the restored volumes:

```bash
docker compose -p docker -f docker/docker-compose.yml up -d
```

Check the service states and the RAGFlow application logs:

```bash
docker compose -p docker -f docker/docker-compose.yml ps --all
docker compose -p docker -f docker/docker-compose.yml logs --tail=200 ragflow-cpu
```

Finally, sign in and confirm that existing knowledge bases and files are present, files can be opened, and retrieval returns content from previously parsed documents. After these checks pass, the backup and restore are complete.
