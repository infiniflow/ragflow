# README

<details open>
<summary></b>📗 Table of Contents</b></summary>

- 🐳 [Docker Compose](#-docker-compose)
- 🐬 [Docker environment variables](#-docker-environment-variables)
- 🐋 [Service configuration](#-service-configuration)

</details>

## 🐳 Docker Compose

- **docker-compose.yml**  
  Sets up environment for RAGFlow and its dependencies.
- **docker-compose-base.yml**  
  Sets up environment for RAGFlow's dependencies: Elasticsearch, MySQL, MinIO, and Redis.

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
  
  - `infiniflow/ragflow:dev-slim` (default): The RAGFlow Docker image without embedding models.  
  - `infiniflow/ragflow:dev`: The RAGFlow Docker image with embedding models including:
    - Built-in embedding models:
      - `BAAI/bge-large-zh-v1.5` 
      - `BAAI/bge-reranker-v2-m3`
      - `maidalun1020/bce-embedding-base_v1`
      - `maidalun1020/bce-reranker-base_v1`
    - Embedding models that will be downloaded once you select them in the RAGFlow UI:
      - `BAAI/bge-base-en-v1.5`
      - `BAAI/bge-large-en-v1.5`
      - `BAAI/bge-small-en-v1.5`
      - `BAAI/bge-small-zh-v1.5`
      - `jinaai/jina-embeddings-v2-base-en`
      - `jinaai/jina-embeddings-v2-small-en`
      - `nomic-ai/nomic-embed-text-v1.5`
      - `sentence-transformers/all-MiniLM-L6-v2`
  
> [!TIP]  
> If you cannot download the RAGFlow Docker image, try the following mirrors.  
> 
> - For the `dev-slim` edition:  
>   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:dev-slim` or,
>   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:dev-slim`.
> - For the `dev` edition:  
>   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:dev` or,
>   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:dev`.

### Timezone

- `TIMEZONE`  
  The local time zone. Defaults to `'Asia/Shanghai'`.

### Hugging Face mirror site

- `HF_ENDPOINT`  
  The mirror site for huggingface.co. It is disabled by default. You can uncomment this line if you have limited access to the primary Hugging Face domain.

### MacOS

- `MACOS`  
  Optimizations for MacOS. It is disabled by default. You can uncomment this line if your OS is MacOS.

### Maximum file size

- `MAX_CONTENT_LENGTH`  
  The maximum file size for each uploaded file, in bytes. You can uncomment this line if you wish to change the 128M file size limit.

## 🐋 Service configuration

[service_conf.yaml](./service_conf.yaml) specifies the system-level configuration for RAGFlow and is used by its API server and task executor. In a dockerized setup, this file is automatically created based on the [service_conf.yaml.template](./service_conf.yaml.template) file (replacing all environment variables by their values).

- `ragflow`
  - `host`: The API server's IP address inside the Docker container. Defaults to `0.0.0.0`.
  - `port`: The API server's serving port inside the Docker container. Defaults to `9380`.

- `mysql`
  - `name`: The MySQL database name. Defaults to `rag_flow`.
  - `user`: The username for MySQL.
  - `password`: The password for MySQL. When updated, you must revise the `MYSQL_PASSWORD` variable in [.env](./.env) accordingly.
  - `port`: The MySQL serving port inside the Docker container. Defaults to `3306`.
  - `max_connections`: The maximum number of concurrent connections to the MySQL database. Defaults to `100`.
  - `stale_timeout`: Timeout in seconds.

- `minio`
  - `user`: The username for MinIO. When updated, you must revise the `MINIO_USER` variable in [.env](./.env) accordingly.
  - `password`: The password for MinIO. When updated, you must revise the `MINIO_PASSWORD` variable in [.env](./.env) accordingly.
  - `host`: The MinIO serving IP *and* port inside the Docker container. Defaults to `minio:9000`.

- `oauth`  
  The OAuth configuration for signing up or signing in to RAGFlow using a third-party account.  It is disabled by default. To enable this feature, uncomment the corresponding lines in **service_conf.yaml.template**.
  - `github`: The GitHub authentication settings for your application. Visit the [Github Developer Settings page](https://github.com/settings/developers) to obtain your client_id and secret_key.

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