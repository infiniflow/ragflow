import inspect
import logging

import pytest

from api.db.services import llm_service

LOGGER = logging.getLogger(__name__)


class _MutatingChatModel:
    def __init__(self):
        self.chat_received = []
        self.stream_received = []

    async def async_chat(self, _system, _history, gen_conf):
        self.chat_received.append(dict(gen_conf))
        LOGGER.debug("Mutating provider generation config in async_chat")
        gen_conf["provider_mutation"] = True
        return "ok", 0

    async def async_chat_streamly(self, _system, _history, gen_conf):
        self.stream_received.append(dict(gen_conf))
        LOGGER.debug("Mutating provider generation config in async_chat_streamly")
        gen_conf["provider_mutation"] = True
        yield "ok"


def _bundle(model):
    bundle = llm_service.LLMBundle.__new__(llm_service.LLMBundle)
    bundle.mdl = model
    bundle.is_tools = False
    bundle.langfuse = None
    bundle.verbose_tool_use = False
    bundle.model_config = {"llm_name": "fake"}
    return bundle


@pytest.mark.asyncio
async def test_async_chat_default_gen_conf_is_isolated_between_calls(monkeypatch):
    model = _MutatingChatModel()
    bundle = _bundle(model)
    caller_conf = {"temperature": 0.3}

    monkeypatch.setattr(llm_service, "record_run_token_usage", lambda *_args: None)

    await bundle.async_chat("", [])
    await bundle.async_chat("", [])
    await bundle.async_chat("", [], caller_conf)

    assert model.chat_received == [{}, {}, {"temperature": 0.3}]
    assert caller_conf == {"temperature": 0.3}


@pytest.mark.parametrize("method_name", ["async_chat_streamly", "async_chat_streamly_delta"])
@pytest.mark.asyncio
async def test_streaming_gen_conf_is_isolated_and_does_not_mutate_caller(method_name, monkeypatch):
    model = _MutatingChatModel()
    bundle = _bundle(model)
    caller_conf = {"temperature": 0.3}

    monkeypatch.setattr(llm_service, "record_run_token_usage", lambda *_args: None)

    async for _ in getattr(bundle, method_name)("", []):
        pass
    async for _ in getattr(bundle, method_name)("", []):
        pass
    async for _ in getattr(bundle, method_name)("", [], caller_conf):
        pass

    assert model.stream_received == [{}, {}, {"temperature": 0.3}]
    assert caller_conf == {"temperature": 0.3}


@pytest.mark.parametrize(
    "method_name",
    ["async_chat", "async_chat_streamly", "async_chat_streamly_delta"],
)
def test_llm_bundle_gen_conf_defaults_to_none(method_name):
    parameter = inspect.signature(getattr(llm_service.LLMBundle, method_name)).parameters["gen_conf"]

    assert parameter.default is None
