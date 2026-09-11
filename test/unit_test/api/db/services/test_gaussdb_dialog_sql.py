#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
import importlib
import logging
import sys
import types
import warnings
from unittest.mock import AsyncMock, Mock

import pytest


GAUSSDB_SQL_RULE_MARKERS = (
    "Use only the static JSONB path literals listed under Fields.",
    "Do not construct dynamic JSONB paths.",
    "A/ORA treats '' as NULL; never compare with = '' or <> ''; use IS NULL or IS NOT NULL.",
    "Return exactly one read-only SELECT statement and no other text.",
    "Do not use LATERAL, JOIN, UNION, INTERSECT, EXCEPT, or OR.",
    "Do not expand JSON with jsonb_each, jsonb_each_text, jsonb_to_record, jsonb_to_recordset, jsonb_array_elements, or jsonb_array_elements_text.",
    "Do not use DML or DDL, including INSERT, UPDATE, DELETE, MERGE, CREATE, ALTER, DROP, TRUNCATE, GRANT, REVOKE, CALL, COPY, or DO.",
)


def _assert_gaussdb_sql_rule_markers(prompt):
    for marker in GAUSSDB_SQL_RULE_MARKERS:
        assert marker in prompt


warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401

        return
    except Exception:
        pass

    stub = types.ModuleType("cv2")
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1
    stub.COLOR_BGR2RGB = 0
    stub.COLOR_BGR2GRAY = 1
    stub.COLOR_GRAY2BGR = 2
    stub.IMREAD_IGNORE_ORIENTATION = 128
    stub.IMREAD_COLOR = 1
    stub.RETR_LIST = 1
    stub.CHAIN_APPROX_SIMPLE = 2

    def _module_getattr(name):
        if name.isupper():
            return 0
        raise RuntimeError(f"cv2.{name} is unavailable in this test environment")

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


def _install_settings_import_stubs(monkeypatch):
    class StubDocEngineConnection:
        def db_type(self):
            return "stub"

    class StubStorage:
        def health(self):
            return True

    def install_module(name, **attrs):
        module = types.ModuleType(name)
        for key, value in attrs.items():
            setattr(module, key, value)
        monkeypatch.setitem(sys.modules, name, module)
        return module

    try:
        import rag.utils
        import memory.utils
    except Exception:
        return

    rag_modules = {
        "es_conn": {"ESConnection": StubDocEngineConnection},
        "infinity_conn": {"InfinityConnection": StubDocEngineConnection},
        "ob_conn": {"OBConnection": StubDocEngineConnection},
        "opensearch_conn": {"OSConnection": StubDocEngineConnection},
        "azure_sas_conn": {"RAGFlowAzureSasBlob": StubStorage},
        "azure_spn_conn": {"RAGFlowAzureSpnBlob": StubStorage},
        "gcs_conn": {"RAGFlowGCS": StubStorage},
        "minio_conn": {"RAGFlowMinio": StubStorage},
        "opendal_conn": {"OpenDALStorage": StubStorage},
        "redis_conn": {"REDIS_CONN": types.SimpleNamespace(health=lambda: True, is_alive=lambda: False, REDIS=None)},
        "s3_conn": {"RAGFlowS3": StubStorage},
        "oss_conn": {"RAGFlowOSS": StubStorage},
    }
    for short_name, attrs in rag_modules.items():
        module = install_module(f"rag.utils.{short_name}", **attrs)
        monkeypatch.setattr(rag.utils, short_name, module, raising=False)

    for short_name in ("es_conn", "infinity_conn", "ob_conn"):
        module = install_module(
            f"memory.utils.{short_name}",
            ESConnection=StubDocEngineConnection,
            InfinityConnection=StubDocEngineConnection,
            OBConnection=StubDocEngineConnection,
        )
        monkeypatch.setattr(memory.utils, short_name, module, raising=False)

    if "json_repair" not in sys.modules:
        import json

        install_module("json_repair", loads=json.loads)
    install_module("roman_numbers")
    install_module("word2number", w2n=types.SimpleNamespace(word_to_num=lambda value: 0))
    install_module("cn2an", cn2an=lambda value, *_args, **_kwargs: 0)
    if "langfuse" not in sys.modules:

        class StubLangfuse:
            pass

        def propagate_attributes(**_kwargs):
            class Context:
                def __enter__(self):
                    return self

                def __exit__(self, *_args):
                    return False

            return Context()

        install_module("langfuse", Langfuse=StubLangfuse, propagate_attributes=propagate_attributes)
    if "peewee" not in sys.modules:
        install_module("peewee", fn=types.SimpleNamespace())
    if "mcp.client.session" not in sys.modules:
        install_module("mcp")
        install_module("mcp.client")

        class ClientSession:
            pass

        async def _client(*_args, **_kwargs):
            raise RuntimeError("mcp client is unavailable in this test")

        class _MCPType:
            pass

        install_module("mcp.client.session", ClientSession=ClientSession)
        install_module("mcp.client.sse", sse_client=_client)
        install_module("mcp.client.streamable_http", streamablehttp_client=_client)
        install_module(
            "mcp.types",
            CallToolResult=_MCPType,
            ListToolsResult=_MCPType,
            TextContent=_MCPType,
            Tool=_MCPType,
        )
    if "beartype" not in sys.modules:

        def beartype(obj=None, **_kwargs):
            if obj is None:
                return lambda wrapped: wrapped
            return obj

        install_module("beartype", beartype=beartype)
        install_module("beartype.claw", beartype_this_package=lambda *_args, **_kwargs: None)

    class _Dummy:
        def __init__(self, *_args, **_kwargs):
            pass

    class _Context:
        def __enter__(self):
            return self

        def __exit__(self, *_args):
            return False

        def __call__(self, func):
            return func

    class _DummyDB:
        @staticmethod
        def connection_context():
            return _Context()

        @staticmethod
        def atomic():
            return _Context()

    install_module("api.db.services.user_service", UserService=_Dummy, TenantService=_Dummy)
    install_module("api.db.services.file_service", FileService=_Dummy)
    install_module("api.db.services.common_service", CommonService=_Dummy)
    install_module("api.db.services.doc_metadata_service", DocMetadataService=_Dummy)
    install_module(
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=types.SimpleNamespace(get_field_map=lambda _kb_ids: {}),
        validate_dataset_embedding_models=lambda _kbs: None,
    )
    install_module("api.db.services.langfuse_service", TenantLangfuseService=_Dummy)
    install_module(
        "api.db.services.llm_service",
        LLMBundle=_Dummy,
        resolve_llm_setting=lambda *_args, **_kwargs: {},
    )
    install_module(
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_args, **_kwargs: {"model_type": "chat", "max_tokens": 8192},
        get_model_config_from_provider_instance=lambda *_args, **_kwargs: {},
        get_model_type_by_name=lambda *_args, **_kwargs: [],
        resolve_model_config=lambda *_args, **_kwargs: ({"model_type": "chat"}, None),
        resolve_model_type=lambda *_args, **_kwargs: None,
        get_model_config_by_id=lambda *_args, **_kwargs: {},
    )
    install_module("api.db.db_models", DB=_DummyDB, Dialog=_Dummy)
    install_module("common.metadata_utils", apply_meta_data_filter=lambda *_args, **_kwargs: None)
    install_module(
        "api.utils.reference_metadata_utils",
        enrich_chunks_with_document_metadata=lambda chunks, *_args, **_kwargs: chunks,
        resolve_reference_metadata_preferences=lambda *_args, **_kwargs: {},
    )
    install_module("rag.graphrag.general.mind_map_extractor", MindMapExtractor=_Dummy)
    advanced_rag = install_module("rag.advanced_rag")
    advanced_rag.__path__ = []
    install_module("rag.advanced_rag.agentic_rag", RAGTools=_Dummy)
    install_module("rag.advanced_rag.knowlege_compile")
    install_module("rag.advanced_rag.knowlege_compile.mind_map_extractor", MindMapExtractor=_Dummy)
    install_module("rag.app.tag", label_question=lambda *_args, **_kwargs: None)
    install_module("rag.nlp.search", index_name=lambda tenant_id: f"ragflow_{tenant_id}")
    install_module(
        "rag.prompts.generator",
        chunks_format=lambda *_args, **_kwargs: "",
        citation_prompt=lambda: "",
        cross_languages=lambda *_args, **_kwargs: "",
        full_question=lambda *_args, **_kwargs: "",
        kb_prompt=lambda *_args, **_kwargs: [],
        keyword_extraction=lambda *_args, **_kwargs: "",
        message_fit_in=lambda *_args, **_kwargs: (0, []),
        PROMPT_JINJA_ENV=types.SimpleNamespace(from_string=lambda *_args, **_kwargs: types.SimpleNamespace(render=lambda **_kw: "")),
        ASK_SUMMARY="",
    )
    install_module("common.token_utils", num_tokens_from_string=lambda *_args, **_kwargs: 0)
    install_module("rag.utils.tavily_conn", Tavily=_Dummy)
    install_module("rag.utils.tts_cache", synthesize_with_cache=lambda *_args, **_kwargs: None)


_install_cv2_stub_if_unavailable()


class FakeChatModel:
    def __init__(self, responses):
        self.responses = list(responses)
        self.calls = []

    async def async_chat(self, sys_prompt, messages, params):
        self.calls.append((sys_prompt, messages, params))
        if not self.responses:
            raise AssertionError("async_chat called more times than expected")
        return self.responses.pop(0)


class RecordingRetriever:
    def __init__(self, fail_first=False, missing_source_once=False):
        self.sqls = []
        self.fail_first = fail_first
        self.missing_source_once = missing_source_once

    def sql_retrieval(self, sql, format="json"):
        assert format == "json"
        self.sqls.append(sql)
        if self.fail_first and len(self.sqls) == 1:
            raise RuntimeError("json_extract_string does not exist")
        missing_source_call = 2 if self.fail_first else 1
        if self.missing_source_once and len(self.sqls) == missing_source_call:
            return {"columns": [{"name": "amount"}], "rows": [["120"]]}
        return {
            "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "kb_id"}, {"name": "amount"}],
            "rows": [["doc1", "finance.csv", "abcdefabcdefabcdefabcdefabcdefab", "120"]],
        }


class AggregateRecordingRetriever:
    def __init__(self):
        self.sqls = []

    def sql_retrieval(self, sql, format="json"):
        assert format == "json"
        self.sqls.append(sql)
        if len(self.sqls) == 1:
            return {"columns": [{"name": "total_amount"}], "rows": [["120"]]}
        return {
            "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "kb_id"}],
            "rows": [["doc1", "finance.csv", "abcdefabcdefabcdefabcdefabcdefab"]],
        }


class MultiKBReferenceRetriever:
    def __init__(self):
        self.sqls = []

    def sql_retrieval(self, sql, format="json"):
        assert format == "json"
        self.sqls.append(sql)
        if len(self.sqls) == 1:
            return {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "amount"}],
                "rows": [
                    ["doc1", "finance-a.csv", "120"],
                    ["doc2", "finance-b.csv", "80"],
                    ["doc3", "finance-c.csv", "40"],
                ],
            }
        return {
            "columns": [{"name": "doc_id"}, {"name": "kb_id"}],
            "rows": [
                ["doc1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"],
                ["doc2", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"],
                ["doc3", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"],
            ],
        }


class MultiKBReferenceWithInlineKB:
    def __init__(self):
        self.sqls = []

    def sql_retrieval(self, sql, format="json"):
        assert format == "json"
        self.sqls.append(sql)
        return {
            "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "kb_id"}],
            "rows": [["doc1", "finance.csv", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]],
        }


class ScriptedRetriever:
    def __init__(self, sql_results=(), retrieval_result=None):
        self.sql_results = list(sql_results)
        self.sqls = []
        self.retrieval = AsyncMock(
            return_value=retrieval_result
            or {
                "total": 1,
                "chunks": [
                    {
                        "id": "chunk-1",
                        "doc_id": "d1",
                        "docnm_kwd": "doc.xlsx",
                        "kb_id": "abcdefabcdefabcdefabcdefabcdefab",
                        "content_with_weight": "fallback content",
                        "content_ltks": "fallback content",
                        "vector": [],
                    }
                ],
                "doc_aggs": [{"doc_id": "d1", "doc_name": "doc.xlsx", "count": 1}],
            }
        )

    def sql_retrieval(self, sql, format="json"):
        assert format == "json"
        self.sqls.append(sql)
        if not self.sql_results:
            raise AssertionError("sql_retrieval called more times than expected")
        result = self.sql_results.pop(0)
        if isinstance(result, BaseException):
            raise result
        return result

    @staticmethod
    def retrieval_by_children(chunks, _tenant_ids):
        return chunks

    @staticmethod
    def insert_citations(answer, *_args, **_kwargs):
        return answer, set()


@pytest.fixture
def dialog_service(monkeypatch):
    _install_settings_import_stubs(monkeypatch)
    module = importlib.import_module("api.db.services.dialog_service")
    try:
        yield module
    finally:
        sys.modules.pop("api.db.services.dialog_service", None)
        package = sys.modules.get("api.db.services")
        if package is not None and hasattr(package, "dialog_service"):
            delattr(package, "dialog_service")


@pytest.fixture
def enable_gaussdb_docengine(monkeypatch, dialog_service):
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_INFINITY", False, raising=False)
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_OCEANBASE", False, raising=False)
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_GAUSSDB", True, raising=False)


def test_tc_sql_601_gaussdb_prompt_uses_jsonb_path_not_oceanbase_json_helpers(dialog_service, enable_gaussdb_docengine):
    prompt = dialog_service.gaussdb_text_to_sql.build_sql_prompt(
        table_name="ragflow_0123456789abcdef0123456789abcdef",
        field_map={"amount": "number", "status": "string"},
        question="amount greater than 100",
    )

    assert "JSONB text extraction: chunk_data #>> '{FieldName}'" in prompt
    assert "JSONB value extraction: chunk_data #> '{FieldName}'" in prompt
    assert "Numeric cast: CAST(chunk_data #>> '{FieldName}' AS DOUBLE PRECISION)" in prompt
    assert "Date cast: to_date(chunk_data #>> '{FieldName}', 'YYYY-MM-DD')" in prompt
    assert "NULL check: (chunk_data #>> '{FieldName}') IS NOT NULL" in prompt
    assert "#>> '{amount}'" in prompt
    assert "  - status (string): chunk_data #>> '{status}'" in prompt
    assert "json_extract_string(" not in prompt
    assert "Select doc_id and docnm_kwd for non-aggregate data queries." in prompt
    assert "Do not add kb_id" in prompt
    assert "Do not use json_extract, json_extract_string, or json_extract_isnull" in prompt
    assert "Restrict the query by kb_id" not in prompt
    _assert_gaussdb_sql_rule_markers(prompt)


def test_tc_sql_506_sql_converts_row_values_after_runtime_validation(dialog_service):
    from rag.utils.gaussdb_conn import GaussDBConnection

    conn = object.__new__(GaussDBConnection)
    conn.schema = "public"
    conn.logger = logging.getLogger("test.gaussdb")
    conn._fetch_all_with_description = Mock(
        return_value=(
            [(b"abc", {"a": 1}, [1, "x"], None)],
            [("bytes_col",), ("object_col",), ("list_col",), ("null_col",)],
        )
    )
    sql = (
        "SELECT doc_id AS bytes_col, chunk_data #> '{object_col}' AS object_col, "
        "chunk_data #> '{list_col}' AS list_col, chunk_data #>> '{null_col}' AS null_col "
        "FROM ragflow_tc_sql_506 WHERE kb_id = 'kb-506'"
    )

    result = conn.sql(sql, format="json")

    executed_sql, params = conn._fetch_all_with_description.call_args.args
    assert executed_sql == (
        "SELECT doc_id AS bytes_col, chunk_data #> '{object_col}' AS object_col, "
        "chunk_data #> '{list_col}' AS list_col, chunk_data #>> '{null_col}' AS null_col "
        "FROM \"public\".ragflow_tc_sql_506 WHERE kb_id = 'kb-506' LIMIT 128"
    )
    assert params == []
    assert conn._fetch_all_with_description.call_args.kwargs == {
        "statement_timeout_ms": 30000,
    }
    assert result == {
        "columns": [
            {"name": "bytes_col", "type": "text"},
            {"name": "object_col", "type": "text"},
            {"name": "list_col", "type": "text"},
            {"name": "null_col", "type": "text"},
        ],
        "rows": [["abc", '{"a": 1}', '[1, "x"]', None]],
    }


def test_tc_sql_609_gaussdb_prompt_uses_field_map_value_as_jsonb_path(dialog_service, enable_gaussdb_docengine):
    prompt = dialog_service.gaussdb_text_to_sql.build_sql_prompt(
        table_name="ragflow_0123456789abcdef0123456789abcdef",
        field_map={"customer_alias": "customer_name", "amount": "number"},
        question="show customer amount",
    )

    assert "  - customer_alias (customer_name): chunk_data #>> '{customer_name}'" in prompt
    assert "  - amount (number): chunk_data #>> '{amount}'" in prompt
    _assert_gaussdb_sql_rule_markers(prompt)


def test_tc_sql_410_empty_value_prompt_rules_and_validator_rejects_empty_string_comparison(
    dialog_service,
    enable_gaussdb_docengine,
):
    prompt = dialog_service.gaussdb_text_to_sql.build_sql_prompt(
        "ragflow_t1",
        {"status": "string"},
        "show rows where status is empty",
    )
    validator = dialog_service.gaussdb_text_to_sql.build_validator("ragflow_t1", ["kb-1"], {"status": "string"})

    with pytest.raises(ValueError, match="JSONB text cannot be compared with an empty SQL string"):
        validator.validate_and_patch("SELECT doc_id FROM ragflow_t1 WHERE chunk_data #>> '{status}' = ''")

    assert "NULL check: (chunk_data #>> '{FieldName}') IS NOT NULL" in prompt
    assert "Do not use json_extract, json_extract_string, or json_extract_isnull; use #>> / #> instead" in prompt
    _assert_gaussdb_sql_rule_markers(prompt)


def test_tc_sql_001_async_chat_routes_table_field_map_to_gaussdb_use_sql(monkeypatch, dialog_service, enable_gaussdb_docengine):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    sql_result = {
        "answer": "|amount|\n|---|\n|100|",
        "reference": {"chunks": [], "doc_aggs": []},
        "prompt": "gaussdb prompt",
    }
    get_field_map = Mock(return_value={"amount": "number"})
    use_sql = AsyncMock(return_value=sql_result)
    retrieval = AsyncMock()

    chat = object()
    monkeypatch.setattr(
        dialog_service.settings,
        "retriever",
        types.SimpleNamespace(retrieval=retrieval),
        raising=False,
    )
    monkeypatch.setattr(
        dialog_service,
        "KnowledgebaseService",
        types.SimpleNamespace(get_field_map=get_field_map),
        raising=False,
    )
    monkeypatch.setattr(dialog_service, "use_sql", use_sql)
    monkeypatch.setattr(dialog_service, "get_models", lambda *_args, **_kwargs: ([], None, None, chat, None))
    monkeypatch.setattr(dialog_service, "_resolve_reference_metadata", lambda *_args, **_kwargs: (False, []))
    monkeypatch.setattr(
        dialog_service,
        "TenantLangfuseService",
        types.SimpleNamespace(filter_by_tenant=lambda **_kwargs: None),
        raising=False,
    )
    dialog = types.SimpleNamespace(
        tenant_id=tenant_id,
        kb_ids=[kb_id],
        prompt_config={"quote": True, "parameters": [], "system": ""},
        llm_id=None,
        tenant_llm_id=None,
        meta_data_filter=None,
    )

    async def collect():
        results = []
        async for item in dialog_service.async_chat(dialog, [{"role": "user", "content": "What is the total amount?"}], stream=False):
            results.append(item)
        return results

    results = asyncio.run(collect())

    get_field_map.assert_called_once_with([kb_id])
    use_sql.assert_awaited_once_with(
        "What is the total amount?",
        {"amount": "number"},
        tenant_id,
        chat,
        True,
        [kb_id],
        doc_ids=None,
    )
    retrieval.assert_not_awaited()
    assert results == [sql_result]


def _run_field_map_empty_chat(monkeypatch, dialog_service, chunks, doc_aggs):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    get_field_map = Mock(return_value={})
    use_sql = AsyncMock()
    retrieval = AsyncMock(return_value={"total": len(chunks), "chunks": chunks, "doc_aggs": doc_aggs})
    retriever = types.SimpleNamespace(
        retrieval=retrieval,
        retrieval_by_children=lambda value, _tenant_ids: value,
        insert_citations=lambda answer, *_args, **_kwargs: (answer, set()),
    )
    chat = FakeChatModel(["fallback answer"])

    def sync_get_field_map(kb_ids):
        return get_field_map(kb_ids)

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service,
        "KnowledgebaseService",
        types.SimpleNamespace(get_field_map=sync_get_field_map),
        raising=False,
    )
    monkeypatch.setattr(dialog_service, "use_sql", use_sql)
    monkeypatch.setattr(
        dialog_service,
        "get_tenant_default_model_by_type",
        lambda *_args, **_kwargs: {"model_type": "chat", "max_tokens": 8192},
    )
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda *_args, **_kwargs: ([types.SimpleNamespace(tenant_id=tenant_id)], object(), None, chat, None),
    )
    monkeypatch.setattr(dialog_service, "kb_prompt", lambda *_args, **_kwargs: ["content"])
    monkeypatch.setattr(dialog_service, "message_fit_in", lambda messages, _limit: (0, messages))
    monkeypatch.setattr(dialog_service, "tts", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(dialog_service, "_resolve_reference_metadata", lambda *_args, **_kwargs: (False, []))
    monkeypatch.setattr(
        dialog_service,
        "TenantLangfuseService",
        types.SimpleNamespace(filter_by_tenant=lambda **_kwargs: None),
        raising=False,
    )
    dialog = types.SimpleNamespace(
        tenant_id=tenant_id,
        kb_ids=[kb_id],
        prompt_config={
            "quote": True,
            "parameters": [{"key": "knowledge", "optional": False}],
            "system": "Use {knowledge}",
        },
        llm_id=None,
        tenant_llm_id=None,
        top_n=8,
        top_k=32,
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        meta_data_filter=None,
        llm_setting={},
    )

    async def collect():
        return [
            item
            async for item in dialog_service.async_chat(
                dialog,
                [{"role": "user", "content": "What is the content?"}],
                stream=False,
            )
        ]

    return asyncio.run(collect()), get_field_map, use_sql, retrieval, kb_id


_USE_REAL_SQL = object()


def _run_configured_chat(
    monkeypatch,
    dialog_service,
    *,
    retriever,
    chat,
    field_map=None,
    question="What is the total amount?",
    use_sql_mock=_USE_REAL_SQL,
    embd_mdl=None,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    if embd_mdl is None:
        embd_mdl = object()
    resolved_field_map = field_map if field_map is not None else {"amount": "number"}
    get_field_map = Mock(return_value=resolved_field_map)
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service,
        "KnowledgebaseService",
        types.SimpleNamespace(get_field_map=get_field_map),
        raising=False,
    )
    if use_sql_mock is not _USE_REAL_SQL:
        monkeypatch.setattr(dialog_service, "use_sql", use_sql_mock)
    monkeypatch.setattr(
        dialog_service,
        "get_tenant_default_model_by_type",
        lambda *_args, **_kwargs: {"model_type": "chat", "max_tokens": 8192},
    )
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda *_args, **_kwargs: (
            [types.SimpleNamespace(id=kb_id, tenant_id=tenant_id, parser_config={"field_map": resolved_field_map})],
            embd_mdl,
            None,
            chat,
            None,
        ),
    )
    monkeypatch.setattr(dialog_service, "kb_prompt", lambda *_args, **_kwargs: ["fallback content"])
    monkeypatch.setattr(dialog_service, "message_fit_in", lambda messages, _limit: (0, messages))
    monkeypatch.setattr(dialog_service, "tts", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(dialog_service, "_resolve_reference_metadata", lambda *_args, **_kwargs: (False, []))
    monkeypatch.setattr(
        dialog_service,
        "TenantLangfuseService",
        types.SimpleNamespace(filter_by_tenant=lambda **_kwargs: None),
        raising=False,
    )
    dialog = types.SimpleNamespace(
        tenant_id=tenant_id,
        kb_ids=[kb_id],
        prompt_config={
            "quote": True,
            "parameters": [{"key": "knowledge", "optional": False}],
            "system": "Use {knowledge}",
        },
        llm_id=None,
        tenant_llm_id=None,
        top_n=8,
        top_k=32,
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        meta_data_filter=None,
        llm_setting={},
    )

    async def collect():
        return [
            item
            async for item in dialog_service.async_chat(
                dialog,
                [{"role": "user", "content": question}],
                stream=False,
            )
        ]

    return asyncio.run(collect()), get_field_map, kb_id


def _expected_fallback_reference(kb_id):
    return {
        "total": 1,
        "chunks": [
            {
                "id": "chunk-1",
                "doc_id": "d1",
                "docnm_kwd": "doc.xlsx",
                "kb_id": kb_id,
                "content_with_weight": "fallback content",
                "content_ltks": "fallback content",
                "vector": [],
            }
        ],
        "doc_aggs": [{"doc_id": "d1", "doc_name": "doc.xlsx", "count": 1}],
    }


def test_tc_sql_002_field_map_empty_uses_normal_retrieval(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    chunks = [
        {
            "id": "chunk-1",
            "doc_id": "doc-1",
            "docnm_kwd": "doc.xlsx",
            "kb_id": "abcdefabcdefabcdefabcdefabcdefab",
            "content_with_weight": "content",
            "content_ltks": "content",
            "vector": [],
        }
    ]
    doc_aggs = [{"doc_id": "doc-1", "doc_name": "doc.xlsx", "count": 1}]

    with caplog.at_level(logging.DEBUG):
        results, get_field_map, use_sql, retrieval, kb_id = _run_field_map_empty_chat(
            monkeypatch,
            dialog_service,
            chunks,
            doc_aggs,
        )

    get_field_map.assert_called_once_with([kb_id])
    use_sql.assert_not_awaited()
    retrieval.assert_awaited_once()
    assert retrieval.await_args.args[3] == [kb_id]
    assert results[-1]["answer"] == "fallback answer"
    assert results[-1]["reference"]["chunks"] == chunks
    assert results[-1]["reference"]["doc_aggs"] == doc_aggs
    assert "Use SQL to retrieval" not in caplog.text


def test_tc_sql_004_use_sql_non_gaussdb_engine_does_not_call_gaussdb_prompt(monkeypatch, dialog_service):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    retriever = RecordingRetriever()

    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_INFINITY", False, raising=False)
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_OCEANBASE", False, raising=False)
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_GAUSSDB", False, raising=False)
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    gaussdb_prompt = Mock(side_effect=AssertionError("GaussDB prompt must not be used"))
    gaussdb_validator = Mock(side_effect=AssertionError("GaussDB validator must not be constructed"))
    monkeypatch.setattr(dialog_service.gaussdb_text_to_sql, "build_sql_prompt", gaussdb_prompt)
    monkeypatch.setattr(dialog_service.gaussdb_text_to_sql, "build_validator", gaussdb_validator)
    generated_sql = f"SELECT doc_id, docnm_kwd FROM ragflow_{tenant_id}"
    chat = FakeChatModel([generated_sql])

    result = asyncio.run(
        dialog_service.use_sql(
            "show docs",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    expected_sys_prompt = """You are a Database Administrator. Write SQL queries.

RULES:
1. Use EXACT field names from the schema below (e.g., product_tks, not product)
2. Quote field names starting with digit: "123_field"
3. Add IS NOT NULL in WHERE clause when:
   - Question asks to "show me" or "display" specific columns
4. Include doc_id/docnm in non-aggregate statement
5. Output ONLY the SQL, no explanations"""
    expected_user_prompt = f"""Table: ragflow_{tenant_id}
Available fields:
  - amount (number)
Question: show docs
Write SQL using exact field names above. Include doc_id, docnm_kwd for data queries. Only SQL."""
    expected_sql = f"{generated_sql} WHERE kb_id = '{kb_id}'"

    gaussdb_prompt.assert_not_called()
    gaussdb_validator.assert_not_called()
    assert chat.calls == [
        (
            expected_sys_prompt,
            [{"role": "user", "content": expected_user_prompt}],
            {"temperature": 0.06},
        )
    ]
    assert retriever.sqls == [expected_sql]
    assert result == {
        "answer": "|number|Source|\n|------|------|\n|120| ##0$$|",
        "reference": {
            "chunks": [
                {
                    "doc_id": "doc1",
                    "docnm_kwd": "finance.csv",
                    "kb_id": "abcdefabcdefabcdefabcdefabcdefab",
                }
            ],
            "doc_aggs": [{"doc_id": "doc1", "doc_name": "finance.csv", "count": 1}],
        },
        "prompt": expected_sys_prompt,
    }

    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_INFINITY", True, raising=False)
    infinity_retriever = RecordingRetriever(fail_first=True, missing_source_once=True)
    monkeypatch.setattr(dialog_service.settings, "retriever", infinity_retriever, raising=False)
    infinity_table = f"ragflow_{tenant_id}_{kb_id}"
    infinity_sql = f"SELECT doc_id, docnm FROM {infinity_table}"
    infinity_chat = FakeChatModel([infinity_sql, infinity_sql, infinity_sql])

    infinity_result = asyncio.run(
        dialog_service.use_sql(
            "show docs",
            {"amount": "number"},
            tenant_id,
            infinity_chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    assert len(infinity_chat.calls) == 3
    retry_prompt = infinity_chat.calls[1][1][0]["content"]
    source_repair_prompt = infinity_chat.calls[2][1][0]["content"]
    assert "json_extract_string(chunk_data, '$.field_name')" in retry_prompt
    assert "Correct the SQL for GaussDB" not in retry_prompt
    assert "missing required source columns" in source_repair_prompt
    assert "include doc_id and docnm in the SELECT list" in source_repair_prompt
    assert "json_extract_string(chunk_data, '$.field_name')" in source_repair_prompt
    assert "chunk_data #>>" not in source_repair_prompt
    assert infinity_retriever.sqls == [infinity_sql, infinity_sql, infinity_sql]
    assert infinity_result["reference"]["chunks"] == [
        {
            "doc_id": "doc1",
            "docnm_kwd": "finance.csv",
            "kb_id": "abcdefabcdefabcdefabcdefabcdefab",
        }
    ]
    gaussdb_prompt.assert_not_called()
    gaussdb_validator.assert_not_called()


def test_tc_sql_601_use_sql_routes_gaussdb_prompt_through_retriever(monkeypatch, dialog_service, enable_gaussdb_docengine):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = RecordingRetriever()
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{kb_id}' AND (chunk_data #>> '{{amount}}')::DOUBLE PRECISION > 100"])

    result = asyncio.run(
        dialog_service.use_sql(
            "finance amount greater than 100",
            {"amount": "number", "status": "string"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    assert result is not None
    assert "120" in result["answer"]
    assert len(chat.calls) == 1
    system_prompt, messages, params = chat.calls[0]
    assert f"Table: {table}" in system_prompt
    assert "  - amount (number): chunk_data #>> '{amount}'" in system_prompt
    assert "  - status (string): chunk_data #>> '{status}'" in system_prompt
    assert "json_extract_string(chunk_data" not in system_prompt
    assert messages == [
        {
            "role": "user",
            "content": (
                f"Table: {table}\n"
                "Fields:\n"
                "  - amount (number): chunk_data #>> '{amount}'\n"
                "  - status (string): chunk_data #>> '{status}'\n"
                "Question: finance amount greater than 100\n"
                "Write SQL using GaussDB JSONB #>> / #> syntax. "
                "Include doc_id and docnm_kwd for data queries. Only SQL."
            ),
        }
    ]
    assert params == {"temperature": 0.06}
    assert retriever.sqls == [
        f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{kb_id}' AND CAST((chunk_data #>> '{{amount}}') AS DOUBLE PRECISION) > 100 LIMIT 128"
    ]


def test_tc_sql_601_gaussdb_injects_doc_scope_before_validator(monkeypatch, dialog_service, enable_gaussdb_docengine):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    doc_id = "12345678123456781234567812345678"
    table = f"ragflow_{tenant_id}"
    retriever = RecordingRetriever()
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table}"])

    result = asyncio.run(
        dialog_service.use_sql(
            "show the scoped document amount",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
            doc_ids=[doc_id],
        )
    )

    assert result is not None
    assert retriever.sqls == [f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE doc_id = '{doc_id}' AND kb_id = '{kb_id}' LIMIT 128"]


def test_tc_sql_602_row_count_override_skips_llm_and_executes_scoped_count(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {"columns": [{"name": "rows"}], "rows": [[5]]},
            {"columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}], "rows": []},
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([])

    result = asyncio.run(
        dialog_service.use_sql(
            "How many rows in the dataset?",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    assert chat.calls == []
    assert retriever.sqls[0] == (f"SELECT COUNT(*) AS rows FROM {table} WHERE kb_id = '{kb_id}' LIMIT 128")
    assert retriever.sqls[1] == (f"SELECT doc_id, docnm_kwd FROM {table} WHERE kb_id = '{kb_id}' LIMIT 128")
    assert result["answer"] == "|rows|\n|------\n|5|"
    assert result["reference"] == {"chunks": [], "doc_aggs": []}


def test_tc_sql_603_validator_rejection_builds_retry_prompt(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "amount"}],
                "rows": [["d1", "doc.xlsx", "120"]],
            }
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel(
        [
            f"SELECT * FROM {table}",
            f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table}",
        ]
    )

    with caplog.at_level(logging.WARNING):
        result = asyncio.run(
            dialog_service.use_sql(
                "finance amount greater than 100",
                {"amount": "number", "dept": "string"},
                tenant_id,
                chat,
                quota=False,
                kb_ids=[kb_id],
            )
        )

    assert result["reference"]["chunks"] == [{"doc_id": "d1", "docnm_kwd": "doc.xlsx", "kb_id": kb_id}]
    assert result["answer"] == "|number|Source|\n|------|------|\n|120| ##0$$|"
    assert result["reference"]["doc_aggs"] == [{"doc_id": "d1", "doc_name": "doc.xlsx", "count": 1}]
    assert len(chat.calls) == 2
    assert len(retriever.sqls) == 1
    assert f"kb_id = '{kb_id}'" in retriever.sqls[0]
    retry_prompt = chat.calls[1][1][0]["content"]
    assert "SELECT * is not allowed" in retry_prompt
    assert "Initial SQL execution FAILED with error: SELECT * is not allowed" in caplog.text
    assert "Do not use json_extract, json_extract_string, or json_extract_isnull" in retry_prompt
    _assert_gaussdb_sql_rule_markers(retry_prompt)


def test_tc_sql_604_execution_none_retries(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    sql = f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table}"
    retriever = ScriptedRetriever(
        [
            None,
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "amount"}],
                "rows": [["d1", "doc.xlsx", "120"]],
            },
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([sql, sql])

    result = asyncio.run(
        dialog_service.use_sql(
            "Show all amounts",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    expected_sql = f"{sql} WHERE kb_id = '{kb_id}' LIMIT 128"
    assert len(chat.calls) == 2
    assert retriever.sqls == [expected_sql, expected_sql]
    assert result["answer"] == "|number|Source|\n|------|------|\n|120| ##0$$|"
    assert result["reference"] == {
        "chunks": [{"doc_id": "d1", "docnm_kwd": "doc.xlsx", "kb_id": kb_id}],
        "doc_aggs": [{"doc_id": "d1", "doc_name": "doc.xlsx", "count": 1}],
    }


@pytest.mark.parametrize("mode", ["validator_rejected", "execution_failed"])
def test_tc_sql_605_retry_exhaustion_returns_none(
    mode,
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    if mode == "validator_rejected":
        chat = FakeChatModel([f"SELECT * FROM {table}", f"SELECT * FROM {table}"])
        retriever = ScriptedRetriever([])
        expected_sql_calls = 0
    else:
        valid_sql = f"SELECT doc_id, docnm_kwd FROM {table}"
        chat = FakeChatModel([valid_sql, valid_sql])
        retriever = ScriptedRetriever([None, None])
        expected_sql_calls = 2
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    with caplog.at_level(logging.WARNING):
        result = asyncio.run(
            dialog_service.use_sql(
                "Show all amounts",
                {"amount": "number"},
                tenant_id,
                chat,
                quota=False,
                kb_ids=[kb_id],
            )
        )

    assert result is None
    assert len(chat.calls) == 2
    assert len(retriever.sqls) == expected_sql_calls
    if mode == "validator_rejected":
        assert "SELECT * is not allowed" in caplog.text
    assert "Retry SQL execution also FAILED" in caplog.text


def test_tc_sql_606_retry_sql_is_revalidated_and_scoped(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "amount"}],
                "rows": [["d1", "doc.xlsx", "120"]],
            }
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel(
        [
            f"SELECT doc_id, chunk_data FROM {table}",
            f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table}",
        ]
    )

    result = asyncio.run(
        dialog_service.use_sql(
            "Show all amounts",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    assert len(chat.calls) == 2
    assert retriever.sqls == [f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{kb_id}' LIMIT 128"]
    assert result["reference"]["chunks"][0]["kb_id"] == kb_id


def test_tc_sql_607_missing_source_columns_trigger_dedicated_repair(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {"columns": [{"name": "amount"}], "rows": [["120"]]},
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "amount"}],
                "rows": [["d1", "doc.xlsx", "120"]],
            },
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel(
        [
            f"SELECT chunk_data #>> '{{amount}}' AS amount FROM {table}",
            f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table}",
        ]
    )

    result = asyncio.run(
        dialog_service.use_sql(
            "Show all amounts",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    assert len(chat.calls) == 2
    repair_prompt = chat.calls[1][1][0]["content"]
    assert "missing required source columns" in repair_prompt
    _assert_gaussdb_sql_rule_markers(repair_prompt)
    assert retriever.sqls == [
        f"SELECT chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{kb_id}' LIMIT 128",
        f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{kb_id}' LIMIT 128",
    ]
    assert result["answer"] == "|number|Source|\n|------|------|\n|120| ##0$$|"
    assert result["reference"] == {
        "chunks": [{"doc_id": "d1", "docnm_kwd": "doc.xlsx", "kb_id": kb_id}],
        "doc_aggs": [{"doc_id": "d1", "doc_name": "doc.xlsx", "count": 1}],
    }


def test_tc_sql_608_source_repair_failure_returns_best_effort_answer(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {"columns": [{"name": "amount"}], "rows": [["120"]]},
            RuntimeError("source lookup failed"),
        ]
    )
    chat = FakeChatModel(
        [
            f"SELECT chunk_data #>> '{{amount}}' AS amount FROM {table}",
            f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table}",
        ]
    )

    with caplog.at_level(logging.WARNING):
        results, get_field_map, actual_kb_id = _run_configured_chat(
            monkeypatch,
            dialog_service,
            retriever=retriever,
            chat=chat,
            question="Show all amounts",
        )

    assert actual_kb_id == kb_id
    get_field_map.assert_called_once_with([kb_id])
    assert len(chat.calls) == 2
    assert retriever.sqls == [
        f"SELECT chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{kb_id}' LIMIT 128",
        f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{kb_id}' LIMIT 128",
    ]
    retriever.retrieval.assert_not_awaited()
    assert len(results) == 1
    assert results[0]["answer"] == "|number|\n|------\n|120|"
    assert results[0]["reference"] == {"chunks": [], "doc_aggs": []}
    _assert_gaussdb_sql_rule_markers(results[0]["prompt"])
    assert "Source-column SQL repair failed" in caplog.text
    assert "source lookup failed" in caplog.text
    assert "SQL failed or returned no results, falling back to vector search" not in caplog.text


def test_tc_sql_701_use_sql_aggregate_fallback_fetches_sources_with_kb_scope(monkeypatch, dialog_service, enable_gaussdb_docengine):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = AggregateRecordingRetriever()
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT SUM((chunk_data #>> '{{amount}}')::DOUBLE PRECISION) AS total_amount FROM {table} WHERE chunk_data #>> '{{dept}}' = 'finance' LIMIT 3"])

    result = asyncio.run(
        dialog_service.use_sql(
            "sum finance amount",
            {"amount": "number", "dept": "string"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    assert result is not None
    assert len(retriever.sqls) == 2
    assert retriever.sqls[0].endswith("LIMIT 3")
    source_sql = retriever.sqls[1]
    assert source_sql.startswith(f"SELECT doc_id, docnm_kwd FROM {table} WHERE ")
    assert "chunk_data #>> '{dept}' = 'finance'" in source_sql
    assert f"kb_id = '{kb_id}'" in source_sql
    assert source_sql.endswith("LIMIT 128")
    assert "LIMIT 20" not in source_sql
    assert result["reference"]["chunks"][0]["kb_id"] == kb_id


def test_tc_sql_701_aggregate_source_builder_projects_multikb_scope_and_rejects_having(dialog_service):
    source_sql = dialog_service.gaussdb_text_to_sql.build_aggregate_source_sql(
        "SELECT kb_id, SUM((chunk_data #>> '{amount}')::DOUBLE PRECISION) AS total FROM ragflow_tenant WHERE kb_id IN ('k1', 'k2') GROUP BY kb_id ORDER BY total DESC LIMIT 3 OFFSET 1",
        "docnm_kwd",
        include_kb_id=True,
    )

    assert source_sql == "SELECT doc_id, docnm_kwd, kb_id FROM ragflow_tenant WHERE kb_id IN ('k1', 'k2')"
    reference = dialog_service.gaussdb_text_to_sql.build_source_reference(
        {
            "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "kb_id"}],
            "rows": [["d1", "one.xlsx", "k1"], ["d2", "two.xlsx", "k2"]],
        },
        ["k1", "k2"],
    )
    assert reference == {
        "chunks": [
            {"doc_id": "d1", "docnm_kwd": "one.xlsx", "kb_id": "k1"},
            {"doc_id": "d2", "docnm_kwd": "two.xlsx", "kb_id": "k2"},
        ],
        "doc_aggs": [
            {"doc_id": "d1", "doc_name": "one.xlsx", "count": 1},
            {"doc_id": "d2", "doc_name": "two.xlsx", "count": 1},
        ],
    }
    with pytest.raises(ValueError) as exc_info:
        dialog_service.gaussdb_text_to_sql.build_aggregate_source_sql(
            "SELECT SUM((chunk_data #>> '{amount}')::DOUBLE PRECISION) AS total FROM ragflow_tenant GROUP BY kb_id HAVING SUM((chunk_data #>> '{amount}')::DOUBLE PRECISION) > 0",
            "docnm_kwd",
            include_kb_id=False,
        )
    assert str(exc_info.value) == "GaussDB aggregate source lookup cannot preserve HAVING safely"


@pytest.mark.parametrize(
    ("projection", "value"),
    [
        ("'sum(amount)' AS note", "sum(amount)"),
        ("1 AS note /* sum(amount) */", 1),
    ],
    ids=["literal", "comment"],
)
def test_tc_sql_705_use_sql_does_not_treat_aggregate_names_in_literals_or_comments_as_aggregates(
    projection,
    value,
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {"columns": [{"name": "note"}], "rows": [[value]]},
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "note"}],
                "rows": [["d1", "doc.xlsx", value]],
            },
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel(
        [
            f"SELECT {projection} FROM {table}",
            f"SELECT doc_id, docnm_kwd, {projection} FROM {table}",
        ]
    )

    result = asyncio.run(
        dialog_service.use_sql(
            "show the note",
            {"note": "string"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb_id],
        )
    )

    assert len(chat.calls) == 2
    assert len(retriever.sqls) == 2
    repair_prompt = chat.calls[1][1][0]["content"]
    assert "missing required source columns" in repair_prompt
    _assert_gaussdb_sql_rule_markers(repair_prompt)
    assert result["reference"]["chunks"] == [{"doc_id": "d1", "docnm_kwd": "doc.xlsx", "kb_id": kb_id}]


def test_tc_sql_005_use_sql_requires_kb_ids_for_gaussdb(monkeypatch, dialog_service, enable_gaussdb_docengine):
    validator = Mock()
    monkeypatch.setattr(dialog_service.gaussdb_text_to_sql, "build_validator", validator)

    with pytest.raises(ValueError) as exc_info:
        asyncio.run(
            dialog_service.use_sql(
                "show amount",
                {"amount": "number"},
                "0123456789abcdef0123456789abcdef",
                FakeChatModel([]),
                quota=False,
                kb_ids=[],
            )
        )

    assert str(exc_info.value) == "GaussDB Text-to-SQL requires kb_ids"
    validator.assert_not_called()


def test_tc_sql_703_aggregate_source_lookup_failure_returns_empty_chunks(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    table = f"ragflow_{tenant_id}"
    assert (
        dialog_service.gaussdb_text_to_sql.build_source_reference(
            {"columns": [{"name": "total_amount"}], "rows": [["120"]]},
            [kb_id],
        )
        is None
    )

    class AggregateSourceFailureRetriever:
        def __init__(self):
            self.sqls = []

        def sql_retrieval(self, sql, format="json"):
            assert format == "json"
            self.sqls.append(sql)
            if len(self.sqls) == 1:
                return {"columns": [{"name": "total_amount"}], "rows": [["120"]]}
            raise RuntimeError("source lookup failed")

    retriever = AggregateSourceFailureRetriever()
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT SUM((chunk_data #>> '{{amount}}')::DOUBLE PRECISION) AS total_amount FROM {table} WHERE kb_id = '{kb_id}'"])

    with caplog.at_level(logging.WARNING):
        result = asyncio.run(
            dialog_service.use_sql(
                "What is the total amount?",
                {"amount": "number"},
                tenant_id,
                chat,
                quota=False,
                kb_ids=[kb_id],
            )
        )

    assert len(retriever.sqls) == 2
    assert result == {
        "answer": "|total_number|\n|------\n|120|",
        "reference": {"chunks": [], "doc_aggs": []},
        "prompt": chat.calls[0][0],
    }
    assert "Failed to fetch chunks" in caplog.text


def test_tc_sql_801_use_sql_completes_multikb_reference_kb_id(monkeypatch, dialog_service, enable_gaussdb_docengine):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    kb2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    table = f"ragflow_{tenant_id}"
    retriever = MultiKBReferenceRetriever()
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id IN ('{kb1}', '{kb2}')"])

    result = asyncio.run(
        dialog_service.use_sql(
            "show amount",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb1, kb2],
        )
    )

    assert result["reference"]["chunks"] == [
        {"doc_id": "doc1", "docnm_kwd": "finance-a.csv", "kb_id": kb1},
        {"doc_id": "doc2", "docnm_kwd": "finance-b.csv", "kb_id": kb1},
        {"doc_id": "doc3", "docnm_kwd": "finance-c.csv", "kb_id": kb2},
    ]
    assert len(retriever.sqls) == 2
    assert retriever.sqls[1] == (f"SELECT doc_id, kb_id FROM {table} WHERE doc_id IN ('doc1', 'doc2', 'doc3') AND kb_id IN ('{kb1}', '{kb2}') LIMIT 128")


@pytest.mark.parametrize(
    ("lookup_result", "expects_warning"),
    [
        (RuntimeError("lookup failed"), True),
        ({"columns": [{"name": "doc_id"}], "rows": [["doc1"]]}, False),
        ({"columns": [{"name": "doc_id"}, {"name": "kb_id"}], "rows": []}, False),
        ({"columns": [{"name": "doc_id"}, {"name": "kb_id"}], "rows": [["doc1"]]}, False),
    ],
    ids=["exception", "missing-kb-column", "empty-rows", "short-row"],
)
def test_tc_sql_803_multikb_lookup_failures_preserve_reference(
    lookup_result,
    expects_warning,
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    kb2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "amount"}],
                "rows": [["doc1", "finance.csv", "120"]],
            },
            lookup_result,
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id IN ('{kb1}', '{kb2}')"])

    with caplog.at_level(logging.WARNING):
        result = asyncio.run(
            dialog_service.use_sql(
                "show amount",
                {"amount": "number"},
                tenant_id,
                chat,
                quota=False,
                kb_ids=[kb1, kb2],
            )
        )

    assert result["reference"]["chunks"] == [{"doc_id": "doc1", "docnm_kwd": "finance.csv"}]
    assert len(retriever.sqls) == 2
    if expects_warning:
        assert "Failed to complete GaussDB reference kb_id values" in caplog.text
    else:
        assert "Failed to complete GaussDB reference kb_id values" not in caplog.text


def test_tc_sql_804_multikb_lookup_is_validated_and_scoped(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    kb2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "amount"}],
                "rows": [["doc1", "finance.csv", "120"]],
            },
            {
                "columns": [{"name": "doc_id"}, {"name": "kb_id"}],
                "rows": [["doc1", kb2]],
            },
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id IN ('{kb1}', '{kb2}')"])

    result = asyncio.run(
        dialog_service.use_sql(
            "show amount",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb1, kb2],
        )
    )

    assert retriever.sqls[1] == (f"SELECT doc_id, kb_id FROM {table} WHERE doc_id IN ('doc1') AND kb_id IN ('{kb1}', '{kb2}') LIMIT 128")
    assert result["reference"]["chunks"][0]["kb_id"] == kb2


def test_tc_sql_805_use_sql_uses_inline_multikb_reference_kb_id(monkeypatch, dialog_service, enable_gaussdb_docengine):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    kb2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    table = f"ragflow_{tenant_id}"
    retriever = MultiKBReferenceWithInlineKB()
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    chat = FakeChatModel([f"SELECT doc_id, docnm_kwd, kb_id FROM {table} WHERE kb_id IN ('{kb1}', '{kb2}')"])

    result = asyncio.run(
        dialog_service.use_sql(
            "show amount",
            {"amount": "number"},
            tenant_id,
            chat,
            quota=False,
            kb_ids=[kb1, kb2],
        )
    )

    assert result["reference"]["chunks"][0]["kb_id"] == kb1
    assert len(retriever.sqls) == 1


def test_tc_ret_1001_use_sql_validator_rejects_before_sql_retrieval(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "00000000000000000000000000000001"
    other_kb_id = "00000000000000000000000000000002"
    table = f"ragflow_{tenant_id}"
    invalid_sql = f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table} WHERE kb_id = '{other_kb_id}'"
    chat = types.SimpleNamespace(async_chat=AsyncMock(side_effect=[invalid_sql, invalid_sql]))
    sql_retrieval = Mock(side_effect=AssertionError("validator must reject before SQL execution"))

    monkeypatch.setattr(
        dialog_service.settings,
        "retriever",
        types.SimpleNamespace(sql_retrieval=sql_retrieval),
        raising=False,
    )

    with caplog.at_level(logging.WARNING):
        result = asyncio.run(
            dialog_service.use_sql(
                "show amount",
                {"amount": "number"},
                tenant_id,
                chat,
                quota=True,
                kb_ids=[kb_id],
            )
        )

    assert result is None
    assert chat.async_chat.await_count == 2
    sql_retrieval.assert_not_called()
    assert "SQL crosses the allowed kb_id boundary" in caplog.text


def test_tc_sql_1001_use_sql_none_falls_back_to_retrieval(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    question = "What is the total amount?"
    embd_mdl = object()
    retriever = ScriptedRetriever([])
    use_sql = AsyncMock(return_value=None)
    chat = FakeChatModel(["fallback answer"])

    with caplog.at_level(logging.DEBUG):
        results, get_field_map, actual_kb_id = _run_configured_chat(
            monkeypatch,
            dialog_service,
            retriever=retriever,
            chat=chat,
            question=question,
            use_sql_mock=use_sql,
            embd_mdl=embd_mdl,
        )

    assert actual_kb_id == kb_id
    get_field_map.assert_called_once_with([kb_id])
    use_sql.assert_awaited_once_with(
        question,
        {"amount": "number"},
        tenant_id,
        chat,
        True,
        [kb_id],
        doc_ids=None,
    )
    retriever.retrieval.assert_awaited_once_with(
        question,
        embd_mdl,
        [tenant_id],
        [kb_id],
        1,
        8,
        0.2,
        0.3,
        doc_ids=None,
        knn_top_k=32,
        aggs=True,
        rerank_mdl=None,
        rank_feature=None,
        rerank_candidates_count=64,
    )
    assert results[-1]["answer"] == "fallback answer"
    assert results[-1]["reference"] == _expected_fallback_reference(kb_id)
    assert "SQL failed or returned no results, falling back to vector search" in caplog.text


def test_tc_sql_1002_validator_rejection_falls_back_to_retrieval(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    question = "What is the total amount?"
    embd_mdl = object()
    invalid_sql = "SELECT doc_id FROM other_tenant.ragflow_t1"
    retriever = ScriptedRetriever([])
    chat = FakeChatModel([invalid_sql, invalid_sql, "fallback answer"])

    with caplog.at_level(logging.DEBUG):
        results, get_field_map, actual_kb_id = _run_configured_chat(
            monkeypatch,
            dialog_service,
            retriever=retriever,
            chat=chat,
            question=question,
            embd_mdl=embd_mdl,
        )

    assert actual_kb_id == kb_id
    get_field_map.assert_called_once_with([kb_id])
    assert len(chat.calls) == 3
    assert retriever.sqls == []
    retriever.retrieval.assert_awaited_once_with(
        question,
        embd_mdl,
        [tenant_id],
        [kb_id],
        1,
        8,
        0.2,
        0.3,
        doc_ids=None,
        knn_top_k=32,
        aggs=True,
        rerank_mdl=None,
        rank_feature=None,
        rerank_candidates_count=64,
    )
    assert results[-1]["answer"] == "fallback answer"
    assert results[-1]["reference"] == _expected_fallback_reference(kb_id)
    assert "cross-schema SQL is not allowed" in caplog.text
    assert "SQL failed or returned no results, falling back to vector search" in caplog.text


def test_tc_sql_1003_sql_timeout_falls_back_to_retrieval(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    kb_id = "abcdefabcdefabcdefabcdefabcdefab"
    question = "What is the total amount?"
    embd_mdl = object()
    table = f"ragflow_{tenant_id}"
    valid_sql = f"SELECT doc_id, docnm_kwd, chunk_data #>> '{{amount}}' AS amount FROM {table}"
    retriever = ScriptedRetriever([TimeoutError("query timeout"), TimeoutError("query timeout")])
    chat = FakeChatModel([valid_sql, valid_sql, "fallback answer"])

    with caplog.at_level(logging.DEBUG):
        results, get_field_map, actual_kb_id = _run_configured_chat(
            monkeypatch,
            dialog_service,
            retriever=retriever,
            chat=chat,
            question=question,
            embd_mdl=embd_mdl,
        )

    assert actual_kb_id == kb_id
    get_field_map.assert_called_once_with([kb_id])
    expected_sql = f"{valid_sql} WHERE kb_id = '{kb_id}' LIMIT 128"
    assert retriever.sqls == [expected_sql, expected_sql]
    retriever.retrieval.assert_awaited_once_with(
        question,
        embd_mdl,
        [tenant_id],
        [kb_id],
        1,
        8,
        0.2,
        0.3,
        doc_ids=None,
        knn_top_k=32,
        aggs=True,
        rerank_mdl=None,
        rank_feature=None,
        rerank_candidates_count=64,
    )
    assert results[-1]["answer"] == "fallback answer"
    assert results[-1]["reference"] == _expected_fallback_reference(kb_id)
    assert "query timeout" in caplog.text
    assert "SQL failed or returned no results, falling back to vector search" in caplog.text


def test_tc_sql_1004_aggregate_answer_survives_source_lookup_failure(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    tenant_id = "0123456789abcdef0123456789abcdef"
    table = f"ragflow_{tenant_id}"
    retriever = ScriptedRetriever(
        [
            {"columns": [{"name": "total"}], "rows": [[450]]},
            RuntimeError("source lookup failed"),
        ]
    )
    chat = FakeChatModel([f"SELECT SUM(CAST(chunk_data #>> '{{amount}}' AS DOUBLE PRECISION)) AS total FROM {table}"])

    with caplog.at_level(logging.WARNING):
        results, _get_field_map, _kb_id = _run_configured_chat(
            monkeypatch,
            dialog_service,
            retriever=retriever,
            chat=chat,
        )

    retriever.retrieval.assert_not_awaited()
    assert results[-1]["answer"] == "|total|\n|------\n|450|"
    assert results[-1]["reference"] == {"chunks": [], "doc_aggs": []}
    assert "Failed to fetch chunks" in caplog.text
    assert "source lookup failed" in caplog.text
    assert "SQL failed or returned no results, falling back to vector search" not in caplog.text


def test_tc_sql_1005_nonempty_answer_with_empty_chunks_is_returned(
    monkeypatch,
    dialog_service,
    enable_gaussdb_docengine,
    caplog,
):
    sql_result = {
        "answer": "|total|\n|------|\n|450|",
        "reference": {"chunks": [], "doc_aggs": []},
    }
    retriever = ScriptedRetriever([])
    use_sql = AsyncMock(return_value=sql_result)
    chat = FakeChatModel([])

    with caplog.at_level(logging.DEBUG):
        results, _get_field_map, _kb_id = _run_configured_chat(
            monkeypatch,
            dialog_service,
            retriever=retriever,
            chat=chat,
            use_sql_mock=use_sql,
        )

    use_sql.assert_awaited_once_with(
        "What is the total amount?",
        {"amount": "number"},
        "0123456789abcdef0123456789abcdef",
        chat,
        True,
        ["abcdefabcdefabcdefabcdefabcdefab"],
        doc_ids=None,
    )
    retriever.retrieval.assert_not_awaited()
    assert results == [sql_result]
    assert "use_sql: No rows returned" not in caplog.text
