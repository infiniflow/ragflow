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

from __future__ import annotations

import logging
import os
from typing import Any

from common.doc_store.doc_store_base import FusionExpr, MatchDenseExpr, MatchTextExpr
from memory.backend.base import MemoryBackend
from memory.backend.default_backend import DefaultMemoryBackend


class _BackendSearchResult:
    def __init__(self, messages: list[dict]):
        self.messages = messages
        self.total = len(messages)


class PowerMemBackend(MemoryBackend):
    """
    Adapter layer for optional PowerMem SDK.

    This backend keeps full backward compatibility by delegating connector-compatible
    operations to a default backend while exposing enhanced capabilities and optional
    graph search through PowerMem.
    """

    def __init__(self, fallback_backend: DefaultMemoryBackend):
        self._fallback = fallback_backend
        self._logger = logging.getLogger("ragflow.powermem_backend")
        self._powermem_client = self._init_powermem_client()
        self._features = self._detect_features()

    @property
    def name(self) -> str:
        return "powermem"

    @property
    def mode(self) -> str:
        return "oceanbase"

    def _detect_features(self) -> dict[str, bool]:
        # Conservative by default; only enable when capability can be inferred.
        feature_env = {
            "graph": os.getenv("POWERMEM_ENABLE_GRAPH", "true").lower() == "true",
            "user_profile": os.getenv("POWERMEM_ENABLE_USER_PROFILE", "false").lower() == "true",
            "ebbinghaus": os.getenv("POWERMEM_ENABLE_EBBINGHAUS", "false").lower() == "true",
            "rerank": os.getenv("POWERMEM_ENABLE_RERANK", "false").lower() == "true",
            "sparse_vector": os.getenv("POWERMEM_ENABLE_SPARSE_VECTOR", "false").lower() == "true",
            "intelligent_merge": os.getenv("POWERMEM_ENABLE_INTELLIGENT_MERGE", "false").lower() == "true",
        }
        return {
            "graph": feature_env["graph"],
            "user_profile": feature_env["user_profile"],
            "ebbinghaus": feature_env["ebbinghaus"],
            "rerank": feature_env["rerank"],
            "sparse_vector": feature_env["sparse_vector"],
            "intelligent_merge": feature_env["intelligent_merge"],
        }

    def _init_powermem_client(self):
        try:
            from powermem import Memory as PowerMemMemory  # type: ignore
        except Exception as e:
            raise ImportError(
                "MEMORY_BACKEND=powermem requires optional dependency 'powermem'. "
                "Please install it first."
            ) from e

        config: dict[str, Any] = {
            "provider": os.getenv("POWERMEM_PROVIDER", "oceanbase"),
        }

        for key in [
            "POWERMEM_HOST",
            "POWERMEM_PORT",
            "POWERMEM_USER",
            "POWERMEM_PASSWORD",
            "POWERMEM_DATABASE",
            "POWERMEM_TENANT",
            "POWERMEM_API_KEY",
            "POWERMEM_BASE_URL",
        ]:
            value = os.getenv(key)
            if value:
                config[key.lower().replace("powermem_", "")] = value

        try:
            return PowerMemMemory(config=config)
        except TypeError:
            # Some SDK versions initialize without named arguments.
            return PowerMemMemory(config)

    @staticmethod
    def _extract_memory_id_from_doc_id(doc_id: str) -> tuple[str, str]:
        if "_" not in doc_id:
            return "", doc_id
        memory_id, message_id = doc_id.split("_", 1)
        return memory_id, message_id

    @staticmethod
    def _message_type_name(message_type: Any) -> str:
        if message_type is None:
            return "raw"
        return str(message_type).lower()

    def _to_powermem_message(self, message: dict) -> dict:
        return {
            "id": message.get("id"),
            "message_id": str(message.get("message_id", "")),
            "source_id": str(message.get("source_id", "")),
            "memory_id": message.get("memory_id", ""),
            "user_id": message.get("user_id", ""),
            "agent_id": message.get("agent_id", ""),
            "session_id": message.get("session_id", ""),
            "content": message.get("content", ""),
            "message_type": self._message_type_name(message.get("message_type")),
            "valid_at": message.get("valid_at"),
            "invalid_at": message.get("invalid_at"),
            "forget_at": message.get("forget_at"),
            "status": 1 if bool(message.get("status", True)) else 0,
            "content_embed": message.get("content_embed", []),
        }

    def _from_powermem_message(self, item: dict, fallback_memory_id: str = "") -> dict:
        message_id = item.get("message_id", item.get("id", ""))
        source_id = item.get("source_id", 0)
        try:
            source_id = int(source_id) if source_id not in [None, ""] else None
        except Exception:
            pass
        try:
            message_id_cast = int(message_id)
        except Exception:
            message_id_cast = message_id
        status = item.get("status", True)
        if isinstance(status, int):
            status = bool(status)
        return {
            "id": item.get("id"),
            "message_id": message_id_cast,
            "message_type": item.get("message_type", "raw"),
            "source_id": source_id,
            "memory_id": item.get("memory_id", fallback_memory_id),
            "user_id": item.get("user_id", ""),
            "agent_id": item.get("agent_id", ""),
            "session_id": item.get("session_id", ""),
            "valid_at": item.get("valid_at"),
            "invalid_at": item.get("invalid_at"),
            "forget_at": item.get("forget_at"),
            "status": status,
            "content": item.get("content", ""),
            "content_embed": item.get("content_embed", []),
            "_score": item.get("_score", item.get("score", 0.0)),
            "_text_score": item.get("text_score", item.get("bm25_score")),
            "_vector_score": item.get("vector_score", item.get("dense_score")),
        }

    def _call_method(self, method_name: str, candidates: list[dict[str, Any]]):
        method = getattr(self._powermem_client, method_name, None)
        if method is None:
            raise AttributeError(f"PowerMem client has no method '{method_name}'")

        last_type_error: Exception | None = None
        for payload in candidates:
            try:
                return method(**payload)
            except TypeError as e:
                last_type_error = e
                # Try single-dict payload fallback.
                try:
                    return method(payload)
                except TypeError:
                    continue
        if last_type_error:
            raise last_type_error
        raise RuntimeError(f"PowerMem call failed: {method_name}")

    def _extract_items(self, result: Any) -> list[dict]:
        if isinstance(result, list):
            return [r for r in result if isinstance(r, dict)]
        if isinstance(result, dict):
            for key in ["results", "data", "items", "messages"]:
                val = result.get(key)
                if isinstance(val, list):
                    return [r for r in val if isinstance(r, dict)]
        return []

    def _extract_total(self, result: Any, default: int) -> int:
        if isinstance(result, dict):
            for key in ["total", "count"]:
                if isinstance(result.get(key), int):
                    return int(result[key])
        return default

    @staticmethod
    def _safe_float(value: Any, default: float | None = 0.0) -> float | None:
        try:
            if value is None:
                return default
            return float(value)
        except Exception:
            return default

    def _parse_match_expressions(self, match_expressions: list[Any]) -> dict[str, Any]:
        query_text = ""
        query_vector: list[float] | None = None
        dense_similarity: float | None = None
        text_weight = 0.5
        vector_weight = 0.5

        for expr in match_expressions:
            if isinstance(expr, MatchTextExpr):
                text = getattr(expr, "matching_text", "")
                if isinstance(text, str) and text.strip():
                    query_text = text.strip()
            elif isinstance(expr, MatchDenseExpr):
                embedding_data = getattr(expr, "embedding_data", None)
                if embedding_data is not None:
                    try:
                        query_vector = list(embedding_data)
                    except Exception:
                        query_vector = None
                similarity = getattr(expr, "extra_options", {}).get("similarity")
                if similarity is not None:
                    dense_similarity = self._safe_float(similarity, None)
            elif isinstance(expr, FusionExpr):
                if expr.method == "weighted_sum":
                    weights = expr.fusion_params.get("weights", "")
                    if isinstance(weights, str):
                        tokens = [t.strip() for t in weights.split(",") if t.strip()]
                        if len(tokens) >= 2:
                            parsed_text_weight = self._safe_float(tokens[0], 0.5) or 0.5
                            parsed_vector_weight = self._safe_float(tokens[1], 0.5) or 0.5
                            total = parsed_text_weight + parsed_vector_weight
                            if total > 0:
                                text_weight = parsed_text_weight / total
                                vector_weight = parsed_vector_weight / total

        return {
            "query_text": query_text,
            "query_vector": query_vector,
            "dense_similarity": dense_similarity,
            "text_weight": text_weight,
            "vector_weight": vector_weight,
        }

    def _local_rank_rows(
        self,
        rows: list[dict],
        query_text: str,
        has_vector_query: bool,
        text_weight: float,
        vector_weight: float,
    ) -> list[dict]:
        if not rows:
            return rows

        normalized_query = (query_text or "").strip().lower()
        for row in rows:
            text_score = self._safe_float(row.get("_text_score"), None)
            vector_score = self._safe_float(row.get("_vector_score"), None)
            base_score = self._safe_float(row.get("_score"), 0.0) or 0.0

            if text_score is None and normalized_query:
                content = str(row.get("content", "")).lower()
                text_score = 1.0 if normalized_query and normalized_query in content else 0.0
            elif text_score is None:
                text_score = 0.0

            if vector_score is None:
                vector_score = base_score if has_vector_query else 0.0

            row["_rank_score"] = (
                text_weight * (text_score or 0.0)
                + vector_weight * (vector_score or 0.0)
                + (0.001 * base_score)
            )

        rows.sort(key=lambda r: self._safe_float(r.get("_rank_score"), 0.0) or 0.0, reverse=True)
        return rows

    # --- Connector-compatible operations ---
    def index_exist(self, index_name: str, memory_id: str | None = None) -> bool:
        # PowerMem backend does not require explicit index management.
        return True

    def create_idx(self, index_name: str, memory_id: str, vector_size: int) -> bool:
        # Keep noop for compatibility with existing flow.
        return True

    def delete_idx(self, index_name: str, memory_id: str | None = None) -> bool:
        return True

    def search(self, **kwargs):
        try:
            condition = kwargs.get("condition", {}) or {}
            memory_ids: list[str] = kwargs.get("memory_ids", []) or condition.get("memory_id", [])
            if not memory_ids:
                return _BackendSearchResult([]), 0

            match_expressions = kwargs.get("match_expressions", []) or []
            parsed_match = self._parse_match_expressions(match_expressions)
            query_text: str = parsed_match["query_text"]
            query_vector: list[float] | None = parsed_match["query_vector"]
            dense_similarity: float | None = parsed_match["dense_similarity"]
            text_weight: float = parsed_match["text_weight"]
            vector_weight: float = parsed_match["vector_weight"]

            limit = int(kwargs.get("limit", 50) or 50)
            offset = int(kwargs.get("offset", 0) or 0)
            top_k = max(limit + offset, limit, 1)

            user_id = condition.get("user_id")
            agent_id = condition.get("agent_id")
            session_id = condition.get("session_id")
            status = condition.get("status")

            rows: list[dict] = []
            for memory_id in memory_ids:
                payload = {
                    "query": query_text if query_text else "*",
                    "memory_id": memory_id,
                    "top_k": top_k,
                }
                if query_vector:
                    payload["query_vector"] = query_vector
                if dense_similarity is not None:
                    payload["similarity_threshold"] = dense_similarity
                payload["weights"] = {"text": text_weight, "vector": vector_weight}
                if user_id:
                    payload["user_id"] = user_id
                if agent_id:
                    payload["agent_id"] = agent_id
                if session_id:
                    payload["session_id"] = session_id
                if status is not None:
                    payload["status"] = bool(status)

                res = self._call_method(
                    "search",
                    [
                        payload,
                        {
                            "query": payload["query"],
                            "top_k": top_k,
                            "filters": payload,
                            "vector": query_vector,
                            "text_weight": text_weight,
                            "vector_weight": vector_weight,
                        },
                    ],
                )
                items = self._extract_items(res)
                for i in items:
                    rows.append(self._from_powermem_message(i, fallback_memory_id=memory_id))

            # Local post filtering for robust compatibility.
            if kwargs.get("hide_forgotten", True):
                rows = [r for r in rows if not r.get("forget_at")]

            if match_expressions:
                rows = self._local_rank_rows(
                    rows=rows,
                    query_text=query_text,
                    has_vector_query=bool(query_vector),
                    text_weight=text_weight,
                    vector_weight=vector_weight,
                )

            select_fields = kwargs.get("select_fields", []) or []
            if select_fields:
                slim_rows = []
                for r in rows:
                    slim = {k: r.get(k) for k in set(select_fields + ["id"])}
                    if "id" not in slim or not slim["id"]:
                        slim["id"] = f"{r.get('memory_id', '')}_{r.get('message_id', '')}"
                    slim_rows.append(slim)
                rows = slim_rows

            sliced_rows = rows[offset:offset + limit] if limit > 0 else rows[offset:]
            return _BackendSearchResult(sliced_rows), len(rows)
        except Exception as e:
            self._logger.warning("PowerMem search failed, fallback to default backend: %s", e)
            return self._fallback.search(**kwargs)

    def insert(self, documents: list[dict], index_name: str, memory_id: str | None = None) -> list[str]:
        if not documents:
            return []
        try:
            fail_cases: list[str] = []
            for doc in documents:
                message = self._to_powermem_message(doc)
                payload = {
                    "messages": [message],
                    "memory_id": message.get("memory_id", memory_id),
                    "user_id": message.get("user_id", ""),
                    "agent_id": message.get("agent_id", ""),
                    "session_id": message.get("session_id", ""),
                }
                self._call_method("add", [payload, {"message": message, "memory_id": payload["memory_id"]}])
            return fail_cases
        except Exception as e:
            self._logger.warning("PowerMem insert failed, fallback to default backend: %s", e)
            return self._fallback.insert(documents, index_name, memory_id)

    def update(self, condition: dict, new_value: dict, index_name: str, memory_id: str) -> bool:
        try:
            message_id = condition.get("message_id")
            if message_id is None:
                raise ValueError("PowerMem update currently requires message_id in condition")
            payload = {
                "memory_id": memory_id,
                "message_id": str(message_id),
                "updates": new_value,
            }
            self._call_method("update", [payload, {"id": str(message_id), "memory_id": memory_id, "data": new_value}])
            return True
        except Exception as e:
            self._logger.warning("PowerMem update failed, fallback to default backend: %s", e)
            return self._fallback.update(condition, new_value, index_name, memory_id)

    def delete(self, condition: dict, index_name: str, memory_id: str) -> int:
        try:
            deleted = 0
            message_ids = condition.get("message_id")
            if message_ids is None:
                # Fallback for broad delete semantics.
                return self._fallback.delete(condition, index_name, memory_id)
            if not isinstance(message_ids, list):
                message_ids = [message_ids]
            for message_id in message_ids:
                payload = {
                    "memory_id": memory_id,
                    "message_id": str(message_id),
                }
                self._call_method("delete", [payload, {"id": str(message_id), "memory_id": memory_id}])
                deleted += 1
            return deleted
        except Exception as e:
            self._logger.warning("PowerMem delete failed, fallback to default backend: %s", e)
            return self._fallback.delete(condition, index_name, memory_id)

    def get(self, doc_id: str, index_name: str, memory_ids: list[str]) -> dict | None:
        try:
            memory_id, message_id = self._extract_memory_id_from_doc_id(doc_id)
            if not memory_id and memory_ids:
                memory_id = memory_ids[0]
            payload = {
                "memory_id": memory_id,
                "message_id": str(message_id),
                "top_k": 1,
            }
            res = self._call_method("search", [payload, {"query": str(message_id), "memory_id": memory_id, "top_k": 1}])
            items = self._extract_items(res)
            if not items:
                return None
            msg = self._from_powermem_message(items[0], fallback_memory_id=memory_id)
            if not msg.get("id"):
                msg["id"] = doc_id
            return msg
        except Exception as e:
            self._logger.warning("PowerMem get failed, fallback to default backend: %s", e)
            return self._fallback.get(doc_id, index_name, memory_ids)

    def get_fields(self, res, fields: list[str]) -> dict[str, dict]:
        try:
            if isinstance(res, tuple):
                res = res[0]
            rows = getattr(res, "messages", None)
            if rows is None:
                rows = self._extract_items(res)
            field_map: dict[str, dict] = {}
            for row in rows or []:
                msg = self._from_powermem_message(row) if "message_type" not in row else row
                doc_id = msg.get("id") or f"{msg.get('memory_id', '')}_{msg.get('message_id', '')}"
                field_map[doc_id] = {k: msg.get(k) for k in fields}
            return field_map
        except Exception as e:
            self._logger.warning("PowerMem get_fields failed, fallback to default backend: %s", e)
            return self._fallback.get_fields(res, fields)

    def get_forgotten_messages(self, select_fields: list[str], index_name: str, memory_id: str, limit: int = 512):
        # Not all PowerMem versions expose direct forgotten-message querying;
        # keep compatibility by using fallback connector behavior.
        return self._fallback.get_forgotten_messages(select_fields, index_name, memory_id, limit)

    def get_missing_field_message(self, select_fields: list[str], index_name: str, memory_id: str, field_name: str, limit: int = 512):
        return self._fallback.get_missing_field_message(select_fields, index_name, memory_id, field_name, limit)

    # --- Enhanced capabilities ---
    def supports_graph(self) -> bool:
        return self._features["graph"]

    def supports_user_profile(self) -> bool:
        return self._features["user_profile"]

    def supports_ebbinghaus(self) -> bool:
        return self._features["ebbinghaus"]

    def supports_rerank(self) -> bool:
        return self._features["rerank"]

    def supports_sparse_vector(self) -> bool:
        return self._features["sparse_vector"]

    def supports_intelligent_merge(self) -> bool:
        return self._features["intelligent_merge"]

    def graph_search(self, memory_ids: list[str], query: str, top_n: int = 5, **kwargs) -> list[dict]:
        """
        Optional graph search via PowerMem SDK.
        This method is best-effort and never breaks existing memory flow.
        """
        if not memory_ids or not query:
            return []
        if not self.supports_graph():
            return []
        payload = {
            "query": query,
            "memory_ids": memory_ids,
            "top_k": top_n,
            "mode": "graph",
        }
        payload.update(kwargs or {})

        try:
            res = self._call_method(
                "search",
                [
                    payload,
                    {"query": query, "top_k": top_n, "memory_ids": memory_ids, "search_type": "graph"},
                ],
            )
        except Exception as e:
            self._logger.warning("PowerMem graph search failed: %s", e)
            return []

        return self._extract_items(res)
