---
sidebar_position: 4
title: "Backup and Migration (v1.0.0-rc1 and Later)"
sidebar_label: "Backup and Migration (v1.0.0-rc1+)"
slug: /migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Backup and Migration (v1.0.0-rc1 and Later)

This guide explains how to back up a RAGFlow `v1.0.0-rc1` or later Docker deployment and restore it on another host. Complete the four steps in order. The procedure backs up the Docker volumes used by the deployment, including Kvrocks and any enabled optional services.

## Move a deployment to another host

### 1. Find the data to back up

First, set `project_name` to the Compose project name used by the deployment. The default project name is usually `docker`; if the deployment was started with `-p ragflow`, use `ragflow`. Then list the containers in that project to identify the services that are currently enabled:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps
```

Next, list the Docker volumes available on the host. You will use this list together with the container mounts to find the volumes that belong to RAGFlow:

```bash
docker volume ls
```

For each RAGFlow container shown by the first command, run the following command. It prints the volume name and the location where that volume is mounted in the container:

```bash
docker inspect <container-name> --format '{{range .Mounts}}{{println .Name .Destination}}{{end}}'
```

Write down every RAGFlow volume name. Use the container mounts as the backup list; `docker volume ls` is an inventory of all volumes on the host and may include unrelated volumes. Also record the Compose project name. Its volume names normally start with the same project-name prefix. Use the same `project_name` value in every Compose command in this guide.

Use the following table to check the list you recorded. Include each service enabled in your deployment, together with any additional or externally managed storage used in your environment.

| Service or data | Persistent location |
|-----------------|---------------------|
| Metadata database in bundled MySQL | `<project>_mysql_data` |
| Uploaded objects in bundled MinIO | `<project>_minio_data` |
| Elasticsearch index | `<project>_esdata01` |
| OpenSearch index | `<project>_osdata01` |
| Infinity index | `<project>_infinity_data` |
| SereneDB data | `<project>_serenedb_data` |
| Kvrocks cache and checkpoint data | `<project>_kvrocks_data` |
| Redis data, when the Redis service is enabled | `<project>_redis_data` |
| NATS JetStream data | `<project>_nats_data` |
| ClickHouse analytics data | `<project>_clickhouse_data` |
| Kibana data, when Kibana is enabled | `<project>_kibana_data` |
| Text Embeddings Inference model cache, when enabled | `<project>_tei_data` |
| OceanBase data and configuration | `docker/oceanbase/data` and `docker/oceanbase/conf` bind mounts |
| SeekDB data | `docker/seekdb` bind mount |

If the deployment uses external MySQL, object storage, a search service, or another external dependency, back it up with the provider's supported procedure and keep it at the same recovery point as the Docker volumes.

Also retain `docker/.env`, the configuration template, and any custom certificates or mounted files. Protect the backup because these files may contain credentials.

Before stopping the deployment, run `df -h` and `docker system df`. Make sure the host has enough free space for the volume archives and temporary backup files.

### 2. Stop RAGFlow and create the backup

First, stop RAGFlow and its bundled services. This prevents the database, object storage, and search index from changing while their archives are being created:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml down
```

After all containers have stopped, create a new directory for this backup. The volume archives created below will be stored in this directory:

```bash
mkdir backup-ragflow
```

Use a new, empty `backup-ragflow` directory for each backup. Then repeat the following commands for **each volume you recorded**. Set `volume_name` to the exact Docker volume name. The example archives `docker_mysql_data` as `backup-ragflow/docker_mysql_data.tar.gz`:

```bash
volume_name=docker_mysql_data
if docker volume inspect "$volume_name" >/dev/null 2>&1; then
  if docker run --rm -v "$volume_name":/source:ro -v "$PWD/backup-ragflow":/backup alpine:3.20 \
    tar czf "/backup/$volume_name.tar.gz" -C /source .; then
    tar tzf "backup-ragflow/$volume_name.tar.gz" > /dev/null
  else
    echo "Backup failed: $volume_name"
  fi
else
  echo "Missing volume: $volume_name"
fi
```

The command performs three checks and actions in sequence: it confirms that the volume exists, starts a temporary Alpine container to archive the volume in read-only mode, and checks that the resulting archive can be read. A successful run returns to the shell without printing an error.

If the list in step 1 includes OceanBase, archive its data and configuration directories. The first command archives the database files; the second archives the OceanBase configuration:

```bash
tar czf backup-ragflow/oceanbase-data.tar.gz -C docker/oceanbase data
tar czf backup-ragflow/oceanbase-conf.tar.gz -C docker/oceanbase conf
```

If the list includes SeekDB, archive its data directory:

```bash
tar czf backup-ragflow/seekdb.tar.gz -C docker seekdb
```

Apply the same method to any other bind-mounted data directory recorded in step 1: create one archive for the directory and store it in `backup-ragflow`.

After processing all volumes, list the archives:

```bash
ls -lh backup-ragflow
```

Confirm that every recorded volume has a corresponding `.tar.gz` file. Copy the entire `backup-ragflow` directory, `docker/.env`, the configuration template, certificates, and other custom-mounted files to the target host.

### 3. Restore the backup on the target host


Install the same RAGFlow release on the target host, copy the saved configuration into place, and put `backup-ragflow` in the repository root. Keep the target services stopped while restoring the volumes.

For each archive, run the following commands. `source_volume` is the volume name contained in the archive filename. `target_volume` is the volume name expected by the Compose project on the target host. The commands create a new volume and extract the archive into it:

```bash
source_volume=docker_mysql_data
target_volume=docker_mysql_data
if ! test -f "backup-ragflow/$source_volume.tar.gz"; then
  echo "Missing archive: backup-ragflow/$source_volume.tar.gz"
elif docker volume inspect "$target_volume" >/dev/null 2>&1; then
  echo "Target volume already exists; inspect it before restoring: $target_volume"
else
  if docker volume create "$target_volume"; then
    docker run --rm -v "$target_volume":/target -v "$PWD/backup-ragflow":/backup:ro alpine:3.20 \
      tar xzf "/backup/$source_volume.tar.gz" -C /target
  fi
fi
```

The command first checks that the archive exists and that the target volume name is available. It then creates an empty target volume and uses a temporary Alpine container to extract the archive. If the target Compose project has a different name, keep `source_volume` unchanged and set `target_volume` to the name expected by the target project.

Repeat this operation until every archive has been restored. Restore the metadata database, object data, search index, and other service data from the same backup set.

If the backup contains OceanBase directory archives, recreate the parent directory and extract both archives. These commands restore `docker/oceanbase/data` and `docker/oceanbase/conf`:

```bash
mkdir -p docker/oceanbase
tar xzf backup-ragflow/oceanbase-data.tar.gz -C docker/oceanbase
tar xzf backup-ragflow/oceanbase-conf.tar.gz -C docker/oceanbase
```

If the backup contains the SeekDB directory archive, extract it into `docker` to restore `docker/seekdb`:

```bash
tar xzf backup-ragflow/seekdb.tar.gz -C docker
```

Restore any other bind-mounted directory archive to the same relative location used on the source host.

### 4. Start and verify

Start RAGFlow with the restored volumes. The first command creates and starts the containers; the second displays their current state:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml up -d
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps
```

Wait until the services report a running or healthy state. Then display the application logs and check that startup and database migration completed successfully. In the standard Linux Compose file, the application service is named `ragflow-cpu`:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml logs --tail=200 ragflow-cpu
```

Finally, sign in and confirm that existing knowledge bases and files are present, files can be opened, and retrieval returns content from previously parsed documents. After these checks pass, the backup and restore are complete.
