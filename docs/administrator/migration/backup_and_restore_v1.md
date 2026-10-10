---
sidebar_position: 4
title: "Backup and Restore (v1.x)"
sidebar_label: "Backup and Restore (v1.x)"
slug: /migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Backup and Restore (v1.x)

:::info Backup and host migration only
This guide creates or restores a same-version recovery point and can move a deployment to another host. To upgrade an existing v1 deployment to another release, create this backup first and then follow [Database Migration for v1.x](./database_migration_v1.md).
:::

Choose the steps for your task:

- **Create an upgrade recovery point:** Complete [steps 1–2](#1-find-the-data-to-back-up), verify every archive, and keep the backup unchanged.
- **Move to another host:** Complete all four steps, including restore and verification.

## 1. Find the data to back up

First, set `project_name` to the Compose project name used by the deployment. The default project name is usually `docker`; if the deployment was started with `-p ragflow`, use `ragflow`. Then list the containers in that project to identify the services that are currently enabled:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps --all
```

Next, list the Docker volumes available on the host. You will use this list together with the container mounts to find the volumes that belong to RAGFlow:

```bash
docker volume ls
```

For each RAGFlow container shown by the first command, run the following command. It prints the mount type, host source, volume name, and location inside the container:

```bash
docker inspect <container-name> --format '{{range .Mounts}}{{println .Type .Source .Name .Destination}}{{end}}'
```

Write down every RAGFlow volume name and every persistent host directory shown in the output. Use the container mounts as the backup list; `docker volume ls` is an inventory of all volumes on the host and may include unrelated volumes. Also record the Compose project name. Its volume names normally start with the same project-name prefix. Use the same `project_name` value in every Compose command in this guide.

Use the following table to check the list you recorded. Include each service enabled in your deployment, together with any additional or externally managed storage used in your environment.

| Service or data | Persistent location |
|-----------------|---------------------|
| Metadata database in bundled MySQL | `<project>_mysql_data` |
| Uploaded objects in bundled MinIO | `<project>_minio_data` |
| Elasticsearch index | `<project>_esdata01` |
| OpenSearch index | `<project>_osdata01` |
| Infinity index | `<project>_infinity_data` |
| SereneDB data | `<project>_serenedb_data` |
| Vastbase data | `<project>_vastbase_data` |
| Kvrocks cache and checkpoint data | `<project>_kvrocks_data` |
| NATS JetStream data | `<project>_nats_data` |
| ClickHouse analytics data | `<project>_clickhouse_data` |
| Kibana data, when Kibana is enabled | `<project>_kibana_data` |
| RAGFlow logs | `docker/ragflow-logs` bind mount |
| OceanBase data and configuration | `docker/oceanbase/data` and `docker/oceanbase/conf` bind mounts |
| SeekDB data | `docker/seekdb` bind mount |

This table is a checklist. Always use the mounts reported by `docker inspect` as the final backup scope.

If the deployment uses external MySQL, object storage, a search service, or another external dependency, back it up with the provider's supported procedure and keep it at the same recovery point as the Docker volumes.

Also retain `docker/.env`, the configuration template, and any custom certificates or mounted files. Protect the backup because these files may contain credentials.

Before stopping the deployment, run `df -h` and `docker system df`. Make sure the host has enough free space for the volume archives and temporary backup files.

For an offline host, make sure that the `alpine:3.20` image is available before stopping RAGFlow.

## 2. Stop RAGFlow and create the backup

First, stop RAGFlow and its bundled services. This prevents the database, object storage, and search index from changing while their archives are being created:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml down
```

After all containers have stopped, create a new directory for this backup. The volume archives created below will be stored in this directory:

```bash
backup_dir="backup-ragflow-v1-$(date +%Y%m%d-%H%M%S)"
mkdir "$backup_dir"
```

The `date` command adds the current time to the directory name so that files from different backups are not mixed. Then repeat the following commands for **each volume you recorded**. Set `volume_name` to the exact Docker volume name:

```bash
volume_name=docker_mysql_data
if docker volume inspect "$volume_name" >/dev/null 2>&1; then
  if docker run --rm -v "$volume_name":/source:ro -v "$PWD/$backup_dir":/backup alpine:3.20 \
    tar czf "/backup/$volume_name.tar.gz" -C /source .; then
    tar tzf "$backup_dir/$volume_name.tar.gz" > /dev/null
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
tar czf "$backup_dir/oceanbase-data.tar.gz" -C docker/oceanbase data
tar czf "$backup_dir/oceanbase-conf.tar.gz" -C docker/oceanbase conf
```

If the list includes SeekDB, archive its data directory:

```bash
tar czf "$backup_dir/seekdb.tar.gz" -C docker seekdb
```

Apply the same method to any other bind-mounted data directory recorded in step 1 and store its archive in `"$backup_dir"`.

After processing all volumes, list the archives:

```bash
ls -lh "$backup_dir"
```

Confirm that every recorded volume has a corresponding `.tar.gz` file. Copy the entire backup directory, `docker/.env`, the configuration template, certificates, and other custom-mounted files to the target host.

## 3. Restore the backup on the target host


Install the same RAGFlow release on the target host, copy the saved configuration into place, and put the backup directory in the repository root. Keep the target services stopped while restoring the volumes. Set `backup_dir` to the transferred directory name:

```bash
backup_dir="<backup-directory-name>"
```

For each archive, run the following commands. `source_volume` is the volume name contained in the archive filename. `target_volume` is the volume name expected by the Compose project on the target host. The commands create a new volume and extract the archive into it:

```bash
source_volume=docker_mysql_data
target_volume=docker_mysql_data
if ! test -f "$backup_dir/$source_volume.tar.gz"; then
  echo "Missing archive: $backup_dir/$source_volume.tar.gz"
elif docker volume inspect "$target_volume" >/dev/null 2>&1; then
  echo "Target volume already exists; inspect it before restoring: $target_volume"
else
  if docker volume create "$target_volume"; then
    docker run --rm -v "$target_volume":/target -v "$PWD/$backup_dir":/backup:ro alpine:3.20 \
      tar xzf "/backup/$source_volume.tar.gz" -C /target
  fi
fi
```

The command first checks that the archive exists and that the target volume name is available. It then creates an empty target volume and uses a temporary Alpine container to extract the archive. If the target Compose project has a different name, keep `source_volume` unchanged and set `target_volume` to the name expected by the target project.

Repeat this operation until every archive has been restored. Restore the metadata database, object data, search index, and other service data from the same backup set.

If the backup contains OceanBase directory archives, recreate the parent directory and extract both archives. These commands restore `docker/oceanbase/data` and `docker/oceanbase/conf`:

```bash
mkdir -p docker/oceanbase
tar xzf "$backup_dir/oceanbase-data.tar.gz" -C docker/oceanbase
tar xzf "$backup_dir/oceanbase-conf.tar.gz" -C docker/oceanbase
```

If the backup contains the SeekDB directory archive, extract it into `docker` to restore `docker/seekdb`:

```bash
tar xzf "$backup_dir/seekdb.tar.gz" -C docker
```

Restore any other bind-mounted directory archive to the same relative location used on the source host.

## 4. Start and verify

Start RAGFlow with the restored volumes. The first command creates and starts the containers; the second displays their current state:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml up -d
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps
```

Wait until the services report a running or healthy state. In the output of `docker compose ps --all`, copy the RAGFlow application service name from the `SERVICE` column and set `application_service` to that value:

```bash
project_name=docker
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml ps --all
application_service="<application-service-name-shown-above>"
docker compose -p "$project_name" --env-file docker/.env -f docker/docker-compose.yml logs --tail=200 "$application_service"
```

Finally, sign in and confirm that existing knowledge bases and files are present, files can be opened, and retrieval returns content from previously parsed documents. After these checks pass, the backup and restore are complete.
