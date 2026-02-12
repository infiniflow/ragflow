import os
import types

import pytest

pytestmark = pytest.mark.p2


@pytest.mark.asyncio
async def test_rename_validation_and_updates(document_app):
    mod, state = document_app
    state.json = {"doc_id": "doc1", "name": "doc.txt"}

    mod.DocumentService.accessible = classmethod(lambda cls, *_args, **_kwargs: False)
    res = await mod.rename()
    assert res["code"] == mod.RetCode.AUTHENTICATION_ERROR

    mod.DocumentService.accessible = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (False, None))
    res = await mod.rename()
    assert res["code"] == mod.RetCode.DATA_ERROR

    doc = types.SimpleNamespace(id="doc1", kb_id="kb1", name="doc.txt", token_num=0, chunk_num=0, process_duration=0)
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, doc))
    state.json = {"doc_id": "doc1", "name": "x" * (mod.FILE_NAME_LEN_LIMIT + 1) + ".txt"}
    res = await mod.rename()
    assert res["code"] == mod.RetCode.ARGUMENT_ERROR

    state.json = {"doc_id": "doc1", "name": "dup.txt"}
    mod.DocumentService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(name="dup.txt")])
    res = await mod.rename()
    assert res["code"] == mod.RetCode.DATA_ERROR

    mod.DocumentService.query = classmethod(lambda cls, **_kwargs: [])
    mod.DocumentService.update_by_id = classmethod(lambda cls, *_args, **_kwargs: False)
    res = await mod.rename()
    assert res["code"] == mod.RetCode.DATA_ERROR

    mod.DocumentService.update_by_id = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.File2DocumentService.get_by_document_id = classmethod(lambda cls, _doc_id: [types.SimpleNamespace(file_id="file1")])
    mod.FileService.get_by_id = classmethod(lambda cls, _file_id: (True, types.SimpleNamespace(id=_file_id)))
    updated = {}
    mod.FileService.update_by_id = classmethod(lambda cls, file_id, updates: updated.setdefault("file", (file_id, updates)))
    mod.DocumentService.get_tenant_id = classmethod(lambda cls, _doc_id: "tenant")
    mod.settings.docStoreConn.index_exist = lambda *_args, **_kwargs: True
    mod.settings.docStoreConn.update = lambda *_args, **_kwargs: None
    state.json = {"doc_id": "doc1", "name": "new.txt"}
    res = await mod.rename()
    assert res["code"] == mod.RetCode.SUCCESS
    assert updated["file"][1]["name"] == "new.txt"


@pytest.mark.asyncio
async def test_rename_exception_mapping(document_app):
    mod, state = document_app
    state.json = {"doc_id": "doc1", "name": "doc.txt"}

    async def _raise(_fn, *_args, **_kwargs):
        raise RuntimeError("boom")

    mod.thread_pool_exec = _raise
    res = await mod.rename()
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_get_content_type_headers(document_app):
    mod, _ = document_app

    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (False, None))
    res = await mod.get("missing")
    assert res["code"] == mod.RetCode.DATA_ERROR

    visual_doc = types.SimpleNamespace(id="doc1", kb_id="kb1", name="image.png", type=mod.FileType.VISUAL.value)
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, visual_doc))
    mod.File2DocumentService.get_storage_address = classmethod(lambda cls, doc_id=None: ("bkt", "nm"))
    mod.settings.STORAGE_IMPL.get = lambda *_args, **_kwargs: b"data"
    res = await mod.get("doc1")
    assert res.headers["Content-Type"] == "image/png"

    text_doc = types.SimpleNamespace(id="doc2", kb_id="kb1", name="file.txt", type="text")
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, text_doc))
    res = await mod.get("doc2")
    assert res.headers["Content-Type"] == "text/plain"

    def _raise(_bkt, _nm):
        raise RuntimeError("boom")

    mod.settings.STORAGE_IMPL.get = _raise
    res = await mod.get("doc2")
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_download_attachment_headers_and_error(document_app):
    mod, state = document_app
    state.args = {"ext": "txt"}
    mod.settings.STORAGE_IMPL.get = lambda *_args, **_kwargs: b"data"
    res = await mod.download_attachment("att")
    assert res.headers["Content-Type"] == "text/plain"

    def _raise(_bkt, _nm):
        raise RuntimeError("boom")

    mod.settings.STORAGE_IMPL.get = _raise
    res = await mod.download_attachment("att")
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_change_parser_paths(document_app):
    mod, state = document_app
    mod.DocumentService.accessible = classmethod(lambda cls, *_args, **_kwargs: True)

    doc = types.SimpleNamespace(
        id="doc1",
        kb_id="kb1",
        name="doc.txt",
        token_num=1,
        chunk_num=1,
        process_duration=1,
        pipeline_id="pipe",
        parser_id="parser",
        parser_config={"a": 1},
        type=mod.FileType.VISUAL,
    )

    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, doc))
    mod.DocumentService.update_by_id = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.DocumentService.increment_chunk_num = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.DocumentService.get_tenant_id = classmethod(lambda cls, _doc_id: "tenant")
    mod.DocumentService.delete_chunk_images = classmethod(lambda cls, *_args, **_kwargs: None)
    mod.settings.docStoreConn.index_exist = lambda *_args, **_kwargs: True
    mod.settings.docStoreConn.delete = lambda *_args, **_kwargs: None

    state.json = {"doc_id": "doc1", "pipeline_id": "pipe"}
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS

    state.json = {"doc_id": "doc1", "pipeline_id": "other"}
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS

    state.json = {"doc_id": "doc1", "parser_id": "parser", "parser_config": {"a": 1}}
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS

    state.json = {"doc_id": "doc1", "parser_id": "parser"}
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS

    state.json = {"doc_id": "doc1", "parser_id": "text"}
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.DATA_ERROR

    state.json = {"doc_id": "doc1", "parser_id": "picture", "parser_config": {"b": 2}}
    mod.DocumentService.update_parser_config = classmethod(lambda cls, *_args, **_kwargs: None)
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS


@pytest.mark.asyncio
async def test_change_parser_reset_doc_errors(document_app):
    mod, state = document_app
    mod.DocumentService.accessible = classmethod(lambda cls, *_args, **_kwargs: True)

    doc = types.SimpleNamespace(
        id="doc2",
        kb_id="kb1",
        name="doc.txt",
        token_num=1,
        chunk_num=1,
        process_duration=1,
        pipeline_id="pipe",
        parser_id="parser",
        parser_config={"a": 1},
        type=mod.FileType.DOC,
    )

    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, doc))
    mod.DocumentService.update_by_id = classmethod(lambda cls, *_args, **_kwargs: False)
    state.json = {"doc_id": "doc2", "pipeline_id": "new"}
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS

    mod.DocumentService.update_by_id = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.DocumentService.increment_chunk_num = classmethod(lambda cls, *_args, **_kwargs: False)
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS

    mod.DocumentService.increment_chunk_num = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.DocumentService.get_tenant_id = classmethod(lambda cls, _doc_id: None)
    res = await mod.change_parser()
    assert res["code"] == mod.RetCode.SUCCESS


@pytest.mark.asyncio
async def test_get_image_validation_and_success(document_app):
    mod, _ = document_app

    res = await mod.get_image("bad")
    assert res["code"] == mod.RetCode.DATA_ERROR

    mod.settings.STORAGE_IMPL.get = lambda *_args, **_kwargs: b"data"
    res = await mod.get_image("bkt-img")
    assert res.headers["Content-Type"] == "image/JPEG"


@pytest.mark.asyncio
async def test_upload_and_parse_validation(document_app, file_factory):
    mod, state = document_app

    state.files = state.files.__class__({})
    res = await mod.upload_and_parse()
    assert res["code"] == mod.RetCode.ARGUMENT_ERROR

    state.files = state.files.__class__({"file": [file_factory("")]})
    res = await mod.upload_and_parse()
    assert res["code"] == mod.RetCode.ARGUMENT_ERROR

    state.files = state.files.__class__({"file": [file_factory("doc.txt")]})
    state.form = {"conversation_id": "conv"}
    mod.doc_upload_and_parse = lambda *_args, **_kwargs: ["doc1"]
    res = await mod.upload_and_parse()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"] == ["doc1"]


@pytest.mark.asyncio
async def test_parse_url_invalid_and_html_parse(document_app):
    mod, state = document_app

    mod.is_valid_url = lambda _url: False
    state.json = {"url": "bad"}
    res = await mod.parse()
    assert res["code"] == mod.RetCode.ARGUMENT_ERROR

    mod.is_valid_url = lambda _url: True
    state.selenium_requests = [types.SimpleNamespace(response=types.SimpleNamespace(headers={"x": "y"})), types.SimpleNamespace(response=types.SimpleNamespace(headers={"x": "z"}))]
    state.selenium_page_source = "<html></html>"
    state.html_sections = ["a", "b"]
    state.json = {"url": "http://example.com"}
    res = await mod.parse()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"] == "a\nb"


@pytest.mark.asyncio
async def test_parse_download_and_file_upload(document_app, file_factory, tmp_path):
    mod, state = document_app

    filename = "download.txt"
    download_dir = tmp_path / "logs" / "downloads"
    os.makedirs(download_dir, exist_ok=True)
    (download_dir / filename).write_text("content")

    state.selenium_requests = [types.SimpleNamespace(response=types.SimpleNamespace(headers={"content-disposition": f'attachment; filename="{filename}"'}))]
    mod.is_valid_url = lambda _url: True
    mod.FileService.parse_docs = classmethod(lambda cls, files, _uid: files[0].read().decode())
    state.json = {"url": "http://example.com"}
    res = await mod.parse()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"] == "content"

    state.json = {}
    state.files = state.files.__class__({})
    res = await mod.parse()
    assert res["code"] == mod.RetCode.ARGUMENT_ERROR

    state.files = state.files.__class__({"file": [file_factory("doc.txt")]})
    mod.FileService.parse_docs = classmethod(lambda cls, *_args, **_kwargs: "uploaded")
    res = await mod.parse()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"] == "uploaded"
