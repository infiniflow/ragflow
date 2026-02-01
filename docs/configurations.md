---
sidebar_position: 1
slug: /configurations
sidebar_custom_props: {
  sidebarIcon: LucideCog
}
---
# Configuration

Configurations for deploying RAGFlow via Docker.

## Guidelines

When it comes to system configurations, you will need to manage the following files:

- [.env](https://github.com/infiniflow/ragflow/blob/main/docker/.env): Contains important environment variables for Docker.
- [service_conf.yaml.template](https://github.com/infiniflow/ragflow/blob/main/docker/service_conf.yaml.template): Configures the back-end services. It specifies the system-level configuration for RAGFlow and is used by its API server and task executor. Upon container startup, the `service_conf.yaml` file will be generated based on this template file. This process replaces any environment variables within the template, allowing for dynamic configuration tailored to the container's environment.
- [docker-compose.yml](https://github.com/infiniflow/ragflow/blob/main/docker/docker-compose.yml): The Docker Compose file for starting up the RAGFlow service.

To update the default HTTP serving port (80), go to [docker-compose.yml](https://github.com/infiniflow/ragflow/blob/main/docker/docker-compose.yml) and change `80:80`
to `<YOUR_SERVING_PORT>:80`.

:::tip NOTE
Updates to the above configurations require a reboot of all containers to take effect:

```bash
docker compose -f docker/docker-compose.yml up -d
```

:::

## Docker Compose

- **docker-compose.yml**
  Sets up environment for RAGFlow and its dependencies.
- **docker-compose-base.yml**
  Sets up environment for RAGFlow's dependencies: Elasticsearch/[Infinity](https://github.com/infiniflow/infinity), MySQL, MinIO, and Redis.

:::danger IMPORTANT
We do not actively maintain **docker-compose-CN-oc9.yml**, **docker-compose-macos.yml**, so use them at your own risk. However, you are welcome to file a pull request to improve them.
:::

## Docker environment variables

The [.env](https://github.com/infiniflow/ragflow/blob/main/docker/.env) file contains important environment variables for Docker.

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
- `KIBANA_USER`
  The username for Kibana. Defaults to `rag_flow`.
- `KIBANA_PASSWORD`
  The password for Kibana. Defaults to `infini_rag_flow`.

### Resource management

- `MEM_LIMIT`
  The maximum amount of the memory, in bytes, that *a specific* Docker container can use while running. Defaults to `8073741824`.

### MySQL

- `MYSQL_PASSWORD`
  The password for MySQL.
- `MYSQL_PORT`
  The port to connect to MySQL from RAGFlow container. Defaults to `3306`. Change this if you use an external MySQL.
- `EXPOSE_MYSQL_PORT`
  The port used to expose the MySQL service to the host machine, allowing **external** access to the MySQL database running inside the Docker container. Defaults to `5455`.

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

### Redis

- `REDIS_PORT`
  The port used to expose the Redis service to the host machine, allowing **external** access to the Redis service running inside the Docker container. Defaults to `6379`.
- `REDIS_USERNAME`
  Optional Redis ACL username when using Redis 6+ authentication.
- `REDIS_PASSWORD`
  The password for Redis.

### RAGFlow

- `SVR_HTTP_PORT`
  The port used to expose RAGFlow's HTTP API service to the host machine, allowing **external** access to the service running inside the Docker container. Defaults to `9380`.
- `RAGFLOW-IMAGE`
  The Docker image edition. Defaults to `infiniflow/ragflow:v0.23.1` (the RAGFlow Docker image without embedding models).

:::tip NOTE
If you cannot download the RAGFlow Docker image, try the following mirrors.

- For the `nightly` edition:
  - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:nightly` or,
  - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:nightly`.
:::

### Embedding service

- `TEI_MODEL`
  The embedding model which text-embeddings-inference serves. Allowed values are one of `Qwen/Qwen3-Embedding-0.6B`(default), `BAAI/bge-m3`, and `BAAI/bge-small-en-v1.5`.

- `TEI_PORT`
  The port used to expose the text-embeddings-inference service to the host machine, allowing **external** access to the text-embeddings-inference service running inside the Docker container. Defaults to `6380`.

### Timezone

- `TZ`
  The local time zone. Defaults to `Asia/Shanghai`.

### Hugging Face mirror site

- `HF_ENDPOINT`
  The mirror site for huggingface.co. It is disabled by default. You can uncomment this line if you have limited access to the primary Hugging Face domain.

### MacOS

- `MACOS`
  Optimizations for macOS. It is disabled by default. You can uncomment this line if your OS is macOS.

### User registration

- `REGISTER_ENABLED`
  - `1`: (Default) Enable user registration.
  - `0`: Disable user registration.

## Service configuration

[service_conf.yaml.template](https://github.com/infiniflow/ragflow/blob/main/docker/service_conf.yaml.template) specifies the system-level configuration for RAGFlow and is used by its API server and task executor.

### `ragflow`

- `host`: The API server's IP address inside the Docker container. Defaults to `0.0.0.0`.
- `port`: The API server's serving port inside the Docker container. Defaults to `9380`.

### `mysql`

- `name`: The MySQL database name. Defaults to `rag_flow`.
- `user`: The username for MySQL.
- `password`: The password for MySQL.
- `port`: The MySQL serving port inside the Docker container. Defaults to `3306`.
- `max_connections`: The maximum number of concurrent connections to the MySQL database. Defaults to `100`.
- `stale_timeout`: Timeout in seconds.

### `minio`

- `user`: The username for MinIO.
- `password`: The password for MinIO.
- `host`: The MinIO serving IP *and* port inside the Docker container. Defaults to `minio:9000`.

### `redis`

- `host`: The Redis serving IP *and* port inside the Docker container. Defaults to `redis:6379`.
- `db`: The Redis database index to use. Defaults to `1`.
- `username`: Optional Redis ACL username (Redis 6+).
- `password`: The password for the specified Redis user.

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
  - `redirect_uri`: Required, URI to which the authorization server redirects during the authentication flow to return results. Must match the callback URI registered with the authentication server. Format: `https://your-app.com/v1/user/oauth/callback/<channel>`. For local configuration, you can directly use `http://127.0.0.1:80/v1/user/oauth/callback/<channel>`.

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
    redirect_uri: "https://your-app.com/v1/user/oauth/callback/oauth2"

  oidc:
    display_name: "OIDC"
    client_id: "your_client_id"
    client_secret: "your_client_secret"
    issuer: "https://your-oauth-provider.com/oidc"
    scope: "openid email profile"
    redirect_uri: "https://your-app.com/v1/user/oauth/callback/oidc"

  github:
    # https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/creating-an-oauth-app
    type: "github"
    icon: "github"
    display_name: "Github"
    client_id: "your_client_id"
    client_secret: "your_client_secret"
    redirect_uri: "https://your-app.com/v1/user/oauth/callback/github"
```
:::

### `user_default_llm`

The default LLM to use for a new RAGFlow user. It is disabled by default. To enable this feature, uncomment the corresponding lines in **service_conf.yaml.template**.

- `factory`: The LLM supplier. Available options:
  - `"OpenAI"`
  - `"DeepSeek"`
  - `"Moonshot"`
  - `"Tongyi-Qianwen"`
  - `"VolcEngine"`
  - `"ZHIPU-AI"`
- `api_key`: The API key for the specified LLM. You will need to apply for your model API key online.
- `allowed_factories`: If this is set, the users will be allowed to add only the factories in this list.
  - `"OpenAI"`
  - `"DeepSeek"`
  - `"Moonshot"`

:::tip NOTE
If you do not set the default LLM here, configure the default LLM on the **Settings** page in the RAGFlow UI.
:::
