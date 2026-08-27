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

import base64
from io import BytesIO
from types import SimpleNamespace
from unittest.mock import Mock

import pytest
from PIL import Image

from api.utils import image_utils


def _png_bytes(width, height, color):
    image = Image.new("RGB", (width, height), color)
    buf = BytesIO()
    image.save(buf, format="PNG")
    return buf.getvalue()


class _FakeStorage:
    def __init__(self, existing=None):
        self.objects = dict(existing or {})
        self.removed = []

    def obj_exist(self, bucket, name):
        return (bucket, name) in self.objects

    def get(self, bucket, name):
        return self.objects[(bucket, name)]

    def put(self, bucket, name, data):
        self.objects[(bucket, name)] = data

    def rm(self, bucket, name):
        self.removed.append((bucket, name))
        del self.objects[(bucket, name)]


@pytest.fixture
def fake_settings(monkeypatch):
    storage = _FakeStorage()
    monkeypatch.setattr(image_utils, "settings", SimpleNamespace(STORAGE_IMPL=storage))
    return storage


def test_store_chunk_image_replace_overwrites_existing(fake_settings):
    fake_settings.put("kb-1", "chunk-1", _png_bytes(2, 2, "red"))
    replacement = _png_bytes(3, 4, "blue")

    image_utils.store_chunk_image("kb-1", "chunk-1", replacement, mode="replace")

    assert fake_settings.objects[("kb-1", "chunk-1")] == replacement


def test_store_chunk_image_append_merges_existing(fake_settings):
    fake_settings.put("kb-1", "chunk-1", _png_bytes(2, 2, "red"))
    appended = _png_bytes(1, 1, "blue")

    image_utils.store_chunk_image("kb-1", "chunk-1", appended, mode="append")

    merged = Image.open(BytesIO(fake_settings.objects[("kb-1", "chunk-1")]))
    assert merged.size == (2, 3)


def test_store_chunk_image_append_creates_when_missing(fake_settings):
    new_image = _png_bytes(2, 2, "green")

    image_utils.store_chunk_image("kb-1", "chunk-1", new_image, mode="append")

    assert fake_settings.objects[("kb-1", "chunk-1")] == new_image


def test_remove_chunk_image_deletes_existing_object(fake_settings):
    fake_settings.put("kb-1", "chunk-1", b"image")

    image_utils.remove_chunk_image("kb-1", "chunk-1")

    assert fake_settings.removed == [("kb-1", "chunk-1")]
    assert ("kb-1", "chunk-1") not in fake_settings.objects


def test_remove_chunk_image_noop_when_missing(fake_settings):
    image_utils.remove_chunk_image("kb-1", "chunk-1")

    assert fake_settings.removed == []
