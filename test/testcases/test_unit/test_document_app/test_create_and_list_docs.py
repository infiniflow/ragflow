import types

import pytest

pytestmark = pytest.mark.p2


@pytest.mark.asyncio
async def test_create_folder_missing_and_success(document_app):
    mod, state = document_app
    kb = types.SimpleNamespace(
        id="kb1",
        tenant_id="tenant",
        name="kb",
        parser_id="parser",
        parser_config={"metadata": {"a": 1}},
        pipeline_id="pipe",
    )
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, kb_id: (True, kb))
    mod.DocumentService.query = classmethod(lambda cls, **_kwargs: [])

    state.json = {"name": "doc.txt", "kb_id": "kb1"}
    mod.FileService.get_kb_folder = classmethod(lambda cls, _tenant_id: None)
    res = await mod.create()
    assert res["code"] == mod.RetCode.DATA_ERROR
    assert "root folder" in res["message"].lower()

    mod.FileService.get_kb_folder = classmethod(lambda cls, _tenant_id: {"id": "root"})
    mod.FileService.new_a_file_from_kb = classmethod(lambda cls, *_args, **_kwargs: None)
    res = await mod.create()
    assert res["code"] == mod.RetCode.DATA_ERROR
    assert "kb folder" in res["message"].lower()

    mod.FileService.new_a_file_from_kb = classmethod(lambda cls, *_args, **_kwargs: {"id": "kb-folder"})
    added = {}

    def _insert(data):
        return types.SimpleNamespace(to_dict=lambda: data, to_json=lambda: data, **data)

    mod.DocumentService.insert = classmethod(lambda cls, data: _insert(data))
    mod.FileService.add_file_from_kb = classmethod(lambda cls, doc, folder_id, tenant_id: added.update({"doc": doc, "folder_id": folder_id, "tenant_id": tenant_id}))
    res = await mod.create()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"]["kb_id"] == "kb1"
    assert added["folder_id"] == "kb-folder"

    def _raise(_data):
        raise RuntimeError("boom")

    mod.DocumentService.insert = classmethod(lambda cls, data: _raise(data))
    res = await mod.create()
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_list_docs_return_empty_metadata_unit(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: ["kb"])

    captured = {}

    def _get_by_kb_id(*args, **kwargs):
        captured["doc_ids_filter"] = args[9]
        captured["return_empty_metadata"] = kwargs.get("return_empty_metadata")
        return [], 0

    mod.DocumentService.get_by_kb_id = classmethod(lambda cls, *args, **kwargs: _get_by_kb_id(*args, **kwargs))

    state.args = {"kb_id": "kb1", "page": "0", "page_size": "10", "desc": "true"}
    state.json = {"return_empty_metadata": "true"}
    res = await mod.list_docs()
    assert res["code"] == mod.RetCode.SUCCESS
    assert captured["return_empty_metadata"] is True

    state.json = {"metadata": {"empty_metadata": True, "x": "y"}, "metadata_condition": {"conditions": [{"a": 1}]}}
    res = await mod.list_docs()
    assert res["code"] == mod.RetCode.SUCCESS
    assert captured["return_empty_metadata"] is True
    assert captured["doc_ids_filter"] is None


@pytest.mark.asyncio
async def test_list_docs_metadata_filters_and_doc_ids_filter(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: ["kb"])
    state.args = {"kb_id": "kb1", "page": "0", "page_size": "10", "desc": "true"}

    mod.DocMetadataService.get_flatted_meta_by_kbs = classmethod(lambda cls, _kb_ids: {"field": {"a": ["doc1"], "b": ["doc2"]}})

    mod.meta_filter = lambda _metas, _conds, _logic: []
    state.json = {"metadata_condition": {"conditions": [{"a": 1}]}}
    res = await mod.list_docs()
    assert res["data"]["docs"] == []

    mod.meta_filter = lambda _metas, _conds, _logic: {"doc1", "doc2"}
    state.json = {"metadata_condition": {"conditions": [{"a": 1}]}, "metadata": {"field": ["missing"]}}
    res = await mod.list_docs()
    assert res["data"]["docs"] == []

    captured = {}

    def _get_by_kb_id(*args, **kwargs):
        captured["doc_ids_filter"] = args[9]
        return [], 0

    mod.DocumentService.get_by_kb_id = classmethod(lambda cls, *args, **kwargs: _get_by_kb_id(*args, **kwargs))
    state.json = {"metadata_condition": {"conditions": [{"a": 1}]}, "metadata": {"field": ["a"]}}
    res = await mod.list_docs()
    assert res["code"] == mod.RetCode.SUCCESS
    assert isinstance(captured["doc_ids_filter"], list)


@pytest.mark.asyncio
async def test_list_docs_create_time_filter_and_response_shaping(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: ["kb"])

    docs = [
        {"id": "doc1", "create_time": 200, "thumbnail": "thumb.png", "kb_id": "kb1", "parser_config": {"metadata": {"a": 1}}},
        {"id": "doc2", "create_time": 100, "thumbnail": "data:image/png;base64,abc", "kb_id": "kb1", "parser_config": {"metadata": {"b": 2}}},
    ]

    mod.DocumentService.get_by_kb_id = classmethod(lambda cls, *_args, **_kwargs: (list(docs), len(docs)))
    mod.turn2jsonschema = lambda metadata: {"converted": metadata}

    state.args = {
        "kb_id": "kb1",
        "page": "0",
        "page_size": "10",
        "desc": "true",
        "create_time_from": "150",
        "create_time_to": "250",
    }
    state.json = {}
    res = await mod.list_docs()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"]["total"] == len(docs)
    assert len(res["data"]["docs"]) == 1
    assert res["data"]["docs"][0]["thumbnail"].startswith("/v1/document/image/")
    assert res["data"]["docs"][0]["parser_config"]["metadata"].get("converted") == {"a": 1}


@pytest.mark.asyncio
async def test_list_docs_exception(document_app):
    mod, state = document_app
    mod.UserTenantService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="tenant")])
    mod.KnowledgebaseService.query = classmethod(lambda cls, **_kwargs: ["kb"])
    mod.DocumentService.get_by_kb_id = classmethod(lambda cls, *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("boom")))

    state.args = {"kb_id": "kb1", "page": "0", "page_size": "10", "desc": "true"}
    state.json = {}
    res = await mod.list_docs()
    assert res["code"] == mod.RetCode.SERVER_ERROR
