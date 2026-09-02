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

"""Ingestion LLM-token accounting in the refactored task executor.

The post-chunking LLM phases (keywords, questions, metadata, tagging) credit
their token spend to ``TaskContext.llm_token_num``, which ``TaskHandler`` then
writes to ``document.llm_token_num`` via ``increment_chunk_num``.
"""

from unittest.mock import MagicMock, patch

import pytest

from rag.svr.task_executor_refactor.chunk_post_processor import _counting_bundle, extract_keywords


class TestTaskContextTokenAccumulator:
    """Tests for TaskContext.llm_token_num / add_llm_tokens."""

    def test_starts_at_zero(self, task_context):
        assert task_context.llm_token_num == 0

    def test_accumulates_across_phases(self, task_context):
        task_context.add_llm_tokens(120)
        task_context.add_llm_tokens(30)
        assert task_context.llm_token_num == 150

    @pytest.mark.parametrize("value", [0, -1])
    def test_ignores_non_positive(self, task_context, value):
        task_context.add_llm_tokens(value)
        assert task_context.llm_token_num == 0


class TestCountingBundle:
    """Tests for the _counting_bundle context manager."""

    @staticmethod
    def _bundle(tokens):
        bundle = MagicMock()
        bundle.cumulated_tokens = tokens
        bundle.__enter__ = MagicMock(return_value=bundle)
        bundle.__exit__ = MagicMock(return_value=False)
        return bundle

    def test_credits_tokens_on_normal_exit(self, task_context):
        with (
            patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle", return_value=self._bundle(42)),
            _counting_bundle(task_context, {}) as bundle,
        ):
            assert bundle.cumulated_tokens == 42

        assert task_context.llm_token_num == 42

    def test_credits_tokens_when_phase_raises(self, task_context):
        """A phase that dies half-way still spent the tokens it already burned."""
        with (
            patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle", return_value=self._bundle(17)),
            pytest.raises(RuntimeError),
            _counting_bundle(task_context, {}),
        ):
            raise RuntimeError("provider blew up mid-phase")

        assert task_context.llm_token_num == 17

    def test_phases_accumulate_into_one_total(self, task_context):
        for tokens in (10, 25, 5):
            with (
                patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle", return_value=self._bundle(tokens)),
                _counting_bundle(task_context, {}),
            ):
                pass

        assert task_context.llm_token_num == 40


class TestPhaseCreditsContext:
    """The real phase functions must go through _counting_bundle."""

    @pytest.mark.asyncio
    async def test_extract_keywords_credits_context(self, task_context):
        task_context._task["parser_config"] = {"auto_keywords": 2}
        docs = [{"content_with_weight": "some content"}]

        chat_model = MagicMock()
        chat_model.llm_name = "mock"
        chat_model.cumulated_tokens = 314
        chat_model.__enter__ = MagicMock(return_value=chat_model)
        chat_model.__exit__ = MagicMock(return_value=False)

        with (
            patch("rag.svr.task_executor_refactor.chunk_post_processor.resolve_model_config", return_value=MagicMock()),
            patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle", return_value=chat_model),
            patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache", return_value="key1, key2"),
            patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache"),
            patch("rag.svr.task_executor_refactor.chunk_post_processor.rag_tokenizer.tokenize", return_value="tokenized"),
        ):
            await extract_keywords(docs, task_context)

        assert task_context.llm_token_num == 314
