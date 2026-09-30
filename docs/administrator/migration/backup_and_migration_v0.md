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

If the deployment was started with a project name, include the same `-p` value in every Compose and migration command. For example:

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

## 3. Back up the default four volumes

The bundled migration script archives the default MySQL, MinIO, Redis, and Elasticsearch volumes. First, display its command syntax so you can confirm that the script is available in the checked-out release:

```bash
bash docker/migration.sh help
```

Next, create the four archives in the default `backup/` directory:

```bash
bash docker/migration.sh backup
```

To give this backup its own directory, pass the directory name to the command. The script creates the directory and writes the archives into it:

```bash
bash docker/migration.sh backup my_ragflow_backup
```

If the deployment uses a custom Compose project name, pass that name with `-p`. This makes the script read the correctly prefixed volumes:

```bash
bash docker/migration.sh -p ragflow backup my_ragflow_backup
```

Use a new directory for each backup. After the script finishes, list its contents:

```bash
ls -lh my_ragflow_backup
```

Confirm that it contains the expected MySQL, MinIO, Redis, and Elasticsearch archives. If your deployment uses another document engine, external storage, or additional persistent services, back up those volumes or services now. Keep all of them together as one backup set.

## 4. Transfer the backup

Copy the complete backup directory and retained configuration files to the target host. Keep archive filenames unchanged. You can transfer them with `scp`, `rsync`, removable storage, or another file-transfer method appropriate for your environment.

On the target host, list the transferred directory and confirm that its files and sizes match the source. Keep the source deployment and the backup available until the restored deployment passes the checks in step 6.

## 5. Restore on the target host

Install the same RAGFlow release on the target host and place the backup directory in the repository root. Keep all target services stopped.

Prepare new, empty volumes on the target host. The bundled restore command expects the four default archives for MySQL, MinIO, Redis, and Elasticsearch.

If the backup contains all four default archives, choose one of the following commands. To restore from the default `backup/` directory, run:

```bash
bash docker/migration.sh restore
```

To restore from a custom directory, pass its name. The script reads the four archives from that directory and restores them to the target volumes:

```bash
bash docker/migration.sh restore my_ragflow_backup
```

If the target uses a custom Compose project name, pass it with `-p`. This makes the script restore into volumes with the matching prefix:

```bash
bash docker/migration.sh -p ragflow restore my_ragflow_backup
```

If the deployment uses Infinity, OpenSearch, SereneDB, or another document engine, restore that engine's saved volume with its storage-specific procedure. Restore any additional volumes and external services from the same backup set before continuing.

During a bundled restore, the script lists any target volumes that already exist and asks for confirmation. Review the listed project and volume names. Enter `y` when they match the empty target volumes prepared for this restore. When the script finishes, confirm that it reports a successful restore for all four archives.

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
