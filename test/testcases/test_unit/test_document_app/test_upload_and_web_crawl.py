import types

import pytest

pytestmark = pytest.mark.p2


@pytest.mark.asyncio
async def test_upload_permission_and_upload_errors(document_app, file_factory):
    mod, state = document_app
    kb = types.SimpleNamespace(id="kb1", tenant_id="tenant", name="kb", parser_id="parser", parser_config={}, pipeline_id="pipeline")
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, kb_id: (True, kb))

    state.form = {"kb_id": "kb1"}
    state.files = state.files.__class__({"file": [file_factory("doc.txt")]})

    mod.check_kb_team_permission = lambda _kb, _uid: False
    res = await mod.upload()
    assert res["code"] == mod.RetCode.AUTHENTICATION_ERROR

    mod.check_kb_team_permission = lambda _kb, _uid: True
    mod.FileService.upload_document = classmethod(lambda cls, _kb, _files, _uid: (["err"], [("doc.txt", None)]))
    res = await mod.upload()
    assert res["code"] == mod.RetCode.SERVER_ERROR
    assert "err" in res["message"]

    mod.FileService.upload_document = classmethod(lambda cls, _kb, _files, _uid: ([], []))
    res = await mod.upload()
    assert res["code"] == mod.RetCode.DATA_ERROR
    assert "issue" in res["message"].lower()


@pytest.mark.asyncio
async def test_web_crawl_permission_and_download_failure(document_app):
    mod, state = document_app
    kb = types.SimpleNamespace(id="kb1", tenant_id="tenant", name="kb", parser_id="parser", parser_config={}, pipeline_id="pipeline")
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, kb_id: (True, kb))
    mod.is_valid_url = lambda url: True

    state.form = {"kb_id": "kb1", "name": "doc", "url": "http://example.com"}
    mod.check_kb_team_permission = lambda _kb, _uid: True
    res = await mod.web_crawl()
    assert res["code"] == mod.RetCode.AUTHENTICATION_ERROR

    mod.check_kb_team_permission = lambda _kb, _uid: False
    mod.html2pdf = lambda _url: None
    res = await mod.web_crawl()
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_web_crawl_unsupported_and_success_overrides(document_app):
    mod, state = document_app
    kb = types.SimpleNamespace(id="kb1", tenant_id="tenant", name="kb", parser_id="parser", parser_config={}, pipeline_id="pipeline")
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, kb_id: (True, kb))
    mod.is_valid_url = lambda url: True
    mod.check_kb_team_permission = lambda _kb, _uid: False
    mod.FileService.get_root_folder = classmethod(lambda cls, _uid: {"id": "root"})
    mod.FileService.init_knowledgebase_docs = classmethod(lambda cls, *_args, **_kwargs: None)
    mod.FileService.get_kb_folder = classmethod(lambda cls, _uid: {"id": "kb-root"})
    mod.FileService.new_a_file_from_kb = classmethod(lambda cls, *_args, **_kwargs: {"id": "kb-folder"})
    mod.settings.STORAGE_IMPL.obj_exist = lambda *_args, **_kwargs: False
    mod.settings.STORAGE_IMPL.put = lambda *_args, **_kwargs: None
    mod.FileService.add_file_from_kb = classmethod(lambda cls, *_args, **_kwargs: None)

    state.form = {"kb_id": "kb1", "name": "doc", "url": "http://example.com"}

    mod.duplicate_name = lambda _query, name=None, kb_id=None: "bad.bin"
    mod.filename_type = lambda _filename: mod.FileType.OTHER.value
    res = await mod.web_crawl()
    assert res["code"] == mod.RetCode.SERVER_ERROR

    inserted = []
    mod.DocumentService.insert = classmethod(lambda cls, doc: inserted.append(doc) or doc)
    mod.html2pdf = lambda _url: b"blob"

    cases = [
        ("img.png", mod.FileType.VISUAL, mod.ParserType.PICTURE.value),
        ("sound.mp3", mod.FileType.AURAL, mod.ParserType.AUDIO.value),
        ("slides.pptx", mod.FileType.DOC, mod.ParserType.PRESENTATION.value),
        ("mail.eml", mod.FileType.DOC, mod.ParserType.EMAIL.value),
    ]
    for filename, filetype, expected_parser in cases:
        mod.duplicate_name = lambda _query, name=None, kb_id=None, _filename=filename: _filename
        mod.filename_type = lambda _filename, _ft=filetype: _ft
        res = await mod.web_crawl()
        assert res["code"] == mod.RetCode.SUCCESS
        assert inserted[-1]["parser_id"] == expected_parser

    def _raise(_doc):
        raise RuntimeError("boom")
    mod.DocumentService.insert = classmethod(lambda cls, doc: _raise(doc))
    mod.duplicate_name = lambda _query, name=None, kb_id=None: "doc.pdf"
    mod.filename_type = lambda _filename: mod.FileType.DOC
    res = await mod.web_crawl()
    assert res["code"] == mod.RetCode.SERVER_ERROR
