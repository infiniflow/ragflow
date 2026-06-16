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

Mock strategy: the LLM prompt functions (``keyword_extraction``, ``question_proposal``,
``gen_metadata``, ``content_tagging``) are mocked since they make actual LLM API
calls.  ``get_llm_cache`` / ``set_llm_cache`` run as real code, so cache
population and retrieval are exercised.  ``rag_tokenizer`` is mocked because
it requires NLTK data in the test environment.
"""

import pytest
from unittest.mock import MagicMock, patch
from rag.svr.task_executor_refactor.chunk_post_processor import (
    extract_keywords,
    generate_questions,
    generate_metadata,
    apply_tags,
    count_with_key,
    build_metadata_config,
)
from test.unit_test.rag.svr.task_executor_refactor.conftest import (
    make_task_context,
    MockChatModel,
)


class _BasePostProcessorTest:
    """Shared helpers for post-processor test classes."""

    @staticmethod
    def _mock_llm_binding(chat_model_cls=MockChatModel):
        """Patch model config lookup + LLMBundle to return a MockChatModel."""
        p1 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_model_config_from_provider_instance",
                   return_value=MagicMock())
        p2 = patch("rag.svr.task_executor_refactor.chunk_post_processor.LLMBundle",
                   return_value=chat_model_cls())
        return p1, p2

    @staticmethod
    def _patch_prompt_func(func_path: str, return_value):
        """Patch a prompt-level LLM function (the actual API call)."""
        return patch(func_path, return_value=return_value)


class TestExtractKeywords(_BasePostProcessorTest):
    """Tests for extract_keywords function."""

    @pytest.mark.asyncio
    async def test_extract_keywords_success(self):
        """Test successful keyword extraction — cache miss → LLM prompt runs."""
        ctx = make_task_context(parser_config={"auto_keywords": 5})
        docs = [
            {"content_with_weight": "This is test content one"},
            {"content_with_weight": "This is test content two"},
        ]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value=None)  # cache miss
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")  # Redis stub
        p5 = self._patch_prompt_func(
            "rag.svr.task_executor_refactor.chunk_post_processor.keyword_extraction",
            return_value="keyword1, keyword2",
        )
        p6 = patch("rag.svr.task_executor_refactor.chunk_post_processor.rag_tokenizer")
        with p1, p2, p3, p4, p5, p6 as mock_tok:
            mock_tok.tokenize.return_value = "keyword1 keyword2"
            await extract_keywords(docs, ctx)
            assert "important_kwd" in docs[0]
            assert "important_tks" in docs[0]

    @pytest.mark.asyncio
    async def test_extract_keywords_canceled(self):
        """Test keyword extraction when task is canceled."""
        ctx = make_task_context(parser_config={"auto_keywords": 5},
                                has_canceled_func=MagicMock(return_value=True))
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        with p1, p2:
            await extract_keywords(docs, ctx)
            assert "important_kwd" not in docs[0]

    @pytest.mark.asyncio
    async def test_extract_keywords_empty_docs(self):
        """Test keyword extraction with empty docs list."""
        ctx = make_task_context(parser_config={"auto_keywords": 5})
        docs = []

        p1, p2 = self._mock_llm_binding()
        with p1, p2:
            await extract_keywords(docs, ctx)
            ctx.progress_cb.assert_called()


class TestGenerateQuestions(_BasePostProcessorTest):
    """Tests for generate_questions function."""

    @pytest.mark.asyncio
    async def test_generate_questions_success(self):
        """Test successful question generation — cache miss → LLM prompt runs."""
        ctx = make_task_context(parser_config={"auto_questions": 3})
        docs = [{"content_with_weight": "This is test content one"}]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value=None)
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p5 = self._patch_prompt_func(
            "rag.svr.task_executor_refactor.chunk_post_processor.question_proposal",
            return_value="Question 1\nQuestion 2",
        )
        p6 = patch("rag.svr.task_executor_refactor.chunk_post_processor.rag_tokenizer")
        with p1, p2, p3, p4, p5, p6 as mock_tok:
            mock_tok.tokenize.return_value = "Question 1 Question 2"
            await generate_questions(docs, ctx)
            assert "question_kwd" in docs[0]
            assert "question_tks" in docs[0]

    @pytest.mark.asyncio
    async def test_generate_questions_canceled(self):
        """Test question generation when task is canceled."""
        ctx = make_task_context(parser_config={"auto_questions": 3},
                                has_canceled_func=MagicMock(return_value=True))
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        with p1, p2:
            await generate_questions(docs, ctx)
            assert "question_kwd" not in docs[0]


class TestGenerateMetadata(_BasePostProcessorTest):
    """Tests for generate_metadata function."""

    @pytest.mark.asyncio
    async def test_generate_metadata_success(self):
        """Test successful metadata generation — cache miss → LLM prompt runs."""
        ctx = make_task_context(
            parser_config={
                "enable_metadata": True,
                "metadata": [{"key": "category", "type": "string"}],
                "built_in_metadata": [{"key": "update_time", "type": "time"}],
            },
        )
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value=None)
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService")
        with p1, p2, p3, p4, p5 as mock_meta:
            mock_meta.get_document_metadata.return_value = {}
            mock_meta.update_document_metadata = MagicMock()
            await generate_metadata(docs, ctx)
            mock_meta.update_document_metadata.assert_called_once()

    @pytest.mark.asyncio
    async def test_generate_metadata_with_write_interceptor(self):
        """Test metadata generation with write interceptor."""
        ctx = make_task_context(
            parser_config={
                "enable_metadata": True,
                "metadata": [{"key": "category", "type": "string"}],
                "built_in_metadata": [{"key": "update_time", "type": "time"}],
            },
            write_interceptor=MagicMock(),
        )
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value=None)
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService")
        with p1, p2, p3, p4, p5:
            await generate_metadata(docs, ctx)
            ctx.write_interceptor.intercept.assert_called_once_with(
                "DocMetadataService.update_document_metadata"
            )

    @pytest.mark.asyncio
    async def test_generate_metadata_empty_config_does_not_crash(self):
        """Empty parser_config — no metadata configured — should not crash."""
        ctx = make_task_context(parser_config={})
        docs = [{"content_with_weight": "test"}]
        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService")
        with p1, p2, p3:
            await generate_metadata(docs, ctx)  # no exception = pass

    @pytest.mark.asyncio
    async def test_generate_metadata_enum_none_accepted(self):
        """enum: None in metadata — treated as absent, should not crash."""
        ctx = make_task_context(
            parser_config={
                "enable_metadata": True,
                "metadata": [{"key": "format", "type": "string", "enum": None}],
            },
        )
        docs = [{"content_with_weight": "test"}]
        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value=None)
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService")
        with p1, p2, p3, p4, p5:
            await generate_metadata(docs, ctx)  # no exception = pass

    @pytest.mark.asyncio
    async def test_generate_metadata_description_none_accepted(self):
        """description: None in metadata — should not crash."""
        ctx = make_task_context(
            parser_config={
                "enable_metadata": True,
                "metadata": [{"key": "test", "type": "string", "description": None}],
            },
        )
        docs = [{"content_with_weight": "test"}]
        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value=None)
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService")
        with p1, p2, p3, p4, p5:
            await generate_metadata(docs, ctx)  # no exception = pass

    @pytest.mark.asyncio
    async def test_generate_metadata_built_in_with_enum_none(self):
        """built_in_metadata with enum: None — should not crash."""
        ctx = make_task_context(
            parser_config={
                "enable_metadata": True,
                "built_in_metadata": [
                    {"key": "update_time", "type": "time", "description": None, "enum": None},
                ],
            },
        )
        docs = [{"content_with_weight": "test"}]
        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value=None)
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.DocMetadataService")
        with p1, p2, p3, p4, p5:
            await generate_metadata(docs, ctx)  # no exception = pass


class TestApplyTags(_BasePostProcessorTest):
    """Tests for apply_tags function."""

    @pytest.mark.asyncio
    async def test_apply_tags_success(self):
        """Test successful tag application with tag cache miss."""
        ctx = make_task_context(
            kb_parser_config={"tag_kb_ids": ["kb_1"], "topn_tags": 3},
        )
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.settings")
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value='{"tag1": 1}')  # cache hit → skip LLM
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p6 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_tags_from_cache",
                   return_value=None)
        p7 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_tags_to_cache")
        with p1, p2, p3 as mock_settings, p4, p5, p6 as mock_get_tags, p7 as mock_set_tags:
            mock_settings.retriever.all_tags_in_portion.return_value = {"tag1": 10, "tag2": 5}
            mock_settings.retriever.tag_content.return_value = True
            await apply_tags(docs, ctx)
            assert len(docs) == 1
            mock_get_tags.assert_called_once()
            mock_set_tags.assert_called_once()

    @pytest.mark.asyncio
    async def test_apply_tags_canceled(self):
        """Test tag application when task is canceled."""
        ctx = make_task_context(
            kb_parser_config={"tag_kb_ids": ["kb_1"], "topn_tags": 3},
            has_canceled_func=MagicMock(return_value=True),
        )
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.settings")
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_tags_from_cache",
                   return_value=None)
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_tags_to_cache")
        with p1, p2, p3 as mock_settings, p4, p5:
            mock_settings.retriever.all_tags_in_portion.return_value = {"tag1": 10}
            await apply_tags(docs, ctx)

    @pytest.mark.asyncio
    async def test_apply_tags_tag_cache_miss(self):
        """Test apply_tags when get_tags_from_cache returns None (cache miss)."""
        ctx = make_task_context(
            kb_parser_config={"tag_kb_ids": ["kb_1"], "topn_tags": 3},
        )
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.settings")
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value='{"tag1": 1}')  # cache hit → skip LLM
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p6 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_tags_from_cache",
                   return_value=None)
        p7 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_tags_to_cache")
        with p1, p2, p3 as mock_settings, p4, p5, p6 as mock_get_tags, p7 as mock_set_tags:
            mock_settings.retriever.all_tags_in_portion.return_value = {"tag1": 10, "tag2": 5}
            mock_settings.retriever.tag_content.return_value = True
            await apply_tags(docs, ctx)
            mock_get_tags.assert_called_once_with(["kb_1"])
            mock_set_tags.assert_called_once()
            mock_settings.retriever.all_tags_in_portion.assert_called_once()

    @pytest.mark.asyncio
    async def test_apply_tags_tag_cache_hit(self):
        """Test apply_tags when get_tags_from_cache returns valid data (cache hit)."""
        ctx = make_task_context(
            kb_parser_config={"tag_kb_ids": ["kb_1"], "topn_tags": 3},
        )
        docs = [{"content_with_weight": "This is test content"}]

        p1, p2 = self._mock_llm_binding()
        p3 = patch("rag.svr.task_executor_refactor.chunk_post_processor.settings")
        p4 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_llm_cache",
                   return_value='{"tag1": 1}')  # cache hit → skip LLM
        p5 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_llm_cache")
        p6 = patch("rag.svr.task_executor_refactor.chunk_post_processor.get_tags_from_cache",
                   return_value='{"cached_tag": 10}')
        p7 = patch("rag.svr.task_executor_refactor.chunk_post_processor.set_tags_to_cache")
        with p1, p2, p3 as mock_settings, p4, p5, p6 as mock_get_tags, p7 as mock_set_tags:
            mock_settings.retriever.tag_content.return_value = True
            await apply_tags(docs, ctx)
            mock_get_tags.assert_called_once_with(["kb_1"])
            mock_set_tags.assert_not_called()
            mock_settings.retriever.all_tags_in_portion.assert_not_called()


class TestCountWithKey:
    """Tests for count_with_key function."""

    def test_count_with_key_all_have_key(self):
        docs = [{"tag": 1}, {"tag": 2}, {"tag": 3}]
        assert count_with_key(docs, "tag") == 3

    def test_count_with_key_some_have_key(self):
        docs = [{"tag": 1}, {"other": 2}, {"tag": 3}]
        assert count_with_key(docs, "tag") == 2

    def test_count_with_key_none_have_key(self):
        docs = [{"other": 1}, {"other": 2}]
        assert count_with_key(docs, "tag") == 0

    def test_count_with_key_empty_docs(self):
        assert count_with_key([], "tag") == 0

    def test_count_with_key_falsy_value(self):
        docs = [{"tag": 0}, {"tag": ""}, {"tag": None}]
        assert count_with_key(docs, "tag") == 0

    def test_count_with_key_truthy_value(self):
        docs = [{"tag": 1}, {"tag": "value"}, {"tag": [1, 2]}]
        assert count_with_key(docs, "tag") == 3


class TestBuildMetadataConfig:
    """Tests for build_metadata_config function."""

    def test_dict_without_properties_returns_schema(self):
        parser_config = {"metadata": {"type": "object"}, "built_in_metadata": []}
        assert build_metadata_config(parser_config) == {"type": "object", "properties": {}}

    def test_dict_with_properties_and_built_in(self):
        parser_config = {
            "metadata": {"type": "object", "properties": {"a": {"type": "string"}}},
            "built_in_metadata": [{"key": "author", "description": "Author name", "enum": ["alice", "bob"]}],
        }
        result = build_metadata_config(parser_config)
        assert "a" in result["properties"]
        assert "author" in result["properties"]

    def test_dict_with_properties_no_built_in(self):
        parser_config = {
            "metadata": {"type": "object", "properties": {"a": {"type": "string"}}},
            "built_in_metadata": [],
        }
        result = build_metadata_config(parser_config)
        assert result == {"type": "object", "properties": {"a": {"type": "string"}}}

    def test_list_with_built_in(self):
        parser_config = {
            "metadata": [{"key": "category"}],
            "built_in_metadata": [{"key": "author"}],
        }
        assert build_metadata_config(parser_config) == [{"key": "category"}, {"key": "author"}]

    def test_list_without_built_in(self):
        parser_config = {"metadata": [{"key": "category"}], "built_in_metadata": []}
        assert build_metadata_config(parser_config) == [{"key": "category"}]

    def test_other_type_with_built_in(self):
        parser_config = {"metadata": [], "built_in_metadata": [{"key": "author"}]}
        assert build_metadata_config(parser_config) == [{"key": "author"}]

    def test_idempotent_same_input(self):
        parser_config = {
            "metadata": [{"key": "category"}],
            "built_in_metadata": [{"key": "author"}],
        }
        assert build_metadata_config(parser_config) == build_metadata_config(parser_config)

    def test_missing_metadata_key(self):
        assert build_metadata_config({"built_in_metadata": []}) == []

    def test_metadata_is_none(self):
        """When metadata is None, built_in_metadata alone is returned."""
        parser_config = {"metadata": None, "built_in_metadata": [{"key": "author"}]}
        result = build_metadata_config(parser_config)
        assert result == [{"key": "author"}]
