import pytest

from common.metadata_utils import apply_meta_data_filter
from common.temporal_utils import (
    TemporalRetrievalPolicy,
    filter_visible_metadata_dict,
    parse_temporal_value,
    profile_temporal_field,
)


def test_parse_temporal_value_supported_formats():
    assert parse_temporal_value("2026-06-15").date_norm == "2026-06-15"
    assert parse_temporal_value("2026-06-15T12:00:00Z").source_format == "iso_datetime"
    assert parse_temporal_value("2026").source_format == "year"
    assert parse_temporal_value("1781481600").source_format == "unix_seconds"
    assert parse_temporal_value("1781481600000").source_format == "unix_millis"


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
