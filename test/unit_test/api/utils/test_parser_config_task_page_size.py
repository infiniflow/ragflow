"""Regression tests for ``ParserConfig.task_page_size`` (issue #19039,
rework of closed PR #19067).

The v0.27.x chunk-method dialog schema coerces a stored ``null`` to ``0``
(``z.coerce.number()``) and submits it on every save; the backend
``ge=1`` constraint then rejects the whole update. The validator maps
exactly that documented sentinel to ``None`` and lets pydantic's strict
``int | None`` + ``ge=1`` validation reject everything else with a clean
422 - no silent drops, no truncation, no uncaught ``OverflowError``.
"""

import math

import pytest
from pydantic import ValidationError

from api.utils.validation_utils import ParserConfig, UpdateDocumentReq


pytestmark = pytest.mark.p2


class TestZeroSentinel:
    def test_zero_int_coerced_to_none(self):
        """The headline fix: zod coerces null to 0; the backend maps it back."""
        assert ParserConfig(task_page_size=0).task_page_size is None

    def test_zero_float_coerced_to_none(self):
        assert ParserConfig(task_page_size=0.0).task_page_size is None

    def test_field_omitted_defaults_to_none(self):
        assert ParserConfig().task_page_size is None

    def test_none_passes_through(self):
        assert ParserConfig(task_page_size=None).task_page_size is None


class TestValidValuesUnchanged:
    @pytest.mark.parametrize("value", [1, 12, 100, 9999])
    def test_positive_integers_accepted(self, value):
        assert ParserConfig(task_page_size=value).task_page_size == value


class TestCleanRejections:
    """Everything that is not the documented sentinel must fail with a
    ValidationError (HTTP 422), never silently coerced and never an
    uncaught OverflowError (HTTP 500)."""

    @pytest.mark.parametrize(
        "value",
        [
            -1,                 # negative: ge=1 violation, NOT unset
            1.5,                # fraction: must not truncate to 1
            float("inf"),       # non-finite: must not raise OverflowError
            float("-inf"),
            math.nan,           # not-a-number payload
            "not-a-number",     # unparseable string
            "0",                # string zero: only the numeric sentinel is remapped
            [0],                # wrong container type
        ],
    )
    def test_rejected_with_validation_error(self, value):
        with pytest.raises(ValidationError):
            ParserConfig(task_page_size=value)


class TestEndToEndRequestModel:
    def test_zero_sentinel_through_update_document_req(self):
        """End to end at the request boundary: 0 lands as None on the
        nested ParserConfig, and sibling fields survive the same update -
        the exact save that was blocked by issue #19039."""
        req = UpdateDocumentReq.model_validate(
            {"parser_config": {"task_page_size": 0, "chunk_token_num": 256}}
        )
        assert req.parser_config.task_page_size is None
        assert req.parser_config.chunk_token_num == 256

    def test_invalid_value_through_update_document_req_is_422(self):
        with pytest.raises(ValidationError):
            UpdateDocumentReq.model_validate(
                {"parser_config": {"task_page_size": "not-a-number"}}
            )
