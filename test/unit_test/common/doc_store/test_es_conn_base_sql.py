import importlib.util
import logging
import sys
import types
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[4]
MODULE_PATH = REPO_ROOT / "common/doc_store/es_conn_base.py"



def _install_stubs():
    common_pkg = sys.modules.setdefault("common", types.ModuleType("common"))
    doc_store_pkg = sys.modules.setdefault("common.doc_store", types.ModuleType("common.doc_store"))
    setattr(common_pkg, "doc_store", doc_store_pkg)

    file_utils = types.ModuleType("common.file_utils")
    file_utils.get_project_base_directory = lambda: str(REPO_ROOT)
    sys.modules["common.file_utils"] = file_utils

    misc_utils = types.ModuleType("common.misc_utils")
    misc_utils.convert_bytes = lambda value: str(value)
    sys.modules["common.misc_utils"] = misc_utils

    doc_store_base = types.ModuleType("common.doc_store.doc_store_base")

    class DocStoreConnection:
        pass

    class OrderByExpr:
        pass

    class MatchExpr:
        pass

    doc_store_base.DocStoreConnection = DocStoreConnection
    doc_store_base.OrderByExpr = OrderByExpr
    doc_store_base.MatchExpr = MatchExpr
    sys.modules["common.doc_store.doc_store_base"] = doc_store_base

    settings = types.ModuleType("common.settings")
    settings.ES = {"hosts": "http://example.test"}
    sys.modules["common.settings"] = settings
    setattr(common_pkg, "settings", settings)

    nlp_stub = types.ModuleType("rag.nlp")

    class _Tokenizer:
        @staticmethod
        def tokenize(value):
            return value

        @staticmethod
        def fine_grained_tokenize(value):
            return value

    nlp_stub.rag_tokenizer = _Tokenizer()
    nlp_stub.is_english = lambda *_args, **_kwargs: True
    sys.modules["rag.nlp"] = nlp_stub

    elasticsearch_stub = types.ModuleType("elasticsearch")

    class NotFoundError(Exception):
        pass

    elasticsearch_stub.NotFoundError = NotFoundError
    sys.modules["elasticsearch"] = elasticsearch_stub

    client_stub = types.ModuleType("elasticsearch.client")

    class IndicesClient:
        def __init__(self, *_args, **_kwargs):
            pass

    client_stub.IndicesClient = IndicesClient
    sys.modules["elasticsearch.client"] = client_stub

    dsl_stub = types.ModuleType("elasticsearch_dsl")

    class Index:
        def __init__(self, *_args, **_kwargs):
            pass

    dsl_stub.Index = Index
    sys.modules["elasticsearch_dsl"] = dsl_stub

    transport_stub = types.ModuleType("elastic_transport")

    class ConnectionTimeout(Exception):
        pass

    transport_stub.ConnectionTimeout = ConnectionTimeout
    sys.modules["elastic_transport"] = transport_stub


class _Recorder:
    def __init__(self):
        self.calls = []

    def query(self, body, format, request_timeout):
        self.calls.append({
            "query": body["query"],
            "fetch_size": body["fetch_size"],
            "format": format,
            "request_timeout": request_timeout,
        })
        return {"columns": [], "rows": []}


class _FakeES:
    def __init__(self):
        self.sql = _Recorder()


_install_stubs()
spec = importlib.util.spec_from_file_location("tested_es_conn_base", MODULE_PATH)
es_conn_base = importlib.util.module_from_spec(spec)
spec.loader.exec_module(es_conn_base)


def test_sql_rewrite_consumes_embedded_quotes_in_match_literal():
    conn = es_conn_base.ESConnectionBase.__new__(es_conn_base.ESConnectionBase)
    conn.logger = logging.getLogger("test.es_conn_base")
    conn.es = _FakeES()
    conn._connect = lambda: True

    conn.sql("select * from ragflow_t where content_tks = 'abc'' OR 1=1 --'", 64, "json")

    assert conn.es.sql.calls[0]["query"] == (
        "select * from ragflow_t where"
        " MATCH(content_tks, 'abc'' OR 1=1 --', 'operator=OR;minimum_should_match=30%') "
    )


def test_sql_rewrite_escapes_tokenizer_output_before_match_literal():
    class _QuotingTokenizer:
        @staticmethod
        def tokenize(value):
            return value

        @staticmethod
        def fine_grained_tokenize(value):
            return "token'value"

    es_conn_base.rag_tokenizer = _QuotingTokenizer()

    conn = es_conn_base.ESConnectionBase.__new__(es_conn_base.ESConnectionBase)
    conn.logger = logging.getLogger("test.es_conn_base")
    conn.es = _FakeES()
    conn._connect = lambda: True

    conn.sql("select * from ragflow_t where content_tks = 'safe'", 64, "json")

    assert conn.es.sql.calls[0]["query"] == (
        "select * from ragflow_t where"
        " MATCH(content_tks, 'token''value', 'operator=OR;minimum_should_match=30%') "
    )
