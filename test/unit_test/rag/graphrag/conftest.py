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

"""
Mock heavy dependencies that graphrag/utils.py transitively imports,
so unit tests can run without infrastructure services (Redis, Elasticsearch, etc.).
"""

import sys
from unittest.mock import MagicMock

_modules_to_mock = [
    "quart",
    "common.connection_utils",
    "common.settings",
    "common.doc_store",
    "common.doc_store.doc_store_base",
    "api.db.services",
    "api.db.services.task_service",
    "rag.graphrag.general.leiden",
    "rag.llm.chat_model",
    "rag.nlp",
    "rag.nlp.search",
    "rag.nlp.rag_tokenizer",
    "rag.utils.redis_conn",
]

for mod_name in _modules_to_mock:
    if mod_name not in sys.modules:
        sys.modules[mod_name] = MagicMock()

# Ensure `from common.connection_utils import timeout` returns a no-op decorator
sys.modules["common.connection_utils"].timeout = lambda *a, **kw: (lambda fn: fn)
sys.modules["api.db.services.task_service"].has_canceled = lambda *_a, **_kw: False
sys.modules["rag.graphrag.general.leiden"].run = lambda *_a, **_kw: {}
sys.modules["rag.graphrag.general.leiden"].add_community_info2graph = lambda *_a, **_kw: None
sys.modules["rag.llm.chat_model"].Base = object
