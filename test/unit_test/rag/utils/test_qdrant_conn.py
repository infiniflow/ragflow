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
import copy
import importlib.util
import math
import sys
import types
from pathlib import Path
from types import SimpleNamespace
from uuid import UUID

import pytest

from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchSparseExpr, OrderByExpr, SparseVector


REPO_ROOT = Path(__file__).resolve().parents[4]
MODULE_PATH = REPO_ROOT / "rag" / "utils" / "qdrant_conn.py"


class _Distance:
    COSINE = "cosine"


class _VectorParams:
    def __init__(self, size, distance):
        self.size = size
        self.distance = distance


class _SparseIndexParams:
    def __init__(self, on_disk=None, full_scan_threshold=None, datatype=None):
        self.on_disk = on_disk
        self.full_scan_threshold = full_scan_threshold
        self.datatype = datatype


class _Modifier:
    IDF = "idf"


class _SparseVectorParams:
    def __init__(self, index=None, modifier=None):
        self.index = index
        self.modifier = modifier


class _SparseVector:
    def __init__(self, indices, values):
        self.indices = list(indices)
        self.values = list(values)


class _Range:
    def __init__(self, lt=None):
        self.lt = lt


class _MatchAny:
    def __init__(self, any):
        self.any = any


class _MatchValue:
    def __init__(self, value):
        self.value = value


class _FieldCondition:
    def __init__(self, key, match=None, range=None):
        self.key = key
        self.match = match
        self.range = range


class _Filter:
    def __init__(self, must=None, must_not=None, should=None):
        self.must = must or []
        self.must_not = must_not or []
        self.should = should or []


class _PointStruct:
    def __init__(self, id, vector, payload):
        self.id = id
        self.vector = vector
        self.payload = payload


class _Prefetch:
    def __init__(self, query=None, using=None, filter=None, score_threshold=None, limit=None, **_kwargs):
        self.query = query
        self.using = using
        self.filter = filter
        self.score_threshold = score_threshold
        self.limit = limit


class _Fusion:
    RRF = "rrf"


class _FusionQuery:
    def __init__(self, fusion):
        self.fusion = fusion


class _TextIndexType:
    TEXT = "text"


class _TokenizerType:
    PREFIX = "prefix"
    WHITESPACE = "whitespace"
    WORD = "word"
    MULTILINGUAL = "multilingual"


class _TextIndexParams:
    def __init__(self, type, tokenizer=None, lowercase=None, on_disk=None, **_kwargs):
        self.type = type
        self.tokenizer = tokenizer
        self.lowercase = lowercase
        self.on_disk = on_disk


class _FakeQdrantClient:
    def __init__(self, **kwargs):
        self.url = kwargs.get("url")
        self.collections = {}

    def get_collections(self):
        return SimpleNamespace(collections=[SimpleNamespace(name=name) for name in self.collections])

    def create_collection(self, collection_name, vectors_config, sparse_vectors_config=None, on_disk_payload=True):
        dense_cfg = vectors_config["dense"]
        self.collections[collection_name] = {
            "size": dense_cfg.size,
            "distance": dense_cfg.distance,
            "points": {},
            "sparse_vectors": copy.deepcopy(sparse_vectors_config or {}),
            "payload_indexes": {},
        }
        return True

    def update_collection(self, collection_name, sparse_vectors_config=None, **_kwargs):
        if sparse_vectors_config:
            self.collections[collection_name]["sparse_vectors"].update(copy.deepcopy(sparse_vectors_config))
        return True

    def create_payload_index(self, collection_name, field_name, field_schema=None, wait=True):
        self.collections[collection_name]["payload_indexes"][field_name] = field_schema
        return True

    def delete_collection(self, collection_name):
        self.collections.pop(collection_name, None)
        return True

    def collection_exists(self, collection_name):
        return collection_name in self.collections

    def get_collection(self, collection_name):
        collection = self.collections[collection_name]
        vectors = {"dense": _VectorParams(collection["size"], collection["distance"])}
        sparse_vectors = copy.deepcopy(collection["sparse_vectors"])
        return SimpleNamespace(config=SimpleNamespace(params=SimpleNamespace(vectors=vectors, sparse_vectors=sparse_vectors)))

    def retrieve(self, collection_name, ids, with_payload=True, with_vectors=False):
        collection = self.collections[collection_name]
        result = []
        for point_id in ids:
            if point_id not in collection["points"]:
                continue
            stored = collection["points"][point_id]
            result.append(
                SimpleNamespace(
                    id=stored["id"],
                    payload=copy.deepcopy(stored["payload"]) if with_payload else None,
                    vector=self._select_vectors(stored["vector"], with_vectors),
                )
            )
        return result

    def upsert(self, collection_name, points, wait=False):
        collection = self.collections[collection_name]
        for point in points:
            self._validate_point_id(point.id)
            collection["points"][point.id] = {
                "id": point.id,
                "payload": copy.deepcopy(point.payload),
                "vector": copy.deepcopy(point.vector),
            }
        return True

    def delete(self, collection_name, points_selector, wait=True):
        collection = self.collections[collection_name]
        for point_id in points_selector:
            collection["points"].pop(point_id, None)
        return True

    def scroll(self, collection_name, scroll_filter=None, limit=256, with_payload=True, with_vectors=False, offset=None):
        collection = self.collections[collection_name]
        filtered = [
            point
            for point in collection["points"].values()
            if self._matches_filter(point["payload"], scroll_filter)
        ]
        start = int(offset or 0)
        end = start + limit
        batch = filtered[start:end]
        next_offset = end if end < len(filtered) else None
        points = [
            SimpleNamespace(
                id=stored["id"],
                payload=copy.deepcopy(stored["payload"]) if with_payload else None,
                vector=self._select_vectors(stored["vector"], with_vectors),
            )
            for stored in batch
        ]
        return points, next_offset

    def search(self, collection_name, query_vector, query_filter=None, limit=10, with_payload=True, with_vectors=False, score_threshold=None):
        vector_name, query = query_vector
        result = self._vector_search(
            collection_name,
            vector_name,
            query,
            query_filter,
            limit,
            with_payload,
            with_vectors,
            score_threshold,
        )
        return result

    def query_points(self, collection_name, query, prefetch=None, limit=10, with_payload=True, with_vectors=False, **_kwargs):
        assert query.fusion == _Fusion.RRF
        rankings = []
        for current in prefetch or []:
            if current.using == "dense":
                ranking = self._vector_search(
                    collection_name,
                    current.using,
                    current.query,
                    current.filter,
                    current.limit or limit,
                    with_payload,
                    with_vectors,
                    current.score_threshold,
                )
            else:
                ranking = self._sparse_search(
                    collection_name,
                    current.using,
                    current.query,
                    current.filter,
                    current.limit or limit,
                    with_payload,
                    with_vectors,
                )
            rankings.append(ranking)

        fused_scores = {}
        points_by_id = {}
        for ranking in rankings:
            for idx, point in enumerate(ranking):
                point_id = point.id
                fused_scores[point_id] = fused_scores.get(point_id, 0.0) + 1.0 / (60 + idx + 1)
                points_by_id[point_id] = point
        points = []
        for point_id, score in sorted(fused_scores.items(), key=lambda item: item[1], reverse=True)[:limit]:
            point = points_by_id[point_id]
            points.append(
                SimpleNamespace(
                    id=point.id,
                    payload=copy.deepcopy(point.payload) if with_payload else None,
                    vector=self._select_vectors(point.vector, with_vectors),
                    score=score,
                )
            )
        return SimpleNamespace(points=points)

    def _vector_search(self, collection_name, vector_name, query, query_filter, limit, with_payload, with_vectors, score_threshold=None):
        collection = self.collections[collection_name]
        result = []
        for stored in collection["points"].values():
            if not self._matches_filter(stored["payload"], query_filter):
                continue
            vector = stored["vector"].get(vector_name)
            if vector_name == "dense":
                score = self._cosine_similarity(query, vector)
            else:
                score = self._sparse_similarity(query, vector)
            if score_threshold is not None and score < score_threshold:
                continue
            result.append(
                SimpleNamespace(
                    id=stored["id"],
                    payload=copy.deepcopy(stored["payload"]) if with_payload else None,
                    vector=self._select_vectors(stored["vector"], with_vectors),
                    score=score,
                )
            )
        result.sort(key=lambda point: point.score, reverse=True)
        return result[:limit]

    def _sparse_search(self, collection_name, vector_name, query, query_filter, limit, with_payload, with_vectors):
        points = self._vector_search(collection_name, vector_name, query, query_filter, limit, with_payload, with_vectors)
        return [point for point in points if point.score > 0][:limit]

    @staticmethod
    def _select_vectors(vectors, with_vectors):
        if with_vectors is False:
            return None
        if with_vectors is True or with_vectors is None:
            return copy.deepcopy(vectors)
        if isinstance(with_vectors, list):
            return {name: copy.deepcopy(vectors[name]) for name in with_vectors if name in vectors}
        return None

    @staticmethod
    def _matches_condition(payload, condition):
        if condition.match is not None:
            if hasattr(condition.match, "any"):
                return payload.get(condition.key) in condition.match.any
            return payload.get(condition.key) == condition.match.value
        if condition.range is not None:
            value = payload.get(condition.key, 0)
            return value < condition.range.lt
        return True

    def _matches_filter(self, payload, query_filter):
        if query_filter is None:
            return True
        for condition in query_filter.must:
            if not self._matches_condition(payload, condition):
                return False
        for condition in query_filter.must_not:
            if self._matches_condition(payload, condition):
                return False
        return True

    @staticmethod
    def _cosine_similarity(left, right):
        numerator = sum(float(a) * float(b) for a, b in zip(left, right))
        left_norm = math.sqrt(sum(float(a) * float(a) for a in left))
        right_norm = math.sqrt(sum(float(b) * float(b) for b in right))
        if left_norm == 0 or right_norm == 0:
            return 0.0
        return numerator / (left_norm * right_norm)

    @staticmethod
    def _sparse_similarity(left, right):
        left_map = {int(i): float(v) for i, v in zip(left.indices, left.values)}
        right_indices = getattr(right, "indices", []) or []
        right_values = getattr(right, "values", []) or []
        score = 0.0
        for idx, value in zip(right_indices, right_values):
            score += left_map.get(int(idx), 0.0) * float(value)
        return score

    @staticmethod
    def _validate_point_id(point_id):
        if isinstance(point_id, int) and point_id >= 0:
            return
        if isinstance(point_id, str):
            UUID(point_id)
            return
        raise ValueError(
            f"{point_id} is not a valid point ID, valid values are either an unsigned integer or a UUID"
        )


def _load_qdrant_module(monkeypatch):
    import common

    fake_settings = types.ModuleType("common.settings")
    fake_settings.QDRANT = {
        "host": "qdrant",
        "http_port": 6333,
        "grpc_port": 6334,
        "https": False,
        "prefer_grpc": False,
        "timeout": 10,
        "text_index_tokenizer": "multilingual",
    }
    monkeypatch.setitem(sys.modules, "common.settings", fake_settings)
    monkeypatch.setattr(common, "settings", fake_settings, raising=False)

    rag_pkg = types.ModuleType("rag")
    rag_pkg.__path__ = []
    utils_pkg = types.ModuleType("rag.utils")
    utils_pkg.__path__ = []
    sparse_mod = types.ModuleType("rag.utils.sparse_vector")
    sparse_mod.DENSE_VECTOR_NAME = "dense"
    sparse_mod.MULTIVECTOR_PLACEHOLDER_NAME = "colpali"
    sparse_mod.SPARSE_VECTOR_FIELD = "q_sparse_vec"
    sparse_mod.SPARSE_VECTOR_NAME = "sparse"
    sparse_mod.dense_vector_field_name = lambda size: f"q_{int(size)}_vec"
    nlp_mod = types.ModuleType("rag.nlp")
    nlp_mod.is_english = lambda _tokens: True
    nlp_mod.rag_tokenizer = SimpleNamespace(
        tokenize=lambda text: " ".join(str(text).lower().split()),
        fine_grained_tokenize=lambda text: " ".join(str(text).lower().split()),
    )
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)
    monkeypatch.setitem(sys.modules, "rag.utils", utils_pkg)
    monkeypatch.setitem(sys.modules, "rag.utils.sparse_vector", sparse_mod)
    monkeypatch.setitem(sys.modules, "rag.nlp", nlp_mod)

    fake_models = SimpleNamespace(
        Distance=_Distance,
        VectorParams=_VectorParams,
        SparseVectorParams=_SparseVectorParams,
        SparseIndexParams=_SparseIndexParams,
        Modifier=_Modifier,
        SparseVector=_SparseVector,
        Range=_Range,
        MatchAny=_MatchAny,
        MatchValue=_MatchValue,
        FieldCondition=_FieldCondition,
        Filter=_Filter,
        PointStruct=_PointStruct,
        Prefetch=_Prefetch,
        FusionQuery=_FusionQuery,
        Fusion=_Fusion,
        TextIndexParams=_TextIndexParams,
        TextIndexType=_TextIndexType,
        TokenizerType=_TokenizerType,
    )
    fake_qdrant_client = types.ModuleType("qdrant_client")
    fake_qdrant_client.QdrantClient = _FakeQdrantClient
    fake_qdrant_client.models = fake_models
    monkeypatch.setitem(sys.modules, "qdrant_client", fake_qdrant_client)

    module_name = "test_qdrant_conn_under_test"
    sys.modules.pop(module_name, None)
    spec = importlib.util.spec_from_file_location(module_name, MODULE_PATH)
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def _make_doc(doc_id, vector, sparse=None, **payload):
    doc = {
        "id": doc_id,
        f"q_{len(vector)}_vec": vector,
        **payload,
    }
    if sparse is not None:
        doc["q_sparse_vec"] = sparse
    return doc


@pytest.mark.p2
def test_collection_crud_and_health(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()

    assert conn.db_type() == "qdrant"
    assert conn.health()["status"] == "green"
    assert conn.create_idx("ragflow_tenant", "kb-1", 3) is True
    assert conn.index_exist("ragflow_tenant", "kb-1") is True
    assert conn.client.collections["ragflow_tenant"]["sparse_vectors"]["sparse"].modifier == _Modifier.IDF
    assert "content_with_weight" in conn.client.collections["ragflow_tenant"]["payload_indexes"]
    assert conn.create_doc_meta_idx("ragflow_doc_meta_tenant") is True
    assert conn.index_exist("ragflow_doc_meta_tenant", "") is True

    conn.delete_idx("ragflow_tenant", "")
    assert conn.index_exist("ragflow_tenant", "") is False


@pytest.mark.p2
def test_insert_get_update_and_delete_document(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)

    errors = conn.insert(
        [
            _make_doc(
                "doc-1",
                [1.0, 0.0, 0.0],
                sparse={"indices": [1, 3], "values": [1.0, 0.5]},
                title_tks="alpha",
                content_ltks="alpha content",
                available_int=1,
                tag_kwd=["one", "two"],
            )
        ],
        "ragflow_tenant",
        "kb-1",
    )
    assert errors == []

    stored = conn.get("doc-1", "ragflow_tenant", ["kb-1"])
    assert stored["id"] == "doc-1"
    assert stored["kb_id"] == "kb-1"
    assert stored["q_3_vec"] == [1.0, 0.0, 0.0]

    assert conn.update({"id": "doc-1"}, {"title_tks": "beta", "remove": {"tag_kwd": "one"}}, "ragflow_tenant", "kb-1") is True
    updated = conn.get("doc-1", "ragflow_tenant", ["kb-1"])
    assert updated["title_tks"] == "beta"
    assert updated["tag_kwd"] == ["two"]

    assert conn.delete({"id": "doc-1"}, "ragflow_tenant", "kb-1") == 1
    assert conn.get("doc-1", "ragflow_tenant", ["kb-1"]) is None


@pytest.mark.p2
def test_missing_available_flag_is_treated_as_available(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)

    errors = conn.insert(
        [
            _make_doc(
                "doc-1",
                [1.0, 0.0, 0.0],
                title_tks="alpha",
                content_ltks="alpha content",
            )
        ],
        "ragflow_tenant",
        "kb-1",
    )
    assert errors == []

    stored = conn.get("doc-1", "ragflow_tenant", ["kb-1"])
    assert stored["available_int"] == 1

    result = conn.search(
        select_fields=["title_tks", "available_int"],
        highlight_fields=[],
        condition={"available_int": 1},
        match_expressions=[],
        order_by=OrderByExpr(),
        offset=0,
        limit=10,
        index_names="ragflow_tenant",
        knowledgebase_ids=["kb-1"],
    )

    assert conn.get_doc_ids(result) == ["doc-1"]
    fields = conn.get_fields(result, ["available_int"])
    assert fields["doc-1"]["available_int"] == 1


@pytest.mark.p2
def test_string_ids_are_mapped_to_qdrant_uuids(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)

    raw_id = "8d6b7a4e2b9d9780"
    errors = conn.insert([_make_doc(raw_id, [1.0, 0.0, 0.0], title_tks="alpha")], "ragflow_tenant", "kb-1")

    assert errors == []
    stored_points = conn.client.collections["ragflow_tenant"]["points"]
    assert list(stored_points) != [raw_id]
    stored_point_id = next(iter(stored_points))
    UUID(str(stored_point_id))
    assert stored_points[stored_point_id]["payload"]["id"] == raw_id
    assert conn.get(raw_id, "ragflow_tenant", ["kb-1"])["id"] == raw_id


@pytest.mark.p2
def test_highlight_falls_back_to_raw_chunk_text(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()

    result = {
        "hits": {
            "hits": [
                {
                    "_id": "doc-1",
                    "_source": {
                        "content_with_weight": "Placidus conforms to our zeitgeist, which is characterized by relativity and perspectivism.",
                    },
                    "highlight": {
                        "content_ltks": [
                            "placidus conform to our zeitgeist which is character by relativ and perspectivu"
                        ]
                    },
                }
            ]
        }
    }

    highlight = conn.get_highlight(result, ["relativ"], "content_with_weight")

    assert "relativity" in highlight["doc-1"].lower()
    assert "perspectivism" in highlight["doc-1"].lower()
    assert "perspectivu" not in highlight["doc-1"].lower()


@pytest.mark.p2
def test_dense_retrieval_returns_ranked_hits(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)
    conn.insert(
        [
            _make_doc("doc-1", [1.0, 0.0, 0.0], title_tks="alpha"),
            _make_doc("doc-2", [0.0, 1.0, 0.0], title_tks="beta"),
        ],
        "ragflow_tenant",
        "kb-1",
    )

    result = conn.search(
        select_fields=["title_tks", "q_3_vec"],
        highlight_fields=[],
        condition={},
        match_expressions=[
            MatchDenseExpr("q_3_vec", [1.0, 0.0, 0.0], "float", "cosine", topn=2, extra_options={}),
        ],
        order_by=OrderByExpr(),
        offset=0,
        limit=2,
        index_names="ragflow_tenant",
        knowledgebase_ids=["kb-1"],
    )

    assert conn.get_total(result) == 2
    assert conn.get_doc_ids(result) == ["doc-1", "doc-2"]
    fields = conn.get_fields(result, ["title_tks", "q_3_vec"])
    assert fields["doc-1"]["title_tks"] == "alpha"
    assert fields["doc-1"]["q_3_vec"] == [1.0, 0.0, 0.0]


@pytest.mark.p2
def test_hybrid_retrieval_uses_rrf(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)
    conn.insert(
        [
            _make_doc("doc-1", [1.0, 0.0, 0.0], title_tks="dense-first", sparse={"indices": [], "values": []}),
            _make_doc("doc-2", [0.8, 0.2, 0.0], title_tks="hybrid-first", sparse={"indices": [7], "values": [2.0]}),
            _make_doc("doc-3", [0.0, 1.0, 0.0], title_tks="other", sparse={"indices": [99], "values": [1.0]}),
        ],
        "ragflow_tenant",
        "kb-1",
    )

    result = conn.search(
        select_fields=["title_tks", "q_3_vec"],
        highlight_fields=[],
        condition={},
        match_expressions=[
            MatchDenseExpr("q_3_vec", [1.0, 0.0, 0.0], "float", "cosine", topn=3, extra_options={}),
            MatchSparseExpr("sparse", SparseVector(indices=[7], values=[1.0]), "dot", topn=3),
            FusionExpr("rrf", 3),
        ],
        order_by=OrderByExpr(),
        offset=0,
        limit=3,
        index_names="ragflow_tenant",
        knowledgebase_ids=["kb-1"],
    )

    assert conn.get_doc_ids(result)[:2] == ["doc-2", "doc-1"]


@pytest.mark.p2
def test_tenant_isolation_filters_by_kb_id(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)
    conn.insert([_make_doc("doc-1", [1.0, 0.0, 0.0], title_tks="tenant-one")], "ragflow_tenant", "kb-1")
    conn.insert([_make_doc("doc-2", [1.0, 0.0, 0.0], title_tks="tenant-two")], "ragflow_tenant", "kb-2")

    result = conn.search(
        select_fields=["title_tks"],
        highlight_fields=[],
        condition={},
        match_expressions=[],
        order_by=OrderByExpr(),
        offset=0,
        limit=10,
        index_names="ragflow_tenant",
        knowledgebase_ids=["kb-1"],
    )

    assert conn.get_total(result) == 1
    assert conn.get_doc_ids(result) == ["doc-1"]


@pytest.mark.p2
def test_scroll_pagination_works_past_batch_boundary(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)
    conn.insert(
        [_make_doc(f"doc-{idx}", [1.0, 0.0, 0.0], sort_int=idx) for idx in range(300)],
        "ragflow_tenant",
        "kb-1",
    )

    order_by = OrderByExpr()
    order_by.asc("sort_int")
    result = conn.search(
        select_fields=["sort_int"],
        highlight_fields=[],
        condition={},
        match_expressions=[],
        order_by=order_by,
        offset=260,
        limit=20,
        index_names="ragflow_tenant",
        knowledgebase_ids=["kb-1"],
    )

    assert conn.get_total(result) == 300
    page = conn.get_fields(result, ["sort_int"])
    assert [page[f"doc-{idx}"]["sort_int"] for idx in range(260, 280)] == [str(idx) for idx in range(260, 280)]


@pytest.mark.p2
def test_bulk_upsert_overwrites_existing_documents(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)
    conn.insert(
        [
            _make_doc("doc-1", [1.0, 0.0, 0.0], title_tks="first"),
            _make_doc("doc-2", [0.0, 1.0, 0.0], title_tks="second"),
        ],
        "ragflow_tenant",
        "kb-1",
    )
    conn.insert(
        [
            _make_doc("doc-1", [0.5, 0.5, 0.0], title_tks="first-updated"),
            _make_doc("doc-3", [0.0, 0.0, 1.0], title_tks="third"),
        ],
        "ragflow_tenant",
        "kb-1",
    )

    result = conn.search(
        select_fields=["title_tks"],
        highlight_fields=[],
        condition={},
        match_expressions=[],
        order_by=OrderByExpr(),
        offset=0,
        limit=10,
        index_names="ragflow_tenant",
        knowledgebase_ids=["kb-1"],
    )

    assert conn.get_total(result) == 3
    fields = conn.get_fields(result, ["title_tks"])
    assert fields["doc-1"]["title_tks"] == "first-updated"
    assert fields["doc-3"]["title_tks"] == "third"


@pytest.mark.p2
def test_error_handling_matches_stage1_expectations(monkeypatch):
    module = _load_qdrant_module(monkeypatch)
    monkeypatch.setattr(module.time, "sleep", lambda *_args, **_kwargs: None)
    conn = module.QdrantConnection()
    conn.create_idx("ragflow_tenant", "kb-1", 3)

    with pytest.raises(Exception, match="dense size 3"):
        conn.create_idx("ragflow_tenant", "kb-1", 4)

    def _raise_upsert(*_args, **_kwargs):
        raise RuntimeError("boom")

    monkeypatch.setattr(conn.client, "upsert", _raise_upsert)
    errors = conn.insert([_make_doc("doc-1", [1.0, 0.0, 0.0], title_tks="alpha")], "ragflow_tenant", "kb-1")
    assert errors == ["boom"]

    with pytest.raises(Exception, match="Qdrant does not support SQL"):
        conn.sql("SELECT * FROM ragflow_tenant", 10, "json")
