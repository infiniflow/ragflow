import types

import pytest

pytestmark = pytest.mark.p2


@pytest.mark.asyncio
async def test_thumbnails_missing_and_rewrite(document_app):
    mod, state = document_app

    state.args = {}
    res = mod.thumbnails()
    assert res["code"] == mod.RetCode.ARGUMENT_ERROR

    state.args = {"doc_ids": ["doc1", "doc2"]}
    docs = [
        {"id": "doc1", "kb_id": "kb1", "thumbnail": "thumb.png"},
        {"id": "doc2", "kb_id": "kb1", "thumbnail": f"{mod.IMG_BASE64_PREFIX};base64,abc"},
    ]
    mod.DocumentService.get_thumbnails = classmethod(lambda cls, _doc_ids: docs)
    res = mod.thumbnails()
    assert res["code"] == mod.RetCode.SUCCESS
    assert res["data"]["doc1"].startswith("/v1/document/image/")
    assert res["data"]["doc2"].startswith(mod.IMG_BASE64_PREFIX)


@pytest.mark.asyncio
async def test_thumbnails_exception_mapping(document_app):
    mod, state = document_app
    state.args = {"doc_ids": ["doc1"]}
    mod.DocumentService.get_thumbnails = classmethod(lambda cls, _doc_ids: (_ for _ in ()).throw(RuntimeError("boom")))
    res = mod.thumbnails()
    assert res["code"] == mod.RetCode.SERVER_ERROR


@pytest.mark.asyncio
async def test_change_status_partial_failures(document_app):
    mod, state = document_app
    status = "1"
    doc_ids = [
        "noauth",
        "nodoc",
        "nokb",
        "same",
        "updatefail",
        "docstore3022",
        "docstorefalse",
        "docstoreerr",
        "docstoreok",
    ]

    def _accessible(doc_id, _uid):
        return doc_id != "noauth"

    class _Doc:
        def __init__(self, doc_id, kb_id="kb1", status="0", chunk_num=0):
            self.id = doc_id
            self.kb_id = kb_id
            self.status = status
            self.chunk_num = chunk_num

    docs = {
        "nokb": _Doc("nokb", kb_id="missing"),
        "same": _Doc("same", status=status),
        "updatefail": _Doc("updatefail"),
        "docstore3022": _Doc("docstore3022", chunk_num=1),
        "docstorefalse": _Doc("docstorefalse", chunk_num=1),
        "docstoreerr": _Doc("docstoreerr", chunk_num=1),
        "docstoreok": _Doc("docstoreok", chunk_num=1),
    }

    mod.DocumentService.accessible = classmethod(lambda cls, doc_id, uid: _accessible(doc_id, uid))

    def _get_by_id(_doc_id):
        if _doc_id == "nodoc":
            return False, None
        return True, docs.get(_doc_id)

    mod.DocumentService.get_by_id = classmethod(lambda cls, doc_id: _get_by_id(doc_id))
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, kb_id: (False, None) if kb_id == "missing" else (True, types.SimpleNamespace(id=kb_id, tenant_id="tenant")))

    def _update_by_id(doc_id, _updates):
        return doc_id != "updatefail"

    mod.DocumentService.update_by_id = classmethod(lambda cls, doc_id, updates: _update_by_id(doc_id, updates))

    def _docstore_update(filter_body, _updates, _index, _kb_id):
        doc_id = filter_body.get("doc_id")
        if doc_id == "docstore3022":
            raise Exception("3022 table missing")
        if doc_id == "docstoreerr":
            raise Exception("boom")
        if doc_id == "docstorefalse":
            return False
        return True

    mod.settings.docStoreConn.update = _docstore_update

    state.json = {"doc_ids": doc_ids, "status": status}
    res = await mod.change_status()
    assert res["code"] == mod.RetCode.SERVER_ERROR
    assert res["message"] == "Partial failure"
    assert "noauth" in res["data"]
    assert res["data"]["docstore3022"]["error"] == "Document store table missing."
    assert "Document store update failed" in res["data"]["docstoreerr"]["error"]
    assert "Database error (docStore update)" in res["data"]["docstorefalse"]["error"]


@pytest.mark.asyncio
async def test_rm_auth_and_delete_errors(document_app):
    mod, state = document_app
    state.json = {"doc_id": "doc1"}

    mod.DocumentService.accessible4deletion = classmethod(lambda cls, *_args, **_kwargs: False)
    res = await mod.rm()
    assert res["code"] == mod.RetCode.AUTHENTICATION_ERROR

    mod.DocumentService.accessible4deletion = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.FileService.delete_docs = classmethod(lambda cls, *_args, **_kwargs: "delete failed")
    res = await mod.rm()
    assert res["code"] == mod.RetCode.SERVER_ERROR

    mod.FileService.delete_docs = classmethod(lambda cls, *_args, **_kwargs: None)
    res = await mod.rm()
    assert res["code"] == mod.RetCode.SUCCESS


@pytest.mark.asyncio
async def test_run_cancel_delete_apply_kb(document_app):
    mod, state = document_app

    state.json = {"doc_ids": ["denied"], "run": mod.TaskStatus.RUNNING.value}
    mod.DocumentService.accessible = classmethod(lambda cls, doc_id, _uid: doc_id != "denied")
    res = await mod.run()
    assert res["code"] == mod.RetCode.AUTHENTICATION_ERROR

    state.json = {"doc_ids": ["doc1"], "run": mod.TaskStatus.RUNNING.value}
    mod.DocumentService.accessible = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.DocumentService.get_tenant_id = classmethod(lambda cls, _doc_id: None)
    res = await mod.run()
    assert res["code"] == mod.RetCode.DATA_ERROR

    mod.DocumentService.get_tenant_id = classmethod(lambda cls, _doc_id: "tenant")
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (False, None))
    res = await mod.run()
    assert res["code"] == mod.RetCode.DATA_ERROR

    class _Task:
        def __init__(self, progress):
            self.progress = progress

    doc_running = types.SimpleNamespace(id="doc2", kb_id="kb1", run=mod.TaskStatus.RUNNING.value, parser_config={}, to_dict=lambda: {"id": "doc2"})
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, doc_running))
    mod.TaskService.query = classmethod(lambda cls, *_args, **_kwargs: [_Task(0.5)])
    cancelled = {}
    mod.cancel_all_task_of = lambda doc_id: cancelled.setdefault("doc", doc_id)
    state.json = {"doc_ids": ["doc2"], "run": mod.TaskStatus.CANCEL.value}
    res = await mod.run()
    assert res["code"] == mod.RetCode.SUCCESS
    assert cancelled["doc"] == "doc2"

    doc_done = types.SimpleNamespace(id="doc3", kb_id="kb1", run=mod.TaskStatus.DONE.value, parser_config={}, to_dict=lambda: {"id": "doc3"})
    mod.DocumentService.get_by_id = classmethod(lambda cls, _doc_id: (True, doc_done))
    mod.TaskService.query = classmethod(lambda cls, *_args, **_kwargs: [_Task(1)])
    state.json = {"doc_ids": ["doc3"], "run": mod.TaskStatus.CANCEL.value}
    res = await mod.run()
    assert res["code"] == mod.RetCode.DATA_ERROR

    cleared = {}
    mod.DocumentService.clear_chunk_num_when_rerun = classmethod(lambda cls, doc_id: cleared.setdefault("doc", doc_id))
    mod.DocumentService.update_by_id = classmethod(lambda cls, *_args, **_kwargs: True)
    mod.TaskService.filter_delete = classmethod(lambda cls, *_args, **_kwargs: None)
    mod.settings.docStoreConn.index_exist = lambda *_args, **_kwargs: True
    mod.settings.docStoreConn.delete = lambda *_args, **_kwargs: None

    kb = types.SimpleNamespace(parser_config={"llm_id": "llm", "enable_metadata": True, "metadata": {"a": 1}})
    mod.KnowledgebaseService.get_by_id = classmethod(lambda cls, _kb_id: (True, kb))
    mod.DocumentService.update_parser_config = classmethod(lambda cls, *_args, **_kwargs: None)
    mod.DocumentService.run = classmethod(lambda cls, *_args, **_kwargs: None)

    state.json = {"doc_ids": ["doc3"], "run": mod.TaskStatus.RUNNING.value, "delete": True, "apply_kb": True}
    res = await mod.run()
    assert res["code"] == mod.RetCode.SUCCESS
    assert cleared["doc"] == "doc3"


@pytest.mark.asyncio
async def test_run_exception_mapping(document_app):
    mod, state = document_app
    state.json = {"doc_ids": ["doc1"], "run": mod.TaskStatus.RUNNING.value}

    async def _raise(_fn, *_args, **_kwargs):
        raise RuntimeError("boom")

    mod.thread_pool_exec = _raise
    res = await mod.run()
    assert res["code"] == mod.RetCode.SERVER_ERROR
