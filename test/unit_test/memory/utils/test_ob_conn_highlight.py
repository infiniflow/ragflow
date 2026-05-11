#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use it except in compliance with the License.
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

"""Unit tests for OceanBase memory get_highlight.

Tests the pure highlight logic used by OBConnection.get_highlight,
without requiring a real OceanBase instance or heavy dependencies.
"""

from memory.utils.highlight_utils import get_highlight_from_messages, highlight_text


class TestHighlightText:
    """Tests for highlight_text (word-boundary mode when is_english_fn is None)."""

    def test_empty_text_returns_empty(self):
        assert highlight_text("", ["foo"]) == ""
        assert highlight_text("hello", []) == ""

    def test_wraps_keyword_with_em(self):
        out = highlight_text("The quick brown fox.", ["quick"], None)
        assert "<em>quick</em>" in out
        assert "The" in out and "brown fox" in out

    def test_only_sentences_with_match_included(self):
        out = highlight_text(
            "First sentence. Second has keyword. Third none.",
            ["keyword"],
            None,
        )
        assert "Second has <em>keyword</em>" in out
        assert "First sentence" not in out and "Third none" not in out

    def test_multiple_keywords(self):
        out = highlight_text("Alpha and beta here.", ["Alpha", "beta"], None)
        assert "<em>Alpha</em>" in out and "<em>beta</em>" in out


class TestGetHighlightFromMessages:
    """Tests for get_highlight_from_messages (used by get_highlight)."""

    def test_empty_messages_returns_empty_dict(self):
        assert get_highlight_from_messages([], ["k"], "content_ltks") == {}
        assert get_highlight_from_messages(None, ["k"], "content_ltks") == {}

    def test_empty_keywords_returns_empty_dict(self):
        assert get_highlight_from_messages(
            [{"id": "m1", "content_ltks": "hello"}], [], "content_ltks"
        ) == {}

    def test_returns_id_to_highlighted_text(self):
        messages = [
            {"id": "msg1", "content_ltks": "The cat sat."},
            {"id": "msg2", "content_ltks": "The dog ran."},
        ]
        out = get_highlight_from_messages(messages, ["cat"], "content_ltks")
        assert list(out.keys()) == ["msg1"]
        assert "<em>cat</em>" in out["msg1"]
        out2 = get_highlight_from_messages(messages, ["dog"], "content_ltks")
        assert list(out2.keys()) == ["msg2"]
        assert "<em>dog</em>" in out2["msg2"]

    def test_skips_docs_without_field(self):
        messages = [{"id": "m1"}, {"id": "m2", "content_ltks": "hello world."}]
        out = get_highlight_from_messages(messages, ["hello"], "content_ltks")
        assert "m2" in out and "<em>hello</em>" in out["m2"]
