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
"""ConnectorService.rebuild enforces kb authorization in the service layer.

Rebuild deletes the connector's documents in the caller-supplied kb and
schedules sync tasks, so the service itself (not just the HTTP handler) must
refuse to touch a kb the caller cannot access or that the connector is not
bound to. This mirrors the binding guard already used by
ConnectorService.cleanup_stale_documents_for_task.
"""

from types import SimpleNamespace

import pytest

from api.db.services.connector_service import (
    Connector2KbService,
    ConnectorAuthorizationError,
    ConnectorService,
    DocumentService,
    KnowledgebaseService,
    SyncLogsService,
)
from api.db.services.file_service import FileService


@pytest.mark.p2
def test_rebuild_rejects_kb_the_caller_cannot_access(monkeypatch):
    monkeypatch.setattr(KnowledgebaseService, "accessible", staticmethod(lambda kb_id, uid: False))

    with pytest.raises(ConnectorAuthorizationError, match="no authorization"):
        ConnectorService.rebuild("kb-foreign", "conn-1", "user-1")


@pytest.mark.p2
def test_rebuild_rejects_kb_the_connector_is_not_bound_to(monkeypatch):
    monkeypatch.setattr(KnowledgebaseService, "accessible", staticmethod(lambda kb_id, uid: True))
    monkeypatch.setattr(Connector2KbService, "query", staticmethod(lambda **kwargs: []))

    with pytest.raises(ConnectorAuthorizationError, match="Connector is not bound to this knowledge base."):
        ConnectorService.rebuild("kb-1", "conn-1", "user-1")


@pytest.mark.p2
def test_rebuild_runs_only_after_both_guards_pass(monkeypatch):
    touched = []

    monkeypatch.setattr(KnowledgebaseService, "accessible", staticmethod(lambda kb_id, uid: True))
    monkeypatch.setattr(Connector2KbService, "query", staticmethod(lambda **kwargs: [object()]))
    monkeypatch.setattr(
        ConnectorService,
        "get_by_id",
        staticmethod(lambda cid: (True, SimpleNamespace(id=cid, source="rss", config={}))),
    )
    monkeypatch.setattr(DocumentService, "query", staticmethod(lambda **kwargs: []))
    monkeypatch.setattr(FileService, "delete_docs", staticmethod(lambda *args, **kwargs: None))
    monkeypatch.setattr(
        SyncLogsService,
        "filter_delete",
        staticmethod(lambda *args, **kwargs: touched.append("filter_delete")),
    )
    monkeypatch.setattr(
        SyncLogsService,
        "schedule",
        staticmethod(lambda *args, **kwargs: touched.append("schedule")),
    )

    err = ConnectorService.rebuild("kb-1", "conn-1", "user-1")

    assert err is None
    assert touched == ["filter_delete", "schedule"], "delete/schedule should run once the guards pass"
