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
import warnings
from types import SimpleNamespace

import pytest

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)

from api.db.services import conversation_service, dialog_service, file_service, user_canvas_version


class _FakeOrderField:
    def asc(self):
        return ("create_time", "asc")

    def desc(self):
        return ("create_time", "desc")


class _FakeField:
    def __eq__(self, other):
        return ("eq", other)

    def in_(self, other):
        return ("in", other)


class _FakeQuery:
    """Mimic peewee's immutable query chaining.

    peewee query methods (``where``/``order_by``/``offset``/``limit``) return a
    *new* query object and never mutate in place. Modelling that faithfully is
    what lets these tests catch the "``order_by()`` result discarded" bug: the
    deep-pagination helpers previously called ``q.order_by(...)`` on its own
    line, throwing the ordered query away and paginating an unordered one.
    """

    def __init__(self, rows, ordered=False, offset=0, limit=None):
        self._rows = rows
        self._ordered = ordered
        self._offset = offset
        self._limit = limit

    def where(self, *args, **kwargs):
        return _FakeQuery(self._rows, self._ordered, self._offset, self._limit)

    def order_by(self, *args, **kwargs):
        return _FakeQuery(self._rows, True, self._offset, self._limit)

    def offset(self, offset):
        return _FakeQuery(self._rows, self._ordered, offset, self._limit)

    def limit(self, limit):
        return _FakeQuery(self._rows, self._ordered, self._offset, limit)

    def dicts(self):
        rows = sorted(self._rows, key=lambda r: r["create_time"]) if self._ordered else list(self._rows)
        if self._limit is not None:
            rows = rows[self._offset : self._offset + self._limit]
        return rows


# ``create_time`` is deliberately out of insertion order so that a missing
# ``ORDER BY create_time`` is observable in the returned sequence.
_ROWS = [
    {"id": "c", "create_time": 3},
    {"id": "a", "create_time": 1},
    {"id": "b", "create_time": 2},
]


def _fake_model():
    return SimpleNamespace(
        select=lambda *args, **kwargs: _FakeQuery(list(_ROWS)),
        id=_FakeField(),
        dialog_id=_FakeField(),
        tenant_id=_FakeField(),
        user_canvas_id=_FakeField(),
        create_time=_FakeOrderField(),
    )


def _patch(monkeypatch, module, service):
    monkeypatch.setattr(module.DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(module.DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(service, "model", _fake_model())


@pytest.mark.p2
def test_conversation_helper_orders_by_create_time(monkeypatch):
    _patch(monkeypatch, conversation_service, conversation_service.ConversationService)

    rows = conversation_service.ConversationService.get_all_conversation_by_dialog_ids(["dialog-1"])

    assert [r["id"] for r in rows] == ["a", "b", "c"]


@pytest.mark.p2
def test_file_helper_orders_by_create_time(monkeypatch):
    _patch(monkeypatch, file_service, file_service.FileService)

    rows = file_service.FileService.get_all_file_ids_by_tenant_id("tenant-1")

    assert [r["id"] for r in rows] == ["a", "b", "c"]


@pytest.mark.p2
def test_dialog_helper_orders_by_create_time(monkeypatch):
    _patch(monkeypatch, dialog_service, dialog_service.DialogService)

    rows = dialog_service.DialogService.get_all_dialogs_by_tenant_id("tenant-1")

    assert [r["id"] for r in rows] == ["a", "b", "c"]


@pytest.mark.p2
def test_canvas_version_helper_orders_by_create_time(monkeypatch):
    _patch(monkeypatch, user_canvas_version, user_canvas_version.UserCanvasVersionService)

    rows = user_canvas_version.UserCanvasVersionService.get_all_canvas_version_by_canvas_ids(["canvas-1"])

    assert [r["id"] for r in rows] == ["a", "b", "c"]
