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

from __future__ import annotations

import logging

from memory.backend.default_backend import DefaultMemoryBackend
from memory.backend.powermem_backend import PowerMemBackend


def create_memory_backend(doc_engine: str, backend_mode: str, connector):
    normalized_doc_engine = (doc_engine or "").strip().lower()
    normalized_backend_mode = (backend_mode or "default").strip().lower()

    default_backend = DefaultMemoryBackend(connector=connector, doc_engine=normalized_doc_engine)
    if normalized_backend_mode != "powermem":
        return default_backend

    if normalized_doc_engine != "oceanbase":
        logging.warning(
            "MEMORY_BACKEND=powermem is ignored because DOC_ENGINE=%s is not oceanbase.",
            normalized_doc_engine,
        )
        return default_backend

    return PowerMemBackend(fallback_backend=default_backend)
