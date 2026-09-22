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

This guide covers the Go backend deployed with `docker/docker-compose-go.yml`. It explains how to move a deployment to another host and how to change MinIO or S3 object storage from multiple physical buckets to one bucket.

The bundled `docker/migration.sh` handles a fixed set of four older volumes: MySQL, MinIO, Redis, and Elasticsearch. It does not back up the Go stack's Kvrocks volume or other optional services. Use the volume procedure below for a Go deployment.

## Move a Go deployment to another host

### 1. Identify all persistent data

From the repository root, inspect the running deployment and its Docker volumes:

```bash
docker compose --env-file docker/.env-go -f docker/docker-compose-go.yml ps
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

Other document engines have different volumes, such as `infinity_data`, `osdata01`, `serenedb_data`, `ob_data`, or `seekdb_data`. Check the active `DOC_ENGINE` and the actual mounts before choosing archives. An external MySQL or OceanBase database, external object store, or external search engine is not captured by Docker volume archives; back it up using that service's own procedure. Also retain `docker/.env-go`, the configuration template, and any custom certificates or mounted files. Protect the backup because these files may contain credentials.

### 2. Stop writers and archive the volumes

Stop the Go deployment and any other process that writes to the same stores. For a Compose project named `docker`:

```bash
docker compose --env-file docker/.env-go -f docker/docker-compose-go.yml down
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

Keep the volume name in each archive filename and copy the entire `backup-go` directory to the target host. Verify that every selected volume has a readable archive. For externally hosted services, complete and verify their backups before proceeding.

### 3. Restore on the target host

Install the matching Go deployment and configuration on the target host. Keep services stopped. Restore external databases, object stores, and search services using their own backups. Put `backup-go` in the repository root.

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
docker compose --env-file docker/.env-go -f docker/docker-compose-go.yml up -d
docker compose --env-file docker/.env-go -f docker/docker-compose-go.yml ps
```

The Go image's entrypoint runs the standalone database migration before starting its enabled server modes. Check the container logs for migration errors before using the service. Confirm that an existing dataset can list and open files and that search still returns its documents. If the target uses a different document engine or object store, migrate that service's data and configuration separately; copying the old Docker volumes does not convert their formats.

## Move from multiple buckets to one bucket

The Go MinIO and S3 implementations map ordinary object reads and writes to a configured physical bucket while preserving each original logical bucket name in the object key. With `bucket: ragflow-bucket` and `prefix_path: ragflow`, an object previously stored as `kb_12345/document.pdf` is read from `ragflow-bucket/ragflow/kb_12345/document.pdf`. The same rule applies to user-folder buckets. Setting `prefix_path` alone changes object paths but does not enable one-bucket mode. The current `ListObjects` methods do not apply this mapping; workflows that call them need separate validation before switching.

### Configure the destination

For the Go Docker deployment with bundled MinIO, set the following in `docker/.env-go` or the environment passed to Compose:

```dotenv
MINIO_BUCKET=ragflow-bucket
MINIO_PREFIX_PATH=ragflow
```

The Go Docker entrypoint expands these values into `service_conf.yaml`. If you start the Go binary directly, set `minio.bucket` and `minio.prefix_path` in its configuration file instead. For S3, configure `s3.bucket` and `s3.prefix_path` in the Go configuration file and select the `s3` storage engine. A compatible S3 service such as Tigris uses this S3 implementation; the Go storage type is `s3`, not `AWS_S3`.

### Copy existing objects before switching

1. Record **every** logical bucket used by RAGFlow, including dataset and user-folder buckets, and record the current `bucket` and `prefix_path` settings. Back up the source object store first.
2. Stop all Go services that can write objects. Create the destination physical bucket and grant the Go service access.
3. Copy each source bucket into a key prefix with the same logical bucket name. For example, with MinIO Client aliases already configured:

   ```bash
   mc mirror old-minio/kb_12345/ new-minio/ragflow-bucket/ragflow/kb_12345/
   mc mirror old-minio/folder_abc/ new-minio/ragflow-bucket/ragflow/folder_abc/
   ```

   These examples assume the old layout has one physical bucket per logical bucket and no existing prefix. If the old configuration already has a prefix or fixed bucket, locate each object's actual source path first. Repeat for **all** logical buckets. Match the destination bucket and optional prefix to the configuration you will use. If the new `prefix_path` is empty, copy to `ragflow-bucket/<logical-bucket>/`.
4. Compare object counts and representative file contents at source and destination. Keep the original buckets until the new layout has been verified.
5. Apply the new bucket configuration, then start the Go services. Check that existing datasets and user folders can read their files and that new uploads appear under the expected prefix.

To return to multiple buckets, restore the old configuration **and** copy objects created after the switch back to their original logical buckets. Changing configuration alone does not move objects.

The Go storage factory currently implements MinIO, S3, OSS, and GCS. This procedure describes the object read/write mapping in the Go MinIO and S3 implementations; do not assume the same object-key layout for OSS or GCS without checking their implementation and data first.
