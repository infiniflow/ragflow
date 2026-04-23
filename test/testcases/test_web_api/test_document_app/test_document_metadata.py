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
from test_common import (
    document_change_status,
    document_filter,
    document_infos,
    document_metadata_summary,
    document_update_metadata_setting,
    bulk_upload_documents,
    delete_document,
    list_documents,
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
        res = document_filter(invalid_auth, "kb_id", {})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_infos_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_infos(invalid_auth, "kb_id", {"doc_ids": ["doc_id"]})
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
    def test_change_status_auth_invalid(self, invalid_auth, expected_code, expected_fragment, add_dataset_func):
        dataset_id = add_dataset_func
        res = document_change_status(invalid_auth, dataset_id, {"doc_ids": ["doc_id"], "status": "1"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

class TestDocumentMetadata:
    @pytest.mark.p2
    def test_filter(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        res = document_filter(WebApiAuth, kb_id, {})
        assert res["code"] == 0, res
        assert "filter" in res["data"], res
        assert "total" in res["data"], res

    @pytest.mark.p2
    def test_infos(self, WebApiAuth, add_document_func):
        dataset_id, doc_id = add_document_func
        res = document_infos(WebApiAuth, dataset_id, {"ids": [doc_id]})
        assert res["code"] == 0, res
        docs = res["data"]["docs"]
        assert len(docs) == 1, docs
        assert docs[0]["id"] == doc_id, res

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
        dataset_id, doc_id = add_document_func
        res = document_change_status(WebApiAuth, dataset_id, {"doc_ids": [doc_id], "status": "1"})

        assert res["code"] == 0, res
        assert res["data"][doc_id]["status"] == "1", res
        info_res = document_infos(WebApiAuth, dataset_id, {"ids": [doc_id]})

        assert info_res["code"] == 0, info_res
        assert info_res["data"]["docs"][0]["status"] == "1", info_res


class TestDocumentMetadataNegative:
    @pytest.mark.p2
    def test_filter_missing_kb_id(self, WebApiAuth, add_document_func):
        kb_id, doc_id = add_document_func
        res = document_filter(WebApiAuth, "", {"ids": [doc_id]})
        assert res["code"] == 100, res
        assert "<MethodNotAllowed '405: Method Not Allowed'>" == res["message"], res

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
        dataset_id, doc_id = add_document_func
        res = document_change_status(WebApiAuth, dataset_id, {"doc_ids": [doc_id], "status": "2"})
        assert res["code"] == 101, res
        assert "Status" in res["message"], res


def _run(coro):
    return asyncio.run(coro)


class _DummyArgs:
    def __init__(self, args=None):
        self._args = args or {}

    def get(self, key, default=None):
        return self._args.get(key, default)

    def getlist(self, key):
        value = self._args.get(key, [])
        if isinstance(value, list):
            return value
        return [value]


class _DummyRequest:
    def __init__(self, args=None):
        self.args = _DummyArgs(args)


class _DummyResponse:
    def __init__(self, data=None):
        self.data = data
        self.headers = {}


@pytest.mark.p2
class TestDocumentMetadataUnit:
    def _allow_kb(self, module, monkeypatch, kb_id="kb1", tenant_id="tenant1"):
        monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id=tenant_id)])
        monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: True if _kwargs.get("id") == kb_id else False)

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

    def test_metadata_update_invalid_delete_item_unit(self, document_app_module, monkeypatch):
        module = document_app_module

        async def fake_request_json():
            return {"kb_id": "kb1", "doc_ids": ["doc1"], "updates": [], "deletes": [{}]}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.metadata_update.__wrapped__())
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert "Each delete requires key." in res["message"]

    def test_update_metadata_setting_authorization_and_refetch_not_found_unit(self, document_app_module, monkeypatch):
        module = document_app_module

        async def fake_request_json():
            return {"doc_id": "doc1", "metadata": {"author": "alice"}}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: False)
        res = _run(module.update_metadata_setting.__wrapped__())
        assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
        assert "No authorization." in res["message"]

        doc = SimpleNamespace(id="doc1", to_dict=lambda: {"id": "doc1", "parser_config": {}})
        state = {"count": 0}

        def fake_get_by_id(_doc_id):
            state["count"] += 1
            if state["count"] == 1:
                return True, doc
            return False, None

        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_by_id", fake_get_by_id)
        monkeypatch.setattr(module.DocumentService, "update_parser_config", lambda *_args, **_kwargs: True)
        res = _run(module.update_metadata_setting.__wrapped__())
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Document not found!" in res["message"]

    def test_thumbnails_missing_ids_rewrite_and_exception_unit(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "request", _DummyRequest(args={}))
        res = module.thumbnails()
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert 'Lack of "Document ID"' in res["message"]

        monkeypatch.setattr(module, "request", _DummyRequest(args={"doc_ids": ["doc1", "doc2"]}))
        monkeypatch.setattr(
            module.DocumentService,
            "get_thumbnails",
            lambda _doc_ids: [
                {"id": "doc1", "kb_id": "kb1", "thumbnail": "thumb.jpg"},
                {"id": "doc2", "kb_id": "kb1", "thumbnail": f"{module.IMG_BASE64_PREFIX}blob"},
            ],
        )
        res = module.thumbnails()
        assert res["code"] == 0
        assert res["data"]["doc1"] == "/v1/document/image/kb1-thumb.jpg"
        assert res["data"]["doc2"] == f"{module.IMG_BASE64_PREFIX}blob"

        def raise_error(*_args, **_kwargs):
            raise RuntimeError("thumb boom")

        monkeypatch.setattr(module.DocumentService, "get_thumbnails", raise_error)
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = module.thumbnails()
        assert res["code"] == 500
        assert "thumb boom" in res["message"]

    @pytest.mark.p2
    def test_change_status_partial_failure_matrix(self, WebApiAuth, add_dataset, ragflow_tmp_dir):
        """
        E2E test for partial failure matrix in batch document status change.
        
        This test creates multiple documents and verifies that the batch status change
        operation handles various failure scenarios correctly.
        """
        
        dataset_id = add_dataset
        
        # Create multiple documents for testing
        doc_ids = bulk_upload_documents(WebApiAuth, dataset_id, 3, ragflow_tmp_dir)
        assert len(doc_ids) == 3, f"Expected 3 documents, got {len(doc_ids)}"
        
        try:
            # Test batch status change with all valid documents
            # This should succeed since all documents are valid
            res = document_change_status(WebApiAuth, dataset_id, {"doc_ids": doc_ids, "status": "1"})
            
            # Verify the response structure
            assert res["code"] == 0, f"Expected success code 0, got {res}"
            assert res["data"] is not None, "Response data should not be None"
            
            # Verify each document status was updated
            for doc_id in doc_ids:
                assert doc_id in res["data"], f"Document {doc_id} should be in response"
                assert res["data"][doc_id]["status"] == "1", f"Document {doc_id} status should be 1"
            
            # Verify the status was actually updated in the database
            info_res = document_infos(WebApiAuth, dataset_id, {"ids": doc_ids})
            assert info_res["code"] == 0, info_res
            
            for doc in info_res["data"]["docs"]:
                assert doc["status"] == "1", f"Document {doc['id']} status should be 1 in database"
                
        finally:
            # Cleanup: delete all documents
            delete_document(WebApiAuth, dataset_id, {"ids": doc_ids})

    @pytest.mark.p2
    def test_change_status_invalid_status(self, WebApiAuth, add_document_func):
        """
        E2E test for invalid status value in batch document status change.
        
        This test verifies that the API returns an error when an invalid status
        value (not 0 or 1) is provided.
        """
        
        dataset_id, doc_id = add_document_func
        
        # Try to update with invalid status "2" (only 0 and 1 are valid)
        res = document_change_status(WebApiAuth, dataset_id, {"doc_ids": [doc_id], "status": "2"})
        
        # Verify the error response
        assert res["code"] == 101, f"Expected error code 101, got {res}"
        assert "Status" in res["message"], f"Error message should mention Status: {res}"

    @pytest.mark.p2
    def test_change_status_all_success(self, WebApiAuth, add_document_func):
        """
        E2E test for successful batch document status change.
        
        This test verifies that all documents are successfully updated
        when valid status values are provided.
        """
        
        dataset_id, doc_id = add_document_func
        
        # Verify initial status is "0" (unprocessed)
        info_res = document_infos(WebApiAuth, dataset_id, {"ids": [doc_id]})
        assert info_res["code"] == 0, info_res
        assert info_res["data"]["docs"][0]["status"] == "1", "Initial status should be 1"
        
        # Update status to "1" (processed)
        res = document_change_status(WebApiAuth, dataset_id, {"doc_ids": [doc_id], "status": "1"})
        
        # Verify success
        assert res["code"] == 0, f"Expected success code 0, got {res}"
        assert res["data"][doc_id]["status"] == "1", f"Document status should be 1"
        
        # Verify the status was actually updated in the database
        info_res = document_infos(WebApiAuth, dataset_id, {"ids": [doc_id]})
        assert info_res["code"] == 0, info_res
        assert info_res["data"]["docs"][0]["status"] == "1", "Document status should be 1 in database"

    def test_get_route_not_found_success_and_exception_unit(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (False, None))
        res = _run(module.get("doc1"))
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Document not found!" in res["message"]

        async def fake_thread_pool_exec(*_args, **_kwargs):
            return b"blob-data"

        async def fake_make_response(data):
            return _DummyResponse(data)

        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, SimpleNamespace(name="image.abc", type=module.FileType.VISUAL.value)))
        monkeypatch.setattr(module.File2DocumentService, "get_storage_address", lambda **_kwargs: ("bucket", "name"))
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(get=lambda *_args, **_kwargs: b"blob-data"))
        monkeypatch.setattr(module, "thread_pool_exec", fake_thread_pool_exec)
        monkeypatch.setattr(module, "make_response", fake_make_response)
        monkeypatch.setattr(
            module,
            "apply_safe_file_response_headers",
            lambda response, content_type, extension: response.headers.update({"content_type": content_type, "extension": extension}),
        )
        res = _run(module.get("doc1"))
        assert isinstance(res, _DummyResponse)
        assert res.data == b"blob-data"
        assert res.headers["content_type"] == "image/abc"
        assert res.headers["extension"] == "abc"

        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (_ for _ in ()).throw(RuntimeError("get boom")))
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.get("doc1"))
        assert res["code"] == 500
        assert "get boom" in res["message"]

    def test_download_attachment_success_and_exception_unit(self, document_app_module, monkeypatch):
        module = document_app_module
        monkeypatch.setattr(module, "request", _DummyRequest(args={"ext": "abc"}))

        async def fake_thread_pool_exec(*_args, **_kwargs):
            return b"attachment"

        async def fake_make_response(data):
            return _DummyResponse(data)

        monkeypatch.setattr(module, "thread_pool_exec", fake_thread_pool_exec)
        monkeypatch.setattr(module, "make_response", fake_make_response)
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(get=lambda *_args, **_kwargs: b"attachment"))
        monkeypatch.setattr(
            module,
            "apply_safe_file_response_headers",
            lambda response, content_type, extension: response.headers.update({"content_type": content_type, "extension": extension}),
        )
        res = _run(module.download_attachment("att1"))
        assert isinstance(res, _DummyResponse)
        assert res.data == b"attachment"
        assert res.headers["content_type"] == "application/abc"
        assert res.headers["extension"] == "abc"

        async def raise_error(*_args, **_kwargs):
            raise RuntimeError("download boom")

        monkeypatch.setattr(module, "thread_pool_exec", raise_error)
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.download_attachment("att1"))
        assert res["code"] == 500
        assert "download boom" in res["message"]

    def test_change_parser_guards_and_reset_update_failure_unit(self, document_app_module, monkeypatch):
        module = document_app_module

        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})

        async def req_auth_fail():
            return {"doc_id": "doc1", "parser_id": "naive", "pipeline_id": "pipe2"}

        monkeypatch.setattr(module, "get_request_json", req_auth_fail)
        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: False)
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == module.RetCode.AUTHENTICATION_ERROR

        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (False, None))
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == module.RetCode.DATA_ERROR
        assert "Document not found!" in res["message"]

        async def req_same_pipeline():
            return {"doc_id": "doc1", "parser_id": "naive", "pipeline_id": "pipe1"}

        doc_same = SimpleNamespace(
            id="doc1",
            pipeline_id="pipe1",
            parser_id="naive",
            parser_config={"k": "v"},
            token_num=0,
            chunk_num=0,
            process_duration=0,
            kb_id="kb1",
            type="doc",
            name="doc.txt",
        )
        monkeypatch.setattr(module, "get_request_json", req_same_pipeline)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, doc_same))
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0

        calls = []

        async def req_pipeline_change():
            return {"doc_id": "doc1", "parser_id": "naive", "pipeline_id": "pipe2"}

        doc = SimpleNamespace(
            id="doc1",
            pipeline_id="pipe1",
            parser_id="naive",
            parser_config={},
            token_num=0,
            chunk_num=0,
            process_duration=0,
            kb_id="kb1",
            type="doc",
            name="doc.txt",
        )

        def fake_update_by_id(doc_id, payload):
            calls.append((doc_id, payload))
            return True

        monkeypatch.setattr(module, "get_request_json", req_pipeline_change)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, doc))
        monkeypatch.setattr(module.DocumentService, "update_by_id", fake_update_by_id)
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0
        assert calls[0][1] == {"pipeline_id": "pipe2"}
        assert calls[1][1]["run"] == module.TaskStatus.UNSTART.value

        doc.token_num = 3
        doc.chunk_num = 2
        doc.process_duration = 9
        monkeypatch.setattr(module.DocumentService, "increment_chunk_num", lambda *_args, **_kwargs: False)
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0

        monkeypatch.setattr(module.DocumentService, "increment_chunk_num", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: None)
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0

        side_effects = {"img": [], "delete": []}

        class _DocStore:
            def index_exist(self, _idx, _kb_id):
                return True

            def delete(self, where, _idx, kb_id):
                side_effects["delete"].append((where["doc_id"], kb_id))

        monkeypatch.setattr(module.DocumentService, "get_tenant_id", lambda _doc_id: "tenant1")
        monkeypatch.setattr(module.DocumentService, "delete_chunk_images", lambda _doc, _tenant: side_effects["img"].append((_doc.id, _tenant)))
        monkeypatch.setattr(module.search, "index_name", lambda tenant_id: f"idx_{tenant_id}")
        monkeypatch.setattr(module.settings, "docStoreConn", _DocStore())
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0
        assert ("doc1", "tenant1") in side_effects["img"]
        assert ("doc1", "kb1") in side_effects["delete"]

        async def req_same_parser_with_cfg():
            return {"doc_id": "doc1", "parser_id": "naive", "parser_config": {"a": 1}}

        doc_same_parser = SimpleNamespace(
            id="doc1",
            pipeline_id="pipe1",
            parser_id="naive",
            parser_config={"a": 1},
            token_num=0,
            chunk_num=0,
            process_duration=0,
            kb_id="kb1",
            type="doc",
            name="doc.txt",
        )
        monkeypatch.setattr(module, "get_request_json", req_same_parser_with_cfg)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, doc_same_parser))
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0

        async def req_same_parser_no_cfg():
            return {"doc_id": "doc1", "parser_id": "naive"}

        monkeypatch.setattr(module, "get_request_json", req_same_parser_no_cfg)
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0

        parser_cfg_updates = []

        async def req_parser_update():
            return {"doc_id": "doc1", "parser_id": "paper", "pipeline_id": "", "parser_config": {"beta": True}}

        doc_parser_update = SimpleNamespace(
            id="doc1",
            pipeline_id="pipe1",
            parser_id="naive",
            parser_config={"alpha": 1},
            token_num=0,
            chunk_num=0,
            process_duration=0,
            kb_id="kb1",
            type="doc",
            name="doc.txt",
        )
        monkeypatch.setattr(module, "get_request_json", req_parser_update)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, doc_parser_update))
        monkeypatch.setattr(module.DocumentService, "update_parser_config", lambda doc_id, cfg: parser_cfg_updates.append((doc_id, cfg)))
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 0
        assert parser_cfg_updates == [("doc1", {"beta": True})]

        def raise_parser_config(*_args, **_kwargs):
            raise RuntimeError("parser boom")

        monkeypatch.setattr(module.DocumentService, "update_parser_config", raise_parser_config)
        res = _run(module.change_parser.__wrapped__())
        assert res["code"] == 500
        assert "parser boom" in res["message"]

    def test_get_image_success_and_exception_unit(self, document_app_module, monkeypatch):
        module = document_app_module

        class _Headers(dict):
            def set(self, key, value):
                self[key] = value

        class _ImageResponse:
            def __init__(self, data):
                self.data = data
                self.headers = _Headers()

        async def fake_thread_pool_exec(*_args, **_kwargs):
            return b"image-bytes"

        async def fake_make_response(data):
            return _ImageResponse(data)

        monkeypatch.setattr(module, "thread_pool_exec", fake_thread_pool_exec)
        monkeypatch.setattr(module, "make_response", fake_make_response)
        monkeypatch.setattr(module.settings, "STORAGE_IMPL", SimpleNamespace(get=lambda *_args, **_kwargs: b"image-bytes"))
        res = _run(module.get_image("bucket-name"))
        assert isinstance(res, _ImageResponse)
        assert res.data == b"image-bytes"
        assert res.headers["Content-Type"] == "image/JPEG"

        async def raise_error(*_args, **_kwargs):
            raise RuntimeError("image boom")

        monkeypatch.setattr(module, "thread_pool_exec", raise_error)
        monkeypatch.setattr(module, "server_error_response", lambda e: {"code": 500, "message": str(e)})
        res = _run(module.get_image("bucket-name"))
        assert res["code"] == 500
        assert "image boom" in res["message"]
