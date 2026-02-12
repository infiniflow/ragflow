import json
import types

import pytest

pytestmark = pytest.mark.p2


@pytest.mark.asyncio
async def test_filter_not_owner(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [])
    state.json = {"kb_id": "kb1"}
    res = await mod.get_filter()
    assert res["code"] == mod.RetCode.OPERATING_ERROR


@pytest.mark.asyncio
async def test_filter_exception_mapping(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: ["kb"])
    mod.DocumentService.get_filter_by_kb_id = classmethod(lambda cls, *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("boom")))
    state.json = {"kb_id": "kb1"}
    res = await mod.get_filter()
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_metadata_summary_not_owner(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: [])
    state.json = {"kb_id": "kb1"}
    res = await mod.metadata_summary()
    assert res["code"] == mod.RetCode.OPERATING_ERROR


@pytest.mark.asyncio
async def test_metadata_summary_exception_mapping(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: ["kb"])
    mod.DocMetadataService.get_metadata_summary = classmethod(lambda cls, *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("boom")))
    state.json = {"kb_id": "kb1", "doc_ids": []}
    res = await mod.metadata_summary()
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_update_metadata_setting_not_found(document_app):
    mod, state = document_app
    state.json = {"doc_id": "doc1", "metadata": {"a": 1}}

    mod.DocumentService.accessible = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (False, None))
    res = await mod.update_metadata_setting()
    assert res["code"] == mod.RetCode.DATA_ERROR

    doc = types.SimpleNamespace(id="doc1")
    mod.DocumentService.update_parser_config = classmethod(lambda cls, *_args, **_kwargs: True)
    calls = {"count": 0}

    def _get_by_id(_doc_id):
        calls["count"] += 1
        if calls["count"] == 1:
            return True, doc
        return False, None

    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: _get_by_id(_doc_id))
    res = await mod.update_metadata_setting()
    assert res["code"] == mod.RetCode.DATA_ERROR


@pytest.mark.asyncio
async def test_set_meta_doc_not_found_or_update_fail(document_app):
    mod, state = document_app
    mod.DocumentService.accessible = classmethod(lambda cls, *_args, **_kwargs: True)

    state.json = {"doc_id": "doc1", "meta": json.dumps({"a": 1})}
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (False, None))
    res = await mod.set_meta()
    assert res["code"] == mod.RetCode.DATA_ERROR

    doc = types.SimpleNamespace(id="doc1")
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, doc))
    mod.DocMetadataService.update_document_metadata = classmethod(lambda cls, *_args, **_kwargs: False)
    res = await mod.set_meta()
    assert res["code"] == mod.RetCode.DATA_ERROR

    mod.DocMetadataService.update_document_metadata = classmethod(lambda cls, *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("boom")))
    res = await mod.set_meta()
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_upload_info_success_and_exception(document_app, file_factory):
    mod, state = document_app
    state.files = state.files.__class__({"file": file_factory("info.txt")})
    state.args = {"url": "http://example.com"}
    mod.FileService.upload_info = classmethod(lambda cls, *_args, **_kwargs: {"ok": True})
    res = await mod.upload_info()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"]["ok"] is True

    mod.FileService.upload_info = classmethod(lambda cls, *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("boom")))
    res = await mod.upload_info()
    assert res["code"] == mod.RetCode.SERVER_ERROR
