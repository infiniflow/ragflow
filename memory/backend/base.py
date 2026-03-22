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

from abc import ABC, abstractmethod
from typing import Any


class MemoryBackend(ABC):
    @property
    @abstractmethod
    def name(self) -> str:
        """Backend name used in capability reporting."""

    @property
    @abstractmethod
    def mode(self) -> str:
        """Backend mode, e.g. default/powermem."""

    # --- Connector-compatible operations used by MessageService ---
    @abstractmethod
    def index_exist(self, index_name: str, memory_id: str | None = None) -> bool:
        pass

    @abstractmethod
    def create_idx(self, index_name: str, memory_id: str, vector_size: int) -> bool:
        pass

    @abstractmethod
    def delete_idx(self, index_name: str, memory_id: str | None = None) -> bool:
        pass

    @abstractmethod
    def search(self, **kwargs):
        pass

    @abstractmethod
    def insert(self, documents: list[dict], index_name: str, memory_id: str | None = None) -> list[str]:
        pass

    @abstractmethod
    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        pass

    @abstractmethod
    def delete(self, condition: dict, index_name: str, memory_id: str) -> int:
        pass

    @abstractmethod
    def get(self, doc_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        pass

    @abstractmethod
    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        pass

    @abstractmethod
    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int = 512):
        pass

    @abstractmethod
    def get_missing_field_message(self, select_fields: list[str], index_name: str, memory_id: str, field_name: str, limit: int = 512):
        pass

    # --- Optional enhanced capabilities ---
    def supports_graph(self) -> bool:
        return False

    def supports_user_profile(self) -> bool:
        return False

    def supports_ebbinghaus(self) -> bool:
        return False

    def supports_rerank(self) -> bool:
        return False

    def supports_sparse_vector(self) -> bool:
        return False

    def supports_intelligent_merge(self) -> bool:
        return False

    def graph_search(self, _memory_ids: list[str], _query: str, _top_n: int = 5, **_kwargs) -> list[dict]:
        return []

    def capabilities(self) -> dict[str, Any]:
        return {
            "backend": self.name,
            "mode": self.mode,
            "supports_graph": self.supports_graph(),
            "supports_user_profile": self.supports_user_profile(),
            "supports_ebbinghaus": self.supports_ebbinghaus(),
            "supports_rerank": self.supports_rerank(),
            "supports_sparse_vector": self.supports_sparse_vector(),
            "supports_intelligent_merge": self.supports_intelligent_merge(),
        }
