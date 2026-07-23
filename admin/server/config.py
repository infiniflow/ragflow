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
import threading
from enum import Enum
from urllib.parse import urlparse

from pydantic import BaseModel
from typing import Any
from common.config_utils import read_config


STORAGE_IMPL_STORE_TYPES = {
    "MINIO": "minio",
    "AWS_S3": "s3",
    "OSS": "oss",
    "AZURE_SPN": "azure",
    "AZURE_SAS": "azure",
    "GCS": "gcs",
    "OPENDAL": "opendal",
}


class BaseConfig(BaseModel):
    id: int
    name: str
    host: str
    port: int
    service_type: str
    detail_func_name: str

    def to_dict(self) -> dict[str, Any]:
        return {'id': self.id, 'name': self.name, 'host': self.host, 'port': self.port,
                'service_type': self.service_type}


class ServiceConfigs:
    configs = list[BaseConfig]

    def __init__(self):
        self.configs = []
        self.lock = threading.Lock()


SERVICE_CONFIGS = ServiceConfigs


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
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['meta_type'] = self.meta_type
        result['extra'] = extra_dict
        return result


class MySQLConfig(MetaConfig):
    username: str
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['username'] = self.username
        extra_dict['password'] = self.password
        result['extra'] = extra_dict
        return result


class PostgresConfig(MetaConfig):

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        return result


class RetrievalConfig(BaseConfig):
    retrieval_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['retrieval_type'] = self.retrieval_type
        result['extra'] = extra_dict
        return result


class InfinityConfig(RetrievalConfig):
    db_name: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['db_name'] = self.db_name
        result['extra'] = extra_dict
        return result


class ElasticsearchConfig(RetrievalConfig):
    username: str
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['username'] = self.username
        extra_dict['password'] = self.password
        result['extra'] = extra_dict
        return result


class MessageQueueConfig(BaseConfig):
    mq_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['mq_type'] = self.mq_type
        result['extra'] = extra_dict
        return result


class RedisConfig(MessageQueueConfig):
    database: int
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['database'] = self.database
        extra_dict['password'] = self.password
        result['extra'] = extra_dict
        return result


class RabbitMQConfig(MessageQueueConfig):

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        return result


class RAGFlowServerConfig(BaseConfig):

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        return result


class TaskExecutorConfig(BaseConfig):
    message_queue_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        result['extra']['message_queue_type'] = self.message_queue_type
        return result


class FileStoreConfig(BaseConfig):
    store_type: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['store_type'] = self.store_type
        result['extra'] = extra_dict
        return result


class MinioConfig(FileStoreConfig):
    user: str
    password: str

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        if 'extra' not in result:
            result['extra'] = dict()
        extra_dict = result['extra'].copy()
        extra_dict['user'] = self.user
        extra_dict['password'] = self.password
        result['extra'] = extra_dict
        return result


class S3Config(FileStoreConfig):
    endpoint_url: str
    scheme: str
    region: str | None = None
    bucket: str | None = None

    def to_dict(self) -> dict[str, Any]:
        result = super().to_dict()
        extra_dict = result['extra'].copy()
        extra_dict['endpoint_url'] = self.endpoint_url
        extra_dict['scheme'] = self.scheme
        if self.region:
            extra_dict['region'] = self.region
        if self.bucket:
            extra_dict['bucket'] = self.bucket
        result['extra'] = extra_dict
        return result


def _get_s3_endpoint(config: dict[str, Any]) -> tuple[str, str, int, str]:
    region = config.get("region_name") or config.get("region")
    endpoint_url = config.get("endpoint_url")
    if not endpoint_url:
        domain = "amazonaws.com.cn" if region and region.startswith("cn-") else "amazonaws.com"
        endpoint_url = f"https://s3.{region}.{domain}" if region else f"https://s3.{domain}"
    elif "://" not in endpoint_url:
        endpoint_url = f"https://{endpoint_url}"

    parsed = urlparse(endpoint_url)
    scheme = parsed.scheme or "https"
    host = parsed.hostname or ""
    port = parsed.port or (443 if scheme == "https" else 80)
    return endpoint_url, host, port, scheme


def is_service_active(service_config: BaseConfig, doc_engine: str | None = None, storage_impl: str | None = None) -> bool:
    if service_config.service_type == "retrieval":
        active_doc_engine = doc_engine or os.getenv("DOC_ENGINE", "elasticsearch")
        return service_config.retrieval_type == active_doc_engine

    if service_config.service_type == "file_store":
        active_storage_impl = (storage_impl or os.getenv("STORAGE_IMPL", "MINIO")).upper()
        active_store_type = STORAGE_IMPL_STORE_TYPES.get(active_storage_impl)
        if active_store_type is None:
            logging.warning(f"Unknown storage implementation: {active_storage_impl}")
            return False
        return service_config.store_type == active_store_type

    return True


def load_configurations(config_path: str) -> list[BaseConfig]:
    raw_configs = read_config(config_path)
    configurations = []
    ragflow_count = 0
    id_count = 0
    for k, v in raw_configs.items():
        match k:
            case "ragflow":
                name: str = f'ragflow_{ragflow_count}'
                host: str = v['host']
                http_port: int = v['http_port']
                config = RAGFlowServerConfig(id=id_count, name=name, host=host, port=http_port,
                                             service_type="ragflow_server",
                                             detail_func_name="check_ragflow_server_alive")
                configurations.append(config)
                id_count += 1
            case "es":
                name: str = 'elasticsearch'
                url = v['hosts']
                parsed = urlparse(url)
                host: str = parsed.hostname
                port: int = parsed.port
                username: str = v.get('username')
                password: str = v.get('password')
                config = ElasticsearchConfig(id=id_count, name=name, host=host, port=port, service_type="retrieval",
                                             retrieval_type="elasticsearch",
                                             username=username, password=password,
                                             detail_func_name="get_es_cluster_stats")
                configurations.append(config)
                id_count += 1

            case "infinity":
                name: str = 'infinity'
                url = v['uri']
                parts = url.split(':', 1)
                host = parts[0]
                port = int(parts[1])
                database: str = v.get('db_name', 'default_db')
                config = InfinityConfig(id=id_count, name=name, host=host, port=port, service_type="retrieval",
                                        retrieval_type="infinity",
                                        db_name=database, detail_func_name="get_infinity_status")
                configurations.append(config)
                id_count += 1
            case "minio_0":
                name: str = 'minio_0'
                url = v['host']
                parts = url.split(':', 1)
                host = parts[0]
                port = int(parts[1])
                user = v.get('user')
                password = v.get('password')
                config = MinioConfig(id=id_count, name=name, host=host, port=port, user=user, password=password,
                                     service_type="file_store",
                                     store_type="minio", detail_func_name="check_minio_alive")
                configurations.append(config)
                id_count += 1
            case "minio":
                name: str = 'minio'
                url = v['host']
                parts = url.split(':', 1)
                host = parts[0]
                port = int(parts[1])
                user = v.get('user')
                password = v.get('password')
                config = MinioConfig(id=id_count, name=name, host=host, port=port, user=user, password=password,
                                     service_type="file_store",
                                     store_type="minio", detail_func_name="check_minio_alive")
                configurations.append(config)
                id_count += 1
            case "s3":
                endpoint_url, host, port, scheme = _get_s3_endpoint(v)
                region = v.get("region_name") or v.get("region")
                config = S3Config(
                    id=id_count,
                    name="s3",
                    host=host,
                    port=port,
                    service_type="file_store",
                    detail_func_name="check_storage_alive",
                    store_type="s3",
                    endpoint_url=endpoint_url,
                    scheme=scheme,
                    region=region,
                    bucket=v.get("bucket"),
                )
                configurations.append(config)
                id_count += 1
            case "redis":
                name: str = 'redis'
                url = v['host']
                parts = url.split(':', 1)
                host = parts[0]
                port = int(parts[1])
                password = v.get('password')
                db: int = v.get('db')
                config = RedisConfig(id=id_count, name=name, host=host, port=port, password=password, database=db,
                                     service_type="message_queue", mq_type="redis", detail_func_name="get_redis_info")
                configurations.append(config)
                id_count += 1
            case "mysql":
                name: str = 'mysql'
                host: str = v.get('host')
                port: int = v.get('port')
                username = v.get('user')
                password = v.get('password')
                config = MySQLConfig(id=id_count, name=name, host=host, port=port, username=username, password=password,
                                     service_type="meta_data", meta_type="mysql", detail_func_name="get_mysql_status")
                configurations.append(config)
                id_count += 1
            case "admin":
                pass
            case "task_executor":
                name: str = 'task_executor'
                host: str = v.get('host', '')
                port: int = v.get('port', 0)
                message_queue_type: str = v.get('message_queue_type')
                config = TaskExecutorConfig(id=id_count, name=name, host=host, port=port, message_queue_type=message_queue_type,
                                            service_type="task_executor", detail_func_name="check_task_executor_alive")
                configurations.append(config)
                id_count += 1
            case "rabbitmq":
                name: str = 'rabbitmq'
                host: str = v.get('host')
                port: int = v.get('port')
                config = RabbitMQConfig(id=id_count, name=name, host=host, port=port,
                                        service_type="message_queue", mq_type="rabbitmq", detail_func_name="check_rabbitmq_alive")
                configurations.append(config)
                id_count += 1
            case _:
                logging.warning(f"Unknown configuration key: {k}")
                continue

    return configurations
