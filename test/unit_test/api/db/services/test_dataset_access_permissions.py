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
from types import SimpleNamespace

from api.db import TenantPermission
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from common.constants import StatusEnum


def _unwrapped_kb_accessible():
    return KnowledgebaseService.accessible.__func__.__wrapped__


def _unwrapped_doc_accessible():
    return DocumentService.accessible.__func__.__wrapped__


def test_private_dataset_is_not_accessible_to_other_tenant_member(monkeypatch):
    kb = SimpleNamespace(
        id="kb-private",
        tenant_id="owner-1",
        permission=TenantPermission.ME.value,
        status=StatusEnum.VALID.value,
    )

    monkeypatch.setattr(KnowledgebaseService, "get_by_id", classmethod(lambda cls, kb_id: (True, kb)))
    monkeypatch.setattr(
        "api.db.services.knowledgebase_service.TenantService.get_joined_tenants_by_user_id",
        lambda _user_id: [{"tenant_id": "owner-1"}],
    )

    assert _unwrapped_kb_accessible()(KnowledgebaseService, "kb-private", "member-2") is False


def test_team_dataset_is_accessible_to_joined_tenant_member(monkeypatch):
    kb = SimpleNamespace(
        id="kb-team",
        tenant_id="owner-1",
        permission=TenantPermission.TEAM.value,
        status=StatusEnum.VALID.value,
    )

    monkeypatch.setattr(KnowledgebaseService, "get_by_id", classmethod(lambda cls, kb_id: (True, kb)))
    monkeypatch.setattr(
        "api.db.services.knowledgebase_service.TenantService.get_joined_tenants_by_user_id",
        lambda _user_id: [{"tenant_id": "owner-1"}],
    )

    assert _unwrapped_kb_accessible()(KnowledgebaseService, "kb-team", "member-2") is True


def test_document_access_respects_dataset_permission(monkeypatch):
    doc = SimpleNamespace(id="doc-1", kb_id="kb-private")

    monkeypatch.setattr(DocumentService, "get_by_id", classmethod(lambda cls, doc_id: (True, doc)))
    monkeypatch.setattr(KnowledgebaseService, "accessible", classmethod(lambda cls, kb_id, user_id: False))

    assert _unwrapped_doc_accessible()(DocumentService, "doc-1", "member-2") is False
