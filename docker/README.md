# README

<details open>
<summary></b>📗 Table of Contents</b></summary>

- 🐳 [Docker Compose](#-docker-compose)
- 🐬 [Docker environment variables](#-docker-environment-variables)
- 🐋 [Service configuration](#-service-configuration)
- 📋 [Setup Examples](#-setup-examples)

</details>

## 🐳 Docker Compose

- **docker-compose.yml**  
  Sets up environment for RAGFlow and its dependencies.
- **docker-compose-base.yml**  
  Sets up environment for RAGFlow's dependencies: Elasticsearch/[Infinity](https://github.com/infiniflow/infinity), MySQL, MinIO, and Redis.

> [!CAUTION]
> We do not actively maintain **docker-compose-CN-oc9.yml**, **docker-compose-gpu-CN-oc9.yml**, or **docker-compose-gpu.yml**, so use them at your own risk. However, you are welcome to file a pull request to improve any of them.

## 🐬 Docker environment variables

The [.env](./.env) file contains important environment variables for Docker.

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
  The port used to expose the MySQL service to the host machine, allowing **external** access to the MySQL database running inside the Docker container. Defaults to `5455`.

### MinIO

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
- `REDIS_PASSWORD`  
  The password for Redis.

### RAGFlow

- `SVR_HTTP_PORT`  
  The port used to expose RAGFlow's HTTP API service to the host machine, allowing **external** access to the service running inside the Docker container. Defaults to `9380`.
- `RAGFLOW-IMAGE`  
  The Docker image edition. Available editions:  
  
  - `infiniflow/ragflow:v0.20.4-slim` (default): The RAGFlow Docker image without embedding models.  
  - `infiniflow/ragflow:v0.20.4`: The RAGFlow Docker image with embedding models including:
    - Built-in embedding models:
      - `BAAI/bge-large-zh-v1.5` 
      - `maidalun1020/bce-embedding-base_v1`

  
> [!TIP]  
> If you cannot download the RAGFlow Docker image, try the following mirrors.  
> 
> - For the `nightly-slim` edition:  
>   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:nightly-slim` or,
>   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:nightly-slim`.
> - For the `nightly` edition:  
>   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:nightly` or,
>   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:nightly`.

### Timezone

- `TIMEZONE`  
  The local time zone. Defaults to `'Asia/Shanghai'`.

### Hugging Face mirror site

- `HF_ENDPOINT`  
  The mirror site for huggingface.co. It is disabled by default. You can uncomment this line if you have limited access to the primary Hugging Face domain.

### MacOS

- `MACOS`  
  Optimizations for macOS. It is disabled by default. You can uncomment this line if your OS is macOS.

### Maximum file size

- `MAX_CONTENT_LENGTH`  
  The maximum file size for each uploaded file, in bytes. You can uncomment this line if you wish to change the 128M file size limit. After making the change, ensure you update `client_max_body_size` in nginx/nginx.conf correspondingly.

### Doc bulk size

- `DOC_BULK_SIZE`  
  The number of document chunks processed in a single batch during document parsing. Defaults to `4`.

### Embedding batch size

- `EMBEDDING_BATCH_SIZE`  
  The number of text chunks processed in a single batch during embedding vectorization. Defaults to `16`.

## 🐋 Service configuration

[service_conf.yaml](./service_conf.yaml) specifies the system-level configuration for RAGFlow and is used by its API server and task executor. In a dockerized setup, this file is automatically created based on the [service_conf.yaml.template](./service_conf.yaml.template) file (replacing all environment variables by their values).

- `ragflow`
  - `host`: The API server's IP address inside the Docker container. Defaults to `0.0.0.0`.
  - `port`: The API server's serving port inside the Docker container. Defaults to `9380`.

- `mysql`
  - `name`: The MySQL database name. Defaults to `rag_flow`.
  - `user`: The username for MySQL.
  - `password`: The password for MySQL.
  - `port`: The MySQL serving port inside the Docker container. Defaults to `3306`.
  - `max_connections`: The maximum number of concurrent connections to the MySQL database. Defaults to `100`.
  - `stale_timeout`: Timeout in seconds.

- `minio`
  - `user`: The username for MinIO.
  - `password`: The password for MinIO.
  - `host`: The MinIO serving IP *and* port inside the Docker container. Defaults to `minio:9000`.

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
    - `redirect_uri`: Required, URI to which the authorization server redirects during the authentication flow to return results. Must match the callback URI registered with the authentication server. Format: `https://your-app.com/v1/user/oauth/callback/<channel>`. For local configuration, you can directly use `http://127.0.0.1:80/v1/user/oauth/callback/<channel>`.

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

> [!TIP]  
> If you do not set the default LLM here, configure the default LLM on the **Settings** page in the RAGFlow UI.


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
   Add the certificate volumes to the `ragflow` service in your `docker-compose.yml`:
   ```yaml
   services:
     ragflow:
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
   docker-compose down
   docker-compose up -d
   ```


> [!IMPORTANT]
> - Ensure your domain's DNS A record points to your server's IP address
> - Stop any services running on ports 80/443 before obtaining certificates with `--standalone`

> [!TIP]
> For development or testing, you can use self-signed certificates, but browsers will show security warnings.

#### Alternative: Using existing certificates

If you already have SSL certificates from another provider:

1. Place your certificates in a directory accessible to Docker
2. Update the volume paths in `docker-compose.yml` to point to your certificate files
3. Ensure the certificate file contains the full certificate chain
4. Follow steps 4-5 from the Let's Encrypt guide above