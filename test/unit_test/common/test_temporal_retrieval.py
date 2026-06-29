import logging
import sys
import types
from datetime import UTC, datetime
from unittest.mock import patch

import pytest

from common.metadata_utils import apply_meta_data_filter
from common.temporal_retrieval import resolve_temporal_retrieval_context
from common.temporal_utils import (
    TemporalRetrievalPolicy,
    extract_date_window,
    filter_visible_metadata_dict,
    freshness_score,
    parse_temporal_value,
    profile_temporal_field,
    temporal_sort_score,
)
from common.temporal_validation import (
    merge_temporal_retrieval_config as merge_config,
    validate_half_life_days,
    validate_temporal_retrieval_config,
)


@pytest.mark.p2
def test_parse_temporal_value_supported_formats():
    assert parse_temporal_value("2026-06-15").date_norm == "2026-06-15"
    assert parse_temporal_value("2026-06-15T12:00:00Z").source_format == "iso_datetime"
    assert parse_temporal_value("2026").source_format == "year"
    assert parse_temporal_value("1781481600").source_format == "unix_seconds"
    assert parse_temporal_value("1781481600000").source_format == "unix_millis"


@pytest.mark.p2
def test_profile_temporal_field_and_hide_internal_metadata():
    profile = profile_temporal_field(
        {
            "doc-1": {"post_date": "2026-06-15", "_temporal_date_norm": "2026-06-15"},
            "doc-2": {"post_date": "bad"},
            "doc-3": {},
        },
        "post_date",
    )

    assert profile.detected_format == "date"
    assert profile.parsed_documents == 1
    assert profile.oldest_date == "2026-06-15"
    assert filter_visible_metadata_dict({"post_date": "2026-06-15", "_temporal_ts_norm": 1}) == {
        "post_date": "2026-06-15"
    }


@pytest.mark.p2
def test_temporal_policy_arabic_latest_uses_digit_normalization():
    resolved = TemporalRetrievalPolicy.resolve(
        "آخر أخبار ٢٠٢٦ عن الاقتصاد",
        "آخر أخبار 2026 عن الاقتصاد",
        {"enabled": True, "mode": "auto", "temporal_field": "post_date", "supports_hard_filter": True},
        ["kb-1"],
    )

    assert resolved.intent == "date_range"
    assert resolved.date_window.start_date == "2026-01-01"
    assert resolved.filter_plan.conditions


@pytest.mark.p1
@pytest.mark.parametrize(
    ("value", "expected_error"),
    [
        (7, None),
        (0, "`temporal_retrieval.half_life_days` should be a finite number greater than 0."),
        (-1, "`temporal_retrieval.half_life_days` should be a finite number greater than 0."),
        (float("nan"), "`temporal_retrieval.half_life_days` should be a finite number greater than 0."),
        (float("inf"), "`temporal_retrieval.half_life_days` should be a finite number greater than 0."),
        ("14", "`temporal_retrieval.half_life_days` should be a number."),
        (True, "`temporal_retrieval.half_life_days` should be a number."),
    ],
)
def test_validate_half_life_days(value, expected_error):
    parsed, err = validate_half_life_days(value)
    if expected_error is None:
        assert parsed == float(value)
        assert err is None
    else:
        assert parsed is None
        assert err == expected_error


@pytest.mark.p1
def test_validate_temporal_retrieval_config_rejects_bad_payloads():
    assert validate_temporal_retrieval_config("bad") == "`temporal_retrieval` should be an object."
    assert (
        validate_temporal_retrieval_config({"enabled": True})
        == "`temporal_retrieval.temporal_field` is required when temporal retrieval is enabled."
    )
    assert (
        validate_temporal_retrieval_config({"enabled": True, "temporal_field": "post_date", "half_life_days": 0})
        == "`temporal_retrieval.half_life_days` should be a finite number greater than 0."
    )
    assert validate_temporal_retrieval_config({"future_date_policy": "bad"}) is not None


@pytest.mark.p1
def test_merge_temporal_retrieval_config_preserves_existing_fields():
    existing = {
        "enabled": True,
        "mode": "auto",
        "temporal_field": "post_date",
        "half_life_days": 30,
    }
    merged = merge_config(existing, {"half_life_days": 7})
    assert merged == {
        "enabled": True,
        "mode": "auto",
        "temporal_field": "post_date",
        "half_life_days": 7,
    }
    assert validate_temporal_retrieval_config(merged) is None


@pytest.mark.p1
def test_merge_temporal_retrieval_config_invalid_partial_fails_cleanly():
    existing = {
        "enabled": True,
        "mode": "auto",
        "temporal_field": "post_date",
        "half_life_days": 30,
    }
    merged = merge_config(existing, {"half_life_days": float("nan")})
    assert (
        validate_temporal_retrieval_config(merged)
        == "`temporal_retrieval.half_life_days` should be a finite number greater than 0."
    )


@pytest.mark.p1
@pytest.mark.asyncio
async def test_apply_meta_data_filter_extra_conditions_scope_base_doc_ids():
    metas = {
        "topic": {"news": ["doc-1", "doc-2"], "sports": ["doc-3"]},
        "post_date": {"2025-01-01": ["doc-1"], "2026-06-15": ["doc-2"], "2026-07-01": ["doc-3"]},
    }

    doc_ids = await apply_meta_data_filter(
        {"method": "manual", "manual": [{"key": "topic", "op": "=", "value": "news"}], "logic": "and"},
        metas,
        base_doc_ids=["doc-1", "doc-2", "doc-3"],
        extra_conditions=[{"key": "post_date", "op": "≥", "value": "2026-01-01"}],
    )

    assert doc_ids == ["doc-2"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_apply_meta_data_filter_base_doc_ids_none_is_unscoped():
    metas = {"topic": {"news": ["doc-1", "doc-2"]}}

    doc_ids = await apply_meta_data_filter(
        {"method": "manual", "manual": [{"key": "topic", "op": "=", "value": "news"}], "logic": "and"},
        metas,
        base_doc_ids=None,
    )

    assert sorted(doc_ids) == ["doc-1", "doc-2"]


@pytest.mark.p1
@pytest.mark.asyncio
async def test_apply_meta_data_filter_base_doc_ids_empty_returns_empty():
    metas = {"topic": {"news": ["doc-1", "doc-2"]}}

    doc_ids = await apply_meta_data_filter(
        {"method": "manual", "manual": [{"key": "topic", "op": "=", "value": "news"}], "logic": "and"},
        metas,
        base_doc_ids=[],
    )

    assert doc_ids == []


@pytest.mark.p1
@pytest.mark.asyncio
async def test_apply_meta_data_filter_no_filters_returns_none_base_doc_ids():
    doc_ids = await apply_meta_data_filter({}, {}, base_doc_ids=None)
    assert doc_ids is None


@pytest.mark.p2
@pytest.mark.parametrize(
    ("query", "start", "end"),
    [
        ("2026-01-10 to 2026-01-20", "2026-01-10", "2026-01-20"),
        ("from 2026-01-10 to 2026-01-20", "2026-01-10", "2026-01-20"),
        ("between 2026-01-20 and 2026-01-10", "2026-01-10", "2026-01-20"),
    ],
)
def test_extract_date_window_exact_ranges_remain_exact(query, start, end):
    window = extract_date_window(query)
    assert window is not None
    assert window.start_date == start
    assert window.end_date == end


@pytest.mark.p2
def test_extract_date_window_year_only_expands_to_year():
    window = extract_date_window("earnings in 2026")
    assert window is not None
    assert window.start_date == "2026-01-01"
    assert window.end_date == "2026-12-31"


@pytest.mark.p2
def test_extract_date_window_malformed_range_returns_none():
    assert extract_date_window("from bad-date to also-bad") is None


@pytest.mark.p2
@pytest.mark.parametrize(
    "query",
    [
        "from 2026 to 2026-01-20",
        "between 2026-01-10 and 2026",
        "from 2026-99-99 to 2026-01-20",
    ],
)
def test_extract_date_window_malformed_mixed_ranges_return_none(query):
    assert extract_date_window(query) is None


@pytest.mark.p1
def test_temporal_policy_invalid_mode_type_skips_without_crashing():
    resolved = TemporalRetrievalPolicy.resolve(
        "latest updates",
        "latest updates",
        {"enabled": True, "mode": ["latest"], "temporal_field": "post_date"},
        ["kb-1"],
    )

    assert resolved.strategy == "baseline"
    assert resolved.skipped_reason == "invalid_mode"


@pytest.mark.p1
def test_temporal_policy_logs_mode_override(caplog):
    caplog.set_level(logging.DEBUG)

    resolved = TemporalRetrievalPolicy.resolve(
        "evergreen background",
        "evergreen background",
        {"enabled": True, "mode": "latest", "temporal_field": "post_date"},
        ["kb-1"],
    )

    assert resolved.intent == "latest"
    assert "Temporal intent detected: intent=evergreen" in caplog.text
    assert "Temporal mode override: mode=latest" in caplog.text
    assert "Temporal policy resolved: mode=latest" in caplog.text


@pytest.mark.p1
def test_temporal_policy_logs_evergreen_skip(caplog):
    caplog.set_level(logging.DEBUG)

    resolved = TemporalRetrievalPolicy.resolve(
        "what is revenue recognition",
        "what is revenue recognition",
        {"enabled": True, "mode": "auto", "temporal_field": "post_date"},
        ["kb-1"],
    )

    assert resolved.intent == "evergreen"
    assert resolved.skipped_reason == "evergreen_query"
    assert "Temporal retrieval skipped: reason=evergreen_query" in caplog.text


@pytest.mark.p1
@pytest.mark.asyncio
async def test_temporal_context_logs_filter_outcome(caplog):
    caplog.set_level(logging.INFO)

    async def metadata_filter_func(*args, **kwargs):
        assert len(kwargs["extra_conditions"]) == 2
        return ["doc-1"]

    context = await resolve_temporal_retrieval_context(
        raw_query="reports from 2026-01-01 to 2026-01-31",
        refined_query="reports from 2026-01-01 to 2026-01-31",
        retrieval_query="reports from 2026-01-01 to 2026-01-31",
        meta_data_filter={},
        temporal_retrieval={
            "enabled": True,
            "mode": "auto",
            "temporal_field": "post_date",
            "supports_hard_filter": True,
        },
        kb_ids=["kb-1"],
        metadata_filter_func=metadata_filter_func,
    )

    assert context.doc_ids == ["doc-1"]
    assert "Temporal retrieval context resolved: intent=date_range" in caplog.text
    assert "strategy=metadata_filter" in caplog.text
    assert "output_doc_count=1" in caplog.text


@pytest.mark.p1
@pytest.mark.asyncio
async def test_apply_meta_data_filter_logs_pushdown_success(caplog):
    caplog.set_level(logging.INFO)

    class FakeDocMetadataService:
        @staticmethod
        def filter_doc_ids_by_meta_pushdown(kb_ids, conditions, logic):
            return ["doc-1"]

    module = types.ModuleType("api.db.services.doc_metadata_service")
    module.DocMetadataService = FakeDocMetadataService

    with patch.dict(sys.modules, {"api.db.services.doc_metadata_service": module}):
        doc_ids = await apply_meta_data_filter(
            {"method": "manual", "manual": [{"key": "topic", "op": "=", "value": "news"}], "logic": "and"},
            metas={"topic": {"news": ["doc-2"]}},
            kb_ids=["kb-1"],
        )

    assert doc_ids == ["doc-1"]
    assert "Metadata filter applied: path=pushdown" in caplog.text
    assert "result_count=1" in caplog.text


@pytest.mark.p1
@pytest.mark.asyncio
async def test_apply_meta_data_filter_logs_pushdown_fallback(caplog):
    caplog.set_level(logging.INFO)

    class FakeDocMetadataService:
        @staticmethod
        def filter_doc_ids_by_meta_pushdown(kb_ids, conditions, logic):
            return None

    module = types.ModuleType("api.db.services.doc_metadata_service")
    module.DocMetadataService = FakeDocMetadataService

    with patch.dict(sys.modules, {"api.db.services.doc_metadata_service": module}):
        doc_ids = await apply_meta_data_filter(
            {"method": "manual", "manual": [{"key": "topic", "op": "=", "value": "news"}], "logic": "and"},
            metas={"topic": {"news": ["doc-2"]}},
            kb_ids=["kb-1"],
        )

    assert doc_ids == ["doc-2"]
    assert "Metadata filter pushdown unavailable" in caplog.text
    assert "fallback_reason=unsupported_or_empty" in caplog.text
    assert "Metadata filter applied: path=in_memory" in caplog.text


@pytest.mark.p1
@pytest.mark.parametrize(
    ("policy", "expected"),
    [
        ("include_without_boost", 0.0),
        ("ignore_future", 0.0),
        ("cap_to_now", 1.0),
        ("allow_future", 1.0),
        ("penalize_future", -0.25),
    ],
)
def test_freshness_score_future_date_policies(policy, expected):
    future = parse_temporal_value("2099-01-01")
    now = datetime(2026, 1, 1, tzinfo=UTC)
    score = freshness_score(future, now, 14.0, future_date_policy=policy)
    if policy == "penalize_future":
        assert score < 0
    else:
        assert score == expected


@pytest.mark.p2
def test_temporal_sort_score_applies_penalty():
    assert temporal_sort_score(1.0, -0.25, 0.15) == pytest.approx(0.75)


@pytest.mark.p1
def test_search_dataset_req_rejects_bad_temporal_retrieval():
    from pydantic import ValidationError

    from api.utils.validation_utils import SearchDatasetReq, SearchDatasetsReq

    with pytest.raises(ValidationError):
        SearchDatasetReq(question="latest news", temporal_retrieval="bad")

    with pytest.raises(ValidationError):
        SearchDatasetReq(
            question="latest news",
            temporal_retrieval={"enabled": True, "half_life_days": 0, "temporal_field": "post_date"},
        )

    with pytest.raises(ValidationError):
        SearchDatasetsReq(dataset_ids=["kb-1"], question="latest news", temporal_retrieval="bad")


@pytest.mark.p2
def test_profile_temporal_field_uses_sampled_documents_count():
    profile = profile_temporal_field(
        {f"doc-{idx}": {"post_date": "2026-01-01"} for idx in range(3)},
        "post_date",
    )
    assert profile.total_documents == 3
    assert profile.parsed_documents == 3
