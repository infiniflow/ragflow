"""
Unit tests for markdown chunk merging logic in rag/app/naive.py.

Tests the _is_short_header() helper function to ensure short markdown headers
are correctly identified and will be force-merged with the next section.

Uses lazy import via fixture to avoid triggering deepdoc model loading
at pytest collection time (which would fail in CI without model files).
"""

import sys
from pathlib import Path

import pytest

_REPO = Path(__file__).parents[4]


@pytest.fixture(scope="module")
def is_short_header():
    sys.path.insert(0, str(_REPO))
    from rag.app.naive import _is_short_header

    return _is_short_header


class TestIsShortHeader:
    """Test cases for _is_short_header() function."""

    def test_short_header_h1(self, is_short_header):
        """Short level-1 header should return True."""
        text = "# Quick Start"
        result = is_short_header(text)
        assert result is True

    def test_short_header_h2(self, is_short_header):
        """Short level-2 header should return True."""
        text = "## Quick Travel"
        result = is_short_header(text)
        assert result is True

    def test_short_header_h3(self, is_short_header):
        """Short level-3 header should return True."""
        text = "### Setup"
        result = is_short_header(text)
        assert result is True

    def test_long_header(self, is_short_header):
        """Long header (> 50 tokens) should return False."""
        text = "# " + "Very long header " * 20  # ~100 tokens
        result = is_short_header(text)
        assert result is False

    def test_non_header_short_text(self, is_short_header):
        """Short text without header pattern should return False."""
        text = "This is short"
        result = is_short_header(text)
        assert result is False

    def test_empty_text(self, is_short_header):
        """Empty text should return False."""
        text = ""
        result = is_short_header(text)
        assert result is False

    def test_whitespace_only(self, is_short_header):
        """Whitespace-only text should return False."""
        text = "   "
        result = is_short_header(text)
        assert result is False

    def test_header_exactly_50_tokens(self, is_short_header):
        """Header with exactly 50 tokens should return False (strict <)."""
        words = ["word"] * 49
        text = "# " + " ".join(words)
        result = is_short_header(text, max_tokens=50)
        assert result is False

    def test_header_49_tokens(self, is_short_header):
        """Header with 49 tokens should return True (< 50)."""
        words = ["word"] * 48
        text = "# " + " ".join(words)
        result = is_short_header(text, max_tokens=50)
        assert result is True

    def test_custom_max_tokens(self, is_short_header):
        """Should respect custom max_tokens parameter."""
        text = "# Short"
        result = is_short_header(text, max_tokens=5)
        assert result is False

        result = is_short_header(text, max_tokens=10)
        assert result is True

    def test_header_with_special_chars(self, is_short_header):
        """Header with special characters should still be recognized."""
        text = "## API Endpoint: /api/v1/users"
        result = is_short_header(text)
        assert result is True

    def test_header_with_cjk_chars(self, is_short_header):
        """Header with CJK characters should be recognized."""
        text = "## 快速旅行"
        result = is_short_header(text)
        assert result is True


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
