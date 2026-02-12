import datetime as dt
import types

import pytest

pytestmark = pytest.mark.p2


def _make_kb(name="old_name", pagerank=0, **kwargs):
    kb = types.SimpleNamespace(
        id="kb-1",
        name=name,
        tenant_id="tenant-1",
        pagerank=pagerank,
        graphrag_task_id="",
        raptor_task_id="",
        mindmap_task_id="",
    )
    kb.__dict__.update(kwargs)
    kb.to_dict = lambda: dict(kb.__dict__)
    return kb


def _make_task(progress=0):
    task = types.SimpleNamespace(id="task-1", progress=progress)
    task.to_dict = lambda: {"id": task.id, "progress": task.progress}
    return task


@pytest.mark.asyncio
async def test_create_kb_error_paths(kb_app):
    mod, state = kb_app

    state.json = {"name": "kb-name"}
    state.kb_create_side_effect = [(False, {"code": 0, "message": "ok"})]
    res = await mod.create()
    assert res["message"] == "ok"

    state.json = {"name": "kb-name"}
    state.kb_create_side_effect = [(True, {"id": "kb-1"})]
    state.kb_save_side_effect = [False]
    res = await mod.create()
    assert res["code"] == 102

    state.json = {"name": "kb-name"}
    state.kb_create_side_effect = [(True, {"id": "kb-1"})]
    state.kb_save_side_effect = [Exception("boom")]
    res = await mod.create()
    assert res["code"] == 500
    assert "boom" in res["message"]


@pytest.mark.asyncio
async def test_update_kb_validation_and_auth(kb_app):
    mod, state = kb_app

    async def _call(payload):
        state.json = payload
        return await mod.update()

    res = await _call({"kb_id": "kb-1", "name": 1, "description": "", "parser_id": "naive"})
    assert res["code"] == 102
    assert "must be string" in res["message"]

    res = await _call({"kb_id": "kb-1", "name": " ", "description": "", "parser_id": "naive"})
    assert res["code"] == 102
    assert "can't be empty" in res["message"]

    res = await _call({"kb_id": "kb-1", "name": "a" * 129, "description": "", "parser_id": "naive"})
    assert res["code"] == 102
    assert "large than" in res["message"]

    mod.settings.DOC_ENGINE_INFINITY = True
    res = await _call({"kb_id": "kb-1", "name": " tag ", "description": "", "parser_id": "tag"})
    assert res["code"] == 103
    assert "Tag" in res["message"]

    res = await _call({"kb_id": "kb-1", "name": "pagerank", "description": "", "parser_id": "naive", "pagerank": 1})
    assert res["code"] == 102
    assert "pagerank" in res["message"]

    mod.settings.DOC_ENGINE_INFINITY = False
    state.kb_accessible4deletion = False
    res = await _call({"kb_id": "kb-1", "name": "valid", "description": "", "parser_id": "naive"})
    assert res["code"] == 109
    assert "authorization" in res["message"].lower()

    state.kb_accessible4deletion = True
    state.kb_query_owner_result = []
    res = await _call({"kb_id": "kb-1", "name": "valid", "description": "", "parser_id": "naive"})
    assert res["code"] == 103
    assert "Only owner" in res["message"]

    state.kb_query_owner_result = [object()]
    state.kb_get_by_id_side_effect = [(False, None)]
    res = await _call({"kb_id": "kb-1", "name": "valid", "description": "", "parser_id": "naive"})
    assert res["code"] == 102
    assert "Can't find" in res["message"]

    state.kb_get_by_id_side_effect = [(True, _make_kb(name="old_name"))]
    state.kb_query_duplicate_result = [object()]
    res = await _call({"kb_id": "kb-1", "name": "new_name", "description": "", "parser_id": "naive"})
    assert res["code"] == 102
    assert "Duplicated" in res["message"]

    def _boom(_kwargs):
        raise Exception("boom")

    state.kb_query_side_effect = _boom
    res = await _call({"kb_id": "kb-1", "name": "valid", "description": "", "parser_id": "naive"})
    assert res["code"] == 500
    assert "boom" in res["message"]
    state.kb_query_side_effect = None


@pytest.mark.asyncio
async def test_update_kb_success_connectors_pagerank(kb_app):
    mod, state = kb_app
    mod.settings.DOC_ENGINE_INFINITY = False
    state.kb_accessible4deletion = True
    state.kb_query_owner_result = [object()]
    state.kb_query_duplicate_result = []

    state.kb_get_by_id_side_effect = [(True, _make_kb(name="old_name"))]
    state.kb_update_by_id_side_effect = [False]
    state.json = {
        "kb_id": "kb-1",
        "name": "new_name",
        "description": "",
        "parser_id": "naive",
        "connectors": ["c1"],
    }
    res = await mod.update()
    assert res["code"] == 102

    state.kb_update_by_id_side_effect = []
    state.kb_update_by_id_result = True
    state.connector_link_errors = ["err"]
    state.kb_get_by_id_side_effect = [
        (True, _make_kb(name="old_name", pagerank=0)),
        (True, _make_kb(name="new_name", pagerank=5)),
    ]
    state.json = {
        "kb_id": "kb-1",
        "name": "new_name",
        "description": "",
        "parser_id": "naive",
        "pagerank": 5,
        "connectors": ["c1"],
    }
    res = await mod.update()
    assert res["code"] == 0
    assert res["data"]["connectors"] == ["c1"]

    state.kb_get_by_id_side_effect = [
        (True, _make_kb(name="old_name", pagerank=5)),
        (False, None),
    ]
    state.json = {
        "kb_id": "kb-1",
        "name": "new_name",
        "description": "",
        "parser_id": "naive",
        "pagerank": 0,
    }
    res = await mod.update()
    assert res["code"] == 102
    assert "Database error" in res["message"]


def test_detail_kb_authorization_and_formatting(kb_app):
    mod, state = kb_app
    state.args = {"kb_id": "kb-1"}
    state.user_tenants = [types.SimpleNamespace(tenant_id="t1"), types.SimpleNamespace(tenant_id="t2")]
    state.kb_query_side_effect = lambda _kwargs: []
    res = mod.detail()
    assert res["code"] == 103
    assert "Only owner" in res["message"]

    state.kb_query_side_effect = lambda _kwargs: [object()]
    state.kb_detail = None
    res = mod.detail()
    assert res["code"] == 102
    assert "Can't find" in res["message"]

    now = dt.datetime(2025, 1, 1, 1, 2, 3)
    state.kb_detail = {
        "id": "kb-1",
        "name": "kb",
        "parser_config": {"metadata": {"source": "s"}},
        "graphrag_task_finish_at": now,
        "raptor_task_finish_at": now,
        "mindmap_task_finish_at": now,
    }
    state.kb_total_size = 12
    state.connectors_list = ["c1"]
    res = mod.detail()
    assert res["code"] == 0
    assert res["data"]["size"] == 12
    assert res["data"]["connectors"] == ["c1"]
    assert res["data"]["parser_config"]["metadata"]["schema"] == {"source": "s"}
    assert res["data"]["graphrag_task_finish_at"] == "2025-01-01 01:02:03"

    state.kb_total_size_exc = Exception("boom")
    res = mod.detail()
    assert res["code"] == 500
    assert "boom" in res["message"]
    state.kb_total_size_exc = None


@pytest.mark.asyncio
async def test_list_kbs_owner_ids_and_pagination(kb_app):
    mod, state = kb_app

    state.args = {"desc": "false", "page": "1", "page_size": "1"}
    state.json = {"owner_ids": ["tenant-1"]}
    state.kb_get_by_tenant_ids_result = (
        [
            {"tenant_id": "tenant-1", "id": "kb-1"},
            {"tenant_id": "tenant-2", "id": "kb-2"},
        ],
        2,
    )
    res = await mod.list_kbs()
    assert res["code"] == 0
    assert len(res["data"]["kbs"]) == 1

    state.kb_get_by_tenant_ids_exc = Exception("boom")
    res = await mod.list_kbs()
    assert res["code"] == 500
    assert "boom" in res["message"]
    state.kb_get_by_tenant_ids_exc = None


@pytest.mark.asyncio
async def test_rm_kb_cleanup_and_error_paths(kb_app, monkeypatch):
    mod, state = kb_app

    state.kb_accessible4deletion = False
    state.json = {"kb_id": "kb-1"}
    res = await mod.rm()
    assert res["code"] == 109

    state.kb_accessible4deletion = True
    state.kb_query_owner_result = []
    res = await mod.rm()
    assert res["code"] == 103

    kb_obj = _make_kb(name="kb", pagerank=0)
    state.kb_query_owner_result = [kb_obj]
    state.docs_query = [types.SimpleNamespace(id="doc-1")]
    state.remove_document_result = False
    res = await mod.rm()
    assert res["code"] == 102
    assert "Document removal" in res["message"]

    state.remove_document_result = True
    state.file2doc_result = [types.SimpleNamespace(file_id="file-1")]
    state.kb_delete_by_id_result = False

    def _raise(*_args, **_kwargs):
        raise Exception("drop-fail")

    monkeypatch.setattr(mod.settings.docStoreConn, "delete", _raise)
    res = await mod.rm()
    assert res["code"] == 102
    assert "Knowledgebase removal" in res["message"]

    state.kb_delete_by_id_result = True
    monkeypatch.setattr(mod.settings.docStoreConn, "delete", lambda *_args, **_kwargs: True)
    res = await mod.rm()
    assert res["code"] == 0
    assert res["data"] is True

    async def _boom(*_args, **_kwargs):
        raise Exception("boom")

    monkeypatch.setattr(mod, "thread_pool_exec", _boom)
    res = await mod.rm()
    assert res["code"] == 500
    assert "boom" in res["message"]


@pytest.mark.asyncio
async def test_tags_meta_basic_info_and_update_metadata_setting(kb_app):
    mod, state = kb_app

    state.kb_accessible = False
    res = mod.list_tags("kb-1")
    assert res["code"] == 109

    state.kb_accessible = True
    state.user_tenants_dicts = [{"tenant_id": "t1"}]
    state.retriever_tags = ["tag-1"]
    res = mod.list_tags("kb-1")
    assert res["code"] == 0
    assert "tag-1" in res["data"]

    state.args = {"kb_ids": "kb-1,kb-2"}
    state.kb_accessible = False
    res = mod.list_tags_from_kbs()
    assert res["code"] == 109

    state.kb_accessible = True
    res = mod.list_tags_from_kbs()
    assert res["code"] == 0

    state.kb_accessible = False
    state.json = {"tags": ["tag-1"]}
    res = await mod.rm_tags("kb-1")
    assert res["code"] == 109

    state.kb_accessible = True
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    state.json = {"tags": ["tag-1"]}
    res = await mod.rm_tags("kb-1")
    assert res["code"] == 0
    assert res["data"] is True

    state.kb_accessible = False
    state.json = {"from_tag": "old", "to_tag": "new"}
    res = await mod.rename_tags("kb-1")
    assert res["code"] == 109

    state.kb_accessible = True
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    state.json = {"from_tag": "old", "to_tag": "new"}
    res = await mod.rename_tags("kb-1")
    assert res["code"] == 0
    assert res["data"] is True

    state.kb_accessible = False
    state.args = {"kb_ids": "kb-1"}
    res = mod.get_meta()
    assert res["code"] == 109

    state.kb_accessible = True
    state.meta_flat = {"field": ["v"]}
    res = mod.get_meta()
    assert res["code"] == 0
    assert res["data"]["field"] == ["v"]

    state.kb_accessible = False
    state.args = {"kb_id": "kb-1"}
    res = mod.get_basic_info()
    assert res["code"] == 109

    state.kb_accessible = True
    state.kb_basic_info = {"processing": 1}
    res = mod.get_basic_info()
    assert res["code"] == 0
    assert res["data"]["processing"] == 1

    state.json = {"kb_id": "kb-1", "metadata": {}}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = await mod.update_metadata_setting()
    assert res["code"] == 102
    assert "Database error" in res["message"]


@pytest.mark.asyncio
async def test_knowledge_graph_and_delete(kb_app):
    mod, state = kb_app

    state.kb_accessible = False
    res = await mod.knowledge_graph("kb-1")
    assert res["code"] == 109

    state.kb_accessible = True
    state.doc_store_index_exists = False
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    res = await mod.knowledge_graph("kb-1")
    assert res["code"] == 0
    assert res["data"]["graph"] == {}

    class _SearchResult:
        def __init__(self, ids, field):
            self.ids = ids
            self.field = field

    state.doc_store_index_exists = True
    state.retriever_search_result = _SearchResult([], {})
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    res = await mod.knowledge_graph("kb-1")
    assert res["code"] == 0
    assert res["data"]["graph"] == {}

    state.retriever_search_result = _SearchResult(
        ["id1"],
        {"id1": {"knowledge_graph_kwd": "graph", "content_with_weight": "not-json"}},
    )
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    res = await mod.knowledge_graph("kb-1")
    assert res["code"] == 0
    assert res["data"]["graph"] == {}

    graph_json = {
        "nodes": [{"id": 1, "pagerank": 2}, {"id": 2, "pagerank": 1}],
        "edges": [
            {"source": 1, "target": 1, "weight": 5},
            {"source": 1, "target": 2, "weight": 2},
            {"source": 2, "target": 1, "weight": 1},
        ],
    }
    state.retriever_search_result = _SearchResult(
        ["id1"],
        {"id1": {"knowledge_graph_kwd": "graph", "content_with_weight": __import__("json").dumps(graph_json)}},
    )
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    res = await mod.knowledge_graph("kb-1")
    assert res["code"] == 0
    assert res["data"]["graph"]["nodes"][0]["pagerank"] == 2
    assert all(edge["source"] != edge["target"] for edge in res["data"]["graph"]["edges"])

    state.kb_accessible = False
    res = mod.delete_knowledge_graph("kb-1")
    assert res["code"] == 109

    state.kb_accessible = True
    state.kb_get_by_id_side_effect = [(True, types.SimpleNamespace(tenant_id="tenant-1"))]
    res = mod.delete_knowledge_graph("kb-1")
    assert res["code"] == 0
    assert res["data"] is True


@pytest.mark.asyncio
async def test_pipeline_logs_dataset_logs_and_detail_delete(kb_app):
    mod, state = kb_app

    state.args = {}
    state.json = {}
    res = await mod.list_pipeline_logs()
    assert res["code"] == 101

    state.args = {"kb_id": "kb-1", "create_date_from": "2025-01-01", "create_date_to": "2025-01-02"}
    state.json = {}
    res = await mod.list_pipeline_logs()
    assert res["code"] == 102

    state.args = {"kb_id": "kb-1"}
    state.json = {"operation_status": ["BAD"]}
    res = await mod.list_pipeline_logs()
    assert res["code"] == 102

    state.json = {"types": ["bad"]}
    res = await mod.list_pipeline_logs()
    assert res["code"] == 102

    state.args = {"kb_id": "kb-1", "keywords": "k", "page": "1", "page_size": "1", "desc": "false"}
    state.json = {"suffix": ["pdf"]}
    state.pipeline_logs_result = ([{"id": "log-1"}], 1)
    res = await mod.list_pipeline_logs()
    assert res["code"] == 0
    assert res["data"]["total"] == 1

    state.pipeline_logs_exc = Exception("boom")
    res = await mod.list_pipeline_logs()
    assert res["code"] == 500
    assert "boom" in res["message"]
    state.pipeline_logs_exc = None

    state.args = {}
    state.json = {}
    res = await mod.list_pipeline_dataset_logs()
    assert res["code"] == 101

    state.args = {"kb_id": "kb-1", "create_date_from": "2025-01-01", "create_date_to": "2025-01-02"}
    res = await mod.list_pipeline_dataset_logs()
    assert res["code"] == 102

    state.args = {"kb_id": "kb-1"}
    state.json = {"operation_status": ["BAD"]}
    res = await mod.list_pipeline_dataset_logs()
    assert res["code"] == 102

    state.json = {}
    state.pipeline_dataset_logs_result = ([{"id": "log-1"}], 1)
    res = await mod.list_pipeline_dataset_logs()
    assert res["code"] == 0

    state.pipeline_dataset_logs_exc = Exception("boom")
    res = await mod.list_pipeline_dataset_logs()
    assert res["code"] == 500
    assert "boom" in res["message"]
    state.pipeline_dataset_logs_exc = None

    state.args = {}
    state.json = {"log_ids": []}
    res = await mod.delete_pipeline_logs()
    assert res["code"] == 101

    state.args = {"kb_id": "kb-1"}
    state.json = {"log_ids": ["l1", "l2"]}
    res = await mod.delete_pipeline_logs()
    assert res["code"] == 0
    assert res["data"] is True

    state.args = {}
    res = mod.pipeline_log_detail()
    assert res["code"] == 101

    state.args = {"log_id": "bad"}
    state.pipeline_log_get_result = (False, None)
    res = mod.pipeline_log_detail()
    assert res["code"] == 102

    log_obj = types.SimpleNamespace(id="log-1")
    log_obj.to_dict = lambda: {"id": "log-1"}
    state.pipeline_log_get_result = (True, log_obj)
    res = mod.pipeline_log_detail()
    assert res["code"] == 0
    assert res["data"]["id"] == "log-1"


@pytest.mark.asyncio
async def test_pipeline_tasks_run_and_trace_validations(kb_app):
    mod, state = kb_app

    state.json = {}
    res = await mod.run_graphrag()
    assert res["code"] == 102

    state.json = {"kb_id": "kb-1"}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = await mod.run_graphrag()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(graphrag_task_id="task-1"))]
    state.task_get_result = (False, None)
    state.docs_by_kb = []
    res = await mod.run_graphrag()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(graphrag_task_id="task-1"))]
    state.task_get_result = (True, _make_task(progress=0.5))
    res = await mod.run_graphrag()
    assert res["code"] == 102
    assert "already running" in res["message"]

    state.kb_get_by_id_side_effect = [(True, _make_kb())]
    state.task_get_result = (False, None)
    state.docs_by_kb = [{"id": "doc-1"}]
    state.kb_update_by_id_result = False
    res = await mod.run_graphrag()
    assert res["code"] == 0
    assert "graphrag_task_id" in res["data"]

    state.args = {}
    res = mod.trace_graphrag()
    assert res["code"] == 102

    state.args = {"kb_id": "kb-1"}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = mod.trace_graphrag()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(graphrag_task_id=""))]
    res = mod.trace_graphrag()
    assert res["code"] == 0
    assert res["data"] == {}

    state.kb_get_by_id_side_effect = [(True, _make_kb(graphrag_task_id="task-1"))]
    state.task_get_result = (False, None)
    res = mod.trace_graphrag()
    assert res["code"] == 0
    assert res["data"] == {}

    state.kb_get_by_id_side_effect = [(True, _make_kb(graphrag_task_id="task-1"))]
    state.task_get_result = (True, _make_task(progress=1))
    res = mod.trace_graphrag()
    assert res["code"] == 0
    assert res["data"]["id"] == "task-1"

    state.json = {}
    res = await mod.run_raptor()
    assert res["code"] == 102

    state.json = {"kb_id": "kb-1"}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = await mod.run_raptor()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(raptor_task_id="task-1"))]
    state.task_get_result = (False, None)
    state.docs_by_kb = []
    res = await mod.run_raptor()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(raptor_task_id="task-1"))]
    state.task_get_result = (True, _make_task(progress=0.5))
    res = await mod.run_raptor()
    assert res["code"] == 102
    assert "already running" in res["message"]

    state.kb_get_by_id_side_effect = [(True, _make_kb())]
    state.docs_by_kb = [{"id": "doc-1"}]
    state.kb_update_by_id_result = False
    res = await mod.run_raptor()
    assert res["code"] == 0
    assert "raptor_task_id" in res["data"]

    state.args = {}
    res = mod.trace_raptor()
    assert res["code"] == 102

    state.args = {"kb_id": "kb-1"}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = mod.trace_raptor()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(raptor_task_id=""))]
    res = mod.trace_raptor()
    assert res["code"] == 0
    assert res["data"] == {}

    state.kb_get_by_id_side_effect = [(True, _make_kb(raptor_task_id="task-1"))]
    state.task_get_result = (False, None)
    res = mod.trace_raptor()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(raptor_task_id="task-1"))]
    state.task_get_result = (True, _make_task(progress=1))
    res = mod.trace_raptor()
    assert res["code"] == 0
    assert res["data"]["id"] == "task-1"

    state.json = {}
    res = await mod.run_mindmap()
    assert res["code"] == 102

    state.json = {"kb_id": "kb-1"}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = await mod.run_mindmap()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(mindmap_task_id="task-1"))]
    state.task_get_result = (False, None)
    state.docs_by_kb = []
    res = await mod.run_mindmap()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(mindmap_task_id="task-1"))]
    state.task_get_result = (True, _make_task(progress=0.5))
    res = await mod.run_mindmap()
    assert res["code"] == 102
    assert "already running" in res["message"]

    state.kb_get_by_id_side_effect = [(True, _make_kb())]
    state.docs_by_kb = [{"id": "doc-1"}]
    state.kb_update_by_id_result = False
    res = await mod.run_mindmap()
    assert res["code"] == 0
    assert "mindmap_task_id" in res["data"]

    state.args = {}
    res = mod.trace_mindmap()
    assert res["code"] == 102

    state.args = {"kb_id": "kb-1"}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = mod.trace_mindmap()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(mindmap_task_id=""))]
    res = mod.trace_mindmap()
    assert res["code"] == 0
    assert res["data"] == {}

    state.kb_get_by_id_side_effect = [(True, _make_kb(mindmap_task_id="task-1"))]
    state.task_get_result = (False, None)
    res = mod.trace_mindmap()
    assert res["code"] == 102

    state.kb_get_by_id_side_effect = [(True, _make_kb(mindmap_task_id="task-1"))]
    state.task_get_result = (True, _make_task(progress=1))
    res = mod.trace_mindmap()
    assert res["code"] == 0
    assert res["data"]["id"] == "task-1"


def test_delete_kb_task_variants(kb_app):
    mod, state = kb_app

    state.args = {}
    res = mod.delete_kb_task()
    assert res["code"] == 102

    state.args = {"kb_id": "kb-1"}
    state.kb_get_by_id_side_effect = [(False, None)]
    res = mod.delete_kb_task()
    assert res["code"] == 0
    assert res["data"] is True

    state.args = {"kb_id": "kb-1", "pipeline_task_type": "invalid"}
    state.kb_get_by_id_side_effect = [(True, _make_kb())]
    res = mod.delete_kb_task()
    assert res["code"] == 102
    assert "Invalid task type" in res["message"]

    state.args = {"kb_id": "kb-1", "pipeline_task_type": mod.PipelineTaskType.GRAPH_RAG}
    state.kb_get_by_id_side_effect = [(True, _make_kb(graphrag_task_id="task-1"))]
    state.kb_update_by_id_result = False
    res = mod.delete_kb_task()
    assert res["code"] == 500
    assert "cannot delete task" in res["message"]

    state.kb_update_by_id_result = True
    state.args = {"kb_id": "kb-1", "pipeline_task_type": mod.PipelineTaskType.GRAPH_RAG}
    state.kb_get_by_id_side_effect = [(True, _make_kb(graphrag_task_id="task-1"))]
    res = mod.delete_kb_task()
    assert res["code"] == 0

    state.args = {"kb_id": "kb-1", "pipeline_task_type": mod.PipelineTaskType.RAPTOR}
    state.kb_get_by_id_side_effect = [(True, _make_kb(raptor_task_id="task-2"))]
    res = mod.delete_kb_task()
    assert res["code"] == 0

    state.args = {"kb_id": "kb-1", "pipeline_task_type": mod.PipelineTaskType.MINDMAP}
    state.kb_get_by_id_side_effect = [(True, _make_kb(mindmap_task_id="task-3"))]
    res = mod.delete_kb_task()
    assert res["code"] == 0
