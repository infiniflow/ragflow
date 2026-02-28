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
from concurrent.futures import ThreadPoolExecutor, as_completed
from types import SimpleNamespace

import pytest
from common import list_documents
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils import is_sorted


@pytest.mark.p2
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = list_documents(invalid_auth, {"kb_id": "dataset_id"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsList:
    @pytest.mark.p1
    def test_default(self, WebApiAuth, add_documents):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == 5
        assert res["data"]["total"] == 5

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "kb_id, expected_code, expected_message",
        [
            ("", 101, 'Lack of "KB ID"'),
            ("invalid_dataset_id", 103, "Only owner of dataset authorized for this operation."),
        ],
    )
    def test_invalid_dataset_id(self, WebApiAuth, kb_id, expected_code, expected_message):
        res = list_documents(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page": None, "page_size": 2}, 0, 5, ""),
            ({"page": 0, "page_size": 2}, 0, 5, ""),
            ({"page": 2, "page_size": 2}, 0, 2, ""),
            ({"page": 3, "page_size": 2}, 0, 1, ""),
            ({"page": "3", "page_size": 2}, 0, 1, ""),
            pytest.param({"page": -1, "page_size": 2}, 100, 0, "1064", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"page": "a", "page_size": 2}, 100, 0, """ValueError("invalid literal for int() with base 10: 'a'")""", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_page(self, WebApiAuth, add_documents, params, expected_code, expected_page_size, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size, res
            assert res["data"]["total"] == 5, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page_size": None}, 0, 5, ""),
            ({"page_size": 0}, 0, 5, ""),
            ({"page_size": 1}, 0, 5, ""),
            ({"page_size": 6}, 0, 5, ""),
            ({"page_size": "1"}, 0, 5, ""),
            pytest.param({"page_size": -1}, 100, 0, "1064", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"page_size": "a"}, 100, 0, """ValueError("invalid literal for int() with base 10: 'a'")""", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_page_size(self, WebApiAuth, add_documents, params, expected_code, expected_page_size, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            ({"orderby": None}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"orderby": "create_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "update_time", True)), ""),
            pytest.param({"orderby": "name", "desc": "False"}, 0, lambda r: (is_sorted(r["data"]["docs"], "name", False)), "", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"orderby": "unknown"}, 102, 0, "orderby should be create_time or update_time", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_orderby(self, WebApiAuth, add_documents, params, expected_code, assertions, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            ({"desc": None}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": "true"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": "True"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": True}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            pytest.param({"desc": "false"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), "", marks=pytest.mark.skip(reason="issues/5851")),
            ({"desc": "False"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), ""),
            ({"desc": False}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), ""),
            ({"desc": "False", "orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "update_time", False)), ""),
            pytest.param({"desc": "unknown"}, 102, 0, "desc should be true or false", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_desc(self, WebApiAuth, add_documents, params, expected_code, assertions, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_num",
        [
            ({"keywords": None}, 5),
            ({"keywords": ""}, 5),
            ({"keywords": "0"}, 1),
            ({"keywords": "ragflow_test_upload"}, 5),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, WebApiAuth, add_documents, params, expected_num):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == 0, res
        assert len(res["data"]["docs"]) == expected_num, res
        assert res["data"]["total"] == expected_num, res

    @pytest.mark.p3
    def test_concurrent_list(self, WebApiAuth, add_documents):
        kb_id, _ = add_documents
        count = 100

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_documents, WebApiAuth, {"kb_id": kb_id}) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures), responses


def _run(coro):
    return asyncio.run(coro)


class _DummyArgs(dict):
    def get(self, key, default=None):
        return super().get(key, default)


@pytest.mark.p2
class TestDocumentsListUnit:
    def _set_args(self, module, monkeypatch, **kwargs):
        monkeypatch.setattr(module, "request", SimpleNamespace(args=_DummyArgs(kwargs)))

    def _allow_kb(self, module, monkeypatch, kb_id="kb1", tenant_id="tenant1"):
        monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id=tenant_id)])
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: True if _kwargs.get("id") == kb_id else False)

    def test_missing_kb_id(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch)

        async def fake_request_json():
            return {}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 101
        assert res["message"] == 'Lack of "KB ID"'

    def test_unauthorized_dataset(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant1")])
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: False)

        async def fake_request_json():
            return {}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 103
        assert "Only owner of dataset" in res["message"]

    def test_return_empty_metadata_flags(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        self._allow_kb(module, monkeypatch)
        monkeypatch.setattr(module.DocumentService, "get_by_kb_id", lambda *_args, **_kwargs: ([], 0))

        async def fake_request_json():
            return {"return_empty_metadata": "true", "metadata": {"author": "alice"}}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 0

        async def fake_request_json_empty():
            return {"metadata": {"empty_metadata": True, "author": "alice"}}

        monkeypatch.setattr(module, "get_request_json", fake_request_json_empty)
        res = _run(module.list_docs())
        assert res["code"] == 0

    def test_invalid_filters(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        self._allow_kb(module, monkeypatch)

        async def fake_request_json():
            return {"run_status": ["INVALID"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 102
        assert "Invalid filter run status" in res["message"]

        async def fake_request_json_types():
            return {"types": ["INVALID"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json_types)
        res = _run(module.list_docs())
        assert res["code"] == 102
        assert "Invalid filter conditions" in res["message"]

    def test_invalid_metadata_types(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        self._allow_kb(module, monkeypatch)

        async def fake_request_json():
            return {"metadata_condition": "bad"}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 102
        assert "metadata_condition" in res["message"]

        async def fake_request_json_meta():
            return {"metadata": ["not", "object"]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json_meta)
        res = _run(module.list_docs())
        assert res["code"] == 102
        assert "metadata must be an object" in res["message"]

    def test_metadata_condition_empty_result(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        self._allow_kb(module, monkeypatch)
        monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda *_args, **_kwargs: {})
        monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: set())

        async def fake_request_json():
            return {"metadata_condition": {"conditions": [{"name": "author", "comparison_operator": "is", "value": "alice"}]}}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 0
        assert res["data"]["total"] == 0

    def test_metadata_values_intersection(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        self._allow_kb(module, monkeypatch)
        metas = {
            "author": {"alice": ["doc1", "doc2"]},
            "topic": {"rag": ["doc2"]},
        }
        monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda *_args, **_kwargs: metas)

        captured = {}

        def fake_get_by_kb_id(*_args, **_kwargs):
            if len(_args) >= 10:
                captured["doc_ids_filter"] = _args[9]
            else:
                captured["doc_ids_filter"] = None
            return ([{"id": "doc2", "thumbnail": "", "parser_config": {}}], 1)

        monkeypatch.setattr(module.DocumentService, "get_by_kb_id", fake_get_by_kb_id)

        async def fake_request_json():
            return {"metadata": {"author": ["alice", " ", None], "topic": "rag"}}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 0
        assert captured["doc_ids_filter"] == ["doc2"]

    def test_metadata_intersection_empty(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        self._allow_kb(module, monkeypatch)
        metas = {
            "author": {"alice": ["doc1"]},
            "topic": {"rag": ["doc2"]},
        }
        monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda *_args, **_kwargs: metas)

        async def fake_request_json():
            return {"metadata": {"author": "alice", "topic": "rag"}}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 0
        assert res["data"]["total"] == 0

    def test_desc_time_and_schema(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1", desc="false", create_time_from="150", create_time_to="250")
        self._allow_kb(module, monkeypatch)

        docs = [
            {"id": "doc1", "thumbnail": "", "parser_config": {"metadata": {"a": 1}}, "create_time": 100},
            {"id": "doc2", "thumbnail": "", "parser_config": {"metadata": {"b": 2}}, "create_time": 200},
        ]

        def fake_get_by_kb_id(*_args, **_kwargs):
            return (docs, 2)

        monkeypatch.setattr(module.DocumentService, "get_by_kb_id", fake_get_by_kb_id)
        monkeypatch.setattr(module, "turn2jsonschema", lambda _meta: {"schema": True})

        async def fake_request_json():
            return {}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == 1
        assert res["data"]["docs"][0]["parser_config"]["metadata"] == {"schema": True}

    def test_exception_path(self, document_app_module, monkeypatch):
        module = document_app_module
        self._set_args(module, monkeypatch, kb_id="kb1")
        self._allow_kb(module, monkeypatch)

        def raise_error(*_args, **_kwargs):
            raise RuntimeError("boom")

        monkeypatch.setattr(module.DocumentService, "get_by_kb_id", raise_error)

        async def fake_request_json():
            return {}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.list_docs())
        assert res["code"] == 100
