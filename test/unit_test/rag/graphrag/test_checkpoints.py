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

import json
from unittest.mock import MagicMock

import pytest

from rag.graphrag import checkpoints


def test_checkpoint_keys_are_stable():
    first = checkpoints.community_checkpoint_key("1", "2", ["B", "A"])
    second = checkpoints.community_checkpoint_key("1", "2", ["A", "B"])
    assert first == second

    pairs = [("alpha", "alfa"), ("beta", "bata")]
    assert checkpoints.resolution_checkpoint_key("entity", pairs) == checkpoints.resolution_checkpoint_key("entity", pairs)


@pytest.mark.asyncio
async def test_load_checkpoints_paginates(monkeypatch):
    pages = {
        0: {
            "row-1": {"content_with_weight": json.dumps({"key": "k1", "payload": {"value": 1}})},
            "row-2": {"content_with_weight": json.dumps({"key": "k2", "payload": {"value": 2}})},
        },
        2: {
            "row-3": {"content_with_weight": json.dumps({"key": "k3", "payload": {"value": 3}})},
        },
    }
    offsets = []

    def search_mock(_fields, _filters, _condition, _order, _orderby, offset, _limit, *_args):
        offsets.append(offset)
        return offset

    store = MagicMock()
    store.search.side_effect = search_mock
    store.get_fields.side_effect = lambda offset, _fields: pages[offset]
    monkeypatch.setattr(checkpoints.settings, "docStoreConn", store)

    loaded = await checkpoints.load_checkpoints("tenant-1", "kb-1", checkpoints.COMMUNITY_CHECKPOINT, page_size=2)

    assert offsets == [0, 2]
    assert loaded == {"k1": {"value": 1}, "k2": {"value": 2}, "k3": {"value": 3}}


@pytest.mark.asyncio
async def test_save_checkpoint_degrades_on_insert_failure(monkeypatch):
    store = MagicMock()
    store.insert.return_value = ["insert failed"]
    monkeypatch.setattr(checkpoints.settings, "docStoreConn", store)

    saved = await checkpoints.save_checkpoint("tenant-1", "kb-1", checkpoints.RESOLUTION_CHECKPOINT, "key-1", {"ok": True})

    assert saved is False
    store.insert.assert_called_once()


@pytest.mark.asyncio
async def test_cleanup_checkpoints_deletes_stage_rows(monkeypatch):
    store = MagicMock()
    monkeypatch.setattr(checkpoints.settings, "docStoreConn", store)

    cleaned = await checkpoints.cleanup_checkpoints("tenant-1", "kb-1", checkpoints.RESOLUTION_CHECKPOINT)

    assert cleaned is True
    store.delete.assert_called_once()
    condition = store.delete.call_args.args[0]
    assert condition["knowledge_graph_kwd"] == [checkpoints.RESOLUTION_CHECKPOINT]
    assert condition["kb_id"] == "kb-1"
