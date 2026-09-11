#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#

from __future__ import annotations

import asyncio
import hashlib
import json
import math
import os
from datetime import UTC, datetime
from typing import Any, Callable, Protocol
from urllib.parse import quote

from api.apps.business_documents.errors import BusinessDocumentError
from peewee import IntegrityError

from api.db.db_models import BusinessDocument, BusinessDocumentEvidenceSnapshot, BusinessDocumentJob
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp


MAX_DATASETS = 20
MAX_QUERY_CHARS = 4_000
MAX_CHUNKS = 12
MAX_CHUNK_CHARS = 4_000
MAX_TOTAL_CHARS = 24_000


def related_file_search_enabled() -> bool:
    """Return whether business-document jobs may retrieve dataset evidence."""

    return os.getenv("BUSINESS_DOCUMENT_RELATED_FILE_SEARCH_ENABLED", "true").strip().lower() in {"1", "true", "yes", "on"}


def _sha256_bytes(value: bytes) -> str:
    return f"sha256:{hashlib.sha256(value).hexdigest()}"


def _stable_hash(value: object) -> str:
    encoded = json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True).encode("utf-8")
    return _sha256_bytes(encoded)


def _default_accessible(dataset_id: str, actor_id: str) -> bool:
    from api.db.services.knowledgebase_service import KnowledgebaseService

    return bool(KnowledgebaseService.accessible(dataset_id, actor_id))


def ensure_dataset_access(
    actor_id: str,
    dataset_ids: list[str],
    *,
    access_checker: Callable[[str, str], bool] | None = None,
) -> None:
    """Check all datasets with a single non-enumerating failure contract."""

    if not dataset_ids:
        return
    checker = access_checker or _default_accessible
    try:
        allowed = all(checker(dataset_id, actor_id) for dataset_id in dataset_ids)
    except Exception as exc:
        raise BusinessDocumentError("DATASET_ACCESS_UNAVAILABLE", "Dataset access could not be verified", 503) from exc
    if not allowed:
        raise BusinessDocumentError("DATASET_NOT_FOUND", "One or more datasets are unavailable", 404)


def _default_embedding_names(dataset_ids: list[str]) -> list[str]:
    from api.db.joint_services.tenant_model_service import split_model_name
    from api.db.services.knowledgebase_service import KnowledgebaseService

    datasets = KnowledgebaseService.get_by_ids(dataset_ids)
    if len(datasets) != len(dataset_ids) or {dataset.id for dataset in datasets} != set(dataset_ids):
        raise BusinessDocumentError("DATASET_NOT_FOUND", "One or more datasets are unavailable", 404)
    return [split_model_name(dataset.embd_id)[0] for dataset in datasets]


def ensure_dataset_embedding_compatibility(
    dataset_ids: list[str],
    *,
    embedding_names_loader: Callable[[list[str]], list[str]] | None = None,
) -> None:
    """Reject a mixed embedding space before creating the document aggregate."""

    if len(dataset_ids) <= 1:
        return
    try:
        embedding_names = (embedding_names_loader or _default_embedding_names)(dataset_ids)
    except BusinessDocumentError:
        raise
    except Exception as exc:
        raise BusinessDocumentError("DATASET_PREFLIGHT_UNAVAILABLE", "Dataset compatibility could not be verified", 503) from exc
    if len(embedding_names) != len(dataset_ids):
        raise BusinessDocumentError("DATASET_NOT_FOUND", "One or more datasets are unavailable", 404)
    if len(set(embedding_names)) != 1:
        raise BusinessDocumentError(
            "DATASET_EMBEDDING_INCOMPATIBLE",
            "Selected datasets use incompatible embedding models",
            422,
        )


class DatasetSearchAdapter(Protocol):
    def search(self, actor_id: str, request: dict[str, Any]) -> tuple[bool, dict[str, Any] | str]: ...


class RAGFlowDatasetSearchAdapter:
    """Synchronous worker-thread adapter over RAGFlow's async dataset search."""

    def search(self, actor_id: str, request: dict[str, Any]) -> tuple[bool, dict[str, Any] | str]:
        from api.apps.services.dataset_api_service import search_datasets

        return asyncio.run(search_datasets(actor_id, request))


class BusinessDocumentEvidence:
    def __init__(
        self,
        search_adapter: DatasetSearchAdapter | None = None,
        *,
        access_checker: Callable[[str, str], bool] | None = None,
    ):
        self._search_adapter = search_adapter or RAGFlowDatasetSearchAdapter()
        self._access_checker = access_checker

    def retrieve(self, job: BusinessDocumentJob) -> dict[str, Any]:
        payload = job.payload if isinstance(job.payload, dict) else {}
        dataset_ids = payload.get("dataset_ids", [])
        if not isinstance(dataset_ids, list) or len(dataset_ids) > MAX_DATASETS:
            raise BusinessDocumentError("INVALID_EVIDENCE_DATASETS", "Job contains an invalid dataset selection", 422)
        if not dataset_ids:
            return self._empty_snapshot(job)
        actor_id = payload.get("owner_id")
        if not isinstance(actor_id, str) or not actor_id:
            raise BusinessDocumentError("INVALID_JOB_SNAPSHOT", "Evidence job is missing its owner", 422)
        ensure_dataset_access(actor_id, dataset_ids, access_checker=self._access_checker)
        existing = BusinessDocumentEvidenceSnapshot.get_or_none(BusinessDocumentEvidenceSnapshot.job_id == job.id)
        if existing is not None:
            return self._validate_persisted(job, existing)
        query = self._query(payload)
        request = {
            "dataset_ids": dataset_ids,
            "question": query,
            "page": 1,
            "size": MAX_CHUNKS,
            "top_k": MAX_CHUNKS,
            "similarity_threshold": 0.0,
            "vector_similarity_weight": 0.3,
            "use_kg": False,
        }
        try:
            success, raw = self._search_adapter.search(actor_id, request)
        except BusinessDocumentError:
            raise
        except Exception as exc:
            raise BusinessDocumentError("EVIDENCE_RETRIEVAL_FAILED", "Evidence retrieval failed", 503) from exc
        if not success or not isinstance(raw, dict) or not isinstance(raw.get("chunks", []), list):
            raise BusinessDocumentError("EVIDENCE_RETRIEVAL_FAILED", "Evidence retrieval failed", 503)
        snapshot = self._normalize(job, query, dataset_ids, raw["chunks"])
        return self._pin(job, snapshot)

    @staticmethod
    def _query(payload: dict[str, Any]) -> str:
        title = payload.get("title") if isinstance(payload.get("title"), str) else ""
        idea = payload.get("idea") if isinstance(payload.get("idea"), str) else ""
        revision = payload.get("current_revision")
        revision_text = revision.get("body_markdown", "") if isinstance(revision, dict) else ""
        protocol = payload.get("protocol")
        protocol_text = json.dumps(protocol, ensure_ascii=False, separators=(",", ":")) if isinstance(protocol, dict) else ""
        task_type = payload.get("task_type")
        if task_type in {"ASSESS_REVIEW", "PLAN_CHANGES"}:
            priority_protocol = ""
            secondary_protocol = protocol_text
            if isinstance(protocol, dict):
                priority_protocol = json.dumps(
                    {
                        "comments": protocol.get("comments", []),
                        "answered_questions": [
                            {
                                "question_id": question.get("question_id"),
                                "semantic_tag": question.get("semantic_tag"),
                                "target_section_id": question.get("target_section_id"),
                                "answer": question.get("answer"),
                            }
                            for question in protocol.get("questions", [])
                            if isinstance(question, dict) and question.get("answer") is not None
                        ],
                    },
                    ensure_ascii=False,
                    separators=(",", ":"),
                )
            components = (
                ("current_revision", BusinessDocumentEvidence._head_tail(revision_text, 2_100)),
                ("protocol_comments_answers", BusinessDocumentEvidence._head_tail(priority_protocol, 850)),
                ("protocol_other", BusinessDocumentEvidence._head_tail(secondary_protocol, 300)),
                ("title", title[:200]),
                ("idea", BusinessDocumentEvidence._head_tail(idea, 250)),
            )
        elif task_type == "GENERATE_DRAFT":
            components = (("title", title[:400]), ("idea", BusinessDocumentEvidence._head_tail(idea, 2_500)), ("protocol", BusinessDocumentEvidence._head_tail(protocol_text, 900)))
        else:
            components = (("title", title[:450]), ("idea", BusinessDocumentEvidence._head_tail(idea, 3_400)))
        parts = [f"[{name}]\n{value}" for name, value in components if value.strip()]
        query = "\n\n".join(parts)
        return query[:MAX_QUERY_CHARS]

    @staticmethod
    def _head_tail(value: str, budget: int) -> str:
        if len(value) <= budget:
            return value
        marker = "\n…\n"
        available = budget - len(marker)
        head = available // 2
        return value[:head] + marker + value[-(available - head) :]

    @classmethod
    def _normalize(
        cls,
        job: BusinessDocumentJob,
        query: str,
        dataset_ids: list[str],
        raw_chunks: list[object],
    ) -> dict[str, Any]:
        chunks: list[dict[str, Any]] = []
        total_chars = 0
        seen_refs: set[str] = set()
        for raw in raw_chunks:
            if len(chunks) >= MAX_CHUNKS or total_chars >= MAX_TOTAL_CHARS:
                break
            if not isinstance(raw, dict):
                raise BusinessDocumentError("INVALID_EVIDENCE_RESULT", "Evidence retrieval returned an invalid chunk", 503)
            dataset_id = raw.get("dataset_id") or raw.get("kb_id")
            if dataset_id is None and len(dataset_ids) == 1:
                dataset_id = dataset_ids[0]
            document_id = raw.get("document_id") or raw.get("doc_id")
            chunk_id = raw.get("chunk_id") or raw.get("id")
            content = raw.get("content") if "content" in raw else raw.get("content_with_weight")
            if not all(isinstance(value, str) and value for value in (dataset_id, document_id, chunk_id)) or not isinstance(content, str):
                raise BusinessDocumentError("INVALID_EVIDENCE_RESULT", "Evidence retrieval returned incomplete provenance", 503)
            if dataset_id not in dataset_ids:
                raise BusinessDocumentError("EVIDENCE_SOURCE_MISMATCH", "Evidence source is outside the selected datasets", 503)
            source_ref = f"ragflow://dataset/{quote(dataset_id, safe='')}/document/{quote(document_id, safe='')}/chunk/{quote(chunk_id, safe='')}"
            if source_ref in seen_refs:
                continue
            remaining = MAX_TOTAL_CHARS - total_chars
            bounded_content = content[: min(MAX_CHUNK_CHARS, remaining)]
            if not bounded_content:
                continue
            seen_refs.add(source_ref)
            chunk = {
                "source_ref": source_ref,
                "dataset_id": dataset_id,
                "document_id": document_id,
                "chunk_id": chunk_id,
                "score": cls._score(raw.get("similarity")),
                "vector_score": cls._score(raw.get("vector_similarity")),
                "term_score": cls._score(raw.get("term_similarity")),
                "content": bounded_content,
                "content_hash": _sha256_bytes(bounded_content.encode("utf-8")),
            }
            chunks.append(chunk)
            total_chars += len(bounded_content)
        material = {
            "schema_version": "1",
            "job_id": job.id,
            "dataset_ids": dataset_ids,
            "query_hash": _sha256_bytes(query.encode("utf-8")),
            "chunks": chunks,
            "total_chars": total_chars,
        }
        return {
            **material,
            "evidence_hash": _stable_hash(material),
            "retrieved_at": datetime.now(UTC).isoformat(),
        }

    @classmethod
    def _empty_snapshot(cls, job: BusinessDocumentJob) -> dict[str, Any]:
        return cls._normalize(job, "", [], [])

    @classmethod
    def _pin(cls, job: BusinessDocumentJob, snapshot: dict[str, Any]) -> dict[str, Any]:
        database = BusinessDocumentJob._meta.database
        with database.atomic():
            document_query = BusinessDocument.select(BusinessDocument.id).where((BusinessDocument.id == job.document_id) & (BusinessDocument.tenant_id == job.tenant_id))
            if getattr(database, "for_update", False):
                document_query = document_query.for_update()
            document = document_query.first()
            job_query = BusinessDocumentJob.select().where((BusinessDocumentJob.id == job.id) & (BusinessDocumentJob.document_id == job.document_id) & (BusinessDocumentJob.tenant_id == job.tenant_id))
            if getattr(database, "for_update", False):
                job_query = job_query.for_update()
            current = job_query.first() if document is not None else None
            if (
                current is None
                or current.status != "RUNNING"
                or current.lease_owner != job.lease_owner
                or current.lease_token != job.lease_token
                or current.lease_expires_at is None
                or current.lease_expires_at <= current_timestamp()
            ):
                raise BusinessDocumentError("JOB_LEASE_LOST", "Worker lost its lease before evidence could be pinned", 409)
            now_ms = current_timestamp()
            now = datetime.now()
            try:
                # Isolate a concurrent unique-key loser in a savepoint so the
                # outer transaction remains usable on PostgreSQL/MySQL.
                with database.atomic():
                    BusinessDocumentEvidenceSnapshot.create(
                        id=get_uuid(),
                        job_id=current.id,
                        document_id=document.id,
                        tenant_id=current.tenant_id,
                        dataset_ids=list(snapshot["dataset_ids"]),
                        snapshot=snapshot,
                        evidence_hash=snapshot["evidence_hash"],
                        create_time=now_ms,
                        create_date=now,
                        update_time=now_ms,
                        update_date=now,
                    )
            except IntegrityError:
                existing = BusinessDocumentEvidenceSnapshot.get_or_none(BusinessDocumentEvidenceSnapshot.job_id == job.id)
                if existing is None:
                    raise
                return cls._validate_persisted(job, existing)
        return snapshot

    @classmethod
    def _validate_persisted(cls, job: BusinessDocumentJob, row: BusinessDocumentEvidenceSnapshot) -> dict[str, Any]:
        snapshot = row.snapshot
        expected_keys = {
            "schema_version",
            "job_id",
            "dataset_ids",
            "query_hash",
            "chunks",
            "total_chars",
            "evidence_hash",
            "retrieved_at",
        }
        if (
            row.tenant_id != job.tenant_id
            or row.document_id != job.document_id
            or row.dataset_ids != job.payload.get("dataset_ids")
            or not isinstance(snapshot, dict)
            or set(snapshot) != expected_keys
            or snapshot.get("job_id") != job.id
            or snapshot.get("dataset_ids") != row.dataset_ids
            or snapshot.get("evidence_hash") != row.evidence_hash
            or not isinstance(snapshot.get("chunks"), list)
        ):
            raise BusinessDocumentError("EVIDENCE_SNAPSHOT_CORRUPT", "Pinned evidence snapshot failed validation", 500)
        material = {key: snapshot[key] for key in expected_keys - {"evidence_hash", "retrieved_at"}}
        if _stable_hash(material) != row.evidence_hash:
            raise BusinessDocumentError("EVIDENCE_SNAPSHOT_CORRUPT", "Pinned evidence snapshot hash does not match", 500)
        for chunk in snapshot["chunks"]:
            if not isinstance(chunk, dict) or not isinstance(chunk.get("content"), str) or chunk.get("content_hash") != _sha256_bytes(chunk["content"].encode("utf-8")):
                raise BusinessDocumentError("EVIDENCE_SNAPSHOT_CORRUPT", "Pinned evidence content hash does not match", 500)
        return snapshot

    @staticmethod
    def _score(value: object) -> float | None:
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            return None
        score = float(value)
        return score if math.isfinite(score) else None

    @staticmethod
    def audit(snapshot: dict[str, Any], attempt: int) -> dict[str, Any]:
        return {
            "retrieval": {
                "attempt": attempt,
                "retrieved_at": snapshot["retrieved_at"],
                "dataset_ids": list(snapshot["dataset_ids"]),
                "query_hash": snapshot["query_hash"],
                "evidence_hash": snapshot["evidence_hash"],
                "source_refs": [chunk["source_ref"] for chunk in snapshot["chunks"]],
                "chunk_count": len(snapshot["chunks"]),
                "total_chars": snapshot["total_chars"],
            }
        }
