#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

"""Unit tests for chunk_post_processor keyword sanitization."""

from unittest.mock import MagicMock, patch

import pytest

from rag.svr.task_executor_refactor.chunk_post_processor import (
    _ES_KEYWORD_MAX_TERM_BYTES,
    _sanitize_keyword_term,
    extract_keywords,
)
from test.unit_test.rag.svr.task_executor_refactor.conftest import make_task_context


class TestSanitizeKeywordTerm:
    """Tests for _sanitize_keyword_term."""

    def test_returns_small_term_as_is(self):
        assert _sanitize_keyword_term("hello") == ["hello"]

    def test_trims_whitespace(self):
        assert _sanitize_keyword_term("  world  ") == ["world"]

    def test_drops_empty_term(self):
        assert _sanitize_keyword_term("   ") == []

    def test_truncates_oversized_whitespace_term(self):
        oversized = "word " * 20000
        pieces = _sanitize_keyword_term(oversized)
        assert len(pieces) == 1
        assert len(pieces[0].encode("utf-8")) <= _ES_KEYWORD_MAX_TERM_BYTES
        assert pieces[0].startswith("word")

    def test_truncates_oversized_single_word(self):
        oversized = "x" * 40000
        pieces = _sanitize_keyword_term(oversized)
        assert len(pieces) == 1
        assert len(pieces[0].encode("utf-8")) == _ES_KEYWORD_MAX_TERM_BYTES

    def test_preserves_multi_byte_characters(self):
        oversized = "中" * 20000
        pieces = _sanitize_keyword_term(oversized)
        assert len(pieces) == 1
        assert len(pieces[0].encode("utf-8")) == _ES_KEYWORD_MAX_TERM_BYTES

    def test_truncation_stops_at_character_boundary(self):
        # 32766 is divisible by 3, so forcing a boundary just before a
        # multi-byte character proves we are not slicing raw bytes.
        term = "a" * (_ES_KEYWORD_MAX_TERM_BYTES - 2) + "中"
        pieces = _sanitize_keyword_term(term)
        assert len(pieces) == 1
        assert "中" not in pieces[0]
        assert len(pieces[0].encode("utf-8")) < _ES_KEYWORD_MAX_TERM_BYTES


class TestExtractKeywords:
    """Tests for extract_keywords."""

    @pytest.mark.asyncio
    async def test_extract_keywords_from_cache(self):
        ctx = make_task_context(parser_config={"auto_keywords": 2})
        docs = [{"content_with_weight": "some content"}]

        with (
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.resolve_model_config",
                return_value=MagicMock(),
            ),
            patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as MockBundle,
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                return_value="key1, key2",
            ),
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache",
            ),
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.rag_tokenizer.tokenize",
                return_value="tokenized",
            ) as mock_tokenize,
        ):
            chat_model = MagicMock()
            chat_model.llm_name = "mock"
            chat_model.__enter__ = MagicMock(return_value=chat_model)
            chat_model.__exit__ = MagicMock(return_value=False)
            MockBundle.return_value = chat_model

            await extract_keywords(docs, ctx)

        assert docs[0]["important_kwd"] == ["key1", "key2"]
        assert docs[0]["important_tks"] == "tokenized"
        mock_tokenize.assert_called_once_with("key1 key2")

    @pytest.mark.asyncio
    async def test_extract_keywords_avoids_oversized_terms(self):
        ctx = make_task_context(parser_config={"auto_keywords": 2})
        huge = "x" * 40000
        docs = [{"content_with_weight": "some content"}]

        with (
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.resolve_model_config",
                return_value=MagicMock(),
            ),
            patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as MockBundle,
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                return_value=huge,
            ),
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache",
            ),
            patch(
                "rag.svr.task_executor_refactor.chunk_post_processor.rag_tokenizer.tokenize",
            ) as mock_tokenize,
        ):
            chat_model = MagicMock()
            chat_model.llm_name = "mock"
            chat_model.__enter__ = MagicMock(return_value=chat_model)
            chat_model.__exit__ = MagicMock(return_value=False)
            MockBundle.return_value = chat_model

            await extract_keywords(docs, ctx)

        assert len(docs[0]["important_kwd"]) == 1
        assert len(docs[0]["important_kwd"][0].encode("utf-8")) == _ES_KEYWORD_MAX_TERM_BYTES
        mock_tokenize.assert_called_once()
