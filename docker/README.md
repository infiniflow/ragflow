# README

<details open>
<summary></b>📕 Table of Contents</b></summary>

- 🐳 [Docker Compose](#-docker-compose)
- 🐬 [Docker environment variables](#-docker-environment-variables)
- 🔧 [Service configuration](#-service-configuration)

</details>

## Docker Compose

- **docker-compose.yml**
- **docker-compose-gpu.yml**
- docker-compose



##  Docker environment variables

The [.env](./.env) file contains important environment variables for Docker.

### Elasticsearch-specific

- `STACK_VERSION`  
  The version of Elasticsearch. Defaults to `8.11.3`
- `ES_PORT`  
  Port to expose Elasticsearch HTTP API to the host. Defaults to `1200`.
- `ELASTIC_PASSWORD`  
  The Elasticsearch password. When updated, you must also revise the `es.password` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.

### Kibana-specific

- `KIBANA_PORT`
- 

### MySQL-specific

- `MYSQL_PASSWORD`  
  The password for MySQL. When updated, you must also revise the `mysql.password` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.
- `MYSQL_PORT`  
  The exported port number of MySQL Docker container, needed when you access the database from **outside** the Docker container. Defaults to `5455`.

### MinIO-specific

- `MINIO_CONSOLE_PORT`  

  Defaults to `9001`

- `MINIO_PORT`  

  Defaults to `9000`.

- `MINIO_USER`  
  The MinIO username. When updated, you must also revise the `minio.user` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.

- `MINIO_PASSWORD`  
  The MinIO password. When updated, you must also revise the `minio.password` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.

### RAGFlow-specific

- `SVR_HTTP_PORT`  
  The port number on which RAGFlow's API server listens. Defaults to `9380`.

- `RAGFLOW-IMAGE`  
  The Docker image edition. Available editions:  

  - `infiniflow/ragflow:dev-slim` (default): The RAGFlow Docker image without embedding models.  
  - `infiniflow/ragflow:dev`: The RAGFlow Docker image with embedding models including:
    - Embedded embedding models:
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
  >
  > If you cannot download the RAGFlow Docker image, try the following hub.docker.com mirrors.
  >
  > - For `dev-slim`:
  >   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:dev-slim` or,
  >   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:dev-slim`.
  > - For `dev`:
  >   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:dev` or,
  >   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:dev`.

### Miscellaneous

- `TIMEZONE`  
  The local time zone. Defaults to `'Asia/Shanghai'`.


##  Service configuration

[service_conf.yaml](./service_conf.yaml) specifies the system-level configuration for RAGFlow and is used by its API server and task executor.

- `ragflow`
  
  - `host`: The API server's IP address **inside** the Docker container. Defaults to `0.0.0.0`.
  - `port`: The API server's serving port **inside** the Docker container. Defaults to `9380`.
  
- `mysql`
  
  - `name`: The MySQL database name. Defaults to `rag_flow`.
  - `user`: The MySQL username.
  - `password`: The MySQL password. When updated, you must also revise the `MYSQL_PASSWORD` variable in [.env](./.env) accordingly.
  - `port`: The MySQL serving port **inside** the Docker container. Defaults to `3306`.
  - `max_connections`: The maximum number of concurrent connections to the MySQL database. Defaults to `100`.
  - `stale_timeout`: Timeout in seconds.
  
- `minio`
  
  - `user`: The MinIO username. When updated, you must also revise the `MINIO_USER` variable in [.env](./.env) accordingly.
  - `password`: The MinIO password. When updated, you must also revise the `MINIO_PASSWORD` variable in [.env](./.env) accordingly.
  - `host`: The MinIO serving IP *and* port **inside** the Docker container. Defaults to `minio:9000`.
  
- `user_default_llm`   
  
  The default LLM to use for a new RAGFlow user. It is disabled by default. To enable this feature, uncomment the corresponding lines in **service_conf.yaml**.  
  
  > [!TIP]
  >
  > If you do not set the default LLM here, configure the default LLM on the **Settings** page in the RAGFlow UI.  
  
  - `factory`: The LLM supplier. Available options: 
    - `"Baichuan"`
    - `"DeepSeek"`
    - `"Moonshot"`
    - `"OpenAI"`
    - `"Tongyi-Qianwen"`
    - `"VolcEngine"`
    - `"ZHIPU-AI"`
  - `api_key`: The API key for the specified LLM. You will need to apply for your model API key online.
  
- `oauth`  
  The OAuth configuration for signing up or signing in to RAGFlow using a third-party account.  It is disabled by default. To enable this feature, uncomment the corresponding lines in **service_conf.yaml**.
  
  - `github`: The GitHub authentication settings for your application. Visit the [Github Developer Settings page](https://github.com/settings/developers) to obtain your client_id and secret_key.

