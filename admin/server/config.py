#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#


import logging
import os
import re
import threading
from enum import Enum

from pydantic import BaseModel
from typing import Any
from common.config_utils import read_config
from urllib.parse import urlparse


class BaseConfig(BaseModel):
    id: int
    name: str
    host: str
    port: int
    service_type: str
    detail_func_name: str

    def to_dict(self) -> dict[str, Any]:
        return {"id": self.id, "name": self.name, "host": self.host, "port": self.port, "service_type": self.service_type}


class ServiceConfigs:
    configs = list[BaseConfig]

    def __init__(self):
        self.configs = []
        self.lock = threading.Lock()


SERVICE_CONFIGS = ServiceConfigs
# The Admin service list reads the same service_conf/local.service_conf files,
# but the GaussDB metadata connection does not come from those files because
# their top-level gaussdb section belongs to DocEngine and Memory Store. This
# lightweight parser keeps Admin's metadata display aligned with the main
# service DATABASE settings instead of showing an inactive mysql/postgres block.
GAUSSDB_ENV_DEFAULTS = {
    "name": "rag_flow",
    "user": "rag_flow",
    "password": "infini_rag_flow",
    "host": "gaussdb",
    "port": 8000,
    "schema": "public",
}
_GAUSSDB_SCHEMA_PATTERN = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def normalize_database_type(database_type: str | None = None) -> str:
    # Only normalize the new GaussDB values. Existing metadata database
    # selection retains the upstream behavior.
    raw_value = database_type or "mysql"
    normalized = raw_value.strip().lower()
    if normalized in {"gaussdb", "gauss"}:
        return "gaussdb"
    return raw_value


def _get_int_env(name: str, default: int) -> int:
    raw_value = os.environ.get(name)
    if raw_value is None:
        return default
    try:
        return int(raw_value)
    except (TypeError, ValueError):
        logging.warning("Ignoring invalid %s=%r; using default %d", name, raw_value, default)
        return default


def _normalize_gaussdb_metadata_schema(value: str | None = None) -> str:
    # Admin only displays the schema and does not create the connection, but it
    # must apply the same validation. Otherwise an invalid
    # GAUSSDB_METADATA_SCHEMA could look valid in Admin while the main service
    # fails during startup.
    schema = (value or GAUSSDB_ENV_DEFAULTS["schema"]).strip() or GAUSSDB_ENV_DEFAULTS["schema"]
    if not _GAUSSDB_SCHEMA_PATTERN.match(schema):
        raise ValueError(f"invalid GAUSSDB_METADATA_SCHEMA: {schema}")
    return schema


def _gaussdb_env_config() -> dict[str, Any]:
    # This configuration is only for Admin display and health checks; it does
    # not create the application ORM connection. Keep the flat metadata fields
    # aligned with common.settings._gaussdb_env_config() so both surfaces report
    # the same connection information.
    return {
        "name": os.environ.get("GAUSSDB_METADATA_DBNAME", GAUSSDB_ENV_DEFAULTS["name"]),
        "user": os.environ.get("GAUSSDB_METADATA_USER", GAUSSDB_ENV_DEFAULTS["user"]),
        "password": os.environ.get("GAUSSDB_METADATA_PASSWORD", GAUSSDB_ENV_DEFAULTS["password"]),
        "host": os.environ.get("GAUSSDB_METADATA_HOST", GAUSSDB_ENV_DEFAULTS["host"]),
        "port": _get_int_env("GAUSSDB_METADATA_PORT", GAUSSDB_ENV_DEFAULTS["port"]),
        "schema": _normalize_gaussdb_metadata_schema(os.environ.get("GAUSSDB_METADATA_SCHEMA")),
    }


class ServiceType(Enum):
    METADATA = "metadata"
    RETRIEVAL = "retrieval"
    MESSAGE_QUEUE = "message_queue"
    RAGFLOW_SERVER = "ragflow_server"
    TASK_EXECUTOR = "task_executor"
    FILE_STORE = "file_store"


class MetaConfig(BaseConfig):
    meta_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["meta_type"] = self.meta_type
        result["extra"] = extra_dict
        return result


class MySQLConfig(MetaConfig):
    username: str
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["username"] = self.username
        extra_dict["password"] = self.password
        result["extra"] = extra_dict
        return result


class GaussDBMetadataConfig(MySQLConfig):
    metadata_schema: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        result["extra"]["schema"] = self.metadata_schema
        result["extra"]["password"] = "*" * 8
        return result


class PostgresConfig(MetaConfig):
    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        return result


class RetrievalConfig(BaseConfig):
    retrieval_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["retrieval_type"] = self.retrieval_type
        result["extra"] = extra_dict
        return result


class GaussDBRetrievalConfig(RetrievalConfig):
    database: str
    retrieval_schema: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        extra_dict = result["extra"].copy()
        extra_dict.update({"database": self.database, "schema": self.retrieval_schema})
        result["extra"] = extra_dict
        return result


class InfinityConfig(RetrievalConfig):
    db_name: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["db_name"] = self.db_name
        result["extra"] = extra_dict
        return result


class ElasticsearchConfig(RetrievalConfig):
    username: str
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["username"] = self.username
        extra_dict["password"] = self.password
        result["extra"] = extra_dict
        return result


class MessageQueueConfig(BaseConfig):
    mq_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["mq_type"] = self.mq_type
        result["extra"] = extra_dict
        return result


class RedisConfig(MessageQueueConfig):
    database: int
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["database"] = self.database
        extra_dict["password"] = self.password
        result["extra"] = extra_dict
        return result


class RabbitMQConfig(MessageQueueConfig):
    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        return result


class RAGFlowServerConfig(BaseConfig):
    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        return result


class TaskExecutorConfig(BaseConfig):
    message_queue_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        result["extra"]["message_queue_type"] = self.message_queue_type
        return result


class FileStoreConfig(BaseConfig):
    store_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["store_type"] = self.store_type
        result["extra"] = extra_dict
        return result


class MinioConfig(FileStoreConfig):
    user: str
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if "extra" not in result:
            result["extra"] = dict()
        extra_dict = result["extra"].copy()
        extra_dict["user"] = self.user
        extra_dict["password"] = self.password
        result["extra"] = extra_dict
        return result


def load_configurations(config_path: str) -> list[BaseConfig]:
    raw_configs = read_config(config_path)
    configurations = []
    ragflow_count = 0
    id_count = 0
    metadata_db_type = normalize_database_type(os.getenv("DB_TYPE", "mysql"))
    for k, v in raw_configs.items():
        match k:
            case "ragflow":
                name: str = f"ragflow_{ragflow_count}"
                host: str = v["host"]
                http_port: int = v["http_port"]
                config = RAGFlowServerConfig(id=id_count, name=name, host=host, port=http_port, service_type="ragflow_server", detail_func_name="check_ragflow_server_alive")
                configurations.append(config)
                id_count += 1
            case "es":
                name: str = "elasticsearch"
                url = v["hosts"]
                parsed = urlparse(url)
                host: str = parsed.hostname
                port: int = parsed.port
                username: str = v.get("username")
                password: str = v.get("password")
                config = ElasticsearchConfig(
                    id=id_count,
                    name=name,
                    host=host,
                    port=port,
                    service_type="retrieval",
                    retrieval_type="elasticsearch",
                    username=username,
                    password=password,
                    detail_func_name="get_es_cluster_stats",
                )
                configurations.append(config)
                id_count += 1

            case "infinity":
                name: str = "infinity"
                url = v["uri"]
                parts = url.split(":", 1)
                host = parts[0]
                port = int(parts[1])
                database: str = v.get("db_name", "default_db")
                config = InfinityConfig(id=id_count, name=name, host=host, port=port, service_type="retrieval", retrieval_type="infinity", db_name=database, detail_func_name="get_infinity_status")
                configurations.append(config)
                id_count += 1
            case "minio_0":
                name: str = "minio_0"
                url = v["host"]
                parts = url.split(":", 1)
                host = parts[0]
                port = int(parts[1])
                user = v.get("user")
                password = v.get("password")
                config = MinioConfig(id=id_count, name=name, host=host, port=port, user=user, password=password, service_type="file_store", store_type="minio", detail_func_name="check_minio_alive")
                configurations.append(config)
                id_count += 1
            case "minio":
                name: str = "minio"
                url = v["host"]
                parts = url.split(":", 1)
                host = parts[0]
                port = int(parts[1])
                user = v.get("user")
                password = v.get("password")
                config = MinioConfig(id=id_count, name=name, host=host, port=port, user=user, password=password, service_type="file_store", store_type="minio", detail_func_name="check_minio_alive")
                configurations.append(config)
                id_count += 1
            case "s3":
                # AWS S3 (or any S3-compatible service: MinIO, R2, ...).
                # The config block uses `endpoint_url` instead of `host:port`,
                # so parse the URL to derive host/port for the status page.
                name: str = "s3"
                endpoint_url = v.get("endpoint_url") or ""
                if endpoint_url:
                    try:
                        parsed = urlparse(endpoint_url)
                        # `parsed.port` raises ValueError on non-numeric or
                        # out-of-range ports; `urlparse` itself raises
                        # ValueError on malformed IPv6 URLs. Fall back to
                        # the raw endpoint as host and the scheme's
                        # default port so config loading completes.
                        host: str = parsed.hostname or endpoint_url
                        port: int = parsed.port or (443 if parsed.scheme == "https" else 80)
                        logging.debug(
                            "Selected S3 host=%s port=%d for endpoint %r.",
                            host,
                            port,
                            endpoint_url,
                        )
                    except ValueError:
                        logging.warning(
                            "Could not parse S3 endpoint_url %r; using raw value as host with default port.",
                            endpoint_url,
                        )
                        host = endpoint_url
                        port = 443 if endpoint_url.startswith("https://") else 80
                else:
                    host: str = "s3.amazonaws.com"
                    port: int = 443
                    logging.debug("No S3 endpoint_url configured; defaulting to AWS S3 at %s:%d.", host, port)
                config = FileStoreConfig(
                    id=id_count,
                    name=name,
                    host=host,
                    port=port,
                    service_type="file_store",
                    store_type="s3",
                    detail_func_name="check_s3_alive",
                )
                configurations.append(config)
                id_count += 1
            case "redis":
                name: str = "redis"
                url = v["host"]
                parts = url.split(":", 1)
                host = parts[0]
                port = int(parts[1])
                password = v.get("password")
                db: int = v.get("db")
                config = RedisConfig(id=id_count, name=name, host=host, port=port, password=password, database=db, service_type="message_queue", mq_type="redis", detail_func_name="get_redis_info")
                configurations.append(config)
                id_count += 1
            case "mysql":
                if metadata_db_type == "gaussdb":
                    continue
                name: str = "mysql"
                host: str = v.get("host")
                port: int = v.get("port")
                username = v.get("user")
                password = v.get("password")
                config = MySQLConfig(
                    id=id_count,
                    name=name,
                    host=host,
                    port=port,
                    username=username,
                    password=password,
                    service_type="meta_data",
                    meta_type="mysql",
                    detail_func_name="get_mysql_status",
                )
                configurations.append(config)
                id_count += 1
            case "gaussdb":
                # The gaussdb block in the configuration file belongs to
                # DocEngine and Memory Store, not the DB_TYPE=gaussdb metadata
                # connection. Add metadata GaussDB separately from
                # GAUSSDB_METADATA_* after the loop so Admin does not merge the
                # two services.
                doc_config = v
                host = doc_config.get("host")
                port = doc_config.get("port")
                if not host or port in (None, ""):
                    # service_conf.yaml.template always contains a GaussDB
                    # placeholder block. Default deployments leave it empty;
                    # do not let an inactive DocEngine config break Admin.
                    continue
                try:
                    port = int(port)
                except (TypeError, ValueError):
                    logging.warning("Ignoring invalid GaussDB DocEngine port in Admin configuration")
                    continue
                config = GaussDBRetrievalConfig(
                    id=id_count,
                    name="gaussdb",
                    host=host,
                    port=port,
                    database=doc_config.get("database"),
                    retrieval_schema=doc_config.get("schema") or "public",
                    service_type="retrieval",
                    retrieval_type="gaussdb",
                    detail_func_name="get_gaussdb_status",
                )
                configurations.append(config)
                id_count += 1
            case "admin":
                pass
            case "task_executor":
                name: str = "task_executor"
                host: str = v.get("host", "")
                port: int = v.get("port", 0)
                message_queue_type: str = v.get("message_queue_type")
                config = TaskExecutorConfig(
                    id=id_count, name=name, host=host, port=port, message_queue_type=message_queue_type, service_type="task_executor", detail_func_name="check_task_executor_alive"
                )
                configurations.append(config)
                id_count += 1
            case "rabbitmq":
                name: str = "rabbitmq"
                host: str = v.get("host")
                port: int = v.get("port")
                config = RabbitMQConfig(id=id_count, name=name, host=host, port=port, service_type="message_queue", mq_type="rabbitmq", detail_func_name="check_rabbitmq_alive")
                configurations.append(config)
                id_count += 1
            case _:
                logging.warning(f"Unknown configuration key: {k}")
                continue

    if metadata_db_type == "gaussdb":
        # Display the metadata database from the same GAUSSDB_METADATA_*
        # variables as the main service, never from the DocEngine/Memory Store
        # GAUSSDB_* variables.
        v = _gaussdb_env_config()
        config = GaussDBMetadataConfig(
            id=id_count,
            name="gaussdb",
            host=v.get("host"),
            port=v.get("port"),
            username=v.get("user"),
            password=v.get("password"),
            metadata_schema=v.get("schema"),
            service_type="meta_data",
            meta_type="gaussdb",
            detail_func_name="get_database_status",
        )
        configurations.append(config)

    return configurations
