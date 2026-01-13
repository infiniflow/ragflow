#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from typing import Optional, Type
from typing import TypeVar, Generic

from common.doc_store.doc_store_base import DocStoreConnection
from core.providers.base import ProviderBase
from core.types.doc_engine import DocumentEngineType
from memory.utils.es_conn import ESConnection as MemESConnection
from memory.utils.infinity_conn import InfinityConnection as MemInfinityConnection
from rag.utils.es_conn import ESConnection as DocESConnection
from rag.utils.infinity_conn import InfinityConnection as DocInfinityConnection
from rag.utils.opensearch_conn import OSConnection as DocOpenSearchConnection

TConn = TypeVar("TConn", bound="DocStoreConnection")


class BaseDocEngineProvider(ProviderBase, Generic[TConn]):
    _ENGINE_MAPPING: dict[DocumentEngineType, Type[TConn]]

    def __init__(self, config=None):
        super().__init__(config)
        self._conn: Optional[TConn] = None

    @property
    def conn(self) -> TConn:
        if self._conn is not None:
            return self._conn

        engine = self._config.doc_engine.active
        try:
            conn_cls = self._ENGINE_MAPPING[engine]
        except KeyError:
            raise RuntimeError(
                f"Unsupported engine {engine} for provider {self.__class__.__name__}"
            )

        self._conn = conn_cls()
        return self._conn


class DocStoreProvider(BaseDocEngineProvider["DocStoreConnection"]):
    _ENGINE_MAPPING = {
        DocumentEngineType.ELASTICSEARCH: DocESConnection,
        DocumentEngineType.OPENSEARCH: DocOpenSearchConnection,
        DocumentEngineType.INFINITY: DocInfinityConnection,
    }


class MemoryStoreProvider(BaseDocEngineProvider["DocStoreConnection"]):
    _ENGINE_MAPPING = {
        DocumentEngineType.ELASTICSEARCH: MemESConnection,
        DocumentEngineType.INFINITY: MemInfinityConnection,
    }
