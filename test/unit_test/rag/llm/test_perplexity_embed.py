#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import base64
from unittest.mock import patch, MagicMock

import numpy as np
import pytest

from rag.llm.embedding_model import PerplexityEmbed


def _make_b64_int8(values):
    """Helper: encode a list of int8 values to base64 string."""
    arr = np.array(values, dtype=np.int8)
    return base64.b64encode(arr.tobytes()).decode()


def _mock_standard_response(embeddings_b64, total_tokens=10):
    """Build a mock JSON response for the standard embeddings endpoint."""
    return {
        "object": "list",
        "data": [{"object": "embedding", "index": i, "embedding": emb} for i, emb in enumerate(embeddings_b64)],
        "model": "pplx-embed-v1-0.6b",
        "usage": {"total_tokens": total_tokens},
    }


def _mock_contextualized_response(docs_embeddings_b64, total_tokens=20):
    """Build a mock JSON response for the contextualized embeddings endpoint."""
    data = []
    for doc_idx, chunks in enumerate(docs_embeddings_b64):
        data.append(
            {
                "index": doc_idx,
                "data": [{"object": "embedding", "index": chunk_idx, "embedding": emb} for chunk_idx, emb in enumerate(chunks)],
            }
        )
    return {
        "object": "list",
        "data": data,
        "model": "pplx-embed-context-v1-0.6b",
        "usage": {"total_tokens": total_tokens},
    }


class TestPerplexityEmbedInit:
    def test_default_base_url(self):
        embed = PerplexityEmbed("test-key", "pplx-embed-v1-0.6b")
        assert embed.base_url == "https://api.perplexity.ai"
        assert embed.api_key == "test-key"
        assert embed.model_name == "pplx-embed-v1-0.6b"

    def test_custom_base_url(self):
        embed = PerplexityEmbed("key", "pplx-embed-v1-4b", base_url="https://custom.api.com/")
        assert embed.base_url == "https://custom.api.com"

    def test_empty_base_url_uses_default(self):
        embed = PerplexityEmbed("key", "pplx-embed-v1-0.6b", base_url="")
        assert embed.base_url == "https://api.perplexity.ai"

    def test_auth_header(self):
        embed = PerplexityEmbed("my-secret-key", "pplx-embed-v1-0.6b")
        assert embed.headers["Authorization"] == "Bearer my-secret-key"


class TestPerplexityEmbedModelDetection:
    def test_standard_model_not_contextualized(self):
        embed = PerplexityEmbed("key", "pplx-embed-v1-0.6b")
        assert not embed._is_contextualized()

    def test_standard_4b_not_contextualized(self):
        embed = PerplexityEmbed("key", "pplx-embed-v1-4b")
        assert not embed._is_contextualized()

    def test_contextualized_0_6b(self):
        embed = PerplexityEmbed("key", "pplx-embed-context-v1-0.6b")
        assert embed._is_contextualized()

    def test_contextualized_4b(self):
        embed = PerplexityEmbed("key", "pplx-embed-context-v1-4b")
        assert embed._is_contextualized()


class TestDecodeBase64Int8:
    def test_basic_decode(self):
        values = [-1, 0, 1, 127]
        b64 = _make_b64_int8(values)
        result = PerplexityEmbed._decode_base64_int8(b64)
        expected = np.array(values, dtype=np.float32)
        np.testing.assert_array_equal(result, expected)

    def test_empty_decode(self):
        b64 = base64.b64encode(b"").decode()
        result = PerplexityEmbed._decode_base64_int8(b64)
        assert len(result) == 0

    def test_full_range(self):
        values = list(range(-128, 128))
        b64 = _make_b64_int8(values)
        result = PerplexityEmbed._decode_base64_int8(b64)
        expected = np.array(values, dtype=np.float32)
        np.testing.assert_array_equal(result, expected)

    def test_output_dtype_is_float32(self):
        b64 = _make_b64_int8([1, 2, 3])
        result = PerplexityEmbed._decode_base64_int8(b64)
        assert result.dtype == np.float32


class TestPerplexityEmbedStandardEncode:
    @patch("rag.llm.embedding_model.requests.post")
    def test_encode_single_text(self, mock_post):
        emb_b64 = _make_b64_int8([10, 20, 30])
        mock_resp = MagicMock()
        mock_resp.json.return_value = _mock_standard_response([emb_b64], total_tokens=5)
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-v1-0.6b")
        result, tokens = embed.encode(["hello"])

        assert result.shape == (1, 3)
        np.testing.assert_array_equal(result[0], np.array([10, 20, 30], dtype=np.float32))
        assert tokens == 5
        mock_post.assert_called_once()
        call_url = mock_post.call_args[0][0]
        assert call_url == "https://api.perplexity.ai/v1/embeddings"

    @patch("rag.llm.embedding_model.requests.post")
    def test_encode_multiple_texts(self, mock_post):
        emb1 = _make_b64_int8([1, 2])
        emb2 = _make_b64_int8([3, 4])
        emb3 = _make_b64_int8([5, 6])
        mock_resp = MagicMock()
        mock_resp.json.return_value = _mock_standard_response([emb1, emb2, emb3], total_tokens=15)
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-v1-0.6b")
        result, tokens = embed.encode(["a", "b", "c"])

        assert result.shape == (3, 2)
        assert tokens == 15

    @patch("rag.llm.embedding_model.requests.post")
    def test_encode_sends_correct_payload(self, mock_post):
        mock_resp = MagicMock()
        mock_resp.json.return_value = _mock_standard_response([_make_b64_int8([1])], total_tokens=1)
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-v1-4b")
        embed.encode(["test text"])

        call_kwargs = mock_post.call_args
        payload = call_kwargs[1]["json"]
        assert payload["model"] == "pplx-embed-v1-4b"
        assert payload["input"] == ["test text"]
        assert payload["encoding_format"] == "base64_int8"

    @patch("rag.llm.embedding_model.requests.post")
    def test_encode_api_error_raises(self, mock_post):
        mock_resp = MagicMock()
        mock_resp.json.side_effect = Exception("Invalid JSON")
        mock_resp.text = "Internal Server Error"
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-v1-0.6b")
        with pytest.raises(Exception, match="Error"):
            embed.encode(["hello"])


class TestPerplexityEmbedContextualizedEncode:
    @patch("rag.llm.embedding_model.requests.post")
    def test_contextualized_encode(self, mock_post):
        emb1 = _make_b64_int8([10, 20])
        emb2 = _make_b64_int8([30, 40])
        mock_resp = MagicMock()
        mock_resp.json.return_value = _mock_contextualized_response([[emb1], [emb2]], total_tokens=12)
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-context-v1-0.6b")
        result, tokens = embed.encode(["chunk1", "chunk2"])

        assert result.shape == (2, 2)
        np.testing.assert_array_equal(result[0], np.array([10, 20], dtype=np.float32))
        np.testing.assert_array_equal(result[1], np.array([30, 40], dtype=np.float32))
        assert tokens == 12

    @patch("rag.llm.embedding_model.requests.post")
    def test_contextualized_uses_correct_endpoint(self, mock_post):
        mock_resp = MagicMock()
        mock_resp.json.return_value = _mock_contextualized_response([[_make_b64_int8([1])]], total_tokens=1)
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-context-v1-4b")
        embed.encode(["chunk"])

        call_url = mock_post.call_args[0][0]
        assert call_url == "https://api.perplexity.ai/v1/contextualizedembeddings"

    @patch("rag.llm.embedding_model.requests.post")
    def test_contextualized_sends_nested_input(self, mock_post):
        mock_resp = MagicMock()
        mock_resp.json.return_value = _mock_contextualized_response([[_make_b64_int8([1])]], total_tokens=1)
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-context-v1-0.6b")
        embed.encode(["text1"])

        payload = mock_post.call_args[1]["json"]
        assert payload["input"] == [["text1"]]
        assert payload["model"] == "pplx-embed-context-v1-0.6b"


class TestPerplexityEmbedEncodeQueries:
    @patch("rag.llm.embedding_model.requests.post")
    def test_encode_queries_returns_single_vector(self, mock_post):
        emb = _make_b64_int8([5, 10, 15, 20])
        mock_resp = MagicMock()
        mock_resp.json.return_value = _mock_standard_response([emb], total_tokens=3)
        mock_post.return_value = mock_resp

        embed = PerplexityEmbed("key", "pplx-embed-v1-0.6b")
        result, tokens = embed.encode_queries("search query")

        assert result.shape == (4,)
        np.testing.assert_array_equal(result, np.array([5, 10, 15, 20], dtype=np.float32))
        assert tokens == 3


class TestPerplexityEmbedFactoryRegistration:
    def test_factory_name(self):
        assert PerplexityEmbed._FACTORY_NAME == "Perplexity"

    def test_is_subclass_of_base(self):
        from rag.llm.embedding_model import Base

        assert issubclass(PerplexityEmbed, Base)
