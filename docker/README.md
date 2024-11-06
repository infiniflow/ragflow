# README



##  Docker environment variables

Look into [.env](./.env), there're some important variables.

- `STACK_VERSION`  
  The Elasticsearch version. Defaults to `8.11.3`
- `ES_PORT`  
  Port to expose Elasticsearch HTTP API to the host. Defaults to `1200`.
- `ELASTIC_PASSWORD`  
  The Elasticsearch password.
- `MYSQL_PASSWORD`  
  The MySQL password. When updated, you must also revise the `mysql.password` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.
- `MYSQL_PORT`  
  The exported port number of MySQL Docker container, needed when you access the database from outside the Docker container.
- `MINIO_USER`  
  The MinIO username. When updated, you must also revise the `minio.user` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.
- `MINIO_PASSWORD`  
  The MinIO password. When updated, you must also revise the `minio.password` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.
- `SVR_HTTP_PORT`  
  The port number on which RAGFlow's backend API server listens.
- `TIMEZONE`  
  The local time zone.
- `RAGFLOW-IMAGE`  
  The Docker image edition. Available options:  
  - `infiniflow/ragflow:dev-slim` (default): The RAGFlow Docker image without embedding models  
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


##  Service Configuration

[service_conf.yaml](./service_conf.yaml) defines the system-level configuration for RAGFlow and is used by its API server and task executor.

- `ragflow`
  
  - `host`: The IP address of the API server.
  - `port`: The serving port of API server.
  
- `mysql`
  
  - `name`: The database name in MySQL used by RAGFlow. Defaults to `rag_flow`.
  - `user`: The MySQL user name.
  - `password`: The MySQL password. When updated, you must also revise the `MYSQL_PASSWORD` variable in [.env](./.env) accordingly.
  - `port`: The serving port of MySQL inside the Docker container. When updated, you must also revise the `MYSQL_PORT` variable in [.env](./.env) accordingly.
  - `max_connections`: The maximum database connection.
  - `stale_timeout`: Timeout in seconds.
  
- `minio`
  
  - `user`: The MinIO username. When updated, you must also revise the `MINIO_USER` variable in [.env](./.env) accordingly.
  - `password`: The MinIO password. When updated, you must also revise the `MINIO_PASSWORD` variable in [.env](./.env) accordingly.
  - `host`: The serving IP and port inside the docker container. This is not updated until changing the minio part in [docker-compose.yml](./docker-compose.yml)
  
- `user_default_llm`   
  
  The default LLM to use for a new RAGFlow user. It is disabled by default. If you have not set it here, you can configure the default LLM on the **Settings** page in the RAGFlow UI. Newly signed-up users use LLM configured by this part; otherwise, you need to configure your own LLM on the *Settings* page.  
  
  - `factory`: The LLM suppliers. "OpenAI"， "Tongyi-Qianwen", "ZHIPU-AI", "Moonshot", "DeepSeek", "Baichuan", and "VolcEngine" are supported.
  - `api_key`: The API key for the specified LLM.
  
- `oauth`  
  The OAuth configuration for signing up or signing in to RAGFlow using a third-party account.  It is disabled by default. To enable this feature, uncomment the corresponding lines in **service_conf.yaml**.
  
  - `github`: The GitHub authentication settings for your application. Visit the [Github Developer Settings page](https://github.com/settings/developers) to obtain your client_id and secret_key.

