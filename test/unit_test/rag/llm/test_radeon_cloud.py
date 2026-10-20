import json
from pathlib import Path

import httpx
import pytest
from openai import AsyncOpenAI

from rag.llm.chat_model import RadeonCloudChat
from rag.llm.model_meta import RadeonCloud


def test_radeon_cloud_provider_registration_and_defaults():
    assert RadeonCloudChat._FACTORY_NAME == RadeonCloud._FACTORY_NAME == "Radeon Cloud"
    factories = json.loads((Path(__file__).resolve().parents[4] / "conf/llm_factories.json").read_text())["factory_llm_infos"]
    factory = next(factory for factory in factories if factory["name"] == "Radeon Cloud")

    chat = RadeonCloudChat("test-key", "model-id", None)
    assert chat.base_url == "https://developer.amd.com.cn/radeon/api/v1"
    assert factory["url"] == chat.base_url
    assert str(chat.async_client.base_url).rstrip("/") == chat.base_url


def test_radeon_cloud_model_catalog_uses_openai_compatible_format():
    provider = RadeonCloud(api_key="test-key", base_url="https://developer.amd.com.cn/radeon/api/v1")
    assert provider._get_model_list_url() == "https://developer.amd.com.cn/radeon/api/v1/models"
    assert provider._format_model_list({"data": [{"id": "amd/model"}]}) == [{"name": "amd/model", "model_types": ["chat"], "features": [], "max_tokens": 8192}]


@pytest.mark.asyncio
async def test_radeon_cloud_chat_request():
    def respond(request):
        assert str(request.url) == "https://developer.amd.com.cn/radeon/api/v1/chat/completions"
        assert request.headers["Authorization"] == "Bearer rc-test"
        assert json.loads(request.content)["model"] == "DeepSeek-V4-Flash"
        return httpx.Response(
            200,
            json={
                "id": "test",
                "object": "chat.completion",
                "created": 0,
                "model": "DeepSeek-V4-Flash",
                "choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello"}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
            },
        )

    chat = RadeonCloudChat("rc-test", "DeepSeek-V4-Flash")
    async with httpx.AsyncClient(transport=httpx.MockTransport(respond)) as client:
        chat.async_client = AsyncOpenAI(api_key="rc-test", base_url=chat.base_url, http_client=client)
        answer, tokens = await chat.async_chat("", [{"role": "user", "content": "Hi"}])
    assert answer == "Hello"
    assert tokens == 2
