"""
Regression test for the ``ParserConfig.task_page_size`` defensive-coercion
fix (issue #19039).

Background: the v0.27.x web chunk-method dialog renders the "Task Page
Size" input only for PDF + MinerU layout, but the dialog's zod schema
coerces the stored ``null`` to ``0`` and submits it on every save. The
backend schema (``ge=1``) then rejects the whole update and the user
cannot save any other field.

The runtime (``api/db/services/task_service.py::queue_tasks``) already
treats a falsy value as unset
(``page_size = doc["parser_config"].get("task_page_size") or 12``), so the
fix is to make the backend accept the same value at the request boundary:
a ``field_validator("task_page_size", mode="before")`` maps ``0``,
negative numbers, empty strings, and unparseable values to ``None``.
"""

import pytest

from api.utils.validation_utils import ParserConfig


pytestmark = pytest.mark.p2


class TestTaskPageSizeCoercion:
    """``ParserConfig.task_page_size`` must treat falsy / unparseable
    values as ``None`` (un-set) rather than failing validation with
    ``Input should be greater than or equal to 1 - Value: 0``."""

    def test_none_passes_through(self):
        cfg = ParserConfig(task_page_size=None)
        assert cfg.task_page_size is None

    def test_field_omitted_defaults_to_none(self):
        cfg = ParserConfig()
        assert cfg.task_page_size is None

    def test_empty_string_coerced_to_none(self):
        cfg = ParserConfig(task_page_size="")
        assert cfg.task_page_size is None

    def test_zero_coerced_to_none(self):
        """The headline fix: the frontend zod coerces null to 0; the
        backend now coerces 0 back to None instead of failing."""
        cfg = ParserConfig(task_page_size=0)
        assert cfg.task_page_size is None

    def test_negative_integer_coerced_to_none(self):
        cfg = ParserConfig(task_page_size=-1)
        assert cfg.task_page_size is None

    def test_string_zero_coerced_to_none(self):
        cfg = ParserConfig(task_page_size="0")
        assert cfg.task_page_size is None

    def test_unparseable_string_coerced_to_none(self):
        cfg = ParserConfig(task_page_size="not-a-number")
        assert cfg.task_page_size is None

    def test_none_value_coerced_to_none(self):
        """Pydantic's ``int`` parser also accepts ``None`` for an
        ``int | None`` field; ensure the round-trip is still ``None``."""
        cfg = ParserConfig(task_page_size=None)
        assert cfg.task_page_size is None

    def test_valid_value_preserved(self):
        cfg = ParserConfig(task_page_size=12)
        assert cfg.task_page_size == 12

    def test_valid_value_preserved_at_minimum(self):
        cfg = ParserConfig(task_page_size=1)
        assert cfg.task_page_size == 1

    def test_valid_string_value_preserved(self):
        """The validator must also accept string-typed positive integers
        (Pydantic coerces them via ``int()`` after our coercion)."""
        cfg = ParserConfig(task_page_size="22")
        assert cfg.task_page_size == 22

    def test_large_value_preserved(self):
        """The schema has no upper bound on ``task_page_size`` (no
        ``le=`` constraint — the field's role is page-range partitioning,
        which is a runtime convenience, not a quota). The coercion must
        therefore not silently cap large positive values."""
        cfg = ParserConfig(task_page_size=10_000_000)
        assert cfg.task_page_size == 10_000_000


class TestParserConfigDefaultsUnchanged:
    """The coercion must not change any other field's default or
    validation behavior."""

    def test_defaults_unchanged(self):
        cfg = ParserConfig()
        assert cfg.chunk_token_num == 512
        assert cfg.delimiter == "\n"
        assert cfg.layout_recognize == "DeepDOC"
        assert cfg.topn_tags == 1

    def test_unset_task_page_size_does_not_clobber_other_fields(self):
        """A submitter that touches one field and the hidden
        ``task_page_size: 0`` must not lose the other field's update."""
        cfg = ParserConfig(chunk_token_num=1024, task_page_size=0)
        assert cfg.chunk_token_num == 1024
        assert cfg.task_page_size is None


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))
