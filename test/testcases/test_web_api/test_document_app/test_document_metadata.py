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
    delete_document,
    document_change_status,
    document_filter,
    document_infos,
    document_metadata_summary,
    document_metadata_update,
    document_update,
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
        res = document_update_metadata_setting(invalid_auth, "kb_id", "doc_id", {"metadata": {}})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_change_status_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = document_change_status(invalid_auth, {"doc_ids": ["doc_id"], "status": "1"})
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
        res = document_change_status(WebApiAuth, {"doc_ids": [doc_id], "status": "1"})

        assert res["code"] == 0, res
        assert res["data"][doc_id]["status"] == "1", res
        info_res = document_infos(WebApiAuth, dataset_id, {"ids": [doc_id]})

        assert info_res["code"] == 0, info_res
        assert info_res["data"]["docs"][0]["status"] == "1", info_res


    @pytest.mark.p2
    def test_update_document_change_parser(self, WebApiAuth, add_document_func):
        """Test updating document chunk_method via PATCH /api/v1/datasets/<dataset_id>/documents/<doc_id>."""
        dataset_id, doc_id = add_document_func

        # Get initial document info
        res = document_infos(WebApiAuth, dataset_id, {"doc_ids": [doc_id]})

        assert res["code"] == 0, res
        original_parser_id = res["data"]["docs"][0].get("parser_id")

        res = document_update(WebApiAuth, dataset_id, doc_id, {"chunk_method": "invalid_chunk_method"})
        assert res["code"] == 102, res
        assert res["message"] == "Field: <chunk_method> - Message: <`chunk_method` invalid_chunk_method doesn't exist> - Value: <invalid_chunk_method>", res

        # Change to a different parser (naive bayes)
        # valid_chunk_method = {"naive", "manual", "qa", "table", "paper", "book", "laws", "presentation", "picture", "one", "knowledge_graph", "email", "tag"}
        new_parser_id = "naive"
        if original_parser_id == new_parser_id:
            new_parser_id = "paper"
        document_update(WebApiAuth, dataset_id, doc_id, {"chunk_method": new_parser_id})

        # Verify the document was updated
        res = document_infos(WebApiAuth, dataset_id, {"doc_ids": [doc_id]})

        assert res["code"] == 0, res
        assert res["data"]["docs"][0]["chunk_method"] == new_parser_id, res


    @pytest.mark.p2
    def test_update_document_change_pipeline(self, WebApiAuth, add_document_func):
        """Test updating document pipeline via PATCH /api/v1/datasets/<dataset_id>/documents/<doc_id>."""
        dataset_id, doc_id = add_document_func

        # Get initial document info
        res = document_infos(WebApiAuth, dataset_id, {"doc_ids": [doc_id]})
        assert res["code"] == 0, res
        original_pipeline_id = res["data"]["docs"][0].get("pipeline_id")

        # Change to a different pipeline (if available)
        # Note: This test assumes there's at least one other pipeline available
        new_pipeline_id = "general" if original_pipeline_id != "general" else "resume"
        res = document_update(WebApiAuth, dataset_id, doc_id, {"pipeline_id": new_pipeline_id})
        assert res["code"] == 0, res

        # Verify the document was updated
        res = document_infos(WebApiAuth, dataset_id, {"doc_ids": [doc_id]})
        assert res["code"] == 0, res
        assert res["data"]["docs"][0]["pipeline_id"] == new_pipeline_id, res


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

    @pytest.mark.p2
    def test_update_metadata_setting_not_found(self, WebApiAuth, add_document_func):
        """Test updating metadata setting for a non-existent document returns error."""
        dataset_id, doc_id = add_document_func
        # First delete the document
        delete_res = delete_document(WebApiAuth, dataset_id, {"ids": [doc_id]})
        assert delete_res["code"] == 0, delete_res

        # Now try to update metadata setting for the deleted document
        res = document_update_metadata_setting(WebApiAuth, dataset_id, doc_id, {"metadata": {"author": "test"}})
        assert res["code"] == 102, res
        assert f"Document {doc_id} not found in dataset {dataset_id}" in res["message"], res

    @pytest.mark.p3
    def test_change_status_invalid_status(self, WebApiAuth, add_document_func):
        _, doc_id = add_document_func
        res = document_change_status(WebApiAuth, {"doc_ids": [doc_id], "status": "2"})
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

    @pytest.mark.p3
    def test_update_metadata_missing_dataset_id(self, WebApiAuth, add_document_func):
        """Test the new unified update_metadata API - missing dataset_id."""
        # Call with empty dataset_id (should fail validation)
        res = document_metadata_update(WebApiAuth, "", {"dataset_id": "", "selector": {"document_ids": ["doc1"]}, "updates": []})
        assert res["code"] == 404
        assert res["message"] == "Not Found: /api/v1/datasets//documents/metadatas", res

    @pytest.mark.p3
    def test_update_metadata_success(self, WebApiAuth, add_document_func):
        """Test the new unified update_metadata API - success case."""
        kb_id, doc_id = add_document_func
        res = document_metadata_update(
            WebApiAuth, kb_id,
            {
                "selector": {"document_ids": [doc_id]},
                "updates": [{"key": "author", "value": "test_author"}],
                "deletes": []
            }
        )
        assert res["code"] == 0, res


    @pytest.mark.p3
    def test_update_metadata_invalid_delete_item(self, WebApiAuth, add_document_func):
        """Test the new unified update_metadata API - invalid delete item."""
        kb_id, doc_id = add_document_func
        res = document_metadata_update(
            WebApiAuth, kb_id,
            {
                "selector": {"document_ids": [doc_id]},
                "updates": [],
                "deletes": [{}]  # Invalid - missing key
            }
        )
        assert res["code"] == 102
        assert "Each delete requires key" in res["message"], res


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

    def test_change_status_partial_failure_matrix_unit(self, document_app_module, monkeypatch):
        module = document_app_module
        calls = {"docstore_update": []}
        doc_ids = ["unauth", "missing_doc", "missing_kb", "update_fail", "docstore_3022", "docstore_generic", "outer_exc"]

        async def fake_request_json():
            return {"doc_ids": doc_ids, "status": "1"}

        def fake_accessible(doc_id, _uid):
            return doc_id != "unauth"

        def fake_get_by_id(doc_id):
            if doc_id == "missing_doc":
                return False, None
            if doc_id == "outer_exc":
                raise RuntimeError("explode")
            kb_id = "kb_missing" if doc_id == "missing_kb" else "kb1"
            chunk_num = 1 if doc_id in {"docstore_3022", "docstore_generic"} else 0
            doc = SimpleNamespace(id=doc_id, kb_id=kb_id, status="0", chunk_num=chunk_num)
            return True, doc

        def fake_get_kb(kb_id):
            if kb_id == "kb_missing":
                return False, None
            return True, SimpleNamespace(tenant_id="tenant1")

        def fake_update_by_id(doc_id, _payload):
            return doc_id != "update_fail"

        class _DocStore:
            def update(self, where, _payload, _index_name, _kb_id):
                calls["docstore_update"].append(where["doc_id"])
                if where["doc_id"] == "docstore_3022":
                    raise RuntimeError("3022 table missing")
                if where["doc_id"] == "docstore_generic":
                    raise RuntimeError("doc store down")
                return True

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.DocumentService, "accessible", fake_accessible)
        monkeypatch.setattr(module.DocumentService, "get_by_id", fake_get_by_id)
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda kb_id: fake_get_kb(kb_id))
        monkeypatch.setattr(module.DocumentService, "update_by_id", fake_update_by_id)
        monkeypatch.setattr(module.settings, "docStoreConn", _DocStore())
        monkeypatch.setattr(module.search, "index_name", lambda tenant_id: f"idx_{tenant_id}")

        res = _run(module.change_status.__wrapped__())
        assert res["code"] == module.RetCode.SERVER_ERROR
        assert res["message"] == "Partial failure"
        assert res["data"]["unauth"]["error"] == "No authorization."
        assert res["data"]["missing_doc"]["error"] == "No authorization."
        assert res["data"]["missing_kb"]["error"] == "Can't find this dataset!"
        assert res["data"]["update_fail"]["error"] == "Database error (Document update)!"
        assert res["data"]["docstore_3022"]["error"] == "Document store table missing."
        assert "Document store update failed:" in res["data"]["docstore_generic"]["error"]
        assert "Internal server error: explode" == res["data"]["outer_exc"]["error"]
        assert calls["docstore_update"] == ["docstore_3022", "docstore_generic"]

    def test_change_status_invalid_status_unit(self, document_app_module, monkeypatch):
        module = document_app_module

        async def fake_request_json():
            return {"doc_ids": ["doc1"], "status": "2"}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        res = _run(module.change_status.__wrapped__())
        assert res["code"] == module.RetCode.ARGUMENT_ERROR
        assert '"Status" must be either 0 or 1!' in res["message"]

    def test_change_status_all_success_unit(self, document_app_module, monkeypatch):
        module = document_app_module

        async def fake_request_json():
            return {"doc_ids": ["doc1"], "status": "1"}

        monkeypatch.setattr(module, "get_request_json", fake_request_json)
        monkeypatch.setattr(module.DocumentService, "accessible", lambda *_args, **_kwargs: True)
        monkeypatch.setattr(module.DocumentService, "get_by_id", lambda _doc_id: (True, SimpleNamespace(id="doc1", kb_id="kb1", status="0", chunk_num=0)))
        monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (True, SimpleNamespace(tenant_id="tenant1")))
        monkeypatch.setattr(module.DocumentService, "update_by_id", lambda *_args, **_kwargs: True)
        res = _run(module.change_status.__wrapped__())
        assert res["code"] == 0
        assert res["data"]["doc1"]["status"] == "1"

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
