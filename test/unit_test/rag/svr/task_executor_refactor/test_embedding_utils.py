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
Unit tests for EmbeddingUtils module.
"""

import numpy as np
from unittest.mock import patch
from rag.svr.task_executor_refactor.embedding_utils import EmbeddingUtils


class TestEmbeddingUtilsPrepareTexts:
    """Tests for prepare_texts_for_embedding class method."""

    def test_prepare_texts_basic(self):
        """Test basic text preparation."""
        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
            {"docnm_kwd": "Title2", "content_with_weight": "Content2"},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)
        assert titles == ["Title1", "Title2"]
        assert contents == ["Content1", "Content2"]

    def test_prepare_texts_with_question_kwd(self):
        """Test text preparation with question_kwd."""
        docs = [
            {"docnm_kwd": "Title1", "question_kwd": ["Q1", "Q2"], "content_with_weight": "Content1"},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)
        assert titles == ["Title1"]
        assert contents == ["Q1\nQ2"]

    def test_prepare_texts_with_empty_question_kwd(self):
        """Test text preparation with empty question_kwd falls back to content."""
        docs = [
            {"docnm_kwd": "Title1", "question_kwd": [], "content_with_weight": "Content1"},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)
        assert contents == ["Content1"]

    def test_prepare_texts_with_missing_question_kwd(self):
        """Test text preparation without question_kwd uses content."""
        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "Content1"},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)
        assert contents == ["Content1"]

    def test_prepare_texts_normalizes_table_html(self):
        """Test that table HTML tags are normalized."""
        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "<table><tr><td>Cell</td></tr></table>"},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)
        # Table tags should be replaced with spaces
        assert "<table>" not in contents[0]

    def test_prepare_texts_whitespace_only_becomes_none(self):
        """Test that whitespace-only content becomes 'None'."""
        docs = [
            {"docnm_kwd": "Title1", "content_with_weight": "   \n\n  "},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)
        assert contents == ["None"]

    def test_prepare_texts_default_title(self):
        """Test that missing docnm_kwd uses 'Title' as default."""
        docs = [
            {"content_with_weight": "Content1"},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs)
        assert titles == ["Title"]

    def test_prepare_texts_without_question_kwd(self):
        """Test text preparation with use_question_kwd=False."""
        docs = [
            {"docnm_kwd": "Title1", "question_kwd": ["Q1"], "content_with_weight": "Content1"},
        ]
        titles, contents = EmbeddingUtils.prepare_texts_for_embedding(docs, use_question_kwd=False)
        assert contents == ["Content1"]


class TestEmbeddingUtilsPrepareDataflowTexts:
    """Tests for prepare_texts_for_dataflow_embedding class method."""

    def test_prepare_dataflow_texts_with_questions(self):
        """Test dataflow text preparation with questions field."""
        chunks = [
            {"questions": "Q1\nQ2"},
            {"questions": "Q3"},
        ]
        texts = EmbeddingUtils.prepare_texts_for_dataflow_embedding(chunks)
        assert texts == ["Q1\nQ2", "Q3"]

    def test_prepare_dataflow_texts_with_summary(self):
        """Test dataflow text preparation with summary field (no questions)."""
        chunks = [
            {"summary": "Summary1"},
        ]
        texts = EmbeddingUtils.prepare_texts_for_dataflow_embedding(chunks)
        assert texts == ["Summary1"]

    def test_prepare_dataflow_texts_with_text(self):
        """Test dataflow text preparation with text field (no questions/summary)."""
        chunks = [
            {"text": "Text content"},
        ]
        texts = EmbeddingUtils.prepare_texts_for_dataflow_embedding(chunks)
        assert texts == ["Text content"]

    def test_prepare_dataflow_texts_priority(self):
        """Test field priority: questions > summary > text."""
        chunks = [
            {"questions": "Q", "summary": "S", "text": "T"},
        ]
        texts = EmbeddingUtils.prepare_texts_for_dataflow_embedding(chunks)
        assert texts == ["Q"]

        chunks = [
            {"summary": "S", "text": "T"},
        ]
        texts = EmbeddingUtils.prepare_texts_for_dataflow_embedding(chunks)
        assert texts == ["S"]


class TestEmbeddingUtilsTruncateTexts:
    """Tests for truncate_texts class method."""

    @patch("rag.svr.task_executor_refactor.embedding_utils.truncate")
    def test_truncate_texts_calls_truncate(self, mock_truncate):
        """Test truncate_texts calls truncate with correct max_length."""
        mock_truncate.return_value = "truncated"
        texts = ["long text 1", "long text 2"]
        max_length = 100

        _ = EmbeddingUtils.truncate_texts(texts, max_length)

        assert mock_truncate.call_count == 2
        # Should subtract 10 for safety margin
        mock_truncate.assert_called_with("long text 2", 90)

    @patch("rag.svr.task_executor_refactor.embedding_utils.truncate")
    def test_truncate_texts_returns_list(self, mock_truncate):
        """Test truncate_texts returns a list of same length."""
        mock_truncate.return_value = "truncated"
        texts = ["text1", "text2", "text3"]
        result = EmbeddingUtils.truncate_texts(texts, 50)
        assert len(result) == 3


class TestEmbeddingUtilsStackVectors:
    """Tests for stack_vectors class method."""

    def test_stack_vectors_with_multiple_batches(self):
        """Test stacking multiple vector batches."""
        batch1 = np.array([[1.0, 2.0], [3.0, 4.0]])
        batch2 = np.array([[5.0, 6.0]])
        result = EmbeddingUtils.stack_vectors([batch1, batch2])
        expected = np.array([[1.0, 2.0], [3.0, 4.0], [5.0, 6.0]])
        np.testing.assert_array_equal(result, expected)

    def test_stack_vectors_with_empty_batches(self):
        """Test stacking empty batches returns empty array."""
        result = EmbeddingUtils.stack_vectors([])
        assert result.size == 0

    def test_stack_vectors_with_single_batch(self):
        """Test stacking a single batch."""
        batch = np.array([[1.0, 2.0]])
        result = EmbeddingUtils.stack_vectors([batch])
        np.testing.assert_array_equal(result, batch)


class TestEmbeddingUtilsAttachVectors:
    """Tests for attach_vectors class method."""

    def test_attach_vectors_basic(self):
        """Test attaching vectors to docs."""
        docs = [{"id": 1}, {"id": 2}]
        vectors = np.array([[1.0, 2.0, 3.0], [4.0, 5.0, 6.0]])

        vector_size = EmbeddingUtils.attach_vectors(docs, vectors)

        assert vector_size == 3
        assert "q_3_vec" in docs[0]
        assert "q_3_vec" in docs[1]
        assert docs[0]["q_3_vec"] == [1.0, 2.0, 3.0]
        assert docs[1]["q_3_vec"] == [4.0, 5.0, 6.0]

    def test_attach_vectors_custom_key_template(self):
        """Test attaching vectors with custom key template."""
        docs = [{"id": 1}]
        vectors = np.array([[1.0, 2.0]])

        EmbeddingUtils.attach_vectors(docs, vectors, vector_key_template="vec_%d")

        assert "vec_2" in docs[0]

    def test_attach_vectors_modifies_in_place(self):
        """Test that attach_vectors modifies docs in place."""
        docs = [{"id": 1}]
        vectors = np.array([[1.0, 2.0]])
        original_id = id(docs)

        EmbeddingUtils.attach_vectors(docs, vectors)

        assert id(docs) == original_id


class TestEmbeddingUtilsCombineVectors:
    """Tests for combine_title_content_vectors class method."""

    def test_combine_vectors_with_title_and_content(self):
        """Test combining title and content vectors with weight."""
        title_vecs = np.array([[1.0, 2.0], [3.0, 4.0]])
        content_vecs = np.array([[5.0, 6.0], [7.0, 8.0]])

        result = EmbeddingUtils.combine_title_content_vectors(title_vecs, content_vecs, title_weight=0.3)

        # Expected: 0.3 * title + 0.7 * content
        expected = 0.3 * title_vecs + 0.7 * content_vecs
        np.testing.assert_array_almost_equal(result, expected)

    def test_combine_vectors_with_default_weight(self):
        """Test combining with default weight when not specified."""
        title_vecs = np.array([[1.0, 2.0]])
        content_vecs = np.array([[5.0, 6.0]])

        result = EmbeddingUtils.combine_title_content_vectors(title_vecs, content_vecs)

        # Expected: 0.1 * title + 0.9 * content (default weight is 0.1)
        expected = 0.1 * title_vecs + 0.9 * content_vecs
        np.testing.assert_array_almost_equal(result, expected)

    def test_combine_vectors_with_none_title(self):
        """Test combining when title vectors is None returns content."""
        content_vecs = np.array([[5.0, 6.0]])

        result = EmbeddingUtils.combine_title_content_vectors(None, content_vecs, title_weight=0.3)

        np.testing.assert_array_equal(result, content_vecs)

    def test_combine_vectors_with_mismatched_shapes(self):
        """Test combining when shapes don't match returns content."""
        title_vecs = np.array([[1.0, 2.0]])
        content_vecs = np.array([[5.0, 6.0], [7.0, 8.0]])

        result = EmbeddingUtils.combine_title_content_vectors(title_vecs, content_vecs, title_weight=0.3)

        # Should return content_vecs when shapes don't match
        np.testing.assert_array_equal(result, content_vecs)

    def test_combine_vectors_with_zero_weight(self):
        """Test combining when weight is 0 uses default 0.1."""
        title_vecs = np.array([[1.0, 2.0]])
        content_vecs = np.array([[5.0, 6.0]])

        result = EmbeddingUtils.combine_title_content_vectors(title_vecs, content_vecs, title_weight=0)

        # Should use default weight of 0.1
        expected = 0.1 * title_vecs + 0.9 * content_vecs
        np.testing.assert_array_almost_equal(result, expected)


class TestEmbeddingUtilsInternals:
    """Tests for internal helper methods."""

    def test_extract_content_with_question_kwd(self):
        """Test _extract_content with question_kwd."""
        doc = {"question_kwd": ["Q1", "Q2"], "content_with_weight": "Content"}
        result = EmbeddingUtils._extract_content(doc, use_question_kwd=True)
        assert result == "Q1\nQ2"

    def test_extract_content_without_question_kwd(self):
        """Test _extract_content without question_kwd."""
        doc = {"content_with_weight": "Content"}
        result = EmbeddingUtils._extract_content(doc, use_question_kwd=True)
        assert result == "Content"

    def test_extract_content_with_use_question_false(self):
        """Test _extract_content with use_question_kwd=False."""
        doc = {"question_kwd": ["Q1"], "content_with_weight": "Content"}
        result = EmbeddingUtils._extract_content(doc, use_question_kwd=False)
        assert result == "Content"

    def test_normalize_table_html(self):
        """Test _normalize_table_html removes table tags."""
        html = "<table><tr><td>Cell</td></tr></table>"
        result = EmbeddingUtils._normalize_table_html(html)
        assert "<table>" not in result
        assert "<tr>" not in result
        assert "<td>" not in result

    def test_handle_whitespace(self):
        """Test _handle_whitespace replaces whitespace-only with placeholder."""
        assert EmbeddingUtils._handle_whitespace("   \n  ") == "None"
        assert EmbeddingUtils._handle_whitespace("  text  ") == "  text  "

    def test_handle_whitespace_with_empty_string(self):
        """Test _handle_whitespace with empty string."""
        assert EmbeddingUtils._handle_whitespace("") == "None"


class TestEmbeddingUtilsConstants:
    """Tests for class constants."""

    def test_default_title_weight(self):
        """Test DEFAULT_TITLE_WEIGHT value."""
        assert EmbeddingUtils.DEFAULT_TITLE_WEIGHT == 0.1

    def test_default_title_placeholder(self):
        """Test DEFAULT_TITLE_PLACEHOLDER value."""
        assert EmbeddingUtils.DEFAULT_TITLE_PLACEHOLDER == "Title"

    def test_content_placeholder_for_whitespace(self):
        """Test CONTENT_PLACEHOLDER_FOR_WHITESPACE value."""
        assert EmbeddingUtils.CONTENT_PLACEHOLDER_FOR_WHITESPACE == "None"
