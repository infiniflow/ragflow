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
import logging
from numbers import Integral
import re
import time
from uuid import NAMESPACE_URL, UUID, uuid5
from typing import Any

from qdrant_client import QdrantClient, models

from common import settings
from common.constants import PAGERANK_FLD, TAG_FLD
from common.doc_store.doc_store_base import DocStoreConnection, FusionExpr, MatchDenseExpr, MatchExpr, MatchSparseExpr, MatchTextExpr, OrderByExpr, SparseVector
from rag.utils.sparse_vector import (
    DENSE_VECTOR_NAME,
    SPARSE_VECTOR_FIELD,
    SPARSE_VECTOR_NAME,
    dense_vector_field_name,
)
from rag.nlp import is_english, rag_tokenizer

ATTEMPT_TIME = 2
INIT_ATTEMPT_TIME = 24
DEFAULT_SCROLL_BATCH_SIZE = 256
METADATA_VECTOR_SIZE = 1
METADATA_INDEX_PREFIX = "ragflow_doc_meta_"
DEFAULT_TEXT_TOKENIZER = "multilingual"
TEXT_INDEX_FIELDS = ("content_with_weight",)
VECTOR_FIELD_RE = re.compile(r"q_(\d+)_vec$")


class QdrantConnection(DocStoreConnection):
    def __init__(self):
        self.logger = logging.getLogger("ragflow.qdrant_conn")
        conf = getattr(settings, "QDRANT", {}) or {}
        scheme = "https" if self._to_bool(conf.get("https", False)) else "http"
        host = conf.get("host", "qdrant")
        http_port = int(conf.get("http_port", 6333))
        self.prefer_grpc = self._to_bool(conf.get("prefer_grpc", False))
        self.timeout = int(conf.get("timeout", 10))
        api_key = conf.get("api_key") or None
        self.url = conf.get("url") or f"{scheme}://{host}:{http_port}"
        self.client = QdrantClient(
            url=self.url,
            api_key=api_key,
            prefer_grpc=self.prefer_grpc,
            timeout=self.timeout,
        )
        self._wait_until_healthy()
        self.logger.info(f"Use Qdrant {self.url} as the doc engine.")

    def _wait_until_healthy(self):
        last_error = None
        for _ in range(INIT_ATTEMPT_TIME):
            try:
                self.client.get_collections()
                return
            except Exception as error:
                last_error = error
                self.logger.warning(f"{error}. Waiting Qdrant {self.url} to be healthy.")
                time.sleep(5)
        msg = f"Qdrant {self.url} is unhealthy in 120s."
        self.logger.error(msg)
        raise Exception(msg) from last_error

    @staticmethod
    def _to_bool(value: Any) -> bool:
        if isinstance(value, bool):
            return value
        if isinstance(value, str):
            return value.strip().lower() in {"1", "true", "yes", "on"}
        return bool(value)

    @staticmethod
    def _is_metadata_index(index_name: str) -> bool:
        return index_name.startswith(METADATA_INDEX_PREFIX)

    @staticmethod
    def _empty_response(agg_fields: list[str] | None = None):
        res = {"hits": {"total": {"value": 0}, "hits": []}}
        if agg_fields:
            res["aggregations"] = {f"aggs_{fld}": {"buckets": []} for fld in agg_fields}
        return res

    @staticmethod
    def _get_attr(obj: Any, *names: str):
        cur = obj
        for name in names:
            if cur is None:
                return None
            if isinstance(cur, dict):
                cur = cur.get(name)
            else:
                cur = getattr(cur, name, None)
        return cur

    @staticmethod
    def _extract_payload(point: Any) -> dict:
        payload = getattr(point, "payload", None)
        if payload is None and isinstance(point, dict):
            payload = point.get("payload")
        return copy.deepcopy(payload or {})

    @staticmethod
    def _extract_point_id(point: Any):
        payload = QdrantConnection._extract_payload(point)
        if isinstance(payload, dict) and payload.get("id") is not None:
            return payload.get("id")
        return QdrantConnection._extract_storage_point_id(point)

    @staticmethod
    def _extract_storage_point_id(point: Any):
        if isinstance(point, dict):
            return point.get("id")
        return getattr(point, "id", None)

    @staticmethod
    def _to_qdrant_point_id(external_id: Any):
        if external_id is None:
            raise ValueError("missing_document_id")
        if isinstance(external_id, Integral) and not isinstance(external_id, bool):
            if int(external_id) < 0:
                raise ValueError(f"{external_id}:invalid_negative_id")
            return int(external_id)
        external_id = str(external_id).strip()
        try:
            UUID(external_id)
            return external_id
        except Exception:
            return str(uuid5(NAMESPACE_URL, f"ragflow:{external_id}"))

    @staticmethod
    def _extract_point_score(point: Any) -> float:
        if isinstance(point, dict):
            return float(point.get("score", 0.0) or 0.0)
        score = getattr(point, "score", 0.0)
        return float(score or 0.0)

    @staticmethod
    def _extract_vectors(point: Any):
        if isinstance(point, dict):
            return point.get("vector")
        return getattr(point, "vector", None)

    @classmethod
    def _extract_dense_vector(cls, payload: dict):
        for key in list(payload.keys()):
            if not VECTOR_FIELD_RE.fullmatch(key):
                continue
            vector = payload.pop(key)
            return key, vector
        return None, None

    @staticmethod
    def _extract_sparse_vector(payload: dict) -> SparseVector | None:
        raw = payload.pop(SPARSE_VECTOR_FIELD, None)
        if raw is None:
            return None
        if isinstance(raw, SparseVector):
            return raw
        if isinstance(raw, dict) and "indices" in raw:
            return SparseVector.from_dict(raw)
        raise ValueError(f"Invalid sparse vector payload for field `{SPARSE_VECTOR_FIELD}`.")

    @staticmethod
    def _metadata_vector() -> list[float]:
        return [0.0] * METADATA_VECTOR_SIZE

    @staticmethod
    def _named_vectors_config(vector_size: int):
        vectors_config = {
            DENSE_VECTOR_NAME: models.VectorParams(size=vector_size, distance=models.Distance.COSINE),
        }
        # Future multivector extension point for ColPali/ColQwen-style page vectors.
        return vectors_config

    @staticmethod
    def _default_named_vectors(dense_vector: list[float], sparse_vector: SparseVector | dict | None = None):
        vectors = {
            DENSE_VECTOR_NAME: list(dense_vector),
        }
        vectors[SPARSE_VECTOR_NAME] = QdrantConnection._to_qdrant_sparse_vector(sparse_vector)
        return vectors

    @staticmethod
    def _empty_sparse_vector():
        return models.SparseVector(indices=[], values=[])

    @staticmethod
    def _to_qdrant_sparse_vector(sparse_vector: SparseVector | dict | None):
        if sparse_vector is None:
            return QdrantConnection._empty_sparse_vector()
        if isinstance(sparse_vector, dict):
            sparse_vector = SparseVector.from_dict(sparse_vector)
        return models.SparseVector(indices=list(sparse_vector.indices), values=list(sparse_vector.values or []))

    @classmethod
    def _payload_with_vector_alias(cls, payload: dict, vector_name: str | None, vector: Any, score: float) -> dict:
        doc = copy.deepcopy(payload)
        if vector_name and vector is not None:
            doc[vector_name] = vector
        doc["_score"] = score
        return doc

    @staticmethod
    def _requested_vector_field(select_fields: list[str]) -> str | None:
        for field in select_fields:
            if VECTOR_FIELD_RE.fullmatch(field):
                return field
        return None

    @staticmethod
    def _parse_field_weight(field_name: str) -> tuple[str, float]:
        if "^" not in field_name:
            return field_name, 1.0
        base, weight = field_name.split("^", 1)
        try:
            return base, float(weight)
        except Exception:
            return base, 1.0

    @staticmethod
    def _normalize_text(value: Any) -> str:
        if value is None:
            return ""
        if isinstance(value, str):
            return value
        if isinstance(value, list):
            return " ".join(str(v) for v in value)
        return str(value)

    def _text_index_tokenizer(self):
        tokenizer = str((getattr(settings, "QDRANT", {}) or {}).get("text_index_tokenizer", DEFAULT_TEXT_TOKENIZER)).strip().lower()
        tokenizer_map = {
            "prefix": models.TokenizerType.PREFIX,
            "whitespace": models.TokenizerType.WHITESPACE,
            "word": models.TokenizerType.WORD,
            "multilingual": models.TokenizerType.MULTILINGUAL,
        }
        return tokenizer_map.get(tokenizer, models.TokenizerType.MULTILINGUAL)

    @staticmethod
    def _sparse_vector_params():
        return models.SparseVectorParams(
            index=models.SparseIndexParams(on_disk=True),
            modifier=models.Modifier.IDF,
        )

    def _text_index_params(self):
        return models.TextIndexParams(
            type=models.TextIndexType.TEXT,
            tokenizer=self._text_index_tokenizer(),
            lowercase=True,
            on_disk=True,
        )

    @classmethod
    def _query_terms(cls, match_expr: MatchTextExpr | None) -> tuple[str, list[str]]:
        if not match_expr:
            return "", []
        original = ""
        if isinstance(match_expr.extra_options, dict):
            original = match_expr.extra_options.get("original_query", "")
        original = original or match_expr.matching_text or ""
        tokens = [tk for tk in rag_tokenizer.tokenize(original).split() if tk]
        return original, tokens

    @classmethod
    def _text_score(cls, payload: dict, match_expr: MatchTextExpr | None) -> float:
        if not match_expr:
            return 0.0
        original_query, query_terms = cls._query_terms(match_expr)
        if not original_query and not query_terms:
            return 0.0
        score = 0.0
        original_lc = original_query.lower().strip()
        for field in match_expr.fields:
            base_field, weight = cls._parse_field_weight(field)
            text = cls._normalize_text(payload.get(base_field)).lower()
            if not text:
                continue
            token_hits = sum(1 for term in query_terms if term.lower() in text)
            phrase_bonus = 1 if original_lc and original_lc in text else 0
            score += weight * (token_hits + phrase_bonus)
        return score

    @staticmethod
    def _rank_feature_score(payload: dict, rank_feature: dict | None) -> float:
        if not rank_feature:
            return 0.0
        score = 0.0
        for field, weight in rank_feature.items():
            if field == PAGERANK_FLD:
                value = payload.get(field, 0.0)
                try:
                    score += float(value) * float(weight)
                except Exception:
                    continue
                continue
            tag_values = payload.get(TAG_FLD, {})
            if not isinstance(tag_values, dict):
                continue
            try:
                score += float(tag_values.get(field, 0.0)) * float(weight)
            except Exception:
                continue
        return score

    @classmethod
    def _match_field_condition(cls, point_id: str, payload: dict, key: str, value: Any) -> bool:
        if key == "id":
            if isinstance(value, list):
                return point_id in value
            return point_id == value
        field_value = payload.get(key)
        if key == "available_int":
            try:
                # ES treats a missing availability flag as available because the
                # `must_not range lt 1` filter does not exclude missing fields.
                # Mirror that behavior here so legacy chunks without
                # `available_int` remain searchable under Qdrant.
                field_value_num = 1.0 if field_value is None else float(field_value)
                if value == 0:
                    return field_value_num < 1
                return field_value_num >= 1
            except Exception:
                return False
        if isinstance(value, list):
            return field_value in value
        return field_value == value

    @classmethod
    def _matches_condition(cls, point_id: str, payload: dict, condition: dict) -> bool:
        for key, value in (condition or {}).items():
            if key == "exists":
                if value not in payload:
                    return False
                continue
            if key == "must_not":
                if not isinstance(value, dict):
                    continue
                for sub_key, sub_value in value.items():
                    if sub_key == "exists" and sub_value in payload:
                        return False
                    if sub_key in payload and payload.get(sub_key) == sub_value:
                        return False
                continue
            if value is None:
                continue
            if not cls._match_field_condition(point_id, payload, key, value):
                return False
        return True

    @staticmethod
    def _sortable_value(value: Any):
        if value is None:
            return (1, None)
        if isinstance(value, list):
            return (0, tuple(value))
        return (0, value)

    @classmethod
    def _sort_hits(cls, hits: list[dict], order_by: OrderByExpr | None) -> list[dict]:
        if not order_by or not order_by.fields:
            return hits
        sorted_hits = hits
        for field, direction in reversed(order_by.fields):
            reverse = direction == 1
            sorted_hits = sorted(
                sorted_hits,
                key=lambda hit: cls._sortable_value(hit["_source"].get(field)),
                reverse=reverse,
            )
        return sorted_hits

    @classmethod
    def _highlight_hit(cls, payload: dict, highlight_fields: list[str], query_terms: list[str]) -> dict | None:
        if not highlight_fields or not query_terms:
            return None
        highlights = {}
        for field in highlight_fields:
            raw_text = cls._normalize_text(payload.get(field))
            if not raw_text:
                continue
            if not any(term.lower() in raw_text.lower() for term in query_terms):
                continue
            highlighted = raw_text
            for term in query_terms[:16]:
                highlighted = re.sub(re.escape(term), f"<em>{term}</em>", highlighted, flags=re.IGNORECASE)
            highlights[field] = [highlighted]
        return highlights or None

    def _build_filter(self, condition: dict, dataset_ids: list[str], index_name: str):
        must = []
        must_not = []
        if not self._is_metadata_index(index_name):
            if dataset_ids:
                condition = {**condition, "kb_id": dataset_ids}
        for key, value in (condition or {}).items():
            if value is None or key in {"exists", "must_not"}:
                continue
            if key == "available_int":
                if value == 0:
                    must.append(models.FieldCondition(key=key, range=models.Range(lt=1)))
                else:
                    must_not.append(models.FieldCondition(key=key, range=models.Range(lt=1)))
                continue
            if isinstance(value, list):
                must.append(models.FieldCondition(key=key, match=models.MatchAny(any=value)))
            elif isinstance(value, (str, int, float, bool)):
                must.append(models.FieldCondition(key=key, match=models.MatchValue(value=value)))
            else:
                raise Exception(
                    f"Condition `{str(key)}={str(value)}` value type is {str(type(value))}, expected to be int, str or list."
                )
        if not must and not must_not:
            return None
        return models.Filter(must=must or None, must_not=must_not or None)

    def _search_points(self, collection_name: str, dense_expr: MatchDenseExpr, condition: dict, dataset_ids: list[str], with_vectors: bool, limit: int):
        query_filter = self._build_filter(condition, dataset_ids, collection_name)
        query_vector = (DENSE_VECTOR_NAME, list(dense_expr.embedding_data))
        extra_options = dense_expr.extra_options or {}
        similarity = extra_options.get("similarity")
        kwargs = {
            "collection_name": collection_name,
            "query_vector": query_vector,
            "query_filter": query_filter,
            "limit": limit,
            "with_payload": True,
            "with_vectors": [DENSE_VECTOR_NAME] if with_vectors else False,
        }
        if similarity is not None:
            kwargs["score_threshold"] = similarity
        return self.client.search(**kwargs)

    @staticmethod
    def _response_points(query_response: Any):
        return QdrantConnection._get_attr(query_response, "points") or []

    def _query_points_hybrid(
        self,
        collection_name: str,
        dense_expr: MatchDenseExpr,
        sparse_expr: MatchSparseExpr,
        condition: dict,
        dataset_ids: list[str],
        with_vectors: bool,
        limit: int,
    ):
        query_filter = self._build_filter(condition, dataset_ids, collection_name)
        dense_options = dense_expr.extra_options or {}
        dense_similarity = dense_options.get("similarity")
        # Future multivector retrieval should branch here instead of dense+sparse RRF.
        prefetch = [
            models.Prefetch(
                query=list(dense_expr.embedding_data),
                using=DENSE_VECTOR_NAME,
                filter=query_filter,
                score_threshold=dense_similarity,
                limit=max(limit, dense_expr.topn, 1),
            ),
            models.Prefetch(
                query=self._to_qdrant_sparse_vector(sparse_expr.sparse_data),
                using=SPARSE_VECTOR_NAME,
                filter=query_filter,
                limit=max(limit, sparse_expr.topn, 1),
            ),
        ]
        return self.client.query_points(
            collection_name=collection_name,
            query=models.FusionQuery(fusion=models.Fusion.RRF),
            prefetch=prefetch,
            limit=limit,
            with_payload=True,
            with_vectors=[DENSE_VECTOR_NAME] if with_vectors else False,
        )

    def _scroll_points(self, collection_name: str, condition: dict, dataset_ids: list[str], with_vectors: bool):
        points = []
        q_filter = self._build_filter(condition, dataset_ids, collection_name)
        next_offset = None
        while True:
            scroll_kwargs = {
                "collection_name": collection_name,
                "scroll_filter": q_filter,
                "limit": DEFAULT_SCROLL_BATCH_SIZE,
                "with_payload": True,
                "with_vectors": [DENSE_VECTOR_NAME] if with_vectors else False,
            }
            if next_offset is not None:
                scroll_kwargs["offset"] = next_offset
            batch_points, next_offset = self.client.scroll(**scroll_kwargs)
            if not batch_points:
                break
            points.extend(batch_points)
            if next_offset is None:
                break
        return points

    def _collection_dense_size(self, collection_name: str) -> int | None:
        info = self.client.get_collection(collection_name)
        vectors = self._get_attr(info, "config", "params", "vectors")
        if vectors is None:
            return None
        if isinstance(vectors, dict):
            dense = vectors.get(DENSE_VECTOR_NAME)
            if dense is None and vectors:
                dense = next(iter(vectors.values()))
            return self._get_attr(dense, "size")
        return self._get_attr(vectors, "size")

    def _collection_has_sparse_vector(self, collection_name: str) -> bool:
        info = self.client.get_collection(collection_name)
        sparse_vectors = self._get_attr(info, "config", "params", "sparse_vectors")
        if isinstance(sparse_vectors, dict):
            return SPARSE_VECTOR_NAME in sparse_vectors
        return bool(sparse_vectors)

    def _ensure_sparse_schema(self, collection_name: str):
        if self._is_metadata_index(collection_name):
            return
        if self._collection_has_sparse_vector(collection_name):
            return
        self._retry(
            self.client.update_collection,
            collection_name=collection_name,
            sparse_vectors_config={SPARSE_VECTOR_NAME: self._sparse_vector_params()},
        )

    def _ensure_text_indexes(self, collection_name: str):
        if self._is_metadata_index(collection_name):
            return
        for field_name in TEXT_INDEX_FIELDS:
            try:
                self._retry(
                    self.client.create_payload_index,
                    collection_name=collection_name,
                    field_name=field_name,
                    field_schema=self._text_index_params(),
                    wait=True,
                )
            except Exception as error:
                self.logger.debug("Skipping payload index %s on %s: %s", field_name, collection_name, error)

    def _make_point(self, collection_name: str, row: dict, dataset_id: str):
        assert "_id" not in row
        assert "id" in row
        payload = copy.deepcopy(row)
        payload["kb_id"] = dataset_id
        if not self._is_metadata_index(collection_name):
            payload.setdefault("available_int", 1)
        point_id = payload.get("id")
        storage_point_id = self._to_qdrant_point_id(point_id)
        vector_field, dense_vector = self._extract_dense_vector(payload)
        sparse_vector = self._extract_sparse_vector(payload)
        if dense_vector is None:
            if self._is_metadata_index(collection_name):
                dense_vector = self._metadata_vector()
            else:
                raise ValueError(f"{point_id}:missing_dense_vector")
        # Future multivector ingestion should add page/image vectors alongside the primary dense text vector.
        vector = {DENSE_VECTOR_NAME: list(dense_vector)} if self._is_metadata_index(collection_name) else self._default_named_vectors(dense_vector, sparse_vector)
        return point_id, vector_field, models.PointStruct(id=storage_point_id, vector=vector, payload=payload)

    def _point_to_hit(self, point: Any, requested_vector_field: str | None, text_expr: MatchTextExpr | None, highlight_fields: list[str], rank_feature: dict | None = None):
        payload = self._extract_payload(point)
        point_id = self._extract_point_id(point)
        vectors = self._extract_vectors(point)
        dense_vector = None
        if isinstance(vectors, dict):
            dense_vector = vectors.get(DENSE_VECTOR_NAME)
        elif vectors is not None:
            dense_vector = vectors
        # Future multivector retrieval should map page/image vectors back into response fields here.
        score = self._extract_point_score(point)
        score += self._rank_feature_score(payload, rank_feature)
        source = self._payload_with_vector_alias(payload, requested_vector_field, dense_vector, score)
        highlight = self._highlight_hit(source, highlight_fields, self._query_terms(text_expr)[1])
        hit = {"_id": point_id, "_score": score, "_source": source}
        if highlight:
            hit["highlight"] = highlight
        return hit

    def _normalize_dense_scores(self, hits: list[dict]):
        max_score = max((hit.get("_score", 0.0) for hit in hits), default=0.0)
        if max_score <= 0:
            return {hit["_id"]: 0.0 for hit in hits}
        return {hit["_id"]: hit.get("_score", 0.0) / max_score for hit in hits}

    def _combine_dense_text_scores(self, hits: list[dict], text_expr: MatchTextExpr | None, fusion_expr: FusionExpr | None):
        if not hits:
            return hits
        dense_scores = self._normalize_dense_scores(hits)
        text_scores = {hit["_id"]: self._text_score(hit["_source"], text_expr) for hit in hits}
        max_text_score = max(text_scores.values(), default=0.0)
        if max_text_score > 0:
            text_scores = {doc_id: score / max_text_score for doc_id, score in text_scores.items()}
        else:
            text_scores = {doc_id: 0.0 for doc_id in text_scores}
        text_weight = 0.0
        dense_weight = 1.0
        if fusion_expr and fusion_expr.method == "weighted_sum" and fusion_expr.fusion_params and fusion_expr.fusion_params.get("weights"):
            try:
                text_weight, dense_weight = [float(v) for v in fusion_expr.fusion_params["weights"].split(",", 1)]
            except Exception:
                text_weight, dense_weight = 0.0, 1.0
        for hit in hits:
            hit["_score"] = dense_scores.get(hit["_id"], 0.0) * dense_weight + text_scores.get(hit["_id"], 0.0) * text_weight
            hit["_source"]["_score"] = hit["_score"]
        return sorted(hits, key=lambda hit: hit.get("_score", 0.0), reverse=True)

    def _aggregate_hits(self, hits: list[dict], agg_fields: list[str] | None):
        if not agg_fields:
            return None
        aggregations = {}
        for field in agg_fields:
            buckets = {}
            for hit in hits:
                value = hit["_source"].get(field)
                values = value if isinstance(value, list) else [value]
                for item in values:
                    if item in (None, ""):
                        continue
                    buckets[item] = buckets.get(item, 0) + 1
            aggregations[f"aggs_{field}"] = {
                "buckets": [{"key": key, "doc_count": count} for key, count in sorted(buckets.items(), key=lambda item: (-item[1], item[0]))]
            }
        return aggregations

    def _retry(self, func, *args, **kwargs):
        last_error = None
        for _ in range(ATTEMPT_TIME):
            try:
                return func(*args, **kwargs)
            except Exception as error:
                last_error = error
                self.logger.warning(f"Qdrant request failed: {error}")
                time.sleep(1)
                continue
        raise last_error

    def db_type(self) -> str:
        return "qdrant"

    def health(self) -> dict:
        info = self._retry(self.client.get_collections)
        collections = self._get_attr(info, "collections") or []
        return {
            "type": "qdrant",
            "status": "green",
            "url": self.url,
            "collections": len(collections),
        }

    def get_cluster_stats(self):
        stats = self.health()
        stats["cluster_name"] = "qdrant"
        return stats

    def create_idx(self, index_name: str, dataset_id: str, vector_size: int, parser_id: str = None):
        if self.index_exist(index_name, dataset_id):
            current_size = self._collection_dense_size(index_name)
            if current_size is not None and current_size != vector_size:
                raise Exception(
                    f"Qdrant collection {index_name} already exists with dense size {current_size}, but {vector_size} was requested."
                )
            self._ensure_sparse_schema(index_name)
            self._ensure_text_indexes(index_name)
            return True
        created = self._retry(
            self.client.create_collection,
            collection_name=index_name,
            vectors_config=self._named_vectors_config(vector_size),
            sparse_vectors_config={
                SPARSE_VECTOR_NAME: self._sparse_vector_params(),
            },
            on_disk_payload=True,
        )
        self._ensure_text_indexes(index_name)
        return created

    def create_doc_meta_idx(self, index_name: str):
        if self.index_exist(index_name, ""):
            return True
        return self._retry(
            self.client.create_collection,
            collection_name=index_name,
            vectors_config=self._named_vectors_config(METADATA_VECTOR_SIZE),
            on_disk_payload=True,
        )

    def delete_idx(self, index_name: str, dataset_id: str):
        if len(dataset_id) > 0:
            return
        if not self.index_exist(index_name, dataset_id):
            return
        try:
            self._retry(self.client.delete_collection, collection_name=index_name)
        except Exception:
            self.logger.exception("QdrantConnection.delete_idx error %s" % index_name)

    def index_exist(self, index_name: str, dataset_id: str = None) -> bool:
        try:
            return bool(self._retry(self.client.collection_exists, collection_name=index_name))
        except Exception as error:
            self.logger.exception(error)
            return False

    def get(self, data_id: str, index_name: str, dataset_ids: list[str]) -> dict | None:
        try:
            records = self._retry(
                self.client.retrieve,
                collection_name=index_name,
                ids=[self._to_qdrant_point_id(data_id)],
                with_payload=True,
                with_vectors=[DENSE_VECTOR_NAME],
            )
            if not records:
                return None
            point = records[0]
            vector_field = None
            if not self._is_metadata_index(index_name):
                dense_size = len((self._extract_vectors(point) or {}).get(DENSE_VECTOR_NAME, [])) if isinstance(self._extract_vectors(point), dict) else None
                if dense_size:
                    vector_field = dense_vector_field_name(dense_size)
            doc = self._point_to_hit(point, vector_field, None, [])
            doc_source = doc["_source"]
            doc_source["id"] = self._extract_point_id(point)
            return doc_source
        except Exception as error:
            self.logger.exception(f"QdrantConnection.get({data_id}) got exception")
            raise error

    def search(
        self,
        select_fields: list[str],
        highlight_fields: list[str],
        condition: dict,
        match_expressions: list[MatchExpr],
        order_by: OrderByExpr,
        offset: int,
        limit: int,
        index_names: str | list[str],
        knowledgebase_ids: list[str],
        agg_fields: list[str] | None = None,
        rank_feature: dict | None = None,
    ):
        if isinstance(index_names, str):
            index_names = index_names.split(",")
        assert isinstance(index_names, list) and len(index_names) > 0
        assert "_id" not in condition

        existing_indexes = [index_name for index_name in index_names if self.index_exist(index_name, "")]
        if not existing_indexes:
            return self._empty_response(agg_fields)

        dense_expr = next((expr for expr in match_expressions if isinstance(expr, MatchDenseExpr)), None)
        sparse_expr = next((expr for expr in match_expressions if isinstance(expr, MatchSparseExpr)), None)
        text_expr = next((expr for expr in match_expressions if isinstance(expr, MatchTextExpr)), None)
        fusion_expr = next((expr for expr in match_expressions if isinstance(expr, FusionExpr)), None)
        requested_vector_field = self._requested_vector_field(select_fields)
        need_vectors = requested_vector_field is not None

        all_hits = []
        for index_name in existing_indexes:
            if dense_expr is not None:
                query_limit = max(limit + offset, dense_expr.topn, sparse_expr.topn if sparse_expr else 0, fusion_expr.topn if fusion_expr else 0, 1)
                if sparse_expr is not None and not self._is_metadata_index(index_name):
                    self._ensure_sparse_schema(index_name)
                    self._ensure_text_indexes(index_name)
                    points = self._response_points(
                        self._retry(
                            self._query_points_hybrid,
                            index_name,
                            dense_expr,
                            sparse_expr,
                            condition,
                            knowledgebase_ids,
                            need_vectors,
                            query_limit,
                        )
                    )
                else:
                    points = self._retry(
                        self._search_points,
                        index_name,
                        dense_expr,
                        condition,
                        knowledgebase_ids,
                        need_vectors,
                        query_limit,
                    )
                full_condition = {
                    **condition,
                    **(
                        {"kb_id": knowledgebase_ids}
                        if not self._is_metadata_index(index_name) and knowledgebase_ids
                        else {}
                    ),
                }
                index_hits = [
                    self._point_to_hit(point, requested_vector_field, text_expr, highlight_fields, rank_feature=rank_feature)
                    for point in points
                    if self._matches_condition(
                        self._extract_point_id(point),
                        self._extract_payload(point),
                        full_condition,
                    )
                ]
                if sparse_expr is None:
                    index_hits = self._combine_dense_text_scores(index_hits, text_expr, fusion_expr)
            else:
                points = self._retry(self._scroll_points, index_name, condition, knowledgebase_ids, need_vectors)
                index_hits = []
                for point in points:
                    payload = self._extract_payload(point)
                    point_id = self._extract_point_id(point)
                    full_condition = {**condition, **({"kb_id": knowledgebase_ids} if not self._is_metadata_index(index_name) and knowledgebase_ids else {})}
                    if not self._matches_condition(point_id, payload, full_condition):
                        continue
                    score = self._text_score(payload, text_expr)
                    if text_expr and score <= 0:
                        continue
                    hit = self._point_to_hit(point, requested_vector_field, text_expr, highlight_fields, rank_feature=rank_feature)
                    if text_expr:
                        hit["_score"] = score + self._rank_feature_score(payload, rank_feature)
                        hit["_source"]["_score"] = hit["_score"]
                    index_hits.append(hit)
                if text_expr:
                    index_hits = sorted(index_hits, key=lambda hit: hit.get("_score", 0.0), reverse=True)
                else:
                    index_hits = self._sort_hits(index_hits, order_by)
            all_hits.extend(index_hits)

        if dense_expr is not None or text_expr is not None:
            all_hits = sorted(all_hits, key=lambda hit: hit.get("_score", 0.0), reverse=True)
        elif order_by and order_by.fields:
            all_hits = self._sort_hits(all_hits, order_by)

        total = len(all_hits)
        if limit > 0:
            hits = all_hits[offset:offset + limit]
        else:
            hits = []

        res = {
            "hits": {
                "total": {"value": total},
                "hits": hits,
            }
        }
        aggregations = self._aggregate_hits(all_hits, agg_fields)
        if aggregations is not None:
            res["aggregations"] = aggregations
        return res

    def insert(self, documents: list[dict], index_name: str, knowledgebase_id: str = None) -> list[str]:
        res = []
        points = []
        if not self._is_metadata_index(index_name) and self.index_exist(index_name, knowledgebase_id):
            self._ensure_sparse_schema(index_name)
            self._ensure_text_indexes(index_name)
        try:
            for doc in documents:
                _, _, point = self._make_point(index_name, doc, knowledgebase_id)
                points.append(point)
        except Exception as error:
            return [str(error)]

        for _ in range(ATTEMPT_TIME):
            try:
                self.client.upsert(collection_name=index_name, points=points, wait=False)
                return res
            except Exception as error:
                res = [str(error)]
                self.logger.warning("QdrantConnection.insert got exception: " + str(error))
                time.sleep(1)
                continue
        return res

    def update(self, condition: dict, new_value: dict, index_name: str, knowledgebase_id: str) -> bool:
        doc = copy.deepcopy(new_value)
        doc.pop("id", None)
        full_condition = copy.deepcopy(condition)
        if not self._is_metadata_index(index_name):
            full_condition["kb_id"] = knowledgebase_id
            if self.index_exist(index_name, knowledgebase_id):
                self._ensure_sparse_schema(index_name)
                self._ensure_text_indexes(index_name)
        try:
            points = self._scroll_points(index_name, full_condition, [], True)
            matched = []
            for point in points:
                payload = self._extract_payload(point)
                point_id = self._extract_point_id(point)
                if not self._matches_condition(point_id, payload, full_condition):
                    continue
                matched.append(point)
            if not matched:
                return False if isinstance(condition.get("id"), str) else True

            updated_points = []
            for point in matched:
                payload = self._extract_payload(point)
                vectors = self._extract_vectors(point) or {}
                dense_vector = vectors.get(DENSE_VECTOR_NAME) if isinstance(vectors, dict) else vectors
                sparse_vector = vectors.get(SPARSE_VECTOR_NAME) if isinstance(vectors, dict) else None
                for key, value in doc.items():
                    if key == "remove":
                        if isinstance(value, str):
                            payload.pop(value, None)
                        elif isinstance(value, dict):
                            for rm_key, rm_val in value.items():
                                if isinstance(payload.get(rm_key), list):
                                    payload[rm_key] = [item for item in payload.get(rm_key, []) if item != rm_val]
                        continue
                    if key == "add":
                        if isinstance(value, dict):
                            for add_key, add_val in value.items():
                                payload.setdefault(add_key, [])
                                if isinstance(payload[add_key], list):
                                    payload[add_key].append(add_val.strip() if isinstance(add_val, str) else add_val)
                        continue
                    if (not isinstance(key, str) or not value) and key != "available_int":
                        continue
                    if VECTOR_FIELD_RE.fullmatch(key):
                        dense_vector = list(value)
                        continue
                    if key == SPARSE_VECTOR_FIELD:
                        sparse_vector = self._to_qdrant_sparse_vector(value)
                        continue
                    payload[key] = value
                vector = {DENSE_VECTOR_NAME: list(dense_vector or self._metadata_vector())}
                if not self._is_metadata_index(index_name):
                    # Future multivector updates should preserve page/image vectors on partial updates.
                    vector[SPARSE_VECTOR_NAME] = sparse_vector if sparse_vector is not None else self._empty_sparse_vector()
                updated_points.append(
                    models.PointStruct(
                        id=self._extract_storage_point_id(point),
                        vector=vector,
                        payload=payload,
                    )
                )
            self.client.upsert(collection_name=index_name, points=updated_points, wait=True)
            return True
        except Exception as error:
            self.logger.error("QdrantConnection.update got exception: " + str(error))
            return False

    def delete(self, condition: dict, index_name: str, knowledgebase_id: str) -> int:
        assert "_id" not in condition
        full_condition = copy.deepcopy(condition)
        if not self._is_metadata_index(index_name):
            full_condition["kb_id"] = knowledgebase_id
        try:
            points = self._scroll_points(index_name, full_condition, [], False)
            point_ids = []
            for point in points:
                payload = self._extract_payload(point)
                point_id = self._extract_point_id(point)
                if self._matches_condition(point_id, payload, full_condition):
                    point_ids.append(self._extract_storage_point_id(point))
            if not point_ids:
                return 0
            self.client.delete(collection_name=index_name, points_selector=point_ids, wait=True)
            return len(point_ids)
        except Exception as error:
            self.logger.warning("QdrantConnection.delete got exception: " + str(error))
            if re.search(r"(not_found)", str(error), re.IGNORECASE):
                return 0
            return 0

    def get_total(self, res):
        if isinstance(res["hits"]["total"], dict):
            return res["hits"]["total"].get("value", 0)
        return res["hits"]["total"]

    def get_doc_ids(self, res):
        return [doc["_id"] for doc in res["hits"]["hits"]]

    def _get_source(self, res):
        sources = []
        for hit in res["hits"]["hits"]:
            doc = copy.deepcopy(hit["_source"])
            doc["id"] = hit["_id"]
            doc["_score"] = hit["_score"]
            sources.append(doc)
        return sources

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        res_fields = {}
        if not fields:
            return {}
        for doc in self._get_source(res):
            if fields == ["*"]:
                res_fields[doc["id"]] = doc
                continue
            mapped = {name: doc.get(name) for name in fields if doc.get(name) is not None}
            for name, value in list(mapped.items()):
                if isinstance(value, list):
                    continue
                if name == "available_int" and isinstance(value, (int, float)):
                    continue
                if not isinstance(value, str):
                    mapped[name] = str(value)
            if mapped:
                res_fields[doc["id"]] = mapped
        return res_fields

    @staticmethod
    def _raw_highlight_fallback(raw_text: str) -> str:
        if not raw_text:
            return ""
        raw_text = re.sub(r"[\r\n]+", " ", raw_text, flags=re.IGNORECASE | re.MULTILINE).strip()
        if len(raw_text) <= 280:
            return raw_text
        return raw_text[:280].rstrip() + "..."

    def get_highlight(self, res, keywords: list[str], field_name: str):
        ans = {}
        for hit in res["hits"]["hits"]:
            highlights = hit.get("highlight")
            if not highlights:
                continue
            txt = "...".join([part for part in list(highlights.items())[0][1]])
            if not is_english(txt.split()):
                ans[hit["_id"]] = txt
                continue
            raw_txt = hit["_source"].get(field_name, "")
            raw_txt = re.sub(r"[\r\n]", " ", raw_txt, flags=re.IGNORECASE | re.MULTILINE)
            txt_list = []
            for sentence in re.split(r"[.?!;\n]", raw_txt):
                for keyword in keywords:
                    sentence = re.sub(
                        r"(^|[ .?/'\"\(\)!,:;-])(%s)([ .?/'\"\(\)!,:;-])" % re.escape(keyword),
                        r"\1<em>\2</em>\3",
                        sentence,
                        flags=re.IGNORECASE | re.MULTILINE,
                    )
                if not re.search(r"<em>[^<>]+</em>", sentence, flags=re.IGNORECASE | re.MULTILINE):
                    continue
                txt_list.append(sentence)
            ans[hit["_id"]] = "...".join(txt_list) if txt_list else self._raw_highlight_fallback(raw_txt) or txt
        return ans

    def get_aggregation(self, res, field_name: str):
        agg_field = f"aggs_{field_name}"
        if "aggregations" not in res or agg_field not in res["aggregations"]:
            return []
        buckets = res["aggregations"][agg_field]["buckets"]
        return [(bucket["key"], bucket["doc_count"]) for bucket in buckets]

    def sql(self, sql: str, fetch_size: int, format: str):
        raise Exception(f"SQL error: Qdrant does not support SQL.\n\nSQL: {sql}")
