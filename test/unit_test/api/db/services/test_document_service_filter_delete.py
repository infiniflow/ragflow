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
from contextlib import nullcontext
from types import SimpleNamespace
from unittest.mock import MagicMock

warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)

from api.db.services import document_service  # noqa: E402


def test_filter_delete_deletes_only_the_ids_selected_for_invalidation(monkeypatch):
    """The delete and invalidation must operate on the same snapshot of document IDs."""
    original_filter = object()
    invalidated_ids = []

    select_query = MagicMock()
    select_query.where.return_value.tuples.return_value = [("doc-1",), ("doc-2",)]
    delete_query = MagicMock()
    delete_query.where.return_value.execute.return_value = 2
    id_field = MagicMock()
    id_filter = object()
    id_field.in_.return_value = id_filter
    model = SimpleNamespace(
        id=id_field,
        select=MagicMock(return_value=select_query),
        delete=MagicMock(return_value=delete_query),
    )

    monkeypatch.setattr(document_service.DB, "atomic", nullcontext)
    monkeypatch.setattr(document_service.DocumentService, "model", model)
    monkeypatch.setattr(
        document_service.DocumentService,
        "_invalidate_doc_exists_cache",
        classmethod(lambda _cls, doc_ids: invalidated_ids.extend(doc_ids)),
    )

    filter_delete = document_service.DocumentService.filter_delete.__func__.__wrapped__
    deleted = filter_delete(document_service.DocumentService, [original_filter])

    assert deleted == 2
    select_query.where.assert_called_once_with(original_filter)
    id_field.in_.assert_called_once_with(["doc-1", "doc-2"])
    delete_query.where.assert_called_once_with(id_filter)
    assert invalidated_ids == ["doc-1", "doc-2"]
