---
sidebar_position: 0
title: Admin Service
sidebar_label: Admin Service
slug: /admin_service
sidebar_custom_props: {
  categoryIcon: LucideActivity
}
---
# Admin Service

The Go Admin Service provides the administration API used by the Web UI and the Go CLI. It manages users and configuration, checks infrastructure dependencies, and tracks Go API servers, ingestors, and file syncers that report heartbeats. The target Admin Service port is `9381`; the target Go API server port is `9380`.

Start the Admin Service before the API server, ingestors, and syncers so those processes can report their heartbeats as soon as they start. MySQL, the configured document engine and storage service, the cache, and the message queue must be available to the Admin Service. For the default host-run configuration, see [Start supporting services](../../develop/launch_ragflow_from_source.md#2-start-supporting-services) for the dependency list, port settings, and readiness checks.

## Start from source

For a fresh checkout, prepare the native dependencies and build the C++ bindings and Go binaries as described in [Get the source and build dependencies](../../develop/launch_ragflow_from_source.md#1-get-the-source-and-build-dependencies):

```bash
bash build.sh --all
```

Use `bash build.sh --go` for subsequent Go-only rebuilds after the C++ bindings have already been built.

Before proceeding, run `./bin/ragflow_server --admin --help` to check that the binary can load and parse the Admin arguments. This command prints help and exits; it does not start the Admin Service or test its dependencies. If it exits with status `139` without printing help, see the [build troubleshooting notes](../../develop/launch_ragflow_from_source.md#1-get-the-source-and-build-dependencies).

Complete the standalone database migration before starting any server mode. The following source-development commands use `RAGFLOW_DEV_MODE=true` consistently because an untagged development checkout can carry a migration marker newer than its reported version. Do not set this variable in production; for a production release whose binary and database versions match, omit it from every command. Run the migration once for this startup; do not attach `--migrate` to `--admin`, `--api`, or `--ingestor`:

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --migrate
```

Start each process in a separate terminal, in this order:

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin
```

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --api
```

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --ingestor
```

If file synchronization is enabled, also start:

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer
```

To initialize the default superuser when starting Admin, add `--init-superuser` to the Admin command. Do this only when initial account setup is required.

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin --init-superuser
```

If no superuser exists, this option currently creates `admin@ragflow.io` with the initial password `admin`. Change that password immediately after the first login. It does not reset the password of an existing superuser.

To change the Admin listening port, set `admin.http_port` in the server configuration. Do not rely on `--port` for Admin unless that option is explicitly documented as controlling the Admin HTTP listener.

For processes on another host, set `admin.host` and `admin.http_port` in their server configuration to the reachable Admin address and port. Do not rely on `--admin-host <host:port>`: the current server parses that flag but its heartbeat client does not use it. When changing the Admin port, also update the CLI connection address and the health-check URLs below.

## Start with Docker Compose

For the Go deployment, use `docker/docker-compose-go.yml` and its `.env-go` file. The `ragflow-cpu` service already passes `--enable-adminserver` and `--init-superuser` to the Go entrypoint. The entrypoint runs migrations before starting the server processes; it then starts Admin before the API server and ingestor. Its optional syncer starts earlier, so its heartbeat may appear only after Admin becomes available.

```bash
docker compose --env-file docker/.env-go -f docker/docker-compose-go.yml --profile cpu up -d
```

The Go Compose deployment exposes the Admin Service on `9381` and the API server on `9380` by default. Use `ADMIN_SVR_HTTP_PORT` and `SVR_HTTP_PORT` to change the corresponding published host ports.

For both CPU and GPU deployments, use the Go Compose entry point documented in [Go Docker deployment](../../../docker/README.md). Initialize the first superuser with `--init-superuser` only when initial account setup is required.

## Check health and service registration

Use `/live` to check whether the Admin HTTP process is responding:

```bash
curl -f http://127.0.0.1:9381/live
```

Use `/healthz` to check the Admin Service and its required dependencies. It returns HTTP `200` when the database, cache, document engine, storage, and message queue are healthy; otherwise it returns HTTP `500` with component status information.

```bash
curl -f http://127.0.0.1:9381/healthz
```

The API server, ingestor, and syncer periodically send heartbeat reports to Admin. Admin keeps their latest reports in its runtime service registry; it does not register itself there. After those processes start, launch the Go CLI and log in as an administrator. If you changed the Admin port or are connecting from another host, add `--host <host:port>` to the CLI command:

```bash
./bin/ragflow-cli --admin
```

```text
RAGFlow(admin)> LOGIN ADMIN 'admin@ragflow.io';
```

The CLI prompts for the password.

If this is the first login using the initial `admin` password, change it before continuing:

```text
RAGFlow(admin)> ALTER USER PASSWORD 'admin@ragflow.io' '<new_password>';
```

Then inspect the registered services:

```text
RAGFlow(admin)> LIST SERVICES;
```

Look for `api_server`, `ingestor`, and, when enabled, `file_syncer` entries with `status=alive`. A previously registered process can remain listed with `status=timeout` after its heartbeats stop. `LIST SERVICES;` also includes health checks for infrastructure dependencies. The CLI commands `LIST API SERVER;` and `SHOW API SERVER '<name>';` inspect locally saved API connection settings, not this heartbeat registry.
