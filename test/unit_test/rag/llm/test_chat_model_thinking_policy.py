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
from rag.llm.chat_model import _apply_model_family_policies, _move_litellm_provider_body_fields

pytestmark = pytest.mark.p1


def test_qwen3_uses_system_disabled_default():
    """Base-compatible Qwen3 requests disable thinking by default."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-plus",
        backend="base",
        gen_conf={},
        request_kwargs={},
    )

    assert gen_conf == {}
    assert kwargs["extra_body"] == {"chat_template_kwargs": {"enable_thinking": False}}


def test_qwen3_can_enable_thinking_explicitly():
    """An explicit Qwen3 thinking choice reaches chat_template_kwargs."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-plus",
        backend="base",
        gen_conf={"thinking": "enabled", "temperature": 0.2},
        request_kwargs={"extra_body": {"seed": 1}},
    )

    assert gen_conf == {"temperature": 0.2}
    assert kwargs["extra_body"] == {"seed": 1, "chat_template_kwargs": {"enable_thinking": True}}


def test_qwen3_preserves_existing_chat_template_kwargs():
    """Qwen policy updates its field without dropping other template options."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-plus",
        backend="base",
        gen_conf={"thinking": "disabled"},
        request_kwargs={
            "extra_body": {
                "seed": 1,
                "chat_template_kwargs": {"enable_thinking": True, "custom_template_flag": "keep"},
            }
        },
    )

    assert gen_conf == {}
    assert kwargs["extra_body"] == {
        "seed": 1,
        "chat_template_kwargs": {"enable_thinking": False, "custom_template_flag": "keep"},
    }


def test_qwen3_preview_variant_forces_thinking_true():
    """qwen3.x-preview models (e.g. qwen3.8-max-preview) only accept enable_thinking=True."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3.8-max-preview",
        backend="base",
        gen_conf={},
        request_kwargs={},
    )

    assert gen_conf == {}
    assert kwargs["extra_body"]["chat_template_kwargs"]["enable_thinking"] is True


def test_qwen3_preview_ignores_disabled_thinking():
    """Even with thinking=disabled, -preview still forces enable_thinking=True."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3.8-max-preview",
        backend="base",
        gen_conf={"thinking": "disabled", "temperature": 0.2},
        request_kwargs={},
    )

    assert "thinking" not in gen_conf
    assert gen_conf == {"temperature": 0.2}
    assert kwargs["extra_body"]["chat_template_kwargs"]["enable_thinking"] is True


def test_qwen3_24t_a95b_forces_thinking_true():
    """qwen3.8-2.4t-a95b (flagship reasoning model) only accepts enable_thinking=True."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3.8-2.4t-a95b",
        backend="base",
        gen_conf={},
        request_kwargs={},
    )

    assert gen_conf == {}
    assert kwargs["extra_body"]["chat_template_kwargs"]["enable_thinking"] is True


def test_qwen3_24t_a95b_ignores_disabled_thinking():
    """Even with thinking=disabled, qwen3.8-2.4t-a95b still forces enable_thinking=True."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3.8-2.4t-a95b",
        backend="base",
        gen_conf={"thinking": "disabled", "temperature": 0.2},
        request_kwargs={},
    )

    assert "thinking" not in gen_conf
    assert gen_conf == {"temperature": 0.2}
    assert kwargs["extra_body"]["chat_template_kwargs"]["enable_thinking"] is True


@pytest.mark.parametrize(
    "provider",
    [SupportedLiteLLMProvider.Tongyi_Qianwen, SupportedLiteLLMProvider.Dashscope],
)
def test_qwen3_litellm_provider_uses_provider_field(provider):
    """Native DashScope providers keep their provider-specific body field."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-max",
        backend="litellm",
        provider=provider,
        gen_conf={"thinking": "disabled"},
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf["enable_thinking"] is False


def test_qwen3_litellm_openai_uses_nested_extra_body():
    """Non-DashScope LiteLLM providers carry Qwen controls in extra_body."""
    gen_conf, kwargs = _apply_model_family_policies(
        "qwen3-8b",
        backend="litellm",
        provider=SupportedLiteLLMProvider.OpenAI,
        gen_conf={
            "thinking": "enabled",
            "extra_body": {
                "seed": 1,
                "chat_template_kwargs": {"custom_template_flag": "keep"},
            },
        },
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf == {
        "extra_body": {
            "seed": 1,
            "chat_template_kwargs": {
                "custom_template_flag": "keep",
                "enable_thinking": True,
            },
        }
    }


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


def test_deepseek_thinking_disabled_via_extra_body():
    # litellm 1.82.x DeepSeek transformation drops `thinking` from the top
    # level (only {"type": "enabled"} survives) and rejects reasoning_effort
    # alongside thinking: disabled. The toggle must be carried in extra_body
    # and any reasoning_effort stripped. When unspecified, default to disabled.
    for gen_input in (
        {"thinking": "disabled", "reasoning_effort": "high"},
        {"thinking": "default"},
        {"temperature": 0.5},
    ):
        gen_conf, kwargs = _apply_model_family_policies(
            "deepseek-v4-flash",
            backend="litellm",
            provider=SupportedLiteLLMProvider.DeepSeek,
            gen_conf=dict(gen_input),
            request_kwargs={},
        )
        assert kwargs == {}
        assert "thinking" not in gen_conf
        assert "reasoning_effort" not in gen_conf
        assert gen_conf["extra_body"]["thinking"] == {"type": "disabled"}


def test_deepseek_thinking_enabled_via_extra_body():
    gen_conf, kwargs = _apply_model_family_policies(
        "deepseek-v4-flash",
        backend="litellm",
        provider=SupportedLiteLLMProvider.DeepSeek,
        gen_conf={"thinking": "enabled"},
        request_kwargs={},
    )

    assert kwargs == {}
    assert "thinking" not in gen_conf
    assert gen_conf["extra_body"]["thinking"] == {"type": "enabled"}


def test_deepseek_extra_body_keeps_shallow_merge_semantics():
    """The Qwen fix must not recursively merge unrelated provider payloads."""
    gen_conf, kwargs = _apply_model_family_policies(
        "deepseek-v4-flash",
        backend="litellm",
        provider=SupportedLiteLLMProvider.DeepSeek,
        gen_conf={
            "thinking": "disabled",
            "extra_body": {
                "seed": 1,
                "thinking": {"budget_tokens": 128},
            },
        },
        request_kwargs={},
    )

    assert kwargs == {}
    assert gen_conf["extra_body"] == {
        "seed": 1,
        "thinking": {"type": "disabled"},
    }


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
