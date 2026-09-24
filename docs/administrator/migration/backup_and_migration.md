---
sidebar_position: 2
title: "Go Backup and Migration"
sidebar_label: "Go Backup and Migration"
slug: /migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Go Backup and Migration

This guide explains how to move the default Go Docker deployment to another host.

The bundled `docker/migration.sh` covers only the MySQL, MinIO, Redis, and Elasticsearch volumes and is not a complete backup method for the Go Docker deployment. It does not include Kvrocks or optional service volumes. Use the volume procedure below to back up a Go deployment.

## Move a Go deployment to another host

### 1. Identify all persistent data

From the repository root, inspect the running deployment and its Docker volumes:

```bash
docker compose --env-file docker/.env -f docker/docker-compose.yml ps
docker volume ls
```

Record the Compose project name and the volumes actually mounted by its running containers before stopping them. For each relevant container shown by `ps`, check its mounts with `docker inspect <container-name> --format '{{range .Mounts}}{{println .Name .Destination}}{{end}}'`. `docker volume ls` also shows unrelated and unused volumes, so do not treat that list as the backup set. The default project name is usually `docker`; if you started Compose with `-p ragflow`, volume names normally start with `ragflow_`. Use the same `-p` value with every Compose command below.

The default Go stack can use these volumes, depending on the enabled services:

| Service or data | Typical volume |
| --- | --- |
| Metadata database | `<project>_mysql_data` |
| Uploaded objects in bundled MinIO | `<project>_minio_data` |
| Search index with Elasticsearch | `<project>_esdata01` |
| Go cache and persistent Redis-protocol data | `<project>_kvrocks_data` |
| NATS JetStream data | `<project>_nats_data` |
| ClickHouse analytics data | `<project>_clickhouse_data` |

Also retain `docker/.env`, the configuration template, and any custom certificates or mounted files. Protect the backup because these files may contain credentials.

### 2. Stop writers and archive the volumes

Stop the Go deployment and any other process that writes to the same stores. For a Compose project named `docker`:

```bash
docker compose --env-file docker/.env -f docker/docker-compose.yml down
mkdir backup-go
```

Create a new, empty `backup-go` directory for this backup; do not reuse a directory containing earlier archives. Repeat the following command for **each volume you recorded**. Replace `docker_mysql_data` with its exact name; the example creates `backup-go/docker_mysql_data.tar.gz`:

```bash
volume_name=docker_mysql_data
if docker volume inspect "$volume_name" >/dev/null 2>&1; then
  if docker run --rm -v "$volume_name":/source:ro -v "$PWD/backup-go":/backup alpine:3.20 \
    tar czf "/backup/$volume_name.tar.gz" -C /source .; then
    tar tzf "backup-go/$volume_name.tar.gz" > /dev/null
  else
    echo "Backup failed: $volume_name"
  fi
else
  echo "Missing volume: $volume_name"
fi
```

Keep the volume name in each archive filename and copy the entire `backup-go` directory to the target host. Verify that every selected volume has a readable archive.

### 3. Restore on the target host

Install the matching Go deployment and configuration on the target host. Keep services stopped and put `backup-go` in the repository root.

For each archived Docker volume, restore into a **new, empty volume**. Replace both names in this example and repeat. Keep the source name when reading the archive; use the target Compose project's name when creating its volume:

```bash
source_volume=docker_mysql_data
target_volume=docker_mysql_data
if ! test -f "backup-go/$source_volume.tar.gz"; then
  echo "Missing archive: backup-go/$source_volume.tar.gz"
elif docker volume inspect "$target_volume" >/dev/null 2>&1; then
  echo "Target volume already exists; inspect it before restoring: $target_volume"
else
  if docker volume create "$target_volume"; then
    docker run --rm -v "$target_volume":/target -v "$PWD/backup-go":/backup:ro alpine:3.20 \
      tar xzf "/backup/$source_volume.tar.gz" -C /target
  fi
fi
```

If the target Compose project has a different name, set `target_volume` to that project's volume name while retaining `source_volume` from the archive. Do not restore over a populated volume: extracting an archive does not remove files already present. Keep the metadata database, object data, and search index from the same stopped deployment together.

### 4. Start and verify

Start the Go stack with the target configuration and the same Compose project name used for its volumes:

```bash
docker compose --env-file docker/.env -f docker/docker-compose.yml up -d
docker compose --env-file docker/.env -f docker/docker-compose.yml ps
```

The Go image's entrypoint runs the standalone database migration before starting its enabled server modes. Check the container logs for migration errors before using the service. Confirm that an existing dataset can list and open files and that search still returns its documents.
