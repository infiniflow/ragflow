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
"""Integration tests for dynamic model discovery feature"""
import pytest
from unittest.mock import patch, AsyncMock


@pytest.mark.p2
def test_factories_includes_is_dynamic_flag(api_client):
    """Test that /factories endpoint includes is_dynamic flag"""
    response = api_client.get("/api/llm/factories")

    assert response.status_code == 200
    data = response.json()
    assert data["code"] == 0

    factories = data["data"]
    assert len(factories) > 0

    # Find OpenRouter factory
    openrouter = next((f for f in factories if f["name"] == "OpenRouter"), None)
    assert openrouter is not None, "OpenRouter factory must be present in factories list"
    assert "is_dynamic" in openrouter
    assert openrouter["is_dynamic"]


@pytest.mark.p2
def test_get_openrouter_models_dynamic(api_client):
    """Test fetching OpenRouter models via API (mocked)"""
    from api.db.services.openrouter_provider import OpenRouterProvider

    mock_models = [
        {
            "id": "anthropic/claude-3.5-sonnet",
            "llm_name": "anthropic/claude-3.5-sonnet",
            "name": "Claude 3.5 Sonnet",
            "model_type": "chat",
            "max_tokens": 200000,
            "is_tools": True,
            "pricing": {"prompt": 0.003, "completion": 0.015},
            "tags": "LLM,CHAT,200K",
            "architecture": {}
        }
    ]

    with patch.object(OpenRouterProvider, 'fetch_available_models', new_callable=AsyncMock) as mock_fetch:
        mock_fetch.return_value = (mock_models, False)

        response = api_client.get("/api/llm/factories/OpenRouter/models")

        assert response.status_code == 200
        data = response.json()
        assert data["code"] == 0
        assert "models" in data["data"]
        assert data["data"]["is_dynamic"]
        assert len(data["data"]["models"]) > 0
        
        # Verify the mock was actually called
        mock_fetch.assert_awaited_once()
        
        # Verify the mocked model data is actually returned in the response
        resp_model = data["data"]["models"][0]
        assert resp_model["id"] == mock_models[0]["id"]
        assert resp_model["llm_name"] == mock_models[0]["llm_name"]
        assert resp_model["name"] == mock_models[0]["name"]
        assert resp_model["max_tokens"] == mock_models[0]["max_tokens"]
        assert resp_model["pricing"]["prompt"] == mock_models[0]["pricing"]["prompt"]
        assert resp_model["pricing"]["completion"] == mock_models[0]["pricing"]["completion"]


@pytest.mark.p2
def test_get_static_provider_models(api_client):
    """Test fetching models for static provider returns is_dynamic: false"""
    response = api_client.get("/api/llm/factories/OpenAI/models")

    assert response.status_code == 200
    data = response.json()
    assert data["code"] == 0
    assert "models" in data["data"]
    assert not data["data"]["is_dynamic"]


@pytest.mark.p2
def test_cache_refresh_parameter(api_client):
    """Test that refresh parameter works and reflects actual cache behavior"""
    from api.db.services.openrouter_provider import OpenRouterProvider

    mock_models = [{"id": "test-model"}]

    with patch.object(OpenRouterProvider, 'fetch_available_models', new_callable=AsyncMock) as mock_fetch:
        # Configure side_effect to simulate cache hit on first call, cache miss on second
        # First call returns cached=True (cache hit), second returns cached=False (after refresh)
        mock_fetch.side_effect = [
            (mock_models, True),   # First call: cache hit
            (mock_models, False),  # Second call: fresh fetch after refresh
        ]

        # First call without refresh - should return cached data
        response1 = api_client.get("/api/llm/factories/OpenRouter/models")
        assert response1.status_code == 200
        assert response1.json()["data"]["cached"] is True

        # Second call with refresh - should fetch fresh data
        response2 = api_client.get("/api/llm/factories/OpenRouter/models?refresh=true")
        assert response2.status_code == 200
        assert response2.json()["data"]["cached"] is False


@pytest.mark.p3
def test_provider_registration():
    """Test that dynamic providers are correctly registered"""
    from api.db.services.dynamic_model_provider import (
        is_dynamic_provider,
        get_provider,
        DYNAMIC_PROVIDERS
    )
    from api.db.services.openrouter_provider import OpenRouterProvider

    # OpenRouter should be registered
    assert "OpenRouter" in DYNAMIC_PROVIDERS
    assert is_dynamic_provider("OpenRouter")

    # Get provider instance
    provider = get_provider("OpenRouter")
    assert provider is not None
    assert isinstance(provider, OpenRouterProvider)

    # Static providers should not be dynamic
    assert not is_dynamic_provider("OpenAI")
    assert not is_dynamic_provider("Anthropic")

