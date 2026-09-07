#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#

from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path
import sys
from types import ModuleType, SimpleNamespace

import pytest
from quart import Blueprint, Quart


REPO_ROOT = Path(__file__).resolve().parents[5]
if "api.apps" not in sys.modules:
    api_apps = ModuleType("api.apps")
    api_apps.__path__ = [str(REPO_ROOT / "api" / "apps")]
    sys.modules["api.apps"] = api_apps
else:
    api_apps = sys.modules["api.apps"]


ACTOR = "route-owner"


@pytest.fixture()
def route_app(monkeypatch):
    monkeypatch.setattr(api_apps, "current_user", SimpleNamespace(id=ACTOR, is_superuser=False), raising=False)
    monkeypatch.setattr(api_apps, "login_required", lambda function: function, raising=False)
    module_name = "api.apps.restful_apis.business_document_api_boundary_test"
    spec = spec_from_file_location(module_name, REPO_ROOT / "api" / "apps" / "restful_apis" / "business_document_api.py")
    assert spec is not None and spec.loader is not None
    module = module_from_spec(spec)
    module.manager = Blueprint("business_document_api_boundary_test", module_name)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    app = Quart(__name__)
    app.register_blueprint(module.manager)
    yield app, module
    sys.modules.pop(module_name, None)


@pytest.mark.p0
@pytest.mark.asyncio
async def test_create_and_command_reject_non_object_and_malformed_json(route_app):
    app, _module = route_app
    client = app.test_client()
    cases = (
        ("/business-documents", [], "INVALID_DOCUMENT"),
        ("/business-documents/doc-1/commands", [], "INVALID_COMMAND_REQUEST"),
    )
    for path, payload, error_code in cases:
        response = await client.post(path, json=payload)
        body = await response.get_json()
        assert response.status_code == 422
        assert body["code"] == 422
        assert body["data"] == {"error_code": error_code, "details": {}}

    for path, error_code in (
        ("/business-documents", "INVALID_DOCUMENT"),
        ("/business-documents/doc-1/commands", "INVALID_COMMAND_REQUEST"),
    ):
        malformed = await client.post(path, data="{", headers={"Content-Type": "application/json"})
        malformed_body = await malformed.get_json()
        assert malformed.status_code == 422
        assert malformed_body["data"]["error_code"] == error_code

        empty = await client.post(path)
        empty_body = await empty.get_json()
        assert empty.status_code == 422
        assert empty_body["data"]["error_code"] == error_code


@pytest.mark.p0
@pytest.mark.asyncio
async def test_routes_pass_tenant_and_owner_in_service_contract_order(route_app, monkeypatch):
    app, module = route_app
    calls = []

    def create_document(tenant_id, actor_id, data, is_admin, access_role):
        calls.append(("create", tenant_id, actor_id, data, is_admin, access_role))
        return {"document_id": "doc-1"}

    def execute_command(tenant_id, actor_id, document_id, data, is_admin, access_role):
        calls.append(("command", tenant_id, actor_id, document_id, data, is_admin, access_role))
        return {"accepted": True, "document_id": document_id}

    def get_document(tenant_id, document_id, actor_id, is_admin, access_role):
        calls.append(("get", tenant_id, document_id, actor_id, is_admin, access_role))
        return {
            "document_id": document_id,
            "owner_id": actor_id,
            "current_revision": {"revision_id": "revision-1", "section_texts": {"5.5": "Метрика"}},
        }

    monkeypatch.setattr(module.BusinessDocumentService, "create_document", staticmethod(create_document))
    monkeypatch.setattr(module.BusinessDocumentService, "execute_command", staticmethod(execute_command))
    monkeypatch.setattr(module.BusinessDocumentService, "get_document", staticmethod(get_document))
    client = app.test_client()
    create_payload = {"schema_version": "1", "document_type": "business_requirements", "title": "T", "idea": "I"}
    command_payload = {
        "schema_version": "1",
        "command_id": "command-1",
        "idempotency_key": "key-1",
        "expected_state_version": 1,
        "type": "REQUEST_INTAKE_ASSESSMENT",
        "payload": {},
    }

    assert (await client.post("/business-documents", json=create_payload)).status_code == 201
    assert (await client.post("/business-documents/doc-1/commands", json=command_payload)).status_code == 200
    get_response = await client.get("/business-documents/doc-1")
    assert get_response.status_code == 200
    assert (await get_response.get_json())["data"]["current_revision"]["section_texts"] == {"5.5": "Метрика"}
    assert calls == [
        ("create", ACTOR, ACTOR, create_payload, False, "AUTHOR_CREATOR"),
        ("command", ACTOR, ACTOR, "doc-1", command_payload, False, "AUTHOR_CREATOR"),
        ("get", ACTOR, "doc-1", ACTOR, False, "AUTHOR_CREATOR"),
    ]


@pytest.mark.p0
@pytest.mark.asyncio
async def test_delete_route_passes_admin_role_to_service(route_app, monkeypatch):
    app, module = route_app
    module.current_user.is_superuser = True
    calls = []

    def delete_document(actor_id, document_id, is_admin, access_role):
        calls.append((actor_id, document_id, is_admin, access_role))
        return {"document_id": document_id, "deleted": True}

    monkeypatch.setattr(module.BusinessDocumentService, "delete_document", staticmethod(delete_document))
    response = await app.test_client().delete("/business-documents/doc-1")

    assert response.status_code == 200
    assert (await response.get_json())["data"] == {"document_id": "doc-1", "deleted": True}
    assert calls == [(ACTOR, "doc-1", True, "AUTHOR_CREATOR")]


@pytest.mark.p0
@pytest.mark.asyncio
async def test_access_filter_and_assignment_routes_pass_role_and_scope(route_app, monkeypatch):
    app, module = route_app
    module.current_user.business_document_role = "EXTENDED_MODERATOR"
    calls = []

    def list_documents(tenant_id, actor_id, page, page_size, is_admin, access_role, scope):
        calls.append(("list", tenant_id, actor_id, page, page_size, is_admin, access_role, scope))
        return {"items": [], "scope": scope}

    def list_access_users(actor_id, is_admin, access_role):
        calls.append(("users", actor_id, is_admin, access_role))
        return {"items": []}

    def assign_document(actor_id, document_id, data, is_admin, access_role):
        calls.append(("assign", actor_id, document_id, data, is_admin, access_role))
        return {"document_id": document_id, "owner_id": data["owner_id"]}

    monkeypatch.setattr(module.BusinessDocumentService, "list_documents", staticmethod(list_documents))
    monkeypatch.setattr(module.BusinessDocumentService, "list_access_users", staticmethod(list_access_users))
    monkeypatch.setattr(module.BusinessDocumentService, "assign_document", staticmethod(assign_document))
    client = app.test_client()

    assert (await client.get("/business-documents?page=2&page_size=5&scope=mine")).status_code == 200
    assert (await client.get("/business-documents/access/users")).status_code == 200
    assignment = {"owner_id": "author-2", "expected_state_version": 7}
    response = await client.put("/business-documents/doc-1/owner", json=assignment)

    assert response.status_code == 200
    assert calls == [
        ("list", ACTOR, ACTOR, 2, 5, False, "EXTENDED_MODERATOR", "mine"),
        ("users", ACTOR, False, "EXTENDED_MODERATOR"),
        ("assign", ACTOR, "doc-1", assignment, False, "EXTENDED_MODERATOR"),
    ]


@pytest.mark.p0
@pytest.mark.asyncio
async def test_catalog_route_returns_the_service_projection(route_app, monkeypatch):
    app, module = route_app
    catalog = {
        "items": [
            {
                "id": "L2-01.01.04.01.01",
                "title": "Разрешённый документ",
                "capability_level": "L5",
            }
        ],
        "total": 1,
    }
    monkeypatch.setattr(module.BusinessDocumentService, "list_catalog", staticmethod(lambda: catalog))

    response = await app.test_client().get("/business-documents/catalog")

    assert response.status_code == 200
    assert (await response.get_json())["data"] == catalog
