#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from rag.llm import SupportedLiteLLMProvider
from rag.llm.chat_model import _apply_model_family_policies, _merge_gen_conf_and_kwargs, _move_litellm_provider_body_fields

pytestmark = pytest.mark.p1


def test_qwen3_uses_system_disabled_default():
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-plus",
        backend="base",
        gen_conf={},
        request_kwargs={},
    )

    assert gen_conf == {}
    assert kwargs["extra_body"]["enable_thinking"] is False


def test_qwen3_can_enable_thinking_explicitly():
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-plus",
        backend="base",
        gen_conf={"thinking": "enabled", "temperature": 0.2},
        request_kwargs={"extra_body": {"seed": 1}},
    )

    assert gen_conf == {"temperature": 0.2}
    assert kwargs["extra_body"] == {"seed": 1, "enable_thinking": True}


@pytest.mark.parametrize(
    "provider",
    [SupportedLiteLLMProvider.Tongyi_Qianwen, SupportedLiteLLMProvider.Dashscope],
)
def test_qwen3_litellm_provider_uses_provider_field(provider):
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-max",
        backend="litellm",
        provider=provider,
        gen_conf={"thinking": "disabled"},
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf["enable_thinking"] is False


def test_kimi_thinking_maps_to_moonshot_payload():
    gen_conf, kwargs = _apply_model_family_policies(
        "kimi-k2.6-preview",
        backend="litellm",
        provider=SupportedLiteLLMProvider.Moonshot,
        gen_conf={"thinking": "disabled", "temperature": 0.6},
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf["thinking"] == {"type": "disabled"}
    assert "temperature" not in gen_conf


def test_moonshot_explicit_thinking_does_not_require_exact_kimi_model_name():
    gen_conf, kwargs = _apply_model_family_policies(
        "kimi-latest",
        backend="litellm",
        provider=SupportedLiteLLMProvider.Moonshot,
        gen_conf={"thinking": "disabled"},
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf["thinking"] == {"type": "disabled"}


def test_kimi_keeps_provider_default_when_unspecified():
    gen_conf, kwargs = _apply_model_family_policies(
        "kimi-k2.5-preview",
        backend="litellm",
        provider=SupportedLiteLLMProvider.Moonshot,
        gen_conf={"temperature": 0.6},
        request_kwargs={},
    )

    assert kwargs == {}
    assert "thinking" not in gen_conf
    assert "temperature" not in gen_conf
    assert gen_conf["top_p"] == 0.95
    assert gen_conf["n"] == 1
    assert gen_conf["presence_penalty"] == 0.0
    assert gen_conf["frequency_penalty"] == 0.0


def test_glm_keeps_provider_default_when_unspecified():
    gen_conf, kwargs = _apply_model_family_policies(
        "glm-4.7",
        backend="litellm",
        provider=SupportedLiteLLMProvider.ZHIPU_AI,
        gen_conf={},
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf == {}


def test_glm_thinking_maps_to_zhipu_payload():
    gen_conf, kwargs = _apply_model_family_policies(
        "glm-4.7",
        backend="litellm",
        provider=SupportedLiteLLMProvider.ZHIPU_AI,
        gen_conf={"thinking": "enabled"},
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf["thinking"] == {"type": "enabled"}


def test_litellm_provider_body_fields_move_to_extra_body_before_drop_params():
    completion_args = {
        "model": "kimi-latest",
        "messages": [],
        "thinking": {"type": "disabled"},
        "temperature": 0.2,
    }

    _move_litellm_provider_body_fields(SupportedLiteLLMProvider.Moonshot, completion_args)

    assert completion_args["extra_body"]["thinking"] == {"type": "disabled"}
    assert "thinking" not in completion_args
    assert completion_args["temperature"] == 0.2


@pytest.mark.parametrize(
    "thinking_gen_conf,expected_reasoning_effort",
    [
        ({"thinking": "disabled"}, "none"),
        ({}, "none"),  # unspecified -> qwen3's disabled-by-default policy
        ({"thinking": "enabled"}, "medium"),
    ],
)
def test_qwen3_ollama_maps_thinking_to_reasoning_effort(thinking_gen_conf, expected_reasoning_effort):
    """litellm's ollama_chat transformation only understands the standard
    `reasoning_effort` kwarg (which it maps onto Ollama's native `think`
    field) - it does not read `extra_body.enable_thinking`, so the Agent
    node's Thinking toggle must be routed through `reasoning_effort` for
    Ollama specifically, unlike the Dashscope/Tongyi-Qianwen native API."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3.5:9b",
        backend="litellm",
        provider=SupportedLiteLLMProvider.Ollama,
        gen_conf=dict(thinking_gen_conf),
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf["reasoning_effort"] == expected_reasoning_effort
    assert "enable_thinking" not in gen_conf
    assert "thinking" not in gen_conf


def test_qwen3_ollama_frequency_penalty_routes_through_extra_body():
    """litellm (as installed) reproducibly corrupts the response - verified
    via a fixed-seed sweep against a live Ollama instance, 8/8 failures -
    when `frequency_penalty` is passed as a top-level completion kwarg
    alongside `reasoning_effort` for the ollama_chat provider. Routing the
    same value through `extra_body.repeat_penalty` instead avoids the
    broken path. The value must also be rescaled: OpenAI-style
    `frequency_penalty` is centered on 0.0, while Ollama's `repeat_penalty`
    is centered on 1.0 (default 1.1), so a raw 1:1 passthrough would send a
    value that *encourages* repetition instead of discouraging it."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3.5:9b",
        backend="litellm",
        provider=SupportedLiteLLMProvider.Ollama,
        gen_conf={"frequency_penalty": 0.3, "thinking": "disabled"},
        request_kwargs={},
    )

    assert kwargs == {}
    assert "frequency_penalty" not in gen_conf
    assert gen_conf["extra_body"]["repeat_penalty"] == pytest.approx(1.3)
    assert gen_conf["reasoning_effort"] == "none"


def test_ollama_frequency_penalty_rerouted_for_any_model_not_just_qwen3():
    """The rerouting is gated on `provider == Ollama`, not on the qwen3
    model-name check above (that check only controls the reasoning_effort
    mapping) - frequency_penalty is a raw value-semantics mismatch between
    OpenAI's scale and Ollama's `repeat_penalty` scale regardless of which
    model is selected, so the fix intentionally applies to every
    Ollama-hosted model."""
    gen_conf, kwargs = _apply_model_family_policies(
        "llama3.1:8b",
        backend="litellm",
        provider=SupportedLiteLLMProvider.Ollama,
        gen_conf={"frequency_penalty": 0.3},
        request_kwargs={},
    )

    assert kwargs == {}
    assert "frequency_penalty" not in gen_conf
    assert gen_conf["extra_body"]["repeat_penalty"] == pytest.approx(1.3)
    assert "reasoning_effort" not in gen_conf


def test_litellm_provider_body_fields_preserve_existing_extra_body():
    completion_args = {
        "model": "qwen3-max",
        "messages": [],
        "enable_thinking": False,
        "extra_body": {"seed": 1},
    }

    _move_litellm_provider_body_fields(SupportedLiteLLMProvider.Tongyi_Qianwen, completion_args)

    assert completion_args["extra_body"] == {"seed": 1, "enable_thinking": False}
    assert "enable_thinking" not in completion_args


def test_merge_gen_conf_and_kwargs_combines_extra_body_instead_of_clobbering():
    """`async_chat()` builds its final completion args as (conceptually)
    `{**gen_conf, **kwargs}`. If both dicts happen to carry `extra_body`
    (e.g. this PR's Ollama repeat_penalty rerouting lands in gen_conf,
    while some other policy puts a different key into kwargs' extra_body),
    a plain dict-spread would let one `extra_body` silently clobber the
    other instead of merging them."""
    gen_conf = {"temperature": 0.5, "extra_body": {"repeat_penalty": 1.3}}
    kwargs = {"extra_body": {"some_other_option": True}}

    merged = _merge_gen_conf_and_kwargs(gen_conf, kwargs)

    assert merged["temperature"] == 0.5
    assert merged["extra_body"] == {"repeat_penalty": 1.3, "some_other_option": True}


def test_merge_gen_conf_and_kwargs_kwargs_wins_on_conflicting_extra_body_key():
    gen_conf = {"extra_body": {"repeat_penalty": 1.3}}
    kwargs = {"extra_body": {"repeat_penalty": 9.9}}

    merged = _merge_gen_conf_and_kwargs(gen_conf, kwargs)

    assert merged["extra_body"] == {"repeat_penalty": 9.9}


def test_merge_gen_conf_and_kwargs_kwargs_wins_on_other_keys():
    merged = _merge_gen_conf_and_kwargs({"temperature": 0.5}, {"temperature": 0.9})
    assert merged["temperature"] == 0.9


def test_merge_gen_conf_and_kwargs_handles_single_sided_extra_body():
    assert _merge_gen_conf_and_kwargs({"extra_body": {"a": 1}}, {})["extra_body"] == {"a": 1}
    assert _merge_gen_conf_and_kwargs({}, {"extra_body": {"b": 2}})["extra_body"] == {"b": 2}
