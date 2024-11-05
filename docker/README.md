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
  The exported port number of MySQL Docker container, needed when you access the database from outside the docker containers.

- `MINIO_USER`  
  The MinIO username. When updated, you must also revise the `minio.user` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.

- `MINIO_PASSWORD`  
  The MinIO password. When updated, you must also revise the `minio.password` entry in  [service_conf.yaml](./service_conf.yaml) accordingly.



- `SVR_HTTP_PORT`  
  The port number on which RAGFlow's backend API server listens.

- `RAGFLOW-IMAGE`  
  The Docker image edition. Available options:  
  - `infiniflow/ragflow:dev-slim` (default): The RAGFlow Docker image without embedding models  
  - `infiniflow/ragflow:dev`: The RAGFlow Docker image with embedding models. See the 

- `TIMEZONE`  
  The local time zone.


##  Service Configuration

[service_conf.yaml](./service_conf.yaml) defines the system-level configuration for RAGFlow and is used by RAGFlow's *API server* and *task executor*.

- `ragflow`
  - `host`: The IP address of the API server.
  - `port`: The serving port of API server.

- `mysql`
  - `name`: The database name in MySQL used by RAGFlow.
  - `user`: The database name in MySQL used by RAGFlow.
  - `password`: The database password. When updated, you must also revise the `MYSQL_PASSWORD` variable in [.env](./.env) accordingly.
  - `port`: The serving port of MySQL inside the container. When updated, you must also revise the `MYSQL_PORT` variable in [.env](./.env) accordingly.
  - `max_connections`: The maximum database connection.
  - `stale_timeout`: The timeout duration in seconds.

- `minio`
  - `user`: The MinIO username. When updated, you must also revise the `MINIO_USER` variable in [.env](./.env) accordingly.
  - `password`: The MinIO password. When updated, you must also revise the `MINIO_PASSWORD` variable in [.env](./.env) accordingly.
  - `host`: The serving IP and port inside the docker container. This is not updating until changing the minio part in [docker-compose.yml](./docker-compose.yml)

- `user_default_llm`  
  Newly signed-up users use LLM configured by this part; otherwise, you need to configure your own LLM on the *Settings* page.  
  - `factory`: The LLM suppliers. "OpenAI"， "Tongyi-Qianwen", "ZHIPU-AI", "Moonshot", "DeepSeek", "Baichuan", and "VolcEngine" are supported.
  - `api_key`: The API key for the specified LLM.

- `oauth`  
  The OAuth configuration for signing up or signing in to RAGFlow using a third-party account.  
  - `github`: Go to [Github](https://github.com/settings/developers), register a new application, the *client_id* and *secret_key* will be given.

