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

from dataclasses import dataclass
from typing import Any, Callable

from common.metadata_utils import apply_meta_data_filter
from common.temporal_utils import ResolvedTemporalPolicy, TemporalRankPlan, TemporalRetrievalPolicy


@dataclass
class TemporalRetrievalContext:
    """Resolved metadata scope and freshness policy for a retrieval call.

    ``doc_ids`` is the result of applying user metadata filters plus temporal
    filter conditions through the existing metadata filtering path. The rank
    policy is passed to the retriever only when the query should be freshness
    aware.
    """

    doc_ids: list[str] | None
    temporal_rank_policy: TemporalRankPlan | None
    resolved_policy: ResolvedTemporalPolicy
    temporal_skipped_reason: str | None = None


async def resolve_temporal_retrieval_context(
    *,
    raw_query: str,
    refined_query: str,
    retrieval_query: str,
    meta_data_filter: dict | None,
    temporal_retrieval: dict | None,
    kb_ids: list[str],
    chat_mdl: Any = None,
    base_doc_ids: list[str] | None = None,
    manual_value_resolver: Callable[[dict], dict] | None = None,
    metas_loader: Callable[[], dict] | None = None,
    metadata_filter_func: Callable[..., Any] = apply_meta_data_filter,
) -> TemporalRetrievalContext:
    """Resolve temporal intent and merge temporal filters into metadata filtering.

    Returns scoped ``doc_ids`` plus an optional ``TemporalRankPlan`` for retrieval
    reranking. When temporal retrieval is disabled or skipped, ``doc_ids`` still
    reflect user metadata filters and ``base_doc_ids`` scoping semantics.
    """
    import logging

    del retrieval_query
    resolved = TemporalRetrievalPolicy.resolve(raw_query, refined_query, temporal_retrieval, kb_ids)
    extra_conditions = resolved.filter_plan.conditions if resolved.filter_plan else []
    logging.debug(
        "Temporal retrieval context: intent=%s strategy=%s skipped=%s extra_conditions=%d base_doc_ids=%s",
        resolved.intent,
        resolved.strategy,
        resolved.skipped_reason,
        len(extra_conditions),
        "none" if base_doc_ids is None else len(base_doc_ids),
    )

    doc_ids = await metadata_filter_func(
        meta_data_filter or {},
        None,
        refined_query or raw_query,
        chat_mdl,
        base_doc_ids,
        manual_value_resolver=manual_value_resolver,
        kb_ids=kb_ids,
        metas_loader=metas_loader,
        extra_conditions=extra_conditions,
    )
    temporal_field = temporal_retrieval.get("temporal_field") if isinstance(temporal_retrieval, dict) else None
    logging.info(
        "Temporal retrieval context resolved: intent=%s strategy=%s skipped_reason=%s temporal_field=%s rank_enabled=%s extra_conditions=%d base_doc_count=%s output_doc_count=%s kb_count=%d",
        resolved.intent,
        resolved.strategy,
        resolved.skipped_reason,
        temporal_field,
        bool(resolved.rank_plan),
        len(extra_conditions),
        "none" if base_doc_ids is None else len(base_doc_ids),
        "none" if doc_ids is None else len(doc_ids),
        len(kb_ids),
    )

    return TemporalRetrievalContext(
        doc_ids=doc_ids,
        temporal_rank_policy=resolved.rank_plan,
        resolved_policy=resolved,
        temporal_skipped_reason=resolved.skipped_reason,
    )


def merge_temporal_reference_fields(
    include_metadata: bool,
    metadata_fields: set[str] | None,
    temporal_retrieval: dict | None,
) -> set[str] | None:
    """Include the active temporal field in reference metadata when requested.

    When callers request all metadata (``metadata_fields is None``), the helper
    preserves that behavior. Otherwise it adds the selected temporal field so
    citations can show the date used for filtering or freshness reranking.
    """
    if not include_metadata or not isinstance(temporal_retrieval, dict) or not temporal_retrieval.get("enabled"):
        return metadata_fields
    temporal_field = temporal_retrieval.get("temporal_field")
    if not isinstance(temporal_field, str) or not temporal_field.strip():
        return metadata_fields
    if metadata_fields is None:
        return None
    merged = set(metadata_fields)
    merged.add(temporal_field.strip())
    return merged
