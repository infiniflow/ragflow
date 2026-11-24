---
sidebar_position: 2
title: Admin Service
sidebar_label: Admin Service
slug: /admin_service
sidebar_custom_props: {
  categoryIcon: LucideActivity
}
---
# Admin Service

The Go Admin Service provides the administration API used by the Web UI and the Go CLI. It manages users and configuration, checks infrastructure dependencies, and tracks Go API servers, ingestors, and file syncers that report heartbeats. The target Admin Service port is `9381`; the target Go API server port is `9380`.

The OSS server starts Admin first, then API, Ingestor, Syncer and Chatter in the same process. MySQL, the configured document engine and storage service, the cache, and the message queue must be available to the Admin Service. For the default host-run configuration, see [Start supporting services](../../develop/launch_ragflow_from_source.md#2-start-supporting-services) for the dependency list, port settings, and readiness checks.

## Start from source

For a fresh checkout, prepare the native dependencies and build the C++ bindings and Go binaries as described in [Get the source and build dependencies](../../develop/launch_ragflow_from_source.md#1-get-the-source-and-build-dependencies):

```bash
bash build.sh --all
```

Use `bash build.sh --go` for subsequent Go-only rebuilds after the C++ bindings have already been built.

Run `./bin/ragflow_server --help` to verify the binary starts and lists its options.

```bash
./bin/ragflow_server
```

The OSS server runs all services in one Go process. It migrates the database, starts Admin and initializes the superuser,
then starts API, Ingestor, Syncer and Chatter. No mode or initialization flags
are needed. Admin defaults to port `9381` and API to `9380`.

If no superuser exists, startup creates `admin@ragflow.io`. Set
`DEFAULT_SUPERUSER_PASSWORD` or `ADMIN_DEFAULT_PASSWORD` before initialization
to choose its initial password. If neither variable is set, RAGFlow generates
a random password and writes it to `logs/admin_bootstrap_password.txt`.
Changing either variable after a superuser exists does not reset its password.

To change the Admin listening port, set `admin.http_port` in the server configuration.

The services share the server configuration. When changing the Admin port, also update the CLI connection address and the health check URLs below.

## Start with Docker Compose

For the Go deployment, use `docker/docker-compose.yml` and its `.env` file.
The entrypoint starts the single-process server, which owns migration, superuser
initialization and service startup in the same order as a host-run server.

```bash
docker compose --env-file docker/.env -f docker/docker-compose.yml --profile cpu up -d
```

Before first startup, replace `ADMIN_DEFAULT_PASSWORD` in `docker/.env` with a
deployment-specific secret when the Admin Service is reachable by other hosts.
The checked-in value is `admin`. The Go Compose deployment publishes the Admin
Service on host port `9381` by default, so restrict that port to trusted
administrators. Changing the password setting after the first superuser has
been created does not change the account password.

The API server is published on host port `9380` by default. Use
`ADMIN_SVR_HTTP_PORT` and `SVR_HTTP_PORT` to change the corresponding published
host ports.

Use the Go Compose entry point documented in [Go Docker deployment](../../../docker/README.md). The server initializes the first superuser automatically.

## Check health and service registration

Use `/live` to check whether the Admin HTTP process is responding:

```bash
curl -f http://127.0.0.1:9381/live
```

Use `/healthz` to check the Admin Service and its required dependencies. It returns HTTP `200` when the database, cache, document engine, storage, and message queue are healthy; otherwise it returns HTTP `500` with component status information.

```bash
curl -f http://127.0.0.1:9381/healthz
```

The API server, ingestor, and syncer periodically send heartbeat reports to Admin. Admin keeps their latest reports in its runtime service registry; it does not register itself there. After the server starts, launch the Go CLI and log in as an administrator. If you changed the Admin port or are connecting from another host, add `--host <host:port>` to the CLI command:

```bash
ragflow-cli --admin
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
