#
# Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

"""Unit tests for rag.utils.storage_base."""

from rag.utils.storage_base import StorageBase


class InMemoryStorage(StorageBase):
    """Simple in-memory StorageBase implementation for testing."""

    def __init__(self):
        self._data: dict[tuple[str, str], bytes] = {}
        self._removed: list[tuple[str, str]] = []

    def put(self, bucket: str, fnm: str, binary: bytes, tenant_id: str | None = None) -> bool:
        self._data[(bucket, fnm)] = binary
        return True

    def get(self, bucket: str, fnm: str, tenant_id: str | None = None) -> bytes | None:
        return self._data.get((bucket, fnm))

    def rm(self, bucket: str, fnm: str, tenant_id: str | None = None) -> bool:
        self._removed.append((bucket, fnm))
        self._data.pop((bucket, fnm), None)
        return True

    def obj_exist(self, bucket: str, fnm: str, tenant_id: str | None = None) -> bool:
        return (bucket, fnm) in self._data

    def health(self) -> str:
        return "ok"


class TestStorageBaseImplementation:
    def test_put_get_and_exist(self):
        storage = InMemoryStorage()

        assert storage.health() == "ok"

        assert storage.put("b", "f", b"data") is True
        assert storage.obj_exist("b", "f") is True
        assert storage.get("b", "f") == b"data"

    def test_rm_marks_removed_and_deletes(self):
        storage = InMemoryStorage()
        storage.put("b", "f", b"data")

        assert storage.rm("b", "f") is True
        assert storage.obj_exist("b", "f") is False

