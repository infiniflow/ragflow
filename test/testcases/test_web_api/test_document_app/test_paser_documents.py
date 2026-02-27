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
from common import bulk_upload_documents, list_documents, parse_documents
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils import wait_for


def _run(coro):
    return asyncio.run(coro)


@wait_for(30, 1, "Document parsing timeout")
def condition(_auth, _kb_id, _document_ids=None):
    res = list_documents(_auth, {"kb_id": _kb_id})
    target_docs = res["data"]["docs"]

    if _document_ids is None:
        for doc in target_docs:
            if doc["run"] != "3":
                return False
        return True

    target_ids = set(_document_ids)
    for doc in target_docs:
        if doc["id"] in target_ids:
            if doc.get("run") != "3":
                return False
    return True


def validate_document_parse_done(auth, _kb_id, _document_ids):
    res = list_documents(auth, {"kb_id": _kb_id})
    for doc in res["data"]["docs"]:
        if doc["id"] not in _document_ids:
            continue
        assert doc["run"] == "3"
        assert len(doc["process_begin_at"]) > 0
        assert doc["process_duration"] > 0
        assert doc["progress"] > 0
        assert "Task done" in doc["progress_msg"]


def validate_document_parse_cancel(auth, _kb_id, _document_ids):
    res = list_documents(auth, {"kb_id": _kb_id})
    for doc in res["data"]["docs"]:
        if doc["id"] not in _document_ids:
            continue
        assert doc["run"] == "2"
        assert len(doc["process_begin_at"]) > 0
        assert doc["progress"] == 0.0


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
        res = parse_documents(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDocumentsParse:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            pytest.param(None, 101, "required argument are missing: doc_ids, run; ", marks=pytest.mark.skip),
            pytest.param({"doc_ids": [], "run": "1"}, 0, "", marks=pytest.mark.p1),
            pytest.param({"doc_ids": ["invalid_id"], "run": "1"}, 109, "No authorization.", marks=pytest.mark.p3),
            pytest.param({"doc_ids": ["\n!?。；！？\"'"], "run": "1"}, 109, "No authorization.", marks=pytest.mark.p3),
            pytest.param("not json", 101, "required argument are missing: doc_ids, run; ", marks=pytest.mark.skip),
            pytest.param(lambda r: {"doc_ids": r[:1], "run": "1"}, 0, "", marks=pytest.mark.p1),
            pytest.param(lambda r: {"doc_ids": r, "run": "1"}, 0, "", marks=pytest.mark.p1),
        ],
    )
    def test_basic_scenarios(self, WebApiAuth, add_documents_func, payload, expected_code, expected_message):
        kb_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = parse_documents(WebApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            condition(WebApiAuth, kb_id, payload["doc_ids"])
            validate_document_parse_done(WebApiAuth, kb_id, payload["doc_ids"])
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"doc_ids": ["invalid_id"] + r, "run": "1"}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"doc_ids": r[:1] + ["invalid_id"] + r[1:3], "run": "1"}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"doc_ids": r + ["invalid_id"], "run": "1"}, marks=pytest.mark.p3),
        ],
    )
    def test_parse_partial_invalid_document_id(self, WebApiAuth, add_documents_func, payload):
        _, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = parse_documents(WebApiAuth, payload)
        assert res["code"] == 109, res
        assert res["message"] == "No authorization.", res

    @pytest.mark.p3
    def test_repeated_parse(self, WebApiAuth, add_documents_func):
        kb_id, document_ids = add_documents_func
        res = parse_documents(WebApiAuth, {"doc_ids": document_ids, "run": "1"})
        assert res["code"] == 0, res

        condition(WebApiAuth, kb_id, document_ids)

        res = parse_documents(WebApiAuth, {"doc_ids": document_ids, "run": "1"})
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_duplicate_parse(self, WebApiAuth, add_documents_func):
        kb_id, document_ids = add_documents_func
        res = parse_documents(WebApiAuth, {"doc_ids": document_ids + document_ids, "run": "1"})
        assert res["code"] == 0, res
        assert res["message"] == "success", res

        condition(WebApiAuth, kb_id, document_ids)
        validate_document_parse_done(WebApiAuth, kb_id, document_ids)


@pytest.mark.p3
def test_parse_100_files(WebApiAuth, add_dataset_func, tmp_path):
    @wait_for(100, 1, "Document parsing timeout")
    def condition(_auth, _kb_id, _document_num):
        res = list_documents(_auth, {"kb_id": _kb_id, "page_size": _document_num})
        for doc in res["data"]["docs"]:
            if doc["run"] != "3":
                return False
        return True

    document_num = 100
    kb_id = add_dataset_func
    document_ids = bulk_upload_documents(WebApiAuth, kb_id, document_num, tmp_path)
    res = parse_documents(WebApiAuth, {"doc_ids": document_ids, "run": "1"})
    assert res["code"] == 0, res

    condition(WebApiAuth, kb_id, document_num)

    validate_document_parse_done(WebApiAuth, kb_id, document_ids)


@pytest.mark.p3
def test_concurrent_parse(WebApiAuth, add_dataset_func, tmp_path):
    @wait_for(120, 1, "Document parsing timeout")
    def condition(_auth, _kb_id, _document_num):
        res = list_documents(_auth, {"kb_id": _kb_id, "page_size": _document_num})
        for doc in res["data"]["docs"]:
            if doc["run"] != "3":
                return False
        return True

    count = 100
    kb_id = add_dataset_func
    document_ids = bulk_upload_documents(WebApiAuth, kb_id, count, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(
                parse_documents,
                WebApiAuth,
                {"doc_ids": [document_ids[i]], "run": "1"},
            )
            for i in range(count)
        ]
    responses = list(as_completed(futures))
    assert len(responses) == count, responses
    assert all(future.result()["code"] == 0 for future in futures)

    condition(WebApiAuth, kb_id, count)

    validate_document_parse_done(WebApiAuth, kb_id, document_ids)


@pytest.mark.p2
class TestDocumentsParseUnit:
    def test_run_branch_matrix_unit(self, document_app_module, monkeypatch):
        module = document_app_module
        calls = {"clear": [], "filter_delete": [], "docstore_delete": [], "cancel": [], "run": []}

        async def fake_thread_pool_exec(func, *args, **kwargs):
            return func(*args, **kwargs)

        monkeypatch.setattr(module, "thread_pool_exec", fake_thread_pool_exec)
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        monkeypatch.setattr(module.search, "index_name", lambda tenant_id: f"idx_{tenant_id}")
        monkeypatch.setattr(module, "cancel_all_task_of", lambda doc_id: calls["cancel"].append(doc_id))

        class _DocStore:
            def index_exist(self, _index_name, _kb_id):
                return True

            def delete(self, where, _index_name, _kb_id):
                calls["docstore_delete"].append(where["doc_id"])

        monkeypatch.setattr(module.settings, "docStoreConn", _DocStore())

        async def set_request(payload):
            return payload

        def apply_request(payload):
            async def fake_request_json():
                return await set_request(payload)

            monkeypatch.setattr(module, "get_request_json", fake_request_json)

        apply_request({"doc_ids": ["doc1"], "run": module.TaskStatus.RUNNING.value})
        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: False)
        res = _run(module.run.__wrapped__())
        assert res["code"] == module.RetCode.AUTHENTICATION_ERROR

        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: None)
        res = _run(module.run.__wrapped__())
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Tenant not found!" in res["message"]

        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant1")
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (False, None))
        res = _run(module.run.__wrapped__())
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Document not found!" in res["message"]

        apply_request({"doc_ids": ["doc1"], "run": module.TaskStatus.CANCEL.value})
        doc_cancel = SimpleNamespace(id="doc1", run=module.TaskStatus.DONE.value, kb_id="kb1", parser_config={}, to_dict=lambda: {"id": "doc1"})
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, doc_cancel))
        monkeypatch.setattr(module.TaskService, "query", lambda **_kwargs: [SimpleNamespace(progress=1)])
        res = _run(module.run.__wrapped__())
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Cannot cancel a task that is not in RUNNING status" in res["message"]

        apply_request({"doc_ids": ["doc1"], "run": module.TaskStatus.RUNNING.value, "delete": True})
        doc_rerun = SimpleNamespace(id="doc1", run=module.TaskStatus.DONE.value, kb_id="kb1", parser_config={}, to_dict=lambda: {"id": "doc1"})
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, doc_rerun))
        monkeypatch.setattr(module.DocumentService, "clear_chunk_num_when_rerun", lambda doc_id: calls["clear"].append(doc_id))
        monkeypatch.setattr(module.TaskService, "filter_delete", lambda _filters: calls["filter_delete"].append(True))
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "run", lambda tenant_id, doc_dict, _kb_map: calls["run"].append((tenant_id, doc_dict)))
        res = _run(module.run.__wrapped__())
        assert res["code"] == 0
        assert calls["clear"] == ["doc1"]
        assert calls["filter_delete"] == [True]
        assert calls["docstore_delete"] == ["doc1"]
        assert calls["run"] == [("tenant1", {"id": "doc1"})]

        apply_request({"doc_ids": ["doc1"], "run": module.TaskStatus.RUNNING.value, "apply_kb": True})
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
        res = _run(module.run.__wrapped__())
        assert res["code"] == 500
        assert "Can't find this dataset!" in res["message"]

        apply_request({"doc_ids": ["doc1"], "run": module.TaskStatus.RUNNING.value})

        def raise_run_error(*_args, **_kwargs):
            raise RuntimeError("run boom")

        monkeypatch.setattr(module.DocumentService, "run", raise_run_error)
        res = _run(module.run.__wrapped__())
        assert res["code"] == 500
        assert "run boom" in res["message"]


# @pytest.mark.skip
class TestDocumentsParseStop:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            pytest.param(None, 101, "required argument are missing: doc_ids, run; ", marks=pytest.mark.skip),
            pytest.param({"doc_ids": [], "run": "2"}, 0, "", marks=pytest.mark.p1),
            pytest.param({"doc_ids": ["invalid_id"], "run": "2"}, 109, "No authorization.", marks=pytest.mark.p3),
            pytest.param({"doc_ids": ["\n!?。；！？\"'"], "run": "2"}, 109, "No authorization.", marks=pytest.mark.p3),
            pytest.param("not json", 101, "required argument are missing: doc_ids, run; ", marks=pytest.mark.skip),
            pytest.param(lambda r: {"doc_ids": r[:1], "run": "2"}, 0, "", marks=pytest.mark.p1),
            pytest.param(lambda r: {"doc_ids": r, "run": "2"}, 0, "", marks=pytest.mark.p1),
        ],
    )
    def test_basic_scenarios(self, WebApiAuth, add_documents_func, payload, expected_code, expected_message):
        @wait_for(10, 1, "Document parsing timeout")
        def condition(_auth, _kb_id, _doc_ids):
            res = list_documents(_auth, {"kb_id": _kb_id})
            for doc in res["data"]["docs"]:
                if doc["id"] in _doc_ids:
                    if doc["run"] != "3":
                        return False
            return True

        kb_id, document_ids = add_documents_func
        parse_documents(WebApiAuth, {"doc_ids": document_ids, "run": "1"})

        if callable(payload):
            payload = payload(document_ids)

        res = parse_documents(WebApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            completed_document_ids = list(set(document_ids) - set(payload["doc_ids"]))
            condition(WebApiAuth, kb_id, completed_document_ids)
            validate_document_parse_cancel(WebApiAuth, kb_id, payload["doc_ids"])
            validate_document_parse_done(WebApiAuth, kb_id, completed_document_ids)
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.skip
    @pytest.mark.parametrize(
        "payload",
        [
            lambda r: {"doc_ids": ["invalid_id"] + r, "run": "2"},
            lambda r: {"doc_ids": r[:1] + ["invalid_id"] + r[1:3], "run": "2"},
            lambda r: {"doc_ids": r + ["invalid_id"], "run": "2"},
        ],
    )
    def test_stop_parse_partial_invalid_document_id(self, WebApiAuth, add_documents_func, payload):
        kb_id, document_ids = add_documents_func
        parse_documents(WebApiAuth, {"doc_ids": document_ids, "run": "1"})

        if callable(payload):
            payload = payload(document_ids)
        res = parse_documents(WebApiAuth, payload)
        assert res["code"] == 109, res
        assert res["message"] == "No authorization.", res

        validate_document_parse_cancel(WebApiAuth, kb_id, document_ids)
