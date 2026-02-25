#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import asyncio
from types import SimpleNamespace

import pytest
from common import (
    document_change_status,
    document_filter,
    document_infos,
    document_metadata_summary,
    document_rename,
    document_set_meta,
    document_update_metadata_setting,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth

INVALID_AUTH_CASES = [
    (None, 401, "Unauthorized"),
    (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
]


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_filter_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_filter(invalid_auth, {"kb_id": "kb_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_infos_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_infos(invalid_auth, {"doc_ids": ["doc_id"]})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    ## The inputs has been changed to add 'doc_ids'
    ## TODO: 
    #@pytest.mark.p2
    #@pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    #def test_metadata_summary_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
    #    res = document_metadata_summary(invalid_auth, {"kb_id": "kb_id"})
    #    assert res["code"] == expected_code, res
    #    assert expected_fragment in res["message"], res

    ## The inputs has been changed to deprecate 'selector'
    ## TODO: 
    #@pytest.mark.p2
    #@pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    #def test_metadata_update_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
    #    res = document_metadata_update(invalid_auth, {"kb_id": "kb_id", "selector": {"document_ids": ["doc_id"]}, "updates": []})
    #    assert res["code"] == expected_code, res
    #    assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_update_metadata_setting_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_update_metadata_setting(invalid_auth, {"doc_id": "doc_id", "metadata": {}})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_change_status_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_change_status(invalid_auth, {"doc_ids": ["doc_id"], "status": "1"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_rename_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_rename(invalid_auth, {"doc_id": "doc_id", "name": "rename.txt"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_set_meta_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_set_meta(invalid_auth, {"doc_id": "doc_id", "meta": "{}"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res


class TestDocumentMetadata:
    @pytest.mark.p2
    def test_filter(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        res = document_filter(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == 0, res
        assert "filter" in res["data"], res
        assert "total" in res["data"], res

    @pytest.mark.p2
    def test_infos(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_infos(WebApiAuth, {"doc_ids": [doc_id]})
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res
        assert res["data"][0]["id"] == doc_id, res

    ## The inputs has been changed to add 'doc_ids'
    ## TODO: 
    #@pytest.mark.p2
    #def test_metadata_summary(self, WebApiAuth, add_document_func):
    #    kb_id, _ = add_document_func
    #    res = document_metadata_summary(WebApiAuth, {"kb_id": kb_id})
    #    assert res["code"] == 0, res
    #    assert isinstance(res["data"]["summary"], dict), res

    ## The inputs has been changed to deprecate 'selector'
    ## TODO: 
    #@pytest.mark.p2
    #def test_metadata_update(self, WebApiAuth, add_document_func):
    #    kb_id, doc_id = add_document_func
    #    payload = {
    #        "kb_id": kb_id,
    #        "selector": {"document_ids": [doc_id]},
    #        "updates": [{"key": "author", "value": "alice"}],
    #        "deletes": [],
    #    }
    #    res = document_metadata_update(WebApiAuth, payload)
    #    assert res["code"] == 0, res
    #    assert res["data"]["matched_docs"] == 1, res
    #    info_res = document_infos(WebApiAuth, {"doc_ids": [doc_id]})
    #    assert info_res["code"] == 0, info_res
    #    meta_fields = info_res["data"][0].get("meta_fields", {})
    #    assert meta_fields.get("author") == "alice", info_res
    
    ## The inputs has been changed to deprecate 'selector'
    ## TODO: 
    #@pytest.mark.p2
    #def test_update_metadata_setting(self, WebApiAuth, add_document_func):
    #    _, doc_id = add_document_func
    #    metadata = {"source": "test"}
    #    res = document_update_metadata_setting(WebApiAuth, {"doc_id": doc_id, "metadata": metadata})
    #    assert res["code"] == 0, res
    #    assert res["data"]["id"] == doc_id, res
    #    assert res["data"]["parser_config"]["metadata"] == metadata, res

    @pytest.mark.p2
    def test_change_status(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_change_status(WebApiAuth, {"doc_ids": [doc_id], "status": "1"})
        assert res["code"] == 0, res
        assert res["data"][doc_id]["status"] == "1", res
        info_res = document_infos(WebApiAuth, {"doc_ids": [doc_id]})
        assert info_res["code"] == 0, info_res
        assert info_res["data"][0]["status"] == "1", info_res

    @pytest.mark.p2
    def test_rename(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        name = f"renamed_{doc_id}.txt"
        res = document_rename(WebApiAuth, {"doc_id": doc_id, "name": name})
        assert res["code"] == 0, res
        assert res["data"] is True, res
        info_res = document_infos(WebApiAuth, {"doc_ids": [doc_id]})
        assert info_res["code"] == 0, info_res
        assert info_res["data"][0]["name"] == name, info_res

    @pytest.mark.p2
    def test_set_meta(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_set_meta(WebApiAuth, {"doc_id": doc_id, "meta": "{\"author\": \"alice\"}"})
        assert res["code"] == 0, res
        assert res["data"] is True, res
        info_res = document_infos(WebApiAuth, {"doc_ids": [doc_id]})
        assert info_res["code"] == 0, info_res
        meta_fields = info_res["data"][0].get("meta_fields", {})
        assert meta_fields.get("author") == "alice", info_res


class TestDocumentMetadataNegative:
    @pytest.mark.p3
    def test_filter_missing_kb_id(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_filter(WebApiAuth, {"doc_ids": [doc_id]})
        assert res["code"] == 101, res
        assert "KB ID" in res["message"], res

    @pytest.mark.p3
    def test_metadata_summary_missing_kb_id(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_metadata_summary(WebApiAuth, {"doc_ids": [doc_id]})
        assert res["code"] == 101, res
        assert "KB ID" in res["message"], res

    ## The inputs has been changed to deprecate 'selector'
    ## TODO: 
    #@pytest.mark.p3
    #def test_metadata_update_missing_kb_id(self, WebApiAuth, add_document_func):
    #    _, doc_id = add_document_func
    #    res = document_metadata_update(WebApiAuth, {"selector": {"document_ids": [doc_id]}, "updates": []})
    #    assert res["code"] == 101, res
    #    assert "KB ID" in res["message"], res

    @pytest.mark.p3
    def test_infos_invalid_doc_id(self, WebApiAuth):
        res = document_infos(WebApiAuth, {"doc_ids": ["invalid_id"]})
        assert res["code"] == 109, res
        assert "No authorization" in res["message"], res

    @pytest.mark.p3
    def test_update_metadata_setting_missing_metadata(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_update_metadata_setting(WebApiAuth, {"doc_id": doc_id})
        assert res["code"] == 101, res
        assert "required argument are missing" in res["message"], res
        assert "metadata" in res["message"], res

    @pytest.mark.p3
    def test_change_status_invalid_status(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_change_status(WebApiAuth, {"doc_ids": [doc_id], "status": "2"})
        assert res["code"] == 101, res
        assert "Status" in res["message"], res

    @pytest.mark.p3
    def test_rename_extension_mismatch(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_rename(WebApiAuth, {"doc_id": doc_id, "name": "renamed.pdf"})
        assert res["code"] == 101, res
        assert "extension" in res["message"], res

    @pytest.mark.p3
    def test_set_meta_invalid_type(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_set_meta(WebApiAuth, {"doc_id": doc_id, "meta": "[]"})
        assert res["code"] == 101, res
        assert "dictionary" in res["message"], res


def _run(coro):
    return asyncio.run(coro)


@pytest.mark.p2
class TestDocumentMetadataUnit:
    def _allow_kb(self, module, monkeypatch, kb_id="kb1", tenant_id="tenant1"):
        monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id=tenant_id)])
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: True if _kwargs.get("id") == kb_id else False)

    def test_filter_missing_kb_id(self, document_app_module, monkeypatch):
        module = document_app_module

        async def fake_request_json():
            return {}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.get_filter())
        assert res["code"] == 101
        assert "KB ID" in res["message"]

    def test_filter_unauthorized(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant1")])
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: False)

        async def fake_request_json():
            return {"kb_id": "kb1"}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.get_filter())
        assert res["code"] == 103

    def test_filter_invalid_filters(self, document_app_module, monkeypatch):
        module = document_app_module
        self._allow_kb(module, monkeypatch)

        async def fake_request_json():
            return {"kb_id": "kb1", "run_status": ["INVALID"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.get_filter())
        assert res["code"] == 102
        assert "Invalid filter run status" in res["message"]

        async def fake_request_json_types():
            return {"kb_id": "kb1", "types": ["INVALID"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json_types)
        res = _run(module.get_filter())
        assert res["code"] == 102
        assert "Invalid filter conditions" in res["message"]

    def test_filter_keywords_suffix(self, document_app_module, monkeypatch):
        module = document_app_module
        self._allow_kb(module, monkeypatch)
        monkeypatch.setattr(module.DocumentService, "get_filter_by_kb_id", lambda *_args, **_kwargs: ({"run": {}}, 1))

        async def fake_request_json():
            return {"kb_id": "kb1", "keywords": "ragflow", "suffix": ["txt"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.get_filter())
        assert res["code"] == 0
        assert "filter" in res["data"]

    def test_filter_exception(self, document_app_module, monkeypatch):
        module = document_app_module
        self._allow_kb(module, monkeypatch)

        def raise_error(*_args, **_kwargs):
            raise RuntimeError("boom")

        monkeypatch.setattr(module.DocumentService, "get_filter_by_kb_id", raise_error)

        async def fake_request_json():
            return {"kb_id": "kb1"}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.get_filter())
        assert res["code"] == 100

    def test_infos_meta_fields(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)

        class _Docs:
            def dicts(self):
                return [{"id": "doc1"}]

        monkeypatch.setattr(module.DocumentService, "get_by_ids", lambda _ids: _Docs())
        monkeypatch.setattr(module.DocMetadataService, "get_document_metadata", lambda _doc_id: {"author": "alice"})

        async def fake_request_json():
            return {"doc_ids": ["doc1"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.doc_infos())
        assert res["code"] == 0
        assert res["data"][0]["meta_fields"]["author"] == "alice"

    def test_metadata_summary_missing_kb_id(self, document_app_module, monkeypatch):
        module = document_app_module

        async def fake_request_json():
            return {"doc_ids": ["doc1"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.metadata_summary())
        assert res["code"] == 101

    def test_metadata_summary_unauthorized(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant1")])
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: False)

        async def fake_request_json():
            return {"kb_id": "kb1", "doc_ids": ["doc1"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.metadata_summary())
        assert res["code"] == 103

    def test_metadata_summary_success_and_exception(self, document_app_module, monkeypatch):
        module = document_app_module
        self._allow_kb(module, monkeypatch)
        monkeypatch.setattr(module.DocMetadataService, "get_metadata_summary", lambda *_args, **_kwargs: {"author": {"alice": 1}})

        async def fake_request_json():
            return {"kb_id": "kb1", "doc_ids": ["doc1"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.metadata_summary())
        assert res["code"] == 0
        assert "summary" in res["data"]

        def raise_error(*_args, **_kwargs):
            raise RuntimeError("boom")

        monkeypatch.setattr(module.DocMetadataService, "get_metadata_summary", raise_error)
        res = _run(module.metadata_summary())
        assert res["code"] == 100

    def test_metadata_update_missing_kb_id(self, document_app_module, monkeypatch):
        module = document_app_module

        async def fake_request_json():
            return {"doc_ids": ["doc1"], "updates": [], "deletes": []}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.metadata_update.__wrapped__())
        assert res["code"] == 101
        assert "KB ID" in res["message"]

    def test_metadata_update_success(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module.DocMetadataService, "batch_update_metadata", lambda *_args, **_kwargs: 1)

        async def fake_request_json():
            return {"kb_id": "kb1", "doc_ids": ["doc1"], "updates": [{"key": "author", "value": "alice"}], "deletes": []}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.metadata_update.__wrapped__())
        assert res["code"] == 0
        assert res["data"]["matched_docs"] == 1
