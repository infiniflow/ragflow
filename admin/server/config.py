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

import threading
from enum import Enum

from pydantic import BaseModel
from typing import Any

from core.config import app_config


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


def load_configurations() -> list[BaseConfig]:
    configurations = []
    ragflow_count = 0
    id_count = 0
    # RAGFlow Server
    if "ragflow" in app_config.model_fields_set:
        configurations.append(RAGFlowServerConfig(
            id=id_count,
            name=f'ragflow_{ragflow_count}',
            host=app_config.ragflow.host,
            port=app_config.ragflow.http_port,
            service_type="ragflow_server",
            detail_func_name="check_ragflow_server_alive"
        ))
        id_count += 1
        ragflow_count += 1

    # Elasticsearch
    if "elasticsearch" in app_config.doc_engine.model_fields_set:
        es_cfg = app_config.doc_engine.es
        configurations.append(ElasticsearchConfig(
            id=id_count,
            name="elasticsearch",
            host=es_cfg.hosts,
            port=es_cfg.port,
            service_type="retrieval",
            retrieval_type="elasticsearch",
            username=es_cfg.username,
            password=es_cfg.password,
            detail_func_name="get_es_cluster_stats"
        ))
        id_count += 1

    # Infinity
    if "infinity" in app_config.doc_engine.model_fields_set:
        inf_cfg = app_config.doc_engine.infinity
        configurations.append(InfinityConfig(
            id=id_count,
            name="infinity",
            host=inf_cfg.host,
            port=inf_cfg.port,
            service_type="retrieval",
            retrieval_type="infinity",
            db_name=inf_cfg.db_name or "default_db",
            detail_func_name="get_infinity_status"
        ))
        id_count += 1

    # Minio
    if "minio" in app_config.storage.model_fields_set:
        minio_cfg = app_config.storage.minio
        configurations.append(MinioConfig(
            id=id_count,
            name="minio",
            host=minio_cfg.host,
            port=minio_cfg.port,
            user=minio_cfg.user,
            password=minio_cfg.password,
            service_type="file_store",
            store_type="minio",
            detail_func_name="check_minio_alive"
        ))
        id_count += 1

    # Redis
    if "redis" in app_config.cache.model_fields_set:
        redis_cfg = app_config.cache.redis
        configurations.append(RedisConfig(
            id=id_count,
            name="redis",
            host=redis_cfg.host,
            port=redis_cfg.port,
            password=redis_cfg.password,
            database=redis_cfg.db,
            service_type="message_queue",
            mq_type="redis",
            detail_func_name="get_redis_info"
        ))
        id_count += 1

    # MySQL
    if "mysql" in app_config.model_fields_set:
        mysql_cfg = app_config.mysql
        configurations.append(MySQLConfig(
            id=id_count,
            name="mysql",
            host=mysql_cfg.host,
            port=mysql_cfg.port,
            username=mysql_cfg.user,
            password=mysql_cfg.password,
            service_type="meta_data",
            meta_type="mysql",
            detail_func_name="get_mysql_status"
        ))
        id_count += 1

    # Task Executor
    if "task_executor" in app_config.model_fields_set:
        te_cfg = app_config.task_executor
        configurations.append(TaskExecutorConfig(
            id=id_count,
            name="task_executor",
            host=te_cfg.host,
            port=te_cfg.port,
            message_queue_type=te_cfg.message_queue_type,
            service_type="task_executor",
            detail_func_name="check_task_executor_alive"
        ))
        id_count += 1

    return configurations
