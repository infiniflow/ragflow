import inspect

import pytest

from api.db.services import llm_service


class _MutatingChatModel:
    def __init__(self):
        self.received = []

    async def async_chat(self, _system, _history, gen_conf):
        self.received.append(dict(gen_conf))
        gen_conf["provider_mutation"] = True
        return "ok", 0


@pytest.mark.asyncio
async def test_async_chat_default_gen_conf_is_isolated_between_calls(monkeypatch):
    model = _MutatingChatModel()
    bundle = llm_service.LLMBundle.__new__(llm_service.LLMBundle)
    bundle.mdl = model
    bundle.is_tools = False
    bundle.langfuse = None
    bundle.verbose_tool_use = False
    bundle.model_config = {"llm_name": "fake"}

    monkeypatch.setattr(llm_service, "record_run_token_usage", lambda *_args: None)

    await bundle.async_chat("", [])
    await bundle.async_chat("", [])

    assert model.received == [{}, {}]


@pytest.mark.parametrize(
    "method_name",
    ["async_chat", "async_chat_streamly", "async_chat_streamly_delta"],
)
def test_llm_bundle_gen_conf_defaults_to_none(method_name):
    parameter = inspect.signature(getattr(llm_service.LLMBundle, method_name)).parameters["gen_conf"]

    assert parameter.default is None
