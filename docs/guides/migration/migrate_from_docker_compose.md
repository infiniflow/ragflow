# Data Migration Guide

A common scenario is processing large datasets on a powerful instance (e.g., with a GPU) and then migrating the entire RAGFlow service to a different production environment (e.g., a CPU-only server). This guide explains how to safely back up and restore your data using our provided migration script.

## Identifying Your Data

By default, RAGFlow uses Docker volumes to store all persistent data, including your database, uploaded files, and search indexes. You can see these volumes by running:

```bash
docker volume ls
```

The output will look similar to this:

```text
DRIVER    VOLUME NAME
local     docker_esdata01
local     docker_minio_data
local     docker_mysql_data
local     docker_redis_data
```

These volumes contain all the data you need to migrate.

## Step 1: Stop RAGFlow Services

Before starting the migration, you must stop all running RAGFlow services on the **source machine**. Navigate to the project's root directory and run:

```bash
docker-compose -f docker/docker-compose.yml down
```

**Important:** Do **not** use the `-v` flag (e.g., `docker-compose down -v`), as this will delete all your data volumes. The migration script includes a check and will prevent you from running it if services are active.

## Step 2: Back Up Your Data

We provide a convenient script to package all your data volumes into a single backup folder.

For a quick reference of the script's commands and options, you can run:
```bash
bash docker/migration.sh help
```

To create a backup, run the following command from the project's root directory:

```bash
bash docker/migration.sh backup
```

This will create a `backup/` folder in your project root containing compressed archives of your data volumes.

You can also specify a custom name for your backup folder:

```bash
bash docker/migration.sh backup my_ragflow_backup
```

This will create a folder named `my_ragflow_backup/` instead.

## Step 3: Transfer the Backup Folder

Copy the entire backup folder (e.g., `backup/` or `my_ragflow_backup/`) from your source machine to the RAGFlow project directory on your **target machine**. You can use tools like `scp`, `rsync`, or a physical drive for the transfer.

## Step 4: Restore Your Data

On the **target machine**, ensure that RAGFlow services are not running. Then, use the migration script to restore your data from the backup folder.

If your backup folder is named `backup/`, run:

```bash
bash docker/migration.sh restore
```

If you used a custom name, specify it in the command:

```bash
bash docker/migration.sh restore my_ragflow_backup
```

The script will automatically create the necessary Docker volumes and unpack the data.

**Note:** If the script detects that Docker volumes with the same names already exist on the target machine, it will warn you that restoring will overwrite the existing data and ask for confirmation before proceeding.

## Step 5: Start RAGFlow Services

Once the restore process is complete, you can start the RAGFlow services on your new machine:

```bash
docker-compose -f docker/docker-compose.yml up -d
```

**Note:** If you already have built a service by docker-compose before, you may need to backup your data for target machine like this guide above and run like:

```bash
# Please backup by `sh docker/migration.sh backup backup_dir_name` before you do the following line.
# !!! this line -v flag will delete the original docker volume
docker-compose -f docker/docker-compose.yml down -v
docker-compose -f docker/docker-compose.yml up -d
```

Your RAGFlow instance is now running with all the data from your original machine.







