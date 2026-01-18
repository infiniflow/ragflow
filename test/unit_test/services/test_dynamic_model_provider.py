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
#
import pytest
import httpx
from unittest.mock import AsyncMock, patch, MagicMock
from api.db.services.openrouter_provider import OpenRouterProvider


@pytest.mark.asyncio
async def test_fetch_models_with_cache():
    """Test that cached models are returned without API call"""
    provider = OpenRouterProvider()

    # Mock Redis cache hit
    with patch.object(provider, '_get_cached_models') as mock_cache, \
         patch('httpx.AsyncClient') as mock_client:
        mock_cache.return_value = [{"id": "test-model", "name": "Test Model"}]

        models, cache_hit = await provider.fetch_available_models()

        assert len(models) == 1
        assert models[0]["id"] == "test-model"
        assert cache_hit is True
        
        # Verify no HTTP client was invoked when cache hit
        mock_client.assert_not_called()


@pytest.mark.asyncio
async def test_fetch_models_api_call():
    """Test that API is called on cache miss"""
    provider = OpenRouterProvider()

    mock_response_data = {
        "data": [
            {
                "id": "anthropic/claude-3.5-sonnet",
                "name": "Claude 3.5 Sonnet",
                "context_length": 200000,
                "pricing": {"prompt": "0.003", "completion": "0.015"},
                "architecture": {"modality": "text"}
            }
        ]
    }

    with patch.object(provider, '_get_cached_models', return_value=None), \
         patch.object(provider, '_cache_models') as mock_cache, \
         patch('httpx.AsyncClient') as mock_client:

        # Setup mock HTTP client
        mock_response = MagicMock()
        mock_response.json.return_value = mock_response_data
        mock_response.raise_for_status = MagicMock()

        mock_get = AsyncMock(return_value=mock_response)
        mock_client.return_value.__aenter__.return_value.get = mock_get

        models, cache_hit = await provider.fetch_available_models()

        assert len(models) == 1
        assert models[0]["id"] == "anthropic/claude-3.5-sonnet"
        assert models[0]["model_type"] == "chat"
        assert cache_hit is False
        mock_cache.assert_called_once()


@pytest.mark.asyncio
async def test_fetch_models_fallback_on_error():
    """Test that fallback models are returned on API error"""
    provider = OpenRouterProvider()

    with patch.object(provider, '_get_cached_models', return_value=None), \
         patch('httpx.AsyncClient') as mock_client:

        # Simulate HTTP error
        async def raise_error(*args, **kwargs):
            raise httpx.HTTPError("API Error")

        mock_client.return_value.__aenter__.return_value.get = raise_error

        models, cache_hit = await provider.fetch_available_models()

        # Should return fallback models
        assert len(models) > 0
        assert any(m["id"] == "anthropic/claude-3.5-sonnet" for m in models)
        assert cache_hit is False


def test_model_type_inference():
    """Test model type inference from OpenRouter metadata"""
    provider = OpenRouterProvider()

    # Vision model
    model = {
        "id": "openai/gpt-4-vision",
        "architecture": {"modality": "text+image"}
    }
    assert provider._infer_model_type(model, model["id"]) == "chat"

    # Embedding model
    model = {
        "id": "openai/text-embedding-ada-002",
        "architecture": {"modality": "text"}
    }
    assert provider._infer_model_type(model, model["id"]) == "embedding"

    # Audio model
    model = {
        "id": "openai/whisper-1",
        "architecture": {"modality": "text+audio"}
    }
    assert provider._infer_model_type(model, model["id"]) == "speech2text"

    # Regular chat model
    model = {
        "id": "anthropic/claude-3-sonnet",
        "architecture": {"modality": "text"}
    }
    assert provider._infer_model_type(model, model["id"]) == "chat"


def test_supports_tools():
    """Test tool support detection"""
    provider = OpenRouterProvider()

    # Should support tools
    assert provider._supports_tools("openai/gpt-4")
    assert provider._supports_tools("anthropic/claude-3-opus")
    assert provider._supports_tools("google/gemini-pro")

    # Should not support tools
    assert not provider._supports_tools("meta-llama/llama-2-7b")


def test_generate_tags():
    """Test tag generation"""
    provider = OpenRouterProvider()

    model = {
        "id": "test-model",
        "context_length": 128000,
        "architecture": {"modality": "text+image"}
    }

    tags = provider._generate_tags(model, "chat")
    assert "LLM" in tags
    assert "CHAT" in tags
    assert "128K" in tags
    assert "IMAGE2TEXT" in tags


def test_cache_key_and_ttl():
    """Test cache configuration"""
    provider = OpenRouterProvider()

    assert provider.get_cache_key() == "openrouter:models"
    assert provider.get_cache_ttl() == 3600


def test_provider_registration():
    """Test that OpenRouter is registered"""
    from api.db.services.dynamic_model_provider import is_dynamic_provider, get_provider

    assert is_dynamic_provider("OpenRouter")
    assert not is_dynamic_provider("OpenAI")

    provider = get_provider("OpenRouter")
    assert provider is not None
    assert isinstance(provider, OpenRouterProvider)

