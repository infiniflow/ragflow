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

"""
Unit tests for ChunkPostProcessor module.
"""

import pytest
from unittest.mock import MagicMock, patch, AsyncMock
from rag.svr.task_executor_refactor.chunk_post_processor import (
    extract_keywords,
    generate_questions,
    generate_metadata,
    apply_tags,
    count_with_key,
    build_metadata_config,
)


class TestExtractKeywords:
    """Tests for extract_keywords function."""

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.tenant_id = "tenant_1"
        ctx.llm_id = "llm_1"
        ctx.language = "en"
        ctx.parser_config = {"auto_keywords": 5}
        ctx.id = "task_1"
        ctx.progress_cb = MagicMock()
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.chat_limiter = MagicMock()
        ctx.chat_limiter.__aenter__ = AsyncMock()
        ctx.chat_limiter.__aexit__ = AsyncMock()
        return ctx

    @pytest.mark.asyncio
    async def test_extract_keywords_success(self):
        """Test successful keyword extraction."""
        ctx = self._create_mock_context()
        docs = [
            {"content_with_weight": "This is test content one"},
            {"content_with_weight": "This is test content two"},
        ]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache") as mock_cache:
                    mock_cache.return_value = "keyword1, keyword2"

                    with patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache"):
                        with patch("rag.svr.task_executor_refactor.chunk_post_processor.rag_tokenizer") as mock_tokenizer:
                            mock_tokenizer.tokenize.return_value = "keyword1 keyword2"

                            await extract_keywords(docs, ctx)

                            # Verify keywords were set
                            assert "important_kwd" in docs[0]
                            assert "important_tks" in docs[0]

    @pytest.mark.asyncio
    async def test_extract_keywords_canceled(self):
        """Test keyword extraction when task is canceled."""
        ctx = self._create_mock_context()
        ctx.has_canceled_func = MagicMock(return_value=True)
        docs = [{"content_with_weight": "This is test content"}]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache") as mock_cache:
                    mock_cache.return_value = None  # No cache

                    await extract_keywords(docs, ctx)

                    # Should return early due to cancellation
                    assert "important_kwd" not in docs[0]

    @pytest.mark.asyncio
    async def test_extract_keywords_empty_docs(self):
        """Test keyword extraction with empty docs list."""
        ctx = self._create_mock_context()
        docs = []

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                await extract_keywords(docs, ctx)

                # Should complete without error
                ctx.progress_cb.assert_called()


class TestGenerateQuestions:
    """Tests for generate_questions function."""

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.tenant_id = "tenant_1"
        ctx.llm_id = "llm_1"
        ctx.language = "en"
        ctx.parser_config = {"auto_questions": 3}
        ctx.id = "task_1"
        ctx.progress_cb = MagicMock()
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.chat_limiter = MagicMock()
        ctx.chat_limiter.__aenter__ = AsyncMock()
        ctx.chat_limiter.__aexit__ = AsyncMock()
        return ctx

    @pytest.mark.asyncio
    async def test_generate_questions_success(self):
        """Test successful question generation."""
        ctx = self._create_mock_context()
        docs = [
            {"content_with_weight": "This is test content one"},
        ]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache") as mock_cache:
                    mock_cache.return_value = "Question 1\nQuestion 2"

                    with patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache"):
                        with patch("rag.svr.task_executor_refactor.chunk_post_processor.rag_tokenizer") as mock_tokenizer:
                            mock_tokenizer.tokenize.return_value = "Question 1 Question 2"

                            await generate_questions(docs, ctx)

                            # Verify questions were set
                            assert "question_kwd" in docs[0]
                            assert "question_tks" in docs[0]

    @pytest.mark.asyncio
    async def test_generate_questions_canceled(self):
        """Test question generation when task is canceled."""
        ctx = self._create_mock_context()
        ctx.has_canceled_func = MagicMock(return_value=True)
        docs = [{"content_with_weight": "This is test content"}]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache") as mock_cache:
                    mock_cache.return_value = None  # No cache

                    await generate_questions(docs, ctx)

                    # Should return early due to cancellation
                    assert "question_kwd" not in docs[0]


class TestGenerateMetadata:
    """Tests for generate_metadata function."""

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.tenant_id = "tenant_1"
        ctx.llm_id = "llm_1"
        ctx.language = "en"
        ctx.parser_config = {
            "enable_metadata": True,
            "metadata": [{"name": "category", "type": "string"}],
            "built_in_metadata": ["author", "date"],
        }
        ctx.doc_id = "doc_1"
        ctx.id = "task_1"
        ctx.progress_cb = MagicMock()
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.write_interceptor = None
        ctx.chat_limiter = MagicMock()
        ctx.chat_limiter.__aenter__ = AsyncMock()
        ctx.chat_limiter.__aexit__ = AsyncMock()
        return ctx

    @pytest.mark.asyncio
    async def test_generate_metadata_success(self):
        """Test successful metadata generation."""
        ctx = self._create_mock_context()
        docs = [
            {"content_with_weight": "This is test content", "metadata_obj": {"category": "test"}},
        ]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache") as mock_cache:
                    mock_cache.return_value = {"category": "test"}

                    with patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache"):
                        with patch("rag.svr.task_executor_refactor.chunk_post_processor.update_metadata_to") as mock_update:
                            mock_update.return_value = {"category": "test"}

                            with patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService") as mock_meta:
                                mock_meta.get_document_metadata.return_value = {}
                                mock_meta.update_document_metadata = MagicMock()

                                await generate_metadata(docs, ctx)

                                # Verify metadata_obj was processed
                                mock_meta.update_document_metadata.assert_called_once()

    @pytest.mark.asyncio
    async def test_generate_metadata_with_write_interceptor(self):
        """Test metadata generation with write interceptor."""
        ctx = self._create_mock_context()
        ctx.write_interceptor = MagicMock()
        docs = [
            {"content_with_weight": "This is test content", "metadata_obj": {"category": "test"}},
        ]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache") as mock_cache:
                    mock_cache.return_value = {"category": "test"}

                    with patch("rag.svr.task_executor_refactor.chunk_post_processor.update_metadata_to") as mock_update:
                        mock_update.return_value = {"category": "test"}

                        with patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService") as mock_meta:
                            mock_meta.get_document_metadata.return_value = {}
                            mock_meta.update_document_metadata = MagicMock()

                            await generate_metadata(docs, ctx)

                            ctx.write_interceptor.intercept.assert_called_once_with(
                                "DocMetadataService.update_document_metadata"
                            )


class TestApplyTags:
    """Tests for apply_tags function."""

    def _create_mock_context(self):
        """Helper to create a mock TaskContext."""
        ctx = MagicMock()
        ctx.tenant_id = "tenant_1"
        ctx.llm_id = "llm_1"
        ctx.language = "en"
        ctx.kb_parser_config = {"tag_kb_ids": ["kb_1"], "topn_tags": 3}
        ctx.id = "task_1"
        ctx.progress_cb = MagicMock()
        ctx.has_canceled_func = MagicMock(return_value=False)
        ctx.chat_limiter = MagicMock()
        ctx.chat_limiter.__aenter__ = AsyncMock()
        ctx.chat_limiter.__aexit__ = AsyncMock()
        return ctx

    @pytest.mark.asyncio
    async def test_apply_tags_success(self):
        """Test successful tag application."""
        ctx = self._create_mock_context()
        docs = [
            {"content_with_weight": "This is test content"},
        ]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.settings") as mock_settings:
                    mock_settings.retriever.all_tags_in_portion.return_value = {"tag1": 10, "tag2": 5}
                    mock_settings.retriever.tag_content.return_value = True

                    with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache") as mock_cache:
                        mock_cache.return_value = '{"tag1": 1}'

                        with patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache"):
                            await apply_tags(docs, ctx)

                            # Verify tags were applied
                            assert len(docs) == 1

    @pytest.mark.asyncio
    async def test_apply_tags_canceled(self):
        """Test tag application when task is canceled."""
        ctx = self._create_mock_context()
        ctx.has_canceled_func = MagicMock(return_value=True)
        docs = [
            {"content_with_weight": "This is test content"},
        ]

        with patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance") as mock_config:
            mock_config.return_value = MagicMock()

            with patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle") as mock_llm:
                mock_llm_instance = MagicMock()
                mock_llm.return_value.__enter__ = MagicMock(return_value=mock_llm_instance)
                mock_llm.return_value.__exit__ = MagicMock(return_value=False)

                with patch("rag.svr.task_executor_refactor.chunk_post_processor.settings") as mock_settings:
                    mock_settings.retriever.all_tags_in_portion.return_value = {"tag1": 10}

                    await apply_tags(docs, ctx)

                    # Should return early due to cancellation


class TestCountWithKey:
    """Tests for count_with_key function."""

    def test_count_with_key_all_have_key(self):
        """Test counting when all docs have the key."""
        docs = [{"tag": 1}, {"tag": 2}, {"tag": 3}]
        result = count_with_key(docs, "tag")
        assert result == 3

    def test_count_with_key_some_have_key(self):
        """Test counting when some docs have the key."""
        docs = [{"tag": 1}, {"other": 2}, {"tag": 3}]
        result = count_with_key(docs, "tag")
        assert result == 2

    def test_count_with_key_none_have_key(self):
        """Test counting when no docs have the key."""
        docs = [{"other": 1}, {"other": 2}]
        result = count_with_key(docs, "tag")
        assert result == 0

    def test_count_with_key_empty_docs(self):
        """Test counting with empty docs list."""
        result = count_with_key([], "tag")
        assert result == 0

    def test_count_with_key_falsy_value(self):
        """Test counting when key exists but has falsy value."""
        docs = [{"tag": 0}, {"tag": ""}, {"tag": None}]
        result = count_with_key(docs, "tag")
        # Falsy values should not be counted (since d.get(key) returns falsy)
        assert result == 0

    def test_count_with_key_truthy_value(self):
        """Test counting when key has truthy value."""
        docs = [{"tag": 1}, {"tag": "value"}, {"tag": [1, 2]}]
        result = count_with_key(docs, "tag")
        assert result == 3


class TestBuildMetadataConfig:
    """Tests for build_metadata_config function."""

    def test_dict_without_properties_returns_schema(self):
        """When metadata is a dict without properties, return {type: object, properties: {}}."""
        parser_config = {"metadata": {"type": "object"}, "built_in_metadata": []}
        result = build_metadata_config(parser_config)
        assert result == {"type": "object", "properties": {}}

    def test_dict_with_properties_and_built_in(self):
        """When metadata is a dict with properties AND built_in_metadata, merge them."""
        parser_config = {
            "metadata": {"type": "object", "properties": {"a": {"type": "string"}}},
            "built_in_metadata": [{"key": "author", "description": "Author name", "enum": ["alice", "bob"]}],
        }
        result = build_metadata_config(parser_config)
        assert result["type"] == "object"
        assert "a" in result["properties"]
        assert "author" in result["properties"]

    def test_dict_with_properties_no_built_in(self):
        """When metadata is a dict with properties and no built_in, return as-is."""
        parser_config = {
            "metadata": {"type": "object", "properties": {"a": {"type": "string"}}},
            "built_in_metadata": [],
        }
        result = build_metadata_config(parser_config)
        assert result == {"type": "object", "properties": {"a": {"type": "string"}}}

    def test_list_with_built_in(self):
        """When metadata is a list and built_in_metadata is present, concatenate."""
        parser_config = {
            "metadata": [{"key": "category"}],
            "built_in_metadata": [{"key": "author"}],
        }
        result = build_metadata_config(parser_config)
        assert result == [{"key": "category"}, {"key": "author"}]

    def test_list_without_built_in(self):
        """When metadata is a list and built_in_metadata is empty, return metadata as-is."""
        parser_config = {"metadata": [{"key": "category"}], "built_in_metadata": []}
        result = build_metadata_config(parser_config)
        assert result == [{"key": "category"}]

    def test_other_type_with_built_in(self):
        """When metadata is not dict or list (empty list), return built_in_metadata only."""
        parser_config = {"metadata": [], "built_in_metadata": [{"key": "author"}]}
        result = build_metadata_config(parser_config)
        assert result == [{"key": "author"}]

    def test_idempotent_same_input(self):
        """Same input produces structurally equal results."""
        parser_config = {
            "metadata": [{"key": "category"}],
            "built_in_metadata": [{"key": "author"}],
        }
        result1 = build_metadata_config(parser_config)
        result2 = build_metadata_config(parser_config)
        assert result1 == result2

    def test_missing_metadata_key(self):
        """When parser_config has no 'metadata' key, built_in_metadata alone is returned."""
        parser_config = {"built_in_metadata": []}
        result = build_metadata_config(parser_config)
        assert result == []
