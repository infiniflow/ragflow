# Go Docker deployment

This guide documents the Go implementation of RAGFlow. Use
`docker/.env`, `docker/docker-compose.yml`, `Dockerfile`, and
`docker/entrypoint.sh` for a Go deployment.

The Go image runs the API, Admin, Ingestor, and Syncer modes from the single
`bin/ragflow_server` binary. DeepDoc inference runs in-process. Select an
official Go image tag documented in the corresponding release notes, or build
the image locally as described below.

<details open>
<summary><b>📗 Table of Contents</b></summary>

- 🐳 [Docker Compose](#-docker-compose)
- 🐬 [Docker environment variables](#-docker-environment-variables)
- 🐋 [Service configuration](#-service-configuration)
- 📋 [Setup Examples](#-setup-examples)

</details>

## 🐳 Docker Compose

- **docker-compose.yml**
  Starts the Go RAGFlow container together with the selected dependencies.
- **docker-compose-base.yml**
  Defines the dependency services. The default Go profile uses Elasticsearch,
  MySQL, MinIO, Kvrocks, NATS, and ClickHouse. Other document engines and
  metadata databases are selected through `.env`.

> **Note:** `docker-compose-CN-oc9.yml` and `docker-compose-macos.yml` are not
> the Go deployment entry points. On Linux and macOS, use
> `docker-compose.yml`; Apple Silicon runs the current `linux/amd64` Go image
> through Docker Desktop emulation.

### Quick start

Run these commands from the repository root. The `.git` directory must remain
in the build context because `Dockerfile` uses it to stamp the image version.

```bash
docker build --platform linux/amd64 -f Dockerfile -t ragflow:go-local .
```

Set the image in `docker/.env`:

```dotenv
RAGFLOW_IMAGE=ragflow:go-local
```

If you use the default Elasticsearch document engine on Linux, set
`vm.max_map_count` to at least `262144` on the Docker host:

```bash
sudo sysctl -w vm.max_map_count=262144
```

This change is temporary. To preserve it after a reboot, add
`vm.max_map_count=262144` to `/etc/sysctl.conf`. On Docker Desktop, apply the
setting inside its Linux virtual machine as described in the
[Go image build guide](../docs/develop/build_docker_image.mdx#macos-with-docker-desktop).

Start the default CPU stack:

```bash
cd docker
docker compose --env-file .env -f docker-compose.yml up -d
```

The entrypoint migrates the default MySQL metadata database first, then starts
the Go Syncer, Admin server, API server, and Ingestor. Open
`http://localhost` after the HTTP health check succeeds.

Verify the deployment with:

```bash
docker compose --env-file .env -f docker-compose.yml ps
docker compose --env-file .env -f docker-compose.yml logs --tail 100 ragflow-cpu
curl -f http://localhost/api/v1/system/healthz
```

If `SVR_WEB_HTTP_PORT` is not `80`, append the configured port to the URL. For a
GPU deployment, set `DEVICE=gpu`, install NVIDIA Container Toolkit on the host,
and use `ragflow-gpu` in the logs command. In the RAGFlow open-source 1.0
release, DeepDoc layout analysis, OCR, and table recognition use CPU inference,
including when the GPU Compose service is selected.

## 🐬 Docker environment variables

The [.env](./.env) file is the user-facing environment file for the Go deployment. Compose also loads shared defaults from [.env](./.env) for included dependency definitions. When a dependency setting exists in both files, keep the credentials and published ports consistent.

### Metadata database

- `DB_TYPE`
  The business metadata database type. Defaults to `mysql`. Set it to `oceanbase` when connecting to OceanBase through its MySQL-compatible protocol.
- `COMPOSE_PROFILES`
  The Docker Compose profiles to enable. By default it contains `${DOC_ENGINE},${DEVICE},metadata-${METADATA_DB_PROFILE},ragflow-go,clickhouse`.
- `METADATA_DB_PROFILE`
  Defaults to `mysql`, preserving the in-cluster MySQL service.

### Elasticsearch

- `STACK_VERSION`
  The version of Elasticsearch. Defaults to `8.11.3`
- `ES_PORT`
  The port used to expose the Elasticsearch service to the host machine, allowing **external** access to the service running inside the Docker container.  Defaults to `1200`.
- `ELASTIC_PASSWORD`
  The password for Elasticsearch.

### Kibana

- `KIBANA_PORT`
  The port used to expose the optional Kibana service to the host machine.
  Defaults to `6601`. Follow the enablement notes in `.env` before adding the `kibana` profile.

### Resource management

- `MEM_LIMIT`
  The maximum memory available to each Compose service that applies this limit.
  It is a per-container upper limit, not the minimum host memory, the total
  memory reserved by RAGFlow, or a guarantee that every container consumes this
  amount. The default is `8073741824` bytes, approximately `7.52 GiB` (`8.07 GB`).

### MySQL

- `MYSQL_PASSWORD`
  The password for MySQL. Required only when `DB_TYPE=mysql` starts the in-cluster MySQL metadata database.
- `MYSQL_PORT`
  The port to connect to MySQL from RAGFlow container. Defaults to `3306`. Change this if you use an external MySQL.
- `EXPOSE_MYSQL_PORT`
  The port used to expose the MySQL service to the host machine, allowing **external** access to the MySQL database running inside the Docker container. Defaults to `3306`.
- `MYSQL_MAX_PACKET`
  The maximum MySQL communication packet size in bytes. Defaults to
  `1073741824` bytes (`1 GiB`). Keep the MySQL server's
  `max_allowed_packet` setting compatible when using an external database.

### MinIO

- `MINIO_CONSOLE_PORT`
  The port used to expose the MinIO console interface to the host machine, allowing **external** access to the web-based console running inside the Docker container. Defaults to `9001`
- `MINIO_PORT`
  The port used to expose the MinIO API service to the host machine, allowing **external** access to the MinIO object storage service running inside the Docker container. Defaults to `9000`.
- `MINIO_USER`
  The username for MinIO.
- `MINIO_PASSWORD`
  The password for MinIO.

### Kvrocks

The Go services use Kvrocks as their Redis-compatible cache. NATS JetStream provides the message queue.

- `KVROCKS_HOST`
  The hostname used by the Go services. Keep the default `kvrocks` for the
  Compose deployment.
- `KVROCKS_PORT`
  The host-published Kvrocks port. Defaults to `6379`.
- `REDIS_PASSWORD`
  The password shared with the Kvrocks service.

### NATS

The Go services use NATS JetStream for ingestion, synchronization, memory, and knowledge-compilation messages.

- `NATS_HOST`
  The NATS hostname used inside the Compose network. Defaults to `nats`.
- `NATS_PORT`
  The internal NATS client port. Defaults to `4222`.
- `EXPOSE_NATS_PORT`
  The port published on the Docker host. Change this value to avoid a host-side
  port conflict; Go containers continue to use `NATS_PORT` internally.

### ClickHouse

- `CLICKHOUSE_HOST`, `CLICKHOUSE_TCP_PORT`
  The internal address used by the Go services. Defaults to
  `clickhouse:9000`.
- `EXPOSE_CLICKHOUSE_TCP_PORT`
  The native TCP port published on the Docker host. Defaults to `9900`.
- `CLICKHOUSE_HTTP_PORT`
  The HTTP port published on the host. Defaults to `8123`.
- `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `CLICKHOUSE_DATABASE`
  The credentials and database used by the Go services.

### RAGFlow

- `SVR_HTTP_PORT`
  The target Go API port published by Compose. Defaults to `9380`; Nginx proxies
  normal web API requests to this service.
- `ADMIN_SVR_HTTP_PORT`
  The target Go Admin port published by Compose. Defaults to `9381`.
- `SVR_WEB_HTTP_PORT`, `SVR_WEB_HTTPS_PORT`
  The public Nginx ports. Defaults to `80` and `443`.
- `DEVICE`
  Selects the `cpu` or `gpu` RAGFlow service. Defaults to `cpu`.
- `RAGFLOW_DEV_MODE`
  Set to `true` only for development checkouts that intentionally use a
  development migration marker. Keep it `false` in production.
- `RAGFLOW_IMAGE`
  The Go Docker image used by `docker-compose.yml`. Use an official Go image
  tag documented in its release notes, or use the locally built
  `ragflow:go-local` tag. The RAGFlow Docker image does not include embedding
  models.

### Local embedding service

The optional `tei-cpu` and `tei-gpu` profiles start a local
text-embeddings-inference service. Its memory requirement depends on the model,
runtime backend, precision, batch-token limit, and concurrency. The `tei-cpu`
profile uses system RAM; the `tei-gpu` profile primarily uses GPU memory and
also consumes system RAM. Verify model loading and peak request usage on the
target hardware, and leave additional host memory for RAGFlow, the document
engine, databases, and operating system.


> 💡 **Tip:** If you cannot download a Go RAGFlow Docker image, try the following mirrors.
>
> - For the `nightly` edition:
>   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:nightly` or,
>   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:nightly`.

### DeepDoc (in-process)

DeepDoc layout analysis (DLA), OCR (text detection/recognition), and table
structure recognition (TSR) run **in-process** inside the RAGFlow server using
ONNX Runtime — there is no separate DeepDoc service to deploy. ONNX Runtime is
statically linked into the server binary (resolved at runtime via dlopen(NULL);
no `libonnxruntime.so` is required) and the models are loaded at runtime;
`DEEPDOC_MODEL_DIR` overrides the default model directory. `Dockerfile`
copies the required model assets into `/ragflow/rag/res/deepdoc`.

In the RAGFlow open-source 1.0 release, DeepDoc uses CPU inference, including
when the RAGFlow container is started with the GPU Compose profile.

### Timezone

- `TZ`
  The local time zone. Defaults to `'Asia/Shanghai'`.

### Hugging Face mirror site

- `HF_ENDPOINT`
  The mirror site for huggingface.co. It is disabled by default. You can uncomment this line if you have limited access to the primary Hugging Face domain.

### Embedding batch size

- `TOKENIZER_EMBEDDING_BATCH_SIZE`
  An optional positive integer that overrides the number of text chunks sent in
  each embedding request. When it is not set, RAGFlow uses the embedding
  model's batch size, or `16` if the model does not provide one. Larger values
  can increase memory usage and may exceed the model provider's request limit;
  increase it gradually and verify parsing on representative documents.

### SeekDB memory

When `DOC_ENGINE=seekdb`, `SEEKDB_MEMORY_LIMIT` controls the memory limit passed
to the bundled SeekDB service and defaults to `2G`. The
[SeekDB deployment requirements](https://www.oceanbase.ai/docs/V1.1.0/deploy-by-systemd)
specify at least 1 CPU core, 2 GB available memory, and 15 GB free data-disk
space. These are SeekDB-only requirements, not the requirements for the
complete RAGFlow deployment. The container is also subject to the applicable
`MEM_LIMIT` upper limit.

### OceanBase prerequisites

Before setting `DOC_ENGINE=oceanbase`, plan memory separately from the general
RAGFlow host recommendation. For the complete RAGFlow deployment with the
bundled OceanBase service, use at least 4 CPU cores and 32 GB host memory as a
starting point. This leaves room beyond OceanBase's own
[production requirements](https://en.oceanbase.com/docs/common-oceanbase-database-10000000001166993)
for the other RAGFlow services. OceanBase defaults to `OB_MEMORY_LIMIT=10G` and
`OB_SYSTEM_MEMORY=2G`. Its data file and log disk default to
`OB_DATAFILE_SIZE=20G` and `OB_LOG_DISK_SIZE=20G`. These two values describe
OceanBase's configured disk allocation, not the total disk required by the
deployment. Reserve additional space for container images, RAGFlow object
storage, indexes, and logs. Before enabling this profile, raise `MEM_LIMIT` so the
OceanBase container limit is not lower than `OB_MEMORY_LIMIT`; a 12 GiB limit
is recommended to leave container headroom. In **docker/.env**, set:

```dotenv
MEM_LIMIT=12884901888
```

These are deployment recommendations, not memory reserved exclusively for
OceanBase. The host must also provide memory for RAGFlow and the other enabled
services.

Also make sure the host OS allows the file descriptor and core dump limits OceanBase expects.

1. Set host limits:

   ```bash
   sudo tee /etc/security/limits.d/99-oceanbase.conf >/dev/null <<'EOF'
   root soft nofile 655350
   root hard nofile 655350
   * soft nofile 655350
   * hard nofile 655350
   * soft core unlimited
   * hard core unlimited
   EOF
   ```

2. Make sure PAM limits are enabled:

   ```bash
   grep -E 'pam_limits\.so' /etc/pam.d/common-session /etc/pam.d/common-session-noninteractive
   ```

   If missing, add them:

   ```bash
   echo 'session required pam_limits.so' | sudo tee -a /etc/pam.d/common-session
   echo 'session required pam_limits.so' | sudo tee -a /etc/pam.d/common-session-noninteractive
   ```

3. Log out and log back in, or reboot.

4. Verify the effective limit:

   ```bash
   ulimit -n
   ```

   Expected: `655350`, or at least `20000`.

## 🐋 Service configuration

[service_conf.yaml.template](./service_conf.yaml.template) specifies the system-level configuration for the Go API, Admin, Ingestor, and Syncer services. In a dockerized setup, the generated `service_conf.yaml` file is automatically created from this template (replacing all environment variables by their values).

- `ragflow`
  - `host`: The API server's IP address inside the Docker container. Defaults to `0.0.0.0`.
  - `http_port`: The API server's serving port inside the Docker container. Defaults to `9380`.

- `admin`
  - `host`: The Admin server's IP address inside the Docker container. Defaults to `0.0.0.0`.
  - `http_port`: The Admin server's serving port inside the Docker container. Defaults to `9381`.

- `mysql`
  - `name`: The MySQL database name. Defaults to `rag_flow`.
  - `user`: The username for MySQL.
  - `password`: The password for MySQL.
  - `port`: The MySQL serving port inside the Docker container. Defaults to `3306`.
  - `max_connections`: The maximum number of concurrent connections in the MySQL connection pool. Defaults to `900`.
  - `stale_timeout`: The connection stale timeout in seconds. Defaults to `300`.
  - `max_allowed_packet`: The maximum communication packet size. Defaults to `1073741824` bytes (`1 GiB`).

- `minio`
  - `user`: The username for MinIO.
  - `password`: The password for MinIO.
  - `host`: The MinIO serving IP *and* port inside the Docker container. Defaults to `minio:9000`.

- `oceanbase`
  - `scheme`: The connection scheme. Set to `mysql` to use mysql config, or other values to use config below.
  - `config`:
    - `db_name`: The OceanBase database name.
    - `user`: The username for OceanBase.
    - `password`: The password for OceanBase.
    - `host`: The hostname of the OceanBase service.
    - `port`: The port of OceanBase.

- `gaussdb`
  - `host`: The hostname or IP address of the GaussDB instance.
  - `port`: The GaussDB port.
  - `database`: The GaussDB database name. Defaults to `postgres`.
  - `user`: The username for GaussDB.
  - `password`: The password for GaussDB.
  - `schema`: Optional schema used by the DocEngine. Defaults to `public`.
  - RAGFlow does not start or manage GaussDB; set `DOC_ENGINE=gaussdb` only after preparing a GaussDB instance.

- `oss`
  - `access_key`: The access key ID used to authenticate requests to the OSS service.
  - `secret_key`: The secret access key used to authenticate requests to the OSS service.
  - `endpoint_url`: The URL of the OSS service endpoint.
  - `region`: The OSS region where the bucket is located.
  - `bucket`: The name of the OSS bucket where files will be stored. When you want to store all files in a specified bucket, you need this configuration item.
  - `prefix_path`: Optional. A prefix path to prepend to file names in the OSS bucket, which can help organize files within the bucket.

- `s3`:
  - `access_key`: The access key ID used to authenticate requests to the S3 service.
  - `secret_key`: The secret access key used to authenticate requests to the S3 service.
  - `endpoint_url`: The URL of the S3-compatible service endpoint. This is necessary when using an S3-compatible protocol instead of the default AWS S3 endpoint.
  - `bucket`: The name of the S3 bucket where files will be stored. When you want to store all files in a specified bucket, you need this configuration item.
  - `region`: The AWS region where the S3 bucket is located. This is important for directing requests to the correct data center.
  - `signature_version`: Optional. The version of the signature to use for authenticating requests. Common versions include `v4`.
  - `addressing_style`: Optional. The style of addressing to use for the S3 endpoint. This can be `path` or `virtual`.
  - `prefix_path`: Optional. A prefix path to prepend to file names in the S3 bucket, which can help organize files within the bucket.

- `oauth`
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

- `user_default_llm`
  The default LLM to use for a new RAGFlow user. It is disabled by default. To enable this feature, uncomment the corresponding lines in **service_conf.yaml.template**.
  - `factory`: The LLM supplier. Available options:
    - `"OpenAI"`
    - `"DeepSeek"`
    - `"Moonshot"`
    - `"Tongyi-Qianwen"`
    - `"VolcEngine"`
    - `"ZHIPU-AI"`
  - `api_key`: The API key for the specified LLM. You will need to apply for your model API key online.

> 💡 **Tip:** If you do not set the default LLM here, configure the default LLM on the **Settings** page in the RAGFlow UI.


## 📋 Setup Examples

### 🔒 HTTPS Setup

#### Prerequisites

- A registered domain name pointing to your server
- Port 80 and 443 open on your server
- Docker and Docker Compose installed

#### Getting and configuring certificates (Let's Encrypt)

If you want your instance to be available under `https`, follow these steps:

1. **Install Certbot and obtain certificates**
   ```bash
   # Ubuntu/Debian
   sudo apt update && sudo apt install certbot

   # CentOS/RHEL
   sudo yum install certbot

   # Obtain certificates (replace with your actual domain)
   sudo certbot certonly --standalone -d your-ragflow-domain.com
   ```

2. **Locate your certificates**
   Once generated, your certificates will be located at:
   - Certificate: `/etc/letsencrypt/live/your-ragflow-domain.com/fullchain.pem`
   - Private key: `/etc/letsencrypt/live/your-ragflow-domain.com/privkey.pem`

3. **Update docker-compose.yml**
   Add the certificate volumes to the `ragflow-cpu` or `ragflow-gpu` service in `docker-compose.yml`:
   ```yaml
   services:
     ragflow-cpu:
       # ...existing configuration...
       volumes:
         # SSL certificates
         - /etc/letsencrypt/live/your-ragflow-domain.com/fullchain.pem:/etc/nginx/ssl/fullchain.pem:ro
         - /etc/letsencrypt/live/your-ragflow-domain.com/privkey.pem:/etc/nginx/ssl/privkey.pem:ro
         # Switch to HTTPS nginx configuration
         - ./nginx/ragflow.https.conf:/etc/nginx/conf.d/ragflow.conf
         # ...other existing volumes...

   ```

4. **Update nginx configuration**
   Edit `nginx/ragflow.https.conf` and replace `my_ragflow_domain.com` with your actual domain name.

5. **Restart the services**
   ```bash
   docker compose --env-file .env -f docker-compose.yml down
   docker compose --env-file .env -f docker-compose.yml up -d
   ```


> ⚠️ **Important：**
> - Ensure your domain's DNS A record points to your server's IP address.
> - Stop any services running on ports 80/443 before obtaining certificates with `--standalone`.

> 💡 **Tip：** For development or testing, you can use self-signed certificates, but browsers will show security warnings.

#### Alternative: Using existing certificates

If you already have SSL certificates from another provider:

1. Place your certificates in a directory accessible to Docker
2. Update the volume paths in `docker-compose.yml` to point to your certificate files
3. Ensure the certificate file contains the full certificate chain
4. Follow steps 4-5 from the Let's Encrypt guide above
