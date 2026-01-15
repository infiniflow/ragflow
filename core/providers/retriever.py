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

from typing import Optional

from core.providers.base import ProviderBase
from common.doc_store.doc_store_base import DocStoreConnection


class RetrieverProvider(ProviderBase):
    def __init__(self, *, config=None, doc_store_conn: Optional[DocStoreConnection] = None):
        super().__init__(config)
        self._retriever = None
        self._doc_store_conn = doc_store_conn

    @property
    def conn(self):
        from rag.nlp.search import Dealer
        if self._retriever is None:
            self._retriever = Dealer(self._doc_store_conn)
        return self._retriever


class KGRetrieverProvider(ProviderBase):
    def __init__(self, *, config=None, doc_store_conn: Optional[DocStoreConnection] = None):
        super().__init__(config)
        self._kg_retriever = None
        self._doc_store_conn = doc_store_conn

    @property
    def conn(self):
        from graphrag.search import KGSearch
        if self._kg_retriever is None:
            self._kg_retriever = KGSearch(self._doc_store_conn)
        return self._kg_retriever
