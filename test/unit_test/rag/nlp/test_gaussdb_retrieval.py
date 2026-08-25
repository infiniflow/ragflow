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
import sys
import types
from unittest.mock import AsyncMock, Mock

import pytest
from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchTextExpr


class StubDocEngineConnection:
    def db_type(self):
        return "stub"


class StubStorage:
    def health(self):
        return True


def _install_module(name, **attrs):
    module = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    sys.modules[name] = module
    return module


@pytest.fixture
def dealer_cls():
    module_names = [
        "common.settings",
        "common.token_utils",
        "roman_numbers",
        "word2number",
        "cn2an",
        "rag.nlp",
        "rag.nlp.search",
        "rag.nlp.query",
        "rag.nlp.rag_tokenizer",
        "rag.utils.redis_conn",
        "memory.utils",
        "json_repair",
    ]
    module_names.extend(
        f"rag.utils.{name}"
        for name in (
            "es_conn",
            "infinity_conn",
            "ob_conn",
            "opensearch_conn",
            "azure_sas_conn",
            "azure_spn_conn",
            "gcs_conn",
            "minio_conn",
            "opendal_conn",
            "s3_conn",
            "oss_conn",
        )
    )
    module_names.extend(f"memory.utils.{name}" for name in ("es_conn", "infinity_conn", "ob_conn"))
    saved_modules = {name: sys.modules.get(name) for name in module_names}
    import common
    import memory

    common_module = common
    saved_common_settings = getattr(common_module, "settings", None) if common_module else None
    saved_common_token_utils = getattr(common_module, "token_utils", None) if common_module else None
    memory_module = memory
    saved_memory_utils = getattr(memory_module, "utils", None) if memory_module else None

    settings_stub = _install_module(
        "common.settings",
        DOC_ENGINE_GAUSSDB=True,
        DOC_ENGINE_INFINITY=False,
        DOC_ENGINE_OCEANBASE=False,
        DOC_ENGINE_SERENEDB=False,
    )
    if common_module is not None:
        setattr(common_module, "settings", settings_stub)
    token_utils_stub = _install_module("common.token_utils", num_tokens_from_string=lambda value: len(str(value).split()))
    if common_module is not None:
        setattr(common_module, "token_utils", token_utils_stub)

    _install_module("roman_numbers")
    _install_module("word2number", w2n=types.SimpleNamespace(word_to_num=lambda value: 0))
    _install_module("cn2an", cn2an=lambda value, *_args, **_kwargs: 0)
    _install_module(
        "rag.nlp.rag_tokenizer",
        tokenize=lambda value: str(value),
        fine_grained_tokenize=lambda value: str(value),
    )
    _install_module("rag.nlp.query", FulltextQueryer=lambda: types.SimpleNamespace(rmWWW=lambda value: value, hybrid_similarity=lambda *_args, **_kwargs: (0, 0, 0)))
    _install_module("rag.utils.redis_conn", REDIS_CONN=types.SimpleNamespace(health=lambda: True, is_alive=lambda: False, REDIS=None))
    _install_module("json_repair", loads=lambda value: value)
    for short_name, attrs in {
        "es_conn": {"ESConnection": StubDocEngineConnection},
        "infinity_conn": {"InfinityConnection": StubDocEngineConnection},
        "ob_conn": {"OBConnection": StubDocEngineConnection},
        "opensearch_conn": {"OSConnection": StubDocEngineConnection},
        "azure_sas_conn": {"RAGFlowAzureSasBlob": StubStorage},
        "azure_spn_conn": {"RAGFlowAzureSpnBlob": StubStorage},
        "gcs_conn": {"RAGFlowGCS": StubStorage},
        "minio_conn": {"RAGFlowMinio": StubStorage},
        "opendal_conn": {"OpenDALStorage": StubStorage},
        "s3_conn": {"RAGFlowS3": StubStorage},
        "oss_conn": {"RAGFlowOSS": StubStorage},
    }.items():
        _install_module(f"rag.utils.{short_name}", **attrs)

    memory_utils = types.ModuleType("memory.utils")
    memory_utils.__path__ = []
    sys.modules["memory.utils"] = memory_utils
    if memory_module is not None:
        setattr(memory_module, "utils", memory_utils)
    for short_name in ("es_conn", "infinity_conn", "ob_conn"):
        module = _install_module(
            f"memory.utils.{short_name}",
            ESConnection=StubDocEngineConnection,
            InfinityConnection=StubDocEngineConnection,
            OBConnection=StubDocEngineConnection,
        )
        setattr(memory_utils, short_name, module)

    try:
        import importlib

        sys.modules.pop("rag.nlp.search", None)
        search_module = importlib.import_module("rag.nlp.search")
        search_module.settings.DOC_ENGINE_GAUSSDB = True
        search_module.settings.DOC_ENGINE_INFINITY = False
        search_module.settings.DOC_ENGINE_OCEANBASE = False
        search_module.settings.DOC_ENGINE_SERENEDB = False
        yield search_module.Dealer
    finally:
        for name, module in saved_modules.items():
            if module is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = module
        if common_module is not None:
            if saved_common_settings is None:
                if hasattr(common_module, "settings"):
                    delattr(common_module, "settings")
            else:
                setattr(common_module, "settings", saved_common_settings)
            if saved_common_token_utils is None:
                if hasattr(common_module, "token_utils"):
                    delattr(common_module, "token_utils")
            else:
                setattr(common_module, "token_utils", saved_common_token_utils)
        if memory_module is not None:
            if saved_memory_utils is None:
                if hasattr(memory_module, "utils"):
                    delattr(memory_module, "utils")
            else:
                setattr(memory_module, "utils", saved_memory_utils)


def make_dealer(dealer_cls):
    dealer = dealer_cls.__new__(dealer_cls)
    dealer.dataStore = FakeGaussDBStore()
    return dealer


class FakeGaussDBStore:
    def db_type(self):
        return "gaussdb"

    def search(self, *args, **kwargs):
        self.last_search = (args, kwargs)
        return type("SearchResult", (), {"total": getattr(self, "total", 1), "chunks": []})()

    def get_total(self, res):
        return res.total

    def get_doc_ids(self, _res):
        return getattr(self, "ids", [])

    def get_highlight(self, *_args):
        return {}

    def get_aggregation(self, *_args):
        return {}

    def get_fields(self, *_args):
        return getattr(self, "fields", {})

    async def get_scores(self, *_args):
        raise AssertionError("GaussDB retrieval must not call ES _knn_scores")


class FakeQueryer:
    def question(self, text, min_match=0.3):
        return MatchTextExpr(["content_with_weight"], text, 1024), [text]


def retrieval_chunk(score, doc_id, doc_name, content="risk contract"):
    return {
        "_score": score,
        "content_ltks": content,
        "content_with_weight": content,
        "doc_id": doc_id,
        "docnm_kwd": doc_name,
        "kb_id": "kb1",
        "position_int": [1],
    }


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("vector_weight", "expected_weights"),
    [(0.75, "0.25,0.75"), (0.5, "0.5,0.5")],
)
async def test_tc_ret_801_dealer_search_uses_requested_gaussdb_fusion_weight(
    dealer_cls,
    vector_weight,
    expected_weights,
):
    dealer = make_dealer(dealer_cls)
    dealer.qryr = FakeQueryer()

    async def fake_get_vector(_text, _emb_mdl, top_k=10, num_candidates=20, similarity=0.1):
        return MatchDenseExpr("q_4_vec", [0.1, 0.2, 0.3, 0.4], "float", "cosine", top_k, {"similarity": similarity})

    dealer.get_vector = fake_get_vector

    await dealer.search(
        {
            "question": "risk",
            "page": 1,
            "size": 10,
            "knn_top_k": 10,
            "similarity": 0.2,
            "vector_similarity_weight": vector_weight,
        },
        "ragflow_tenant",
        ["kb1"],
        emb_mdl=object(),
        rank_feature={"pagerank_fea": 7},
    )

    args, kwargs = dealer.dataStore.last_search
    match_exprs = args[3]
    dense_expr = next(expr for expr in match_exprs if isinstance(expr, MatchDenseExpr))
    fusion_expr = next(expr for expr in match_exprs if isinstance(expr, FusionExpr))

    assert kwargs == {"rank_feature": {"pagerank_fea": 7}}
    assert dense_expr.topn == 10
    assert dense_expr.extra_options["similarity"] == 0.2
    assert fusion_expr.method == "weighted_sum"
    assert fusion_expr.topn == 10
    assert fusion_expr.fusion_params == {"weights": expected_weights}


@pytest.mark.asyncio
async def test_tc_ret_802_dealer_search_uses_default_gaussdb_fusion_weight(dealer_cls):
    dealer = make_dealer(dealer_cls)
    dealer.qryr = FakeQueryer()

    async def fake_get_vector(_text, _emb_mdl, top_k=10, num_candidates=20, similarity=0.1):
        return MatchDenseExpr("q_4_vec", [0.1, 0.2, 0.3, 0.4], "float", "cosine", top_k, {"similarity": similarity})

    dealer.get_vector = fake_get_vector

    await dealer.search({"question": "risk", "page": 1, "size": 5}, "ragflow_tenant", ["kb1"], emb_mdl=object())

    args, _kwargs = dealer.dataStore.last_search
    fusion_expr = next(expr for expr in args[3] if isinstance(expr, FusionExpr))

    assert fusion_expr.fusion_params["weights"] == "0.7,0.3"


@pytest.mark.asyncio
async def test_tc_ret_306_retrieval_passes_threshold_with_original_similarity_key(dealer_cls):
    dealer = make_dealer(dealer_cls)
    captured = {}

    async def fake_search(req, *_args, **_kwargs):
        captured["req"] = req
        return type("SearchResult", (), {"total": 0, "chunks": []})()

    async def fake_prune(sres):
        return sres

    dealer.search = fake_search
    dealer._prune_deleted_chunks = fake_prune

    await dealer.retrieval(
        "risk",
        object(),
        "tenant",
        ["kb1"],
        page=1,
        page_size=10,
        similarity_threshold=0.2,
        vector_similarity_weight=0.75,
        knn_top_k=77,
        doc_ids=["doc1"],
    )

    assert captured["req"]["doc_ids"] == ["doc1"]
    assert captured["req"]["knn_top_k"] == 77
    assert captured["req"]["similarity"] == 0.2
    assert captured["req"]["available_int"] == 1
    assert captured["req"]["vector_similarity_weight"] == 0.75
    assert "similarity_threshold" not in captured["req"]


async def _run_tc_ret_807_808_retrieval_scenario(dealer_cls):
    dealer = make_dealer(dealer_cls)
    dealer.qryr = FakeQueryer()
    dealer.dataStore.total = 3
    dealer.dataStore.ids = ["low", "high", "mid"]
    dealer.dataStore.fields = {
        "low": retrieval_chunk(0.1, "doc-low", "Low", "low"),
        "high": retrieval_chunk(0.9, "doc-high", "High", "high"),
        "mid": retrieval_chunk(0.4, "doc-mid", "Mid", "mid"),
    }

    async def fake_get_vector(_text, _emb_mdl, top_k=10, num_candidates=20, similarity=0.1):
        return MatchDenseExpr("q_4_vec", [0.1, 0.2, 0.3, 0.4], "float", "cosine", top_k, {"similarity": similarity})

    async def fake_prune(sres):
        return sres

    dealer.get_vector = fake_get_vector
    dealer._prune_deleted_chunks = fake_prune
    dealer._knn_scores = AsyncMock(side_effect=AssertionError("_knn_scores must not be called"))
    dealer.rerank_by_model = Mock(side_effect=AssertionError("rerank_by_model must not be called"))
    dealer.rerank = Mock(side_effect=AssertionError("rerank must not be called"))
    dealer.rerank_with_knn = Mock(side_effect=AssertionError("rerank_with_knn must not be called"))

    ranks = await dealer.retrieval(
        "risk",
        object(),
        "tenant",
        ["kb1"],
        page=1,
        page_size=10,
        similarity_threshold=0.2,
        vector_similarity_weight=0.75,
        rerank_mdl=None,
    )

    args, kwargs = dealer.dataStore.last_search
    dense_expr = next(expr for expr in args[3] if isinstance(expr, MatchDenseExpr))
    fusion_expr = next(expr for expr in args[3] if isinstance(expr, FusionExpr))

    return dealer, ranks, dense_expr, fusion_expr, kwargs


@pytest.mark.asyncio
async def test_tc_ret_807_retrieval_uses_gaussdb_fusion_weight_without_es_default(monkeypatch, dealer_cls):
    dealer, _, dense_expr, fusion_expr, kwargs = await _run_tc_ret_807_808_retrieval_scenario(dealer_cls)

    assert kwargs == {"rank_feature": {"pagerank_fea": 10}}
    assert dense_expr.extra_options["similarity"] == 0.2
    assert fusion_expr.fusion_params == {"weights": "0.25,0.75"}
    assert fusion_expr.fusion_params["weights"] != "0.05,0.95"

    search_module = sys.modules[dealer_cls.__module__]
    monkeypatch.setattr(search_module.settings, "DOC_ENGINE_GAUSSDB", False)
    await dealer.search(
        {
            "question": "risk",
            "page": 1,
            "size": 10,
            "knn_top_k": 10,
            "similarity": 0.2,
            "vector_similarity_weight": 0.75,
        },
        "ragflow_tenant",
        ["kb1"],
        emb_mdl=object(),
    )
    non_gauss_match_exprs = dealer.dataStore.last_search[0][3]
    non_gauss_fusion = next(expr for expr in non_gauss_match_exprs if isinstance(expr, FusionExpr))
    assert non_gauss_fusion.fusion_params == {"weights": "0.001,1"}


@pytest.mark.asyncio
async def test_tc_ret_808_retrieval_uses_gaussdb_scores_without_local_rerank(dealer_cls):
    dealer, ranks, _, _, _ = await _run_tc_ret_807_808_retrieval_scenario(dealer_cls)

    assert ranks["total"] == 2
    assert [chunk["chunk_id"] for chunk in ranks["chunks"]] == ["high", "mid"]
    assert "low" not in {chunk["chunk_id"] for chunk in ranks["chunks"]}
    assert [chunk["similarity"] for chunk in ranks["chunks"]] == [0.9, 0.4]
    assert [chunk["vector_similarity"] for chunk in ranks["chunks"]] == [0.9, 0.4]
    assert [chunk["term_similarity"] for chunk in ranks["chunks"]] == [0.9, 0.4]
    assert ranks["chunks"][0]["vector"] == [0.0, 0.0, 0.0, 0.0]
    assert ranks["doc_aggs"] == [
        {"doc_name": "High", "doc_id": "doc-high", "count": 1},
        {"doc_name": "Mid", "doc_id": "doc-mid", "count": 1},
    ]
    dealer._knn_scores.assert_not_awaited()
    dealer.rerank_by_model.assert_not_called()
    dealer.rerank.assert_not_called()
    dealer.rerank_with_knn.assert_not_called()


async def _run_gaussdb_rank_feature_retrieval(dealer_cls, fields, rank_feature):
    dealer = make_dealer(dealer_cls)
    dealer.qryr = FakeQueryer()
    dealer.dataStore.total = len(fields)
    dealer.dataStore.ids = list(fields)
    dealer.dataStore.fields = fields

    async def fake_get_vector(_text, _emb_mdl, top_k=10, num_candidates=20, similarity=0.1):
        return MatchDenseExpr("q_4_vec", [0.1, 0.2, 0.3, 0.4], "float", "cosine", top_k, {"similarity": similarity})

    async def fake_prune(sres):
        return sres

    dealer.get_vector = fake_get_vector
    dealer._prune_deleted_chunks = fake_prune
    dealer._knn_scores = AsyncMock(side_effect=AssertionError("_knn_scores must not be called"))
    dealer.rerank_by_model = Mock(side_effect=AssertionError("rerank_by_model must not be called"))
    dealer.rerank = Mock(side_effect=AssertionError("rerank must not be called"))
    dealer.rerank_with_knn = Mock(side_effect=AssertionError("rerank_with_knn must not be called"))

    return await dealer.retrieval(
        "risk",
        object(),
        "tenant",
        ["kb1"],
        page=1,
        page_size=10,
        similarity_threshold=0.2,
        vector_similarity_weight=0.75,
        rerank_mdl=None,
        rank_feature=rank_feature,
    )


@pytest.mark.asyncio
async def test_tc_ret_704_gaussdb_retrieval_adds_tag_feature_score_to_sql_score(dealer_cls):
    fields = {
        "without-tag": retrieval_chunk(0.4, "doc-plain", "Plain"),
        "with-tag": {**retrieval_chunk(0.4, "doc-tagged", "Tagged"), "tag_feas": {"risk": 1.0}},
    }

    ranks = await _run_gaussdb_rank_feature_retrieval(dealer_cls, fields, {"risk": 1.0})

    assert [chunk["chunk_id"] for chunk in ranks["chunks"]] == ["with-tag", "without-tag"]
    assert [chunk["similarity"] for chunk in ranks["chunks"]] == [10.4, 0.4]
    assert [chunk["vector_similarity"] for chunk in ranks["chunks"]] == [0.4, 0.4]
    assert [chunk["term_similarity"] for chunk in ranks["chunks"]] == [0.4, 0.4]


@pytest.mark.asyncio
async def test_tc_ret_704_gaussdb_retrieval_does_not_add_pagerank_twice(dealer_cls):
    fields = {
        "higher-sql-score": {**retrieval_chunk(0.8, "doc-high", "High"), "pagerank_fea": 0.1},
        "higher-pagerank": {**retrieval_chunk(0.7, "doc-pagerank", "PageRank"), "pagerank_fea": 0.9},
    }

    ranks = await _run_gaussdb_rank_feature_retrieval(dealer_cls, fields, {"pagerank_fea": 10})

    assert [chunk["chunk_id"] for chunk in ranks["chunks"]] == ["higher-sql-score", "higher-pagerank"]
    assert [chunk["similarity"] for chunk in ranks["chunks"]] == [0.8, 0.7]
    assert [chunk["vector_similarity"] for chunk in ranks["chunks"]] == [0.8, 0.7]
    assert [chunk["term_similarity"] for chunk in ranks["chunks"]] == [0.8, 0.7]


@pytest.mark.asyncio
async def test_tc_ret_311_retrieval_keeps_term_only_gaussdb_scores_when_vector_weight_is_zero(dealer_cls):
    dealer = make_dealer(dealer_cls)
    captured = {}

    async def fake_search(req, *_args, **_kwargs):
        captured["req"] = req
        return dealer_cls.SearchResult(
            total=2,
            ids=["term-low", "term-zero"],
            query_vector=[0.1, 0.2],
            field={
                "term-low": retrieval_chunk(0.1, "doc-low", "Low"),
                "term-zero": retrieval_chunk(0.0, "doc-zero", "Zero"),
            },
        )

    async def fake_prune(sres):
        return sres

    dealer.search = fake_search
    dealer._prune_deleted_chunks = fake_prune
    dealer._knn_scores = AsyncMock(side_effect=AssertionError("_knn_scores must not be called"))
    dealer.rerank_by_model = Mock(side_effect=AssertionError("rerank_by_model must not be called"))
    dealer.rerank = Mock(side_effect=AssertionError("rerank must not be called"))
    dealer.rerank_with_knn = Mock(side_effect=AssertionError("rerank_with_knn must not be called"))

    ranks = await dealer.retrieval(
        "risk",
        object(),
        "tenant",
        ["kb1"],
        page=1,
        page_size=10,
        similarity_threshold=0.8,
        vector_similarity_weight=0.0,
    )

    assert captured["req"]["similarity"] == 0.8
    assert captured["req"]["vector_similarity_weight"] == 0.0
    assert ranks["total"] == 2
    assert [chunk["chunk_id"] for chunk in ranks["chunks"]] == ["term-low", "term-zero"]
    assert [chunk["similarity"] for chunk in ranks["chunks"]] == [0.1, 0.0]
    assert [chunk["vector_similarity"] for chunk in ranks["chunks"]] == [0.1, 0.0]
    assert [chunk["term_similarity"] for chunk in ranks["chunks"]] == [0.1, 0.0]
    dealer._knn_scores.assert_not_awaited()
    dealer.rerank_by_model.assert_not_called()
    dealer.rerank.assert_not_called()
    dealer.rerank_with_knn.assert_not_called()
