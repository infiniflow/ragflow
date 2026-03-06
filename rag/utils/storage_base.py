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

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any


class StorageBase(ABC):
    """Abstract base class for storage backends used by encrypted storage.

    Implementations should provide basic object CRUD operations and a simple
    health check. Additional optional methods like ``bucket_exists`` or
    ``get_presigned_url`` can be added by concrete backends as needed.
    """

    @abstractmethod
    def put(self, bucket: str, fnm: str, binary: bytes, tenant_id: str | None = None) -> Any:  # pragma: no cover - interface
        """Store an object."""
        raise NotImplementedError

    @abstractmethod
    def get(self, bucket: str, fnm: str, tenant_id: str | None = None) -> bytes | None:  # pragma: no cover - interface
        """Retrieve an object."""
        raise NotImplementedError

    @abstractmethod
    def rm(self, bucket: str, fnm: str, tenant_id: str | None = None) -> Any:  # pragma: no cover - interface
        """Remove an object."""
        raise NotImplementedError

    @abstractmethod
    def obj_exist(self, bucket: str, fnm: str, tenant_id: str | None = None) -> bool:  # pragma: no cover - interface
        """Check whether an object exists."""
        raise NotImplementedError

    @abstractmethod
    def health(self) -> Any:  # pragma: no cover - interface
        """Return backend health information or raise on failure."""
        raise NotImplementedError

