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

"""Ordering of the `extract` list returned by MessageService.list_message."""

import pytest

from common import settings
from memory.services import messages as messages_module
from memory.services.messages import MessageService, _message_id_sort_key

pytestmark = pytest.mark.p1

RAW_ID = 100
TIED_AT = "2026-10-20 10:00:00"


class FakeStore:
    """Return the raw batch first and the extract batch second."""

    def __init__(self, raw_messages, extract_messages):
        self._results = [raw_messages, extract_messages]

    def search(self, **_kwargs):
        return self._results.pop(0), 1

    def get_fields(self, result, _select_fields):
        return result


def make_message(message_id, source_id=RAW_ID, valid_at=TIED_AT):
    return {"message_id": message_id, "source_id": source_id, "valid_at": valid_at}


def as_batch(messages):
    return {str(i): m for i, m in enumerate(messages)}


def list_extract(raw_messages, extract_messages, monkeypatch):
    store = FakeStore(as_batch(raw_messages), as_batch(extract_messages))
    monkeypatch.setattr(settings, "msgStoreConn", store)
    result = MessageService.list_message("uid", "memory-1")
    return result["message_list"][0]["extract"]


def test_tied_valid_at_falls_back_to_message_id(monkeypatch):
    """String ids past 9 must not compare lexicographically."""
    extract = list_extract([make_message(RAW_ID)], [make_message("11"), make_message("9"), make_message("10")], monkeypatch)

    assert [m["message_id"] for m in extract] == ["9", "10", "11"]


def test_valid_at_stays_the_primary_sort_key(monkeypatch):
    extract = list_extract(
        [make_message(RAW_ID)],
        [
            make_message(1, valid_at="2026-10-19 10:00:00"),
            make_message(2, valid_at="2026-10-21 10:00:00"),
            make_message(3, valid_at="2026-10-20 10:00:00"),
        ],
        monkeypatch,
    )

    assert [m["message_id"] for m in extract] == [2, 3, 1]


def test_extract_of_each_raw_message_is_sorted_independently(monkeypatch):
    raw = [make_message(RAW_ID, valid_at="2026-10-21 10:00:00"), make_message(200, valid_at="2026-10-19 10:00:00")]
    extract = [
        make_message(4, source_id=200),
        make_message(2, source_id=200),
        make_message(3, source_id=RAW_ID),
        make_message(1, source_id=RAW_ID),
    ]
    store = FakeStore(as_batch(raw), as_batch(extract))
    monkeypatch.setattr(settings, "msgStoreConn", store)

    message_list = MessageService.list_message("uid", "memory-1")["message_list"]

    assert [m["message_id"] for m in message_list[0]["extract"]] == [1, 3]
    assert [m["message_id"] for m in message_list[1]["extract"]] == [2, 4]


def test_message_id_sort_key_handles_int_str_and_missing_values():
    assert _message_id_sort_key({"message_id": 2}) == 2
    assert _message_id_sort_key({"message_id": "10"}) == 10
    assert _message_id_sort_key({}) == -1
    assert _message_id_sort_key({"message_id": None}) == -1


def test_valid_at_sort_key_accepts_datetime_objects():
    from datetime import datetime

    key = messages_module._valid_at_sort_key({"valid_at": datetime(2026, 10, 20, 10, 0, 0)})

    assert key == "2026-10-20 10:00:00"
