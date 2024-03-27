
# Docker Environment Variable

Look into [.env](./.env), there're some important variables.

## MYSQL_PASSWORD
The mysql password could be changed by this variable. But you need to change *mysql.password* in [service_conf.yaml](./service_conf.yaml) at the same time.


## MYSQL_PORT
It refers to exported port number of mysql docker container, it's useful if you want to access the database outside the docker containers.

## MINIO_USER
It refers to user name of [Mino](https://github.com/minio/minio). The modification should be synchronous updating at minio.user of  [service_conf.yaml](./service_conf.yaml).

## MINIO_PASSWORD
It refers to user password of [Mino](https://github.com/minio/minio). The modification should be synchronous updating at minio.password of  [service_conf.yaml](./service_conf.yaml).


## SVR_HTTP_PORT
It refers to The API server serving port.


# Service Configuration
[service_conf.yaml](./service_conf.yaml) is used by the *API server* and *task executor*. It's the most important configuration of the system.

## ragflow

### host
The IP address used by the API server.

### port
The serving port of API server.

## mysql

### name
The database name in mysql used by this system.

### user
The database user name.

### password
The database password. The modification should be synchronous updating at *MYSQL_PASSWORD* in [.env](./.env).

### port
The serving port of mysql inside the container. The modification should be synchronous updating at [docker-compose.yml](./docker-compose.yml)

### max_connections
The max database connection.

### stale_timeout
The timeout duation in seconds.

## minio

### user
The username of minio. The modification should be synchronous updating at *MINIO_USER* in [.env](./.env).

### password
The password of minio. The modification should be synchronous updating at *MINIO_PASSWORD* in [.env](./.env).

### host
The serving IP and port inside the docker container. This is not updating until changing the minio part in [docker-compose.yml](./docker-compose.yml)

## user_default_llm
Newly signed-up users use LLM configured by this part. Otherwise, user need to configure his own LLM in *setting*.
  
### factory
The LLM suppliers. 'Tongyi-Qianwen', "OpenAI"， "Moonshot" and "ZHIPU-AI" are supported.

### api_key
The corresponding API key of your assigned LLM vendor.

## oauth
This is OAuth configuration which allows your system using the third-party account to sign-up and sign-in to the system.

### github
Got to [Github](https://github.com/settings/developers), register new application, the *client_id* and *secret_key* will be given.

