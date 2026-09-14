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


class TestIsShortHeader:
    """Test cases for _is_short_header() function."""

    @pytest.fixture(autouse=True)
    def _lazy_import(self):
        sys.path.insert(0, str(_REPO))
        from rag.app.naive import _is_short_header

        self._is_short_header = _is_short_header

    def test_short_header_h1(self):
        """Short level-1 header should return True."""
        text = "# Quick Start"
        result = self._is_short_header(text)
        assert result is True

    def test_short_header_h2(self):
        """Short level-2 header should return True."""
        text = "## Quick Travel"
        result = self._is_short_header(text)
        assert result is True

    def test_short_header_h3(self):
        """Short level-3 header should return True."""
        text = "### Setup"
        result = self._is_short_header(text)
        assert result is True

    def test_long_header(self):
        """Long header (> 50 tokens) should return False."""
        text = "# " + "Very long header " * 20  # ~100 tokens
        result = self._is_short_header(text)
        assert result is False

    def test_non_header_short_text(self):
        """Short text without header pattern should return False."""
        text = "This is short"
        result = self._is_short_header(text)
        assert result is False

    def test_empty_text(self):
        """Empty text should return False."""
        text = ""
        result = self._is_short_header(text)
        assert result is False

    def test_whitespace_only(self):
        """Whitespace-only text should return False."""
        text = "   "
        result = self._is_short_header(text)
        assert result is False

    def test_header_exactly_50_tokens(self):
        """Header with exactly 50 tokens should return False (strict <)."""
        words = ["word"] * 49
        text = "# " + " ".join(words)
        result = self._is_short_header(text, max_tokens=50)
        assert result is False

    def test_header_49_tokens(self):
        """Header with 49 tokens should return True (< 50)."""
        words = ["word"] * 48
        text = "# " + " ".join(words)
        result = self._is_short_header(text, max_tokens=50)
        assert result is True

    def test_custom_max_tokens(self):
        """Should respect custom max_tokens parameter."""
        # "# Short" = 2 tokens in cl100k_base encoding
        text = "# Short"
        result = self._is_short_header(text, max_tokens=5)
        assert result is True  # 2 < 5 → short

        result = self._is_short_header(text, max_tokens=2)
        assert result is False  # 2 < 2 → not short

    def test_header_with_special_chars(self):
        """Header with special characters should still be recognized."""
        text = "## API Endpoint: /api/v1/users"
        result = self._is_short_header(text)
        assert result is True

    def test_header_with_cjk_chars(self):
        """Header with CJK characters should be recognized."""
        text = "## 快速旅行"
        result = self._is_short_header(text)
        assert result is True


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
