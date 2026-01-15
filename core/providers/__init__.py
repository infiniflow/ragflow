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

from dataclasses import dataclass
from functools import lru_cache

from core.config import app_config, AppConfig
from core.providers.base import ProviderBase
from core.providers.cache import CacheProvider
from core.providers.database import DatabaseProvider
from core.providers.doc_engine import DocStoreProvider, MemoryStoreProvider
from core.providers.retriever import RetrieverProvider, KGRetrieverProvider
from core.providers.storage import StorageProvider

__all__ = [
    "get_providers",
    "providers",
]


@dataclass(frozen=True)
class Providers:
    storage: StorageProvider
    database: DatabaseProvider
    cache: CacheProvider
    doc_store: DocStoreProvider
    msg_store: MemoryStoreProvider
    retriever: RetrieverProvider
    kg_retriever: KGRetrieverProvider


@lru_cache(maxsize=1)
def get_providers(config: AppConfig = None) -> Providers:
    """
    Return a singleton Providers instance.

    Args:
        config: Optional AppConfig. Defaults to global app_config.

    Returns:
        Providers: Initialized providers (storage, database, doc_engine, retriever)
    """
    if config is None:
        config = app_config

    doc_store = DocStoreProvider(config)
    retriever = RetrieverProvider(config=config, doc_store_conn=doc_store.conn)
    kg_retriever = KGRetrieverProvider(config=config, doc_store_conn=doc_store.conn)

    return Providers(
        storage=StorageProvider(config),
        database=DatabaseProvider(config),
        cache=CacheProvider(config),
        doc_store=DocStoreProvider(config),
        msg_store=MemoryStoreProvider(config),
        retriever=retriever,
        kg_retriever=kg_retriever,
    )


providers = get_providers()
