"""
Unit tests for markdown chunk merging logic in rag/app/naive.py.

Tests the _is_short_header() helper function to ensure short markdown headers
are correctly identified and will be force-merged with the next section.
"""

import sys
import os

# Add project root to path for imports
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from rag.app.naive import _is_short_header


class TestIsShortHeader:
    """Test cases for _is_short_header() function."""

    def test_short_header_h1(self):
        """Short level-1 header should return True."""
        text = "# Quick Start"
        result = _is_short_header(text)
        assert result is True

    def test_short_header_h2(self):
        """Short level-2 header should return True."""
        text = "## Quick Travel"
        result = _is_short_header(text)
        assert result is True

    def test_short_header_h3(self):
        """Short level-3 header should return True."""
        text = "### Setup"
        result = _is_short_header(text)
        assert result is True

    def test_long_header(self):
        """Long header (> 50 tokens) should return False."""
        text = "# " + "Very long header " * 20  # ~100 tokens
        result = _is_short_header(text)
        assert result is False

    def test_non_header_short_text(self):
        """Short text without header pattern should return False."""
        text = "This is short"
        result = _is_short_header(text)
        assert result is False

    def test_empty_text(self):
        """Empty text should return False."""
        text = ""
        result = _is_short_header(text)
        assert result is False

    def test_whitespace_only(self):
        """Whitespace-only text should return False."""
        text = "   "
        result = _is_short_header(text)
        assert result is False

    def test_header_exactly_50_tokens(self):
        """Header with exactly 50 tokens should return False (strict <)."""
        # Construct a header with exactly 50 tokens
        words = ["word"] * 49  # 49 words = 49 tokens, plus "# " = 1 token
        text = "# " + " ".join(words)
        result = _is_short_header(text, max_tokens=50)
        # 50 tokens = not < 50, so should return False
        assert result is False

    def test_header_49_tokens(self):
        """Header with 49 tokens should return True (< 50)."""
        words = ["word"] * 48  # 48 words = 48 tokens, plus "# " = 1 token = 49 tokens
        text = "# " + " ".join(words)
        result = _is_short_header(text, max_tokens=50)
        assert result is True

    def test_custom_max_tokens(self):
        """Should respect custom max_tokens parameter."""
        text = "# Short"
        result = _is_short_header(text, max_tokens=5)
        assert result is False  # "# Short" is ~2 tokens, but wait...

        result = _is_short_header(text, max_tokens=10)
        assert result is True

    def test_header_with_special_chars(self):
        """Header with special characters should still be recognized."""
        text = "## API Endpoint: /api/v1/users"
        result = _is_short_header(text)
        assert result is True

    def test_header_with_cjk_chars(self):
        """Header with CJK characters should be recognized."""
        text = "## 快速旅行"
        result = _is_short_header(text)
        assert result is True


if __name__ == "__main__":
    import pytest
    pytest.main([__file__, "-v"])
