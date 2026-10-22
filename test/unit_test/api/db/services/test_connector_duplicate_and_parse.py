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
"""Deferred connector metadata refresh still runs if parse/queue fails."""

from types import SimpleNamespace

import pytest

from api.db.services import connector_service as connector_mod
from api.db.services.connector_service import SyncLogsService
from api.db.services.file_service import FileService


TENANT_ID = "tenant-1"
FILENAME = "row-1.txt"
DOC = {"id": "doc-1", "name": FILENAME}


def _docs():
    return [
        {
            "id": "row-1",
            "semantic_identifier": "row-1",
            "extension": ".txt",
            "blob": b"payload",
            "metadata": {"source": "mysql"},
        }
    ]


def _stub_upload(monkeypatch, pairs):
    monkeypatch.setattr(FileService, "_is_sync_cancelled", staticmethod(lambda *_args, **_kwargs: False))
    monkeypatch.setattr(
        FileService,
        "upload_document",
        classmethod(lambda *_args, **_kwargs: ([], pairs)),
    )


@pytest.mark.p2
def test_duplicate_and_parse_refreshes_metadata_when_run_fails(monkeypatch):
    _stub_upload(monkeypatch, [(DOC, b"payload")])
    monkeypatch.setattr(connector_mod.DocMetadataService, "update_document_metadata", classmethod(lambda *_args, **_kwargs: True))
    refresh_calls = []
    monkeypatch.setattr(
        connector_mod.DocMetadataService,
        "refresh_tenant_index",
        classmethod(lambda cls, tenant_id: refresh_calls.append(tenant_id)),
    )

    def _fail_run(*_args, **_kwargs):
        raise RuntimeError("queue failed")

    monkeypatch.setattr(connector_mod.DocumentService, "run", classmethod(_fail_run))

    with pytest.raises(RuntimeError, match="queue failed"):
        SyncLogsService.duplicate_and_parse(SimpleNamespace(), _docs(), TENANT_ID, "mysql/1", auto_parse=True)

    assert refresh_calls == [TENANT_ID]


@pytest.mark.p2
def test_duplicate_and_parse_skips_refresh_when_no_metadata_was_written(monkeypatch):
    docs = [
        {
            "id": "row-1",
            "semantic_identifier": "row-1",
            "extension": ".txt",
            "blob": b"payload",
        }
    ]
    _stub_upload(monkeypatch, [(DOC, b"payload")])
    refresh_calls = []
    monkeypatch.setattr(
        connector_mod.DocMetadataService,
        "refresh_tenant_index",
        classmethod(lambda cls, tenant_id: refresh_calls.append(tenant_id)),
    )

    def _fail_run(*_args, **_kwargs):
        raise RuntimeError("queue failed")

    monkeypatch.setattr(connector_mod.DocumentService, "run", classmethod(_fail_run))

    with pytest.raises(RuntimeError, match="queue failed"):
        SyncLogsService.duplicate_and_parse(SimpleNamespace(), docs, TENANT_ID, "mysql/1", auto_parse=True)

    assert refresh_calls == []
