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

import os
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from rag.llm.cv_model import TwelveLabsCV
from rag.llm.embedding_model import EmbeddingError, TwelveLabsEmbed


def _mock_embedding_response(vector):
    """Build a mock twelvelabs EmbeddingResponse with a single text segment."""
    seg = MagicMock()
    seg.float_ = list(vector)
    res = MagicMock()
    res.text_embedding.segments = [seg]
    return res


class TestTwelveLabsEmbed:
    def test_defaults_to_marengo(self):
        with patch("twelvelabs.TwelveLabs"):
            embed = TwelveLabsEmbed("key", "")
            assert embed.model_name == "marengo3.0"

    def test_encode_returns_matrix(self):
        vec = [0.1] * 512
        with patch("twelvelabs.TwelveLabs") as Client:
            Client.return_value.embed.create.return_value = _mock_embedding_response(vec)
            embed = TwelveLabsEmbed("key", "marengo3.0")
            mat, tokens = embed.encode(["a", "b"])
        assert mat.shape == (2, 512)
        assert tokens > 0
        assert Client.return_value.embed.create.call_count == 2

    def test_encode_queries_returns_vector(self):
        vec = [0.2] * 512
        with patch("twelvelabs.TwelveLabs") as Client:
            Client.return_value.embed.create.return_value = _mock_embedding_response(vec)
            embed = TwelveLabsEmbed("key", "marengo3.0")
            arr, tokens = embed.encode_queries("hello")
        assert isinstance(arr, np.ndarray)
        assert arr.shape == (512,)
        assert tokens > 0

    def test_empty_response_raises(self):
        res = MagicMock()
        res.text_embedding = None
        with patch("twelvelabs.TwelveLabs") as Client:
            Client.return_value.embed.create.return_value = res
            embed = TwelveLabsEmbed("key", "marengo3.0")
            with pytest.raises(EmbeddingError):
                embed.encode_queries("x")


class TestTwelveLabsCV:
    def test_defaults_to_pegasus(self):
        with patch("twelvelabs.TwelveLabs"):
            cv = TwelveLabsCV("key", "")
            assert cv.model_name == "pegasus1.5"

    def test_describe_with_url(self):
        analyze_res = MagicMock()
        analyze_res.data = "A person rides a bike through a city."
        analyze_res.usage.output_tokens = 9
        with patch("twelvelabs.TwelveLabs") as Client:
            Client.return_value.analyze.return_value = analyze_res
            cv = TwelveLabsCV("key", "pegasus1.5")
            text, tokens = cv.describe_with_prompt("https://example.com/clip.mp4", "Describe it.")
        assert "bike" in text
        assert tokens == 9
        # The video argument must be a url-typed VideoContext.
        _, kwargs = Client.return_value.analyze.call_args
        assert kwargs["video"].type == "url"
        assert kwargs["video"].url == "https://example.com/clip.mp4"

    def test_asset_reference(self):
        analyze_res = MagicMock()
        analyze_res.data = "ok"
        analyze_res.usage = None
        with patch("twelvelabs.TwelveLabs") as Client:
            Client.return_value.analyze.return_value = analyze_res
            cv = TwelveLabsCV("key", "pegasus1.5")
            cv.describe_with_prompt("tl-asset:abc123", "Summarize.")
        _, kwargs = Client.return_value.analyze.call_args
        assert kwargs["video"].type == "asset_id"
        assert kwargs["video"].asset_id == "abc123"

    def test_rejects_bad_string(self):
        with patch("twelvelabs.TwelveLabs"):
            cv = TwelveLabsCV("key", "pegasus1.5")
            with pytest.raises(ValueError):
                cv.describe_with_prompt("not a url or asset", "Describe.")


@pytest.mark.skipif(not os.environ.get("TWELVELABS_API_KEY"), reason="TWELVELABS_API_KEY not set")
class TestTwelveLabsLive:
    def test_marengo_text_embedding_is_512_dim(self):
        embed = TwelveLabsEmbed(os.environ["TWELVELABS_API_KEY"], "marengo3.0")
        arr, tokens = embed.encode_queries("a cat playing the piano")
        assert arr.shape == (512,)
        assert tokens > 0
