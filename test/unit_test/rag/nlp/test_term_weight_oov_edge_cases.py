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

"""Edge case tests for _alphabetic_oov_frequency in rag.nlp.term_weight."""

import pytest

from rag.nlp.term_weight import _alphabetic_oov_frequency


@pytest.mark.p2
class TestAlphabeticOovFrequencyEdgeCases:
    """Verify _alphabetic_oov_frequency return values for edge cases not
    covered by the shared fixture in test_term_weight_oov.py."""

    def test_short_ascii_word_returns_base_frequency(self):
        """Words with 3 or fewer letters return the base frequency (300)."""
        assert _alphabetic_oov_frequency("a") == 300
        assert _alphabetic_oov_frequency("the") == 300
        assert _alphabetic_oov_frequency("cat") == 300

    def test_frequency_values_match_formula(self):
        """Verify exact frequency values: max(10, round(300 / 2^((n-3)/2)))."""
        assert _alphabetic_oov_frequency("maior") == 150
        assert _alphabetic_oov_frequency("laptop") == 106
        assert _alphabetic_oov_frequency("largest") == 75
        assert _alphabetic_oov_frequency("supplier") == 53
        assert _alphabetic_oov_frequency("equipment") == 38

    def test_frequency_never_drops_below_minimum(self):
        """The clamped minimum frequency is 10."""
        assert _alphabetic_oov_frequency("a" * 100) == 10

    def test_returns_none_for_cjk_characters(self):
        """CJK terms are not alphabetic OOV and return None."""
        assert _alphabetic_oov_frequency("北京") is None
        assert _alphabetic_oov_frequency("東京") is None

    def test_returns_none_for_digits_only(self):
        """Pure digit strings have no alphabetic content."""
        assert _alphabetic_oov_frequency("123") is None
        assert _alphabetic_oov_frequency("42") is None

    def test_returns_none_for_empty_string(self):
        """Empty string has no alphabetic content."""
        assert _alphabetic_oov_frequency("") is None

    def test_returns_none_for_mixed_alphanumeric(self):
        """Mixed alphanumeric terms are rejected."""
        assert _alphabetic_oov_frequency("abc123") is None
        assert _alphabetic_oov_frequency("v2") is None

    def test_returns_none_for_underscore_separator(self):
        """Underscores are not valid separators."""
        assert _alphabetic_oov_frequency("hello_world") is None

    def test_hyphen_is_valid_separator(self):
        """Hyphens are preserved as valid separators."""
        assert _alphabetic_oov_frequency("hospital-equipment") == 10
        assert _alphabetic_oov_frequency("self-contained") == 10

    def test_space_is_valid_separator(self):
        """Spaces are preserved as valid separators."""
        result = _alphabetic_oov_frequency("hospital equipment")
        assert result is not None
        assert result >= 10

    def test_dots_are_valid_separators(self):
        """Dots/periods are preserved as valid separators."""
        assert _alphabetic_oov_frequency("...") is None
        assert _alphabetic_oov_frequency("a.b.c") is not None

    def test_accented_latin_characters_counted(self):
        """Accented Latin characters are counted as alphabetic."""
        result = _alphabetic_oov_frequency("café")
        assert result is not None
        assert result == 212

    def test_uppercase_latin_characters_counted(self):
        """Uppercase Latin characters are counted as alphabetic."""
        result = _alphabetic_oov_frequency("CAFÉ")
        assert result is not None
        assert result == 212

    def test_greek_characters_counted(self):
        """Greek characters are counted as alphabetic."""
        result = _alphabetic_oov_frequency("κόσμος")
        assert result is not None
        assert result == 106

    def test_cyrillic_characters_counted(self):
        """Cyrillic characters are counted as alphabetic."""
        result = _alphabetic_oov_frequency("привет")
        assert result is not None
        assert result == 106

    def test_frequency_ordering_longer_is_lower(self):
        """Longer words always get lower or equal frequency than shorter ones."""
        freq_short = _alphabetic_oov_frequency("cat")
        freq_medium = _alphabetic_oov_frequency("laptop")
        freq_long = _alphabetic_oov_frequency("equipment")
        assert freq_short > freq_medium > freq_long

    def test_single_non_ascii_alpha_rejected(self):
        """A single non-Latin/Greek/Cyrillic alpha character returns None."""
        assert _alphabetic_oov_frequency("京") is None
