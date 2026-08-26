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
warnings.filterwarnings(
    "ignore",
    message="\\[Errno 13\\] Permission denied\\.  joblib will operate in serial mode",
    category=UserWarning,
)

from api.db.services import document_service


class _FakeOrderField:
    def desc(self):
        return self

    def asc(self):
        return self


class _FakeField:
    def __eq__(self, other):
        return self

    def in_(self, other):
        return self

    def not_in(self, other):
        return self


class _FakeQuery:
    def __init__(self, docs):
        self._all = list(docs)
        self._current = list(docs)

    def join(self, *args, **kwargs):
        return self

    def where(self, *args, **kwargs):
        return self

    def order_by(self, *args, **kwargs):
        return self

    def count(self):
        return len(self._all)

    def paginate(self, page, page_size):
        if page and page_size:
            start = (page - 1) * page_size
            end = start + page_size
            self._current = self._all[start:end]
        return self

    def dicts(self):
        return list(self._current)


@pytest.fixture
def metadata_calls(monkeypatch):
    sample_docs = [
        {"id": "doc-1"},
        {"id": "doc-2"},
        {"id": "doc-3"},
    ]

    model = SimpleNamespace(
        select=lambda *args, **kwargs: _FakeQuery(sample_docs),
        id=_FakeField(),
        kb_id=_FakeField(),
        name=_FakeField(),
        suffix=_FakeField(),
        run=_FakeField(),
        type=_FakeField(),
        created_by=_FakeField(),
        pipeline_id=_FakeField(),
        getter_by=lambda *_args, **_kwargs: _FakeOrderField(),
    )

    monkeypatch.setattr(document_service.DB, "connect", lambda *args, **kwargs: None)
    monkeypatch.setattr(document_service.DB, "close", lambda *args, **kwargs: None)
    monkeypatch.setattr(document_service.DocumentService, "model", model)
    monkeypatch.setattr(
        document_service.DocumentService,
        "get_cls_model_fields",
        classmethod(lambda cls: []),
    )

    calls = []

    def _fake_get_metadata_for_documents(cls, doc_ids, kb_id):
        calls.append((doc_ids, kb_id))
        return {doc_id: {"source_url": f"url-{doc_id}"} for doc_id in (doc_ids or [])}

    monkeypatch.setattr(
        document_service.DocMetadataService,
        "get_metadata_for_documents",
        classmethod(_fake_get_metadata_for_documents),
    )

    return calls


@pytest.mark.p2
def test_get_list_fetches_metadata_for_page_document_ids(metadata_calls):
    docs, count = document_service.DocumentService.get_list(
        "kb-1",
        1,
        2,
        "create_time",
        True,
        "",
        None,
        None,
    )

    assert count == 3
    assert [doc["id"] for doc in docs] == ["doc-1", "doc-2"]
    assert docs[0]["meta_fields"]["source_url"] == "url-doc-1"
    assert metadata_calls == [(["doc-1", "doc-2"], "kb-1")]


@pytest.mark.p2
def test_get_by_kb_id_fetches_metadata_for_page_document_ids(metadata_calls):
    docs, count = document_service.DocumentService.get_by_kb_id(
        "kb-1",
        2,
        1,
        "create_time",
        True,
        "",
        [],
        [],
        [],
        return_empty_metadata=False,
    )

    assert count == 3
    assert [doc["id"] for doc in docs] == ["doc-2"]
    assert docs[0]["meta_fields"]["source_url"] == "url-doc-2"
    assert metadata_calls == [(["doc-2"], "kb-1")]


@pytest.mark.p2
def test_get_by_kb_id_return_empty_metadata_keeps_dataset_wide_lookup(metadata_calls, monkeypatch):
    def _fake_get_metadata_for_documents(cls, doc_ids, kb_id):
        metadata_calls.append((doc_ids, kb_id))
        return {"doc-1": {"source_url": "url-doc-1"}} if doc_ids is None else {}

    monkeypatch.setattr(
        document_service.DocMetadataService,
        "get_metadata_for_documents",
        classmethod(_fake_get_metadata_for_documents),
    )

    docs, count = document_service.DocumentService.get_by_kb_id(
        "kb-1",
        1,
        2,
        "create_time",
        True,
        "",
        [],
        [],
        [],
        return_empty_metadata=True,
    )

    assert count == 3
    assert docs[0]["meta_fields"] == {}
    assert metadata_calls == [(None, "kb-1")]
