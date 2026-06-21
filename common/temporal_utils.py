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
import math
import re
from collections import Counter
from dataclasses import asdict, dataclass
from datetime import UTC, date, datetime, timedelta
from typing import Any, Iterable, Sequence

from common.text_utils import normalize_arabic_digits

INTERNAL_METADATA_PREFIXES = ("_temporal_",)

_DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
_YEAR_RE = re.compile(r"^(?:1[0-9]{3}|2[0-9]{3})$")
_UNIX_SECONDS_RE = re.compile(r"^\d{10}$")
_UNIX_MILLIS_RE = re.compile(r"^\d{13}$")
_EXACT_DATE_RE = re.compile(r"\b(20\d{2}|19\d{2})-(0[1-9]|1[0-2])-([0-2]\d|3[01])\b")
_YEAR_IN_QUERY_RE = re.compile(r"\b(20\d{2}|19\d{2})\b")
_QUARTER_RE = re.compile(r"\b(?:q([1-4])|quarter\s+([1-4]))\s+(20\d{2}|19\d{2})\b", re.I)
_MONTH_RE = re.compile(
    r"\b(january|february|march|april|may|june|july|august|september|october|november|december)\s+(20\d{2}|19\d{2})\b",
    re.I,
)
_FROM_TO_RE = re.compile(
    r"\b(?:from|between)\s+((?:20\d{2}|19\d{2})(?:-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01]))?)\s+(?:to|and|until|-)\s+((?:20\d{2}|19\d{2})(?:-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01]))?)\b",
    re.I,
)
_EXACT_DATE_RANGE_RE = re.compile(
    r"\b((?:20\d{2}|19\d{2})-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01]))\s+(?:to|until|-)\s+((?:20\d{2}|19\d{2})-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01]))\b",
    re.I,
)
_RANGE_LIKE_RE = re.compile(
    r"\b(?:from|between)\s+(?:20\d{2}|19\d{2})(?:-\d{2}-\d{2})?\s+(?:to|and|until|-)\s+(?:20\d{2}|19\d{2})(?:-\d{2}-\d{2})?\b",
    re.I,
)

_MONTH_NUM = {
    "january": 1,
    "february": 2,
    "march": 3,
    "april": 4,
    "may": 5,
    "june": 6,
    "july": 7,
    "august": 8,
    "september": 9,
    "october": 10,
    "november": 11,
    "december": 12,
}

_LATEST_TERMS = (
    "latest",
    "current",
    "recent",
    "today",
    "this week",
    "update",
    "updates",
    "breaking",
    "now",
    "آخر",
    "أحدث",
    "اليوم",
    "الآن",
    "حاليا",
    "حالياً",
    "عاجل",
    "مباشر",
    "تطورات",
    "مستجدات",
    "الجديد",
)
_BALANCED_TERMS = (
    "timeline",
    "history and current",
    "how did it change",
    "compare",
    "comparison",
    "تطور",
    "تسلسل",
    "حتى الآن",
    "كيف تغير",
    "مقارنة",
)
_DATE_TERMS = (
    "yesterday",
    "last month",
    "this year",
    "last year",
    "عام",
    "سنة",
    "منذ",
    "بين",
    "خلال",
    "الأسبوع الماضي",
    "الشهر الماضي",
    "هذا العام",
)


@dataclass(frozen=True)
class ParsedTemporalValue:
    source_format: str
    date_norm: str
    ts_norm: int
    granularity: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True)
class TemporalFieldProfile:
    temporal_field: str
    detected_format: str | None
    parsed_percentage: float
    missing_percentage: float
    invalid_percentage: float
    oldest_date: str | None
    newest_date: str | None
    supports_hard_filter: bool
    supports_freshness_score: bool
    total_documents: int
    parsed_documents: int

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True)
class DateWindow:
    start_date: str
    end_date: str
    source: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    def to_conditions(self, field: str) -> list[dict[str, Any]]:
        return [
            {"key": field, "op": "≥", "value": self.start_date},
            {"key": field, "op": "≤", "value": self.end_date},
        ]


@dataclass(frozen=True)
class TemporalFilterPlan:
    conditions: list[dict[str, Any]]
    skipped_reason: str | None = None


@dataclass(frozen=True)
class TemporalRankPlan:
    enabled: bool
    temporal_field: str
    half_life_days: float = 14.0
    freshness_weight: float = 0.15
    freshness_offset_days: float = 0.0
    shadow_mode: bool = False
    future_date_policy: str = "include_without_boost"


@dataclass(frozen=True)
class ResolvedTemporalPolicy:
    intent: str
    strategy: str
    confidence: float
    source: str
    filter_plan: TemporalFilterPlan
    rank_plan: TemporalRankPlan | None = None
    date_window: DateWindow | None = None
    skipped_reason: str | None = None


def is_internal_metadata_key(key: Any) -> bool:
    return isinstance(key, str) and any(key.startswith(prefix) for prefix in INTERNAL_METADATA_PREFIXES)


def filter_visible_metadata_dict(meta: dict[str, Any] | None) -> dict[str, Any]:
    if not isinstance(meta, dict):
        return {}
    return {k: v for k, v in meta.items() if not is_internal_metadata_key(k)}


def filter_visible_flattened_metadata(metas: dict[str, Any] | None) -> dict[str, Any]:
    if not isinstance(metas, dict):
        return {}
    return {k: v for k, v in metas.items() if not is_internal_metadata_key(k)}


def parse_temporal_value(value: Any) -> ParsedTemporalValue | None:
    if isinstance(value, list):
        for item in value:
            parsed = parse_temporal_value(item)
            if parsed:
                return parsed
        return None

    if isinstance(value, bool) or value is None:
        return None

    if isinstance(value, (int, float)):
        text = str(int(value))
    elif isinstance(value, str):
        text = value.strip()
    else:
        return None

    if not text:
        return None

    if _DATE_RE.match(text):
        try:
            dt = datetime.strptime(text, "%Y-%m-%d").replace(tzinfo=UTC)
        except ValueError:
            return None
        return ParsedTemporalValue("date", dt.date().isoformat(), int(dt.timestamp()), "day")

    if _YEAR_RE.match(text):
        try:
            dt = datetime(int(text), 1, 1, tzinfo=UTC)
        except ValueError:
            return None
        return ParsedTemporalValue("year", dt.date().isoformat(), int(dt.timestamp()), "year")

    if _UNIX_MILLIS_RE.match(text) or _UNIX_SECONDS_RE.match(text):
        try:
            raw = int(text)
            ts = raw / 1000 if _UNIX_MILLIS_RE.match(text) else raw
            dt = datetime.fromtimestamp(ts, tz=UTC)
        except (OverflowError, OSError, ValueError):
            return None
        source_format = "unix_millis" if _UNIX_MILLIS_RE.match(text) else "unix_seconds"
        return ParsedTemporalValue(source_format, dt.date().isoformat(), int(dt.timestamp()), "second")

    try:
        iso_text = text.replace("Z", "+00:00")
        dt = datetime.fromisoformat(iso_text)
    except ValueError:
        return None
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=UTC)
    else:
        dt = dt.astimezone(UTC)
    return ParsedTemporalValue("iso_datetime", dt.date().isoformat(), int(dt.timestamp()), "second")


def profile_temporal_field(metadata_by_doc: dict[str, dict[str, Any]], temporal_field: str) -> TemporalFieldProfile:
    total = len(metadata_by_doc)
    missing = 0
    invalid = 0
    parsed_values: list[ParsedTemporalValue] = []
    formats: Counter[str] = Counter()

    for meta in metadata_by_doc.values():
        if not isinstance(meta, dict) or temporal_field not in meta or meta.get(temporal_field) in (None, ""):
            missing += 1
            continue
        parsed = parse_temporal_value(meta.get(temporal_field))
        if not parsed:
            invalid += 1
            continue
        parsed_values.append(parsed)
        formats[parsed.source_format] += 1

    parsed_count = len(parsed_values)
    denominator = total or 1
    detected_format = formats.most_common(1)[0][0] if formats else None
    dates = sorted(p.date_norm for p in parsed_values)
    parse_rate = parsed_count / denominator
    directly_comparable = detected_format in {"date", "year"} and parse_rate >= 0.8

    return TemporalFieldProfile(
        temporal_field=temporal_field,
        detected_format=detected_format,
        parsed_percentage=round(parse_rate * 100, 2),
        missing_percentage=round((missing / denominator) * 100, 2),
        invalid_percentage=round((invalid / denominator) * 100, 2),
        oldest_date=dates[0] if dates else None,
        newest_date=dates[-1] if dates else None,
        supports_hard_filter=directly_comparable,
        supports_freshness_score=parsed_count > 0,
        total_documents=total,
        parsed_documents=parsed_count,
    )


def extract_date_window(query: str | None, now: datetime | None = None) -> DateWindow | None:
    """Extract an explicit temporal window from a user query.

    Exact ``YYYY-MM-DD`` ranges are preserved without expanding to full years.
    Year-only expressions still resolve to calendar-year boundaries.
    """
    text = normalize_arabic_digits(query or "") or ""
    text_l = text.lower()
    now = now or datetime.now(UTC)
    today = now.date()

    if "today" in text_l or "اليوم" in text:
        return DateWindow(today.isoformat(), today.isoformat(), "today")
    if "yesterday" in text_l:
        day = today - timedelta(days=1)
        return DateWindow(day.isoformat(), day.isoformat(), "yesterday")
    if "this week" in text_l:
        start = today - timedelta(days=today.weekday())
        return DateWindow(start.isoformat(), today.isoformat(), "this_week")
    if "last month" in text_l or "الشهر الماضي" in text:
        first_this_month = today.replace(day=1)
        last_prev_month = first_this_month - timedelta(days=1)
        first_prev_month = last_prev_month.replace(day=1)
        return DateWindow(first_prev_month.isoformat(), last_prev_month.isoformat(), "last_month")

    match = _EXACT_DATE_RANGE_RE.search(text_l)
    if match:
        start_date, end_date = _normalize_range_bounds(match.group(1), match.group(2))
        if start_date and end_date:
            return DateWindow(start_date, end_date, "exact_date_range")

    match = _FROM_TO_RE.search(text_l)
    if match:
        start_date, end_date = _normalize_range_bounds(match.group(1), match.group(2))
        if start_date and end_date:
            source = "exact_date_range" if len(match.group(1)) == 10 and len(match.group(2)) == 10 else "year_range"
            return DateWindow(start_date, end_date, source)

    if _RANGE_LIKE_RE.search(text_l):
        logging.debug("Temporal date range skipped because range tokens are malformed: %s", text_l)
        return None

    match = _EXACT_DATE_RE.search(text_l)
    if match:
        value = match.group(0)
        return DateWindow(value, value, "exact_date")

    match = _QUARTER_RE.search(text_l)
    if match:
        quarter = int(match.group(1) or match.group(2))
        year = int(match.group(3))
        start_month = ((quarter - 1) * 3) + 1
        end_month = start_month + 2
        start = date(year, start_month, 1)
        end = _month_end(year, end_month)
        return DateWindow(start.isoformat(), end.isoformat(), "quarter")

    match = _MONTH_RE.search(text_l)
    if match:
        month = _MONTH_NUM[match.group(1).lower()]
        year = int(match.group(2))
        return DateWindow(date(year, month, 1).isoformat(), _month_end(year, month).isoformat(), "month")

    match = _YEAR_IN_QUERY_RE.search(text_l)
    if match:
        year = int(match.group(1))
        return DateWindow(f"{year}-01-01", f"{year}-12-31", "year")

    return None


class TemporalRetrievalPolicy:
    @staticmethod
    def resolve(
        raw_query: str | None,
        refined_query: str | None,
        config: dict[str, Any] | None,
        kb_ids: Sequence[str] | None = None,
    ) -> ResolvedTemporalPolicy:
        del kb_ids
        config = config or {}
        if not config or not config.get("enabled"):
            return _skipped_policy("disabled")

        temporal_field = config.get("temporal_field")
        if not isinstance(temporal_field, str) or not temporal_field.strip():
            return _skipped_policy("missing_temporal_field")
        temporal_field = temporal_field.strip()

        mode_raw = config.get("mode") or "auto"
        if not isinstance(mode_raw, str):
            logging.debug("Temporal retrieval skipped: invalid mode type=%s", type(mode_raw).__name__)
            return _skipped_policy("invalid_mode")
        mode = mode_raw.lower()
        query_text = " ".join(
            part for part in [normalize_arabic_digits(raw_query), normalize_arabic_digits(refined_query)] if part
        )
        explicit_window = extract_date_window(raw_query) or extract_date_window(refined_query)
        intent, confidence = _detect_temporal_intent(query_text, explicit_window)

        if mode == "latest":
            intent, confidence = "latest", 0.95
        elif mode == "date_range":
            intent, confidence = "date_range", 0.95 if explicit_window else 0.6
        elif mode == "balanced":
            intent, confidence = "balanced", max(confidence, 0.8)
        elif mode != "auto":
            logging.debug("Temporal retrieval skipped: invalid mode=%s", mode)
            return _skipped_policy("invalid_mode")

        if intent == "evergreen":
            return ResolvedTemporalPolicy(
                intent="evergreen",
                strategy="baseline",
                confidence=confidence,
                source="rules",
                filter_plan=TemporalFilterPlan([]),
                skipped_reason="evergreen_query",
            )

        filter_plan = TemporalFilterPlan([])
        if intent == "date_range":
            if explicit_window and config.get("supports_hard_filter", False):
                filter_field = config.get("temporal_date_field") or config.get("normalized_date_field") or temporal_field
                filter_plan = TemporalFilterPlan(explicit_window.to_conditions(filter_field))
            else:
                filter_plan = TemporalFilterPlan([], "date_range_without_hard_filter")

        rank_enabled = intent in {"latest", "balanced"} or (intent == "date_range" and not filter_plan.conditions)
        rank_plan = None
        if rank_enabled:
            rank_plan = TemporalRankPlan(
                enabled=True,
                temporal_field=temporal_field,
                half_life_days=_positive_float(config.get("half_life_days"), 14.0),
                freshness_weight=_positive_float(config.get("freshness_weight"), 0.15),
                freshness_offset_days=max(0.0, _positive_float(config.get("freshness_offset_days"), 0.0)),
                shadow_mode=bool(config.get("shadow_mode", False)),
                future_date_policy=str(config.get("future_date_policy") or "include_without_boost"),
            )

        strategy = "metadata_filter" if filter_plan.conditions else "freshness_rerank" if rank_plan else "baseline"
        logging.debug(
            "Temporal policy resolved: intent=%s strategy=%s field=%s window=%s future_date_policy=%s",
            intent,
            strategy,
            temporal_field,
            explicit_window.to_dict() if explicit_window else None,
            rank_plan.future_date_policy if rank_plan else None,
        )
        return ResolvedTemporalPolicy(
            intent=intent,
            strategy=strategy,
            confidence=confidence,
            source="rules",
            filter_plan=filter_plan,
            rank_plan=rank_plan,
            date_window=explicit_window,
            skipped_reason=filter_plan.skipped_reason,
        )


def freshness_score(
    parsed: ParsedTemporalValue | None,
    now: datetime | None,
    half_life_days: float,
    offset_days: float = 0.0,
    future_date_policy: str = "include_without_boost",
) -> float:
    """Compute exponential freshness in ``[0, 1]`` for a parsed temporal value.

    Future-dated documents are handled according to ``future_date_policy``:
    - ``include_without_boost`` / ``ignore_future``: no freshness boost
    - ``cap_to_now``: treat the document as if it were published today
    - ``penalize_future``: return a negative penalty factor for reranking
    - ``allow_future``: treat future dates as maximally fresh
    """
    if parsed is None:
        return 0.0
    now = now or datetime.now(UTC)
    age_days = (now.timestamp() - parsed.ts_norm) / 86400
    if age_days < 0:
        if future_date_policy in {"include_without_boost", "ignore_future"}:
            return 0.0
        if future_date_policy == "cap_to_now":
            age_days = 0.0
        elif future_date_policy == "penalize_future":
            return -0.25
        elif future_date_policy == "allow_future":
            age_days = 0.0
        else:
            return 0.0
    effective_age = max(0.0, age_days - max(0.0, offset_days))
    half_life_days = max(0.0001, half_life_days)
    return max(0.0, min(1.0, math.pow(0.5, effective_age / half_life_days)))


def temporal_sort_score(base_score: float, freshness: float, freshness_weight: float) -> float:
    if freshness < 0:
        return base_score * max(0.0, 1.0 + freshness)
    return base_score * (1 + max(0.0, freshness_weight) * max(0.0, min(1.0, freshness)))


def normalized_scores(scores: Sequence[float]) -> list[float]:
    if not scores:
        return []
    min_score = min(scores)
    max_score = max(scores)
    if max_score == min_score:
        return [1.0 for _ in scores]
    scale = max_score - min_score
    return [(score - min_score) / scale for score in scores]


def _detect_temporal_intent(text: str, explicit_window: DateWindow | None) -> tuple[str, float]:
    text_l = (text or "").lower()
    if explicit_window:
        return "date_range", 0.95
    if any(term in text_l or term in text for term in _BALANCED_TERMS):
        return "balanced", 0.8
    if any(term in text_l or term in text for term in _LATEST_TERMS):
        return "latest", 0.9
    if any(term in text_l or term in text for term in _DATE_TERMS):
        return "date_range", 0.6
    return "evergreen", 0.95


def _normalize_range_bounds(start_token: str, end_token: str) -> tuple[str | None, str | None]:
    start_token = (start_token or "").strip()
    end_token = (end_token or "").strip()
    if not start_token or not end_token:
        return None, None

    if len(start_token) == 4 and len(end_token) == 4:
        start_year = int(start_token)
        end_year = int(end_token)
        if start_year > end_year:
            start_year, end_year = end_year, start_year
        return f"{start_year}-01-01", f"{end_year}-12-31"

    if len(start_token) == 10 and len(end_token) == 10:
        if start_token > end_token:
            start_token, end_token = end_token, start_token
        return start_token, end_token

    return None, None


def _month_end(year: int, month: int) -> date:
    if month == 12:
        return date(year, 12, 31)
    return date(year, month + 1, 1) - timedelta(days=1)


def _positive_float(value: Any, default: float) -> float:
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return default
    return parsed if parsed > 0 else default


def _skipped_policy(reason: str) -> ResolvedTemporalPolicy:
    return ResolvedTemporalPolicy(
        intent="disabled",
        strategy="baseline",
        confidence=1.0,
        source="rules",
        filter_plan=TemporalFilterPlan([]),
        skipped_reason=reason,
    )


def profile_metadata_documents(
    metadata_by_doc: dict[str, dict[str, Any]],
    temporal_field: str,
) -> dict[str, Any]:
    return profile_temporal_field(metadata_by_doc, temporal_field).to_dict()


def visible_metadata_keys_from_docs(docs: Iterable[dict[str, Any]]) -> list[str]:
    keys: set[str] = set()
    for doc in docs:
        if not isinstance(doc, dict):
            continue
        keys.update(k for k in doc if not is_internal_metadata_key(k))
    return sorted(keys)
