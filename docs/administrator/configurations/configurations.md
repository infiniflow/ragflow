---
sidebar_position: 0
slug: /configurations
sidebar_custom_props: {
  categoryIcon: LucideCog
}
---
# Configuration

Configuration reference for deploying the Go implementation of RAGFlow with Docker Compose.

## Guidelines

The Go Docker deployment uses the following files:

- [.env](https://github.com/infiniflow/ragflow/blob/main/docker/.env): Defines the Docker image, service profiles, published ports, credentials, and other deployment environment variables.
- [service_conf.yaml.template](https://github.com/infiniflow/ragflow/blob/main/docker/service_conf.yaml.template): Defines the configuration consumed by the Go services. The container generates `service_conf.yaml` from this template and substitutes its environment variables during startup.
- [docker-compose.yml](https://github.com/infiniflow/ragflow/blob/main/docker/docker-compose.yml): Starts the Go RAGFlow service together with the dependencies selected through Compose profiles.
- [docker-compose-base.yml](https://github.com/infiniflow/ragflow/blob/main/docker/docker-compose-base.yml): Defines shared dependencies such as the document engine, metadata database, MinIO, Kvrocks, NATS, and ClickHouse.

To change the public HTTP or HTTPS port, update `SVR_WEB_HTTP_PORT` or
`SVR_WEB_HTTPS_PORT` in **docker/.env**. Their defaults are `80` and `443`.

:::tip NOTE
Updates to the above configurations require a reboot of all containers to take effect:

```bash
docker compose --env-file docker/.env -f docker/docker-compose.yml up -d
```

:::

## Docker Compose

- **docker-compose.yml**
  Starts the Go RAGFlow service and selects the required dependency profiles.
- **docker-compose-base.yml**
  Defines the shared dependency services used by the selected document engine,
  metadata database, object storage, cache, queue, and analytical storage.

## Docker Environment Variables

The [.env](https://github.com/infiniflow/ragflow/blob/main/docker/.env) file contains the environment variables for the Go Docker deployment.

### Metadata Database

- `DB_TYPE`
  The business metadata database type. Defaults to `mysql`. Set it to `oceanbase` when connecting to OceanBase through its MySQL-compatible protocol.
- `METADATA_DB_PROFILE`
  The Compose profile for the metadata database. Defaults to `mysql`.

### Elasticsearch

- `STACK_VERSION`
  The version of Elasticsearch. Defaults to `8.11.3`
- `ES_PORT`
  The port used to expose the Elasticsearch service to the host machine, allowing **external** access to the service running inside the Docker container.  Defaults to `1200`.
- `ELASTIC_PASSWORD`
  The password for Elasticsearch.

### Kibana

- `KIBANA_PORT`
  The port used to expose the Kibana service to the host machine, allowing **external** access to the service running inside the Docker container. Defaults to `6601`.

### Resource Management

- `MEM_LIMIT`
  The maximum memory available to each Compose service that applies this limit.
  It is a per-container upper limit, not the minimum host memory, the total
  memory reserved by RAGFlow, or a guarantee that every container consumes this
  amount. The default is `8073741824` bytes, approximately `7.52 GiB` (`8.07 GB`).

### MySQL

- `MYSQL_PASSWORD`
  The password for MySQL.
- `MYSQL_PORT`
  The port to connect to MySQL from RAGFlow container. Defaults to `3306`. Change this if you use an external MySQL.
- `EXPOSE_MYSQL_PORT`
  The port used to expose the MySQL service to the host machine, allowing **external** access to the MySQL database running inside the Docker container. Defaults to `3306`.
- `MYSQL_MAX_PACKET`
  The maximum MySQL communication packet size in bytes. Defaults to
  `1073741824` bytes (`1 GiB`). Keep the MySQL server's
  `max_allowed_packet` setting compatible when using an external database.

### MinIO

RAGFlow utilizes MinIO as its object storage solution, leveraging its scalability to store and manage all uploaded files.

- `MINIO_CONSOLE_PORT`
  The port used to expose the MinIO console interface to the host machine, allowing **external** access to the web-based console running inside the Docker container. Defaults to `9001`
- `MINIO_PORT`
  The port used to expose the MinIO API service to the host machine, allowing **external** access to the MinIO object storage service running inside the Docker container. Defaults to `9000`.
- `MINIO_USER`
  The username for MinIO.
- `MINIO_PASSWORD`
  The password for MinIO.

### Kvrocks

Kvrocks provides the Redis-compatible cache. It is separate from the NATS JetStream message queue.

- `KVROCKS_HOST`
  The hostname used by the Go services. Keep the default `kvrocks` when using the provided Compose deployment.
- `KVROCKS_PORT`
  The host port mapped to the Kvrocks container port `6379`. Defaults to `6379`.
- `REDIS_PASSWORD`
  The password used to access Kvrocks through its Redis-compatible protocol.

### NATS

NATS JetStream provides the message queue used by the Go services.

- `NATS_HOST`
  The NATS hostname used inside the Compose network. Defaults to `nats`.
- `NATS_PORT`
  The internal NATS client port. Defaults to `4222`.
- `EXPOSE_NATS_PORT`
  The NATS port published on the Docker host.

### RAGFlow

- `SVR_HTTP_PORT`
  The target Go API port published by Compose. Defaults to `9380`. Normal browser and API traffic should use the public Nginx port unless direct access is required.
- `ADMIN_SVR_HTTP_PORT`
  The target Go Admin port published by Compose. Defaults to `9381`.
- `SVR_WEB_HTTP_PORT`, `SVR_WEB_HTTPS_PORT`
  The public Nginx ports. Defaults to `80` and `443`.
- `RAGFLOW_IMAGE`
  The Go RAGFlow image used by `docker-compose.yml`. Select the official Go image for the required release, or use the locally built `ragflow:go-local` image.

:::tip NOTE
If you cannot download the RAGFlow Docker image, try the following mirrors.

- For the `nightly` edition:
  - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:nightly` or,
  - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:nightly`.
  :::

### Embedding Service

- `TEI_MODEL`
  The embedding model served by the optional local text-embeddings-inference
  service. Its memory requirement depends on the model, runtime backend,
  precision, batch-token limit, and concurrency. The `tei-cpu` profile uses
  system RAM; the `tei-gpu` profile primarily uses GPU memory and also consumes
  system RAM. Verify peak usage on the target hardware and reserve additional
  host memory for RAGFlow and the other enabled services.

- `TEI_PORT`
  The port used to expose the text-embeddings-inference service to the host machine, allowing **external** access to the text-embeddings-inference service running inside the Docker container. Defaults to `6380`.

### OceanBase and SeekDB Memory

- `OB_MEMORY_LIMIT`
  The memory limit configured for the bundled OceanBase service. Defaults to
  `10G`.
- `OB_SYSTEM_MEMORY`
  The OceanBase system-memory setting. Defaults to `2G`.
- `OB_DATAFILE_SIZE`
  The configured size of the OceanBase data file. Defaults to `20G`.
- `OB_LOG_DISK_SIZE`
  The configured size of the OceanBase log disk. Defaults to `20G`.
- `SEEKDB_MEMORY_LIMIT`
  The memory limit passed to the bundled SeekDB service. Defaults to `2G`.

For an OceanBase deployment, use at least 4 CPU cores and 32 GB host memory as a
starting point, leaving room beyond OceanBase's own
[production requirements](https://en.oceanbase.com/docs/common-oceanbase-database-10000000001166993)
for the other RAGFlow services. Set `MEM_LIMIT` to no less than
`OB_MEMORY_LIMIT`; a 12 GiB container limit is recommended, expressed as
`MEM_LIMIT=12884901888` in **docker/.env**. The
[SeekDB deployment requirements](https://www.oceanbase.ai/docs/V1.1.0/deploy-by-systemd)
specify at least 1 CPU core, 2 GB available memory, and 15 GB free data-disk
space. These values apply to their respective database services only and do not
replace the resource recommendation for the complete RAGFlow deployment.
The OceanBase data-file and log-disk defaults account for `40G` before
container images, RAGFlow object storage, indexes, and logs. Plan additional
free disk space for the complete deployment rather than treating `40G` as the
host disk requirement.

### Timezone

- `TZ`
  The local time zone. Defaults to `Asia/Shanghai`.

### Hugging Face Mirror Site

- `HF_ENDPOINT`
  The mirror site for huggingface.co. It is disabled by default. You can uncomment this line if you have limited access to the primary Hugging Face domain.

### macOS

- `MACOS`
  Optimizations for macOS. It is disabled by default. You can uncomment this line if your OS is macOS.

### User Registration

- `REGISTER_ENABLED`
  - `1`: (Default) Enable user registration.
  - `0`: Disable user registration.

- `OAUTH_AUTO_REGISTER`
  - `true`: (Default) Allow new users to be provisioned on OAuth/OIDC login. Also accepts `1`, `yes`, `on`.
  - Any other value, `false` included: require OAuth/OIDC users to already exist. This is independent of `REGISTER_ENABLED`.

## Service Configuration

[service_conf.yaml.template](https://github.com/infiniflow/ragflow/blob/main/docker/service_conf.yaml.template) specifies the system-level configuration used by the Go API, Admin, Ingestor, and Syncer services.

### `ragflow`

- `host`: The API server's IP address inside the Docker container. Defaults to `0.0.0.0`.
- `http_port`: The API server's serving port inside the Docker container. Defaults to `9380`.
- `trusted_proxies`: The proxy IPs or CIDRs whose `X-Forwarded-For` / `X-Real-IP` headers the Go API server trusts when resolving the client address (used by the agent webhook `ip_whitelist` and login audit records). Defaults to loopback (`['127.0.0.0/8', '::1/128']`), i.e. the nginx bundled in the Docker image. An explicit list *replaces* the default rather than extending it, so keep the loopback entries when adding a further proxy placed in front of nginx, for example `['127.0.0.0/8', '::1/128', '10.0.0.0/8']`; an empty list trusts no proxy headers at all.

### `mysql`

- `name`: The MySQL database name. Defaults to `rag_flow`.
- `user`: The username for MySQL.
- `password`: The password for MySQL.
- `port`: The MySQL serving port inside the Docker container. Defaults to `3306`.
- `max_connections`: The maximum number of concurrent connections in the MySQL connection pool. Defaults to `900` in the Go deployment configuration.
- `stale_timeout`: The connection stale timeout in seconds. Defaults to `300` in the Go deployment configuration.
- `max_allowed_packet`: The maximum communication packet size in bytes. Defaults to `1073741824` bytes (`1 GiB`).

### `minio`

- `user`: The username for MinIO.
- `password`: The password for MinIO.
- `host`: The MinIO serving IP *and* port inside the Docker container. Defaults to `minio:9000`.

### `S3` (Tigris)

To use [Tigris](https://www.tigrisdata.com) as an S3-compatible storage backend, set `STORAGE_IMPL=AWS_S3` in `.env` and configure the `s3:` section:

```yaml
s3:
  access_key: 'tid_YOUR_ACCESS_KEY'
  secret_key: 'tsec_YOUR_SECRET_KEY'
  region_name: 'auto'
  endpoint_url: 'https://t3.storage.dev'
  bucket: 'ragflow'
  prefix_path: 'ragflow'
  signature_version: 'v4'
  addressing_style: 'virtual'
```

- `access_key` / `secret_key`: Create at [console.tigris.dev](https://console.tigris.dev).
- `region_name`: Must be `auto`.
- `endpoint_url`: `https://t3.storage.dev`, or `https://fly.storage.tigris.dev` on Fly.io.
- `addressing_style`: Must be `virtual`.
- `bucket` / `prefix_path`: Optional. Enables single-bucket mode — see [Migrate from multi-bucket to single-bucket mode](../migration/backup_and_migration.md#migrate-from-multi-bucket-to-single-bucket-mode).

When using an external storage backend, you can remove the `minio` service from `docker-compose-base.yml`.

For other S3-compatible backends (AWS S3, Alibaba Cloud OSS, Azure Blob, Google Cloud Storage), see the commented examples in [service_conf.yaml.template](https://github.com/infiniflow/ragflow/blob/main/docker/service_conf.yaml.template).

### `kvrocks`

- `host`: The Kvrocks address used by the Go services. Defaults to `kvrocks:6379` in the Compose deployment.
- `db`: The logical database index. Defaults to `1`.
- `username`: Optional Kvrocks ACL username.
- `password`: The password used by the Go services to access Kvrocks.

### `oauth`

The OAuth configuration for signing up or signing in to RAGFlow using a third-party account.

- `<channel>`: Custom channel ID.
  - `type`: Authentication type, options include `oauth2`, `oidc`, `github`. Default is `oauth2`, when `issuer` parameter is provided, defaults to `oidc`.
  - `icon`: Icon ID, options include `github`, `sso`, default is `sso`.
  - `display_name`: Channel name, defaults to the Title Case format of the channel ID.
  - `client_id`: Required, unique identifier assigned to the client application.
  - `client_secret`: Required, secret key for the client application, used for communication with the authentication server.
  - `authorization_url`: Base URL for obtaining user authorization.
  - `token_url`: URL for exchanging authorization code and obtaining access token.
  - `userinfo_url`: URL for obtaining user information (username, email, etc.).
  - `issuer`: Base URL of the identity provider. OIDC clients can dynamically obtain the identity provider's metadata (`authorization_url`, `token_url`, `userinfo_url`) through `issuer`.
  - `scope`: Requested permission scope, a space-separated string. For example, `openid profile email`.
  - `redirect_uri`: Required, URI to which the authorization server redirects during the authentication flow to return results. Must match the callback URI registered with the authentication server. Format: `https://your-app.com/api/v1/auth/oauth/<channel>/callback`. For local configuration, you can directly use `http://127.0.0.1:80/api/v1/auth/oauth/<channel>/callback`.

:::tip NOTE
The following are best practices for configuring various third-party authentication methods. You can configure one or multiple third-party authentication methods for Ragflow:
```yaml
oauth:
  oauth2:
    display_name: "OAuth2"
    client_id: "your_client_id"
    client_secret: "your_client_secret"
    authorization_url: "https://your-oauth-provider.com/oauth/authorize"
    token_url: "https://your-oauth-provider.com/oauth/token"
    userinfo_url: "https://your-oauth-provider.com/oauth/userinfo"
    redirect_uri: "https://your-app.com/api/v1/auth/oauth/oauth2/callback"

  oidc:
    display_name: "OIDC"
    client_id: "your_client_id"
    client_secret: "your_client_secret"
    issuer: "https://your-oauth-provider.com/oidc"
    scope: "openid email profile"
    redirect_uri: "https://your-app.com/api/v1/auth/oauth/oidc/callback"

  github:
    # https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/creating-an-oauth-app
    type: "github"
    icon: "github"
    display_name: "Github"
    client_id: "your_client_id"
    client_secret: "your_client_secret"
    redirect_uri: "https://your-app.com/api/v1/auth/oauth/github/callback"
```
:::

### `user_default_llm`

Assigning default models to newly registered users through `user_default_llm` is deprecated and no longer supported in the open-source version. Uncommenting this section in **service_conf.yaml.template** does not configure models for new users.

Each tenant must configure its own model provider instances and credentials. New users do not automatically inherit an administrator's configured instances or default model selections.

Go to **User settings** **>** **Model providers** to configure provider instances, add models, and select default models. See [Configure Model API Key](../../guides/models/llm_api_key_setup.md) for instructions.

The Enterprise Edition provides role-level default model settings.

:::note Builtin embedding
If you deploy TEI, keep the shipped `user_default_llm.default_models.embedding_model` connection settings in **service_conf.yaml.template**. They are used by the TEI `Builtin` embedding service and do not assign administrator-owned model instances to new tenants.
:::
