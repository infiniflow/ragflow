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

from common.model_thinking_utils import (
    apply_enable_thinking_policy,
    detect_thinking_family,
    is_qwen3_thinking_model,
)


pytestmark = pytest.mark.p2


def test_detect_thinking_family():
    assert detect_thinking_family("Tongyi-Qianwen") == "dashscope"
    assert detect_thinking_family("DeepSeek") == "openai_thinking"
    assert detect_thinking_family("Moonshot") == "openai_thinking"
    assert detect_thinking_family("ZHIPU-AI") == "openai_thinking"
    assert detect_thinking_family("GPUStack") == "gpustack_multi_engine"


def test_is_qwen3_thinking_model():
    assert is_qwen3_thinking_model("qwen3-235b-a22b-thinking-2507")
    assert is_qwen3_thinking_model("qwen3-vl-30b-a3b-thinking")
    assert not is_qwen3_thinking_model("qwen3-32b")
    assert not is_qwen3_thinking_model("qwq-32b")


def test_apply_enable_thinking_policy_qwen():
    conf, kwargs = apply_enable_thinking_policy("qwen3-32b", "Tongyi-Qianwen", {"reasoning": False})
    assert "reasoning" not in conf
    assert kwargs["extra_body"]["enable_thinking"] is False


def test_apply_enable_thinking_policy_qwen3_thinking_skip_disable():
    conf, kwargs = apply_enable_thinking_policy(
        "qwen3-235b-a22b-thinking-2507", "Tongyi-Qianwen", {"reasoning": False}
    )
    assert conf == {}
    assert kwargs == {}


def test_apply_enable_thinking_policy_kimi():
    conf, kwargs = apply_enable_thinking_policy("kimi-k2.5", "Moonshot", {"reasoning": False})
    assert kwargs == {}
    assert conf["thinking"] == {"type": "disabled"}
    assert conf["temperature"] == 0.6
    assert conf["top_p"] == 0.95


def test_apply_enable_thinking_policy_kimi_default_enabled():
    conf, kwargs = apply_enable_thinking_policy("kimi-k2.6", "Moonshot", {})
    assert kwargs == {}
    assert conf["thinking"] == {"type": "enabled"}
    assert conf["temperature"] == 1.0


def test_apply_enable_thinking_policy_deepseek():
    conf, kwargs = apply_enable_thinking_policy("deepseek-chat", "DeepSeek", {"reasoning": False})
    assert conf == {}
    assert kwargs["extra_body"]["thinking"] == {"type": "disabled"}


def test_apply_enable_thinking_policy_gpustack():
    conf, kwargs = apply_enable_thinking_policy("qwen3-32b", "GPUStack", {"reasoning": False})
    assert conf == {}
    assert kwargs["extra_body"]["thinking"] == {"type": "disabled"}
    assert kwargs["extra_body"]["chat_template_kwargs"]["enable_thinking"] is False
    assert kwargs["extra_body"]["chat_template_kwargs"]["thinking"] is False


def test_apply_enable_thinking_policy_gpustack_qwen3_thinking_skip_disable():
    conf, kwargs = apply_enable_thinking_policy(
        "qwen3-235b-a22b-thinking-2507", "GPUStack", {"reasoning": False}
    )
    assert conf == {}
    assert kwargs == {}
