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

from memory.backend.base import MemoryBackend


class DefaultMemoryBackend(MemoryBackend):
    def __init__(self, connector, doc_engine: str):
        self._connector = connector
        self._doc_engine = (doc_engine or "unknown").lower()

    @property
    def name(self) -> str:
        return "default"

    @property
    def mode(self) -> str:
        return self._doc_engine

    def index_exist(self, index_name: str, memory_id: str | None = None) -> bool:
        return self._connector.index_exist(index_name, memory_id)

    def create_idx(self, index_name: str, memory_id: str, vector_size: int) -> bool:
        return self._connector.create_idx(index_name, memory_id, vector_size)

    def delete_idx(self, index_name: str, memory_id: str | None = None) -> bool:
        return self._connector.delete_idx(index_name, memory_id)

    def search(self, **kwargs):
        return self._connector.search(**kwargs)

    def insert(self, documents: list[dict], index_name: str, memory_id: str | None = None) -> list[str]:
        return self._connector.insert(documents, index_name, memory_id)

    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        return self._connector.update(condition, new_value, index_name, memory_id)

    def delete(self, condition: dict, index_name: str, memory_id: str) -> int:
        return self._connector.delete(condition, index_name, memory_id)

    def get(self, doc_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        return self._connector.get(doc_id, index_name, memory_ids)

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        return self._connector.get_fields(res, fields)

    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int = 512):
        return self._connector.get_forgotten_messages(select_fields, index_name, memory_id, limit)

    def get_missing_field_message(self, select_fields: list[str], index_name: str, memory_id: str, field_name: str, limit: int = 512):
        return self._connector.get_missing_field_message(select_fields, index_name, memory_id, field_name, limit)
