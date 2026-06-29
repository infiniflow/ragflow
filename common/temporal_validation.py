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
"""Shared validation helpers for temporal retrieval configuration.

The REST APIs, OpenAI-compatible endpoint, bot retrieval, and typed dataset
search request models all accept temporal retrieval settings. Keeping the
schema rules here prevents one route from silently accepting bad freshness
configuration that another route rejects.
"""

from __future__ import annotations

import math
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, field_validator, model_validator
from pydantic_core import PydanticCustomError

TEMPORAL_MODES = frozenset({"auto", "latest", "date_range", "balanced"})
FUTURE_DATE_POLICIES = frozenset(
    {
        "include_without_boost",
        "ignore_future",
        "cap_to_now",
        "penalize_future",
        "allow_future",
    }
)
MAX_TEMPORAL_PROFILE_SAMPLE = 500


def validate_half_life_days(value: Any) -> tuple[float | None, str | None]:
    """Validate ``half_life_days`` as a finite positive number.

    Args:
        value: Raw request value. Only integer and float values are accepted;
            booleans, strings, arrays, ``NaN``, and infinities are rejected.

    Returns:
        A tuple of ``(parsed_value, error_message)``. When validation fails the
        parsed value is ``None`` and the second element contains a user-facing
        error string.
    """
    if value is None:
        return None, None
    if isinstance(value, bool) or isinstance(value, str):
        return None, "`temporal_retrieval.half_life_days` should be a number."
    if not isinstance(value, (int, float)):
        return None, "`temporal_retrieval.half_life_days` should be a number."
    parsed = float(value)
    if not math.isfinite(parsed) or parsed <= 0:
        return None, "`temporal_retrieval.half_life_days` should be a finite number greater than 0."
    return parsed, None


def validate_temporal_retrieval_config(config: Any) -> str | None:
    """Validate a temporal retrieval config dict.

    Validation is intentionally side-effect free so API handlers can call it
    before saving config and typed Pydantic models can reuse the same rules.
    ``enabled=False`` permits incomplete temporal settings, while
    ``enabled=True`` requires a non-empty ``temporal_field``.

    Args:
        config: Raw config value from a request body. ``None`` is allowed.

    Returns:
        ``None`` when valid, otherwise a concise validation error message.
    """
    if config is None:
        return None
    if not isinstance(config, dict):
        return "`temporal_retrieval` should be an object."

    enabled = config.get("enabled")
    if enabled is not None and not isinstance(enabled, bool):
        return "`temporal_retrieval.enabled` should be a boolean."

    mode = config.get("mode")
    if mode is not None:
        if not isinstance(mode, str):
            return "`temporal_retrieval.mode` should be one of auto, latest, date_range, balanced."
        if mode.lower() not in TEMPORAL_MODES:
            return "`temporal_retrieval.mode` should be one of auto, latest, date_range, balanced."

    if enabled:
        field = config.get("temporal_field")
        if not isinstance(field, str) or not field.strip():
            return "`temporal_retrieval.temporal_field` is required when temporal retrieval is enabled."

    if "half_life_days" in config:
        _, err = validate_half_life_days(config.get("half_life_days"))
        if err:
            return err

    policy = config.get("future_date_policy")
    if policy is not None:
        if not isinstance(policy, str) or policy not in FUTURE_DATE_POLICIES:
            supported = ", ".join(sorted(FUTURE_DATE_POLICIES))
            return f"`temporal_retrieval.future_date_policy` should be one of {supported}."

    return None


def merge_temporal_retrieval_config(
    existing: dict[str, Any] | None,
    patch: dict[str, Any] | None,
) -> dict[str, Any]:
    """Merge a partial temporal retrieval patch into stored config.

    PATCH requests can update a single field such as ``half_life_days`` without
    resending ``mode`` or ``temporal_field``. This helper preserves the stored
    fields and lets the caller validate the merged result before persisting it.
    """
    merged = dict(existing or {})
    if isinstance(patch, dict):
        merged.update(patch)
    return merged


class TemporalRetrievalConfig(BaseModel):
    """Typed temporal retrieval settings accepted by search and dataset APIs.

    The model mirrors the shared validator while producing a normalized object
    for service-layer search calls. UI-derived profile fields are accepted so
    existing clients can round-trip saved settings, but unknown fields are
    ignored to preserve backward compatibility.
    """

    model_config = ConfigDict(extra="ignore")

    enabled: bool = False
    mode: Literal["auto", "latest", "date_range", "balanced"] = "auto"
    temporal_field: str | None = None
    half_life_days: float = 14.0
    future_date_policy: Literal[
        "include_without_boost",
        "ignore_future",
        "cap_to_now",
        "penalize_future",
        "allow_future",
    ] = "include_without_boost"
    detected_format: str | None = None
    supports_hard_filter: bool | None = None
    supports_freshness_score: bool | None = None
    temporal_date_field: str | None = None
    normalized_date_field: str | None = None
    freshness_weight: float | None = None
    freshness_offset_days: float | None = None
    shadow_mode: bool | None = None

    @field_validator("temporal_field", mode="before")
    @classmethod
    def normalize_temporal_field(cls, value: Any) -> Any:
        """Trim empty temporal field values before required-field validation."""
        if isinstance(value, str):
            stripped = value.strip()
            return stripped or None
        return value

    @field_validator("half_life_days", mode="before")
    @classmethod
    def validate_half_life(cls, value: Any) -> Any:
        """Reuse shared finite-positive validation for Pydantic requests."""
        parsed, err = validate_half_life_days(value)
        if err:
            raise PydanticCustomError("format_invalid", err)
        return parsed if parsed is not None else 14.0

    @model_validator(mode="after")
    def require_field_when_enabled(self) -> "TemporalRetrievalConfig":
        """Require a usable metadata field only when temporal retrieval is on."""
        if self.enabled and not self.temporal_field:
            raise PydanticCustomError(
                "format_invalid",
                "`temporal_retrieval.temporal_field` is required when temporal retrieval is enabled.",
            )
        return self
