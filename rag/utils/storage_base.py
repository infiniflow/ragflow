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

"""Lightweight base type for storage backends used by EncryptedStorageWrapper.

This is implemented as a Protocol so existing storage implementations
do not need to inherit from it explicitly; it mainly serves as
documentation and for static type checkers, while runtime behaviour
remains duck-typed.
"""

from __future__ import annotations

from typing import Any, Protocol, runtime_checkable


@runtime_checkable
class StorageBase(Protocol):
    """Minimal interface expected by EncryptedStorageWrapper."""

    def put(self, bucket: str, fnm: str, binary: bytes, tenant_id: str | None = None) -> Any:  # pragma: no cover - interface
        ...

    def get(self, bucket: str, fnm: str, tenant_id: str | None = None) -> bytes | None:  # pragma: no cover - interface
        ...

    def rm(self, bucket: str, fnm: str, tenant_id: str | None = None) -> Any:  # pragma: no cover - interface
        ...

    def obj_exist(self, bucket: str, fnm: str, tenant_id: str | None = None) -> bool:  # pragma: no cover - interface
        ...

    def health(self) -> Any:  # pragma: no cover - interface
        ...

