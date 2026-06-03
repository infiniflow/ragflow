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
"""
Regression guard for issue #15427: LiteLLM-routed chats failed with
``model_type: Extra inputs are not permitted`` (Anthropic) /
``Unknown parameter: 'model_type'`` (OpenAI).

A chat assistant's ``llm_setting`` is forwarded to the provider as
``gen_conf``. ``llm_setting`` can legitimately carry RAGFlow-internal
metadata such as ``model_type`` (the new chat REST APIs read it back out).
``Base._clean_conf`` already whitelisted the keys it forwards, so OpenAI-
compatible providers were unaffected, but ``LiteLLMBase._clean_conf`` only
dropped ``max_tokens`` and passed everything else straight through to
``litellm.acompletion`` — which forwarded ``model_type`` to the upstream
provider and got rejected.

These tests pin the whitelisting behaviour for both backends so the leak
cannot reappear.
"""

import pytest

from rag.llm.chat_model import (
    ALLOWED_GEN_CONF_KEYS,
    LITELLM_ALLOWED_GEN_CONF_KEYS,
    Base,
    LiteLLMBase,
)


class _ConcreteBase(Base):
    """Concrete subclass so we can build an instance without touching the
    real OpenAI client constructor."""


class _ConcreteLiteLLM(LiteLLMBase):
    """Concrete subclass for the same reason on the LiteLLM path."""


def _make_base(model_name="gpt-4o"):
    inst = _ConcreteBase.__new__(_ConcreteBase)
    inst.model_name = model_name
    return inst


def _make_litellm(model_name="gpt-4o", provider=""):
    inst = _ConcreteLiteLLM.__new__(_ConcreteLiteLLM)
    inst.model_name = model_name
    inst.provider = provider
    return inst


# --------------------------------------------------------------------------- #
# The actual bug: model_type must never reach the provider.
# --------------------------------------------------------------------------- #
def test_litellm_drops_model_type():
    cleaned = _make_litellm()._clean_conf({"temperature": 0.5, "model_type": "chat"})
    assert "model_type" not in cleaned
    assert cleaned["temperature"] == 0.5


def test_base_drops_model_type():
    cleaned = _make_base()._clean_conf({"temperature": 0.5, "model_type": "chat"})
    assert "model_type" not in cleaned
    assert cleaned["temperature"] == 0.5


@pytest.mark.parametrize("stray_key", ["model_type", "llm_id", "parameter", "icon", "foo"])
def test_litellm_drops_arbitrary_internal_keys(stray_key):
    cleaned = _make_litellm()._clean_conf({stray_key: "x", "top_p": 0.9})
    assert stray_key not in cleaned
    assert cleaned["top_p"] == 0.9


# --------------------------------------------------------------------------- #
# The fix must not over-filter: genuine generation params still pass through.
# --------------------------------------------------------------------------- #
def test_litellm_preserves_known_generation_params():
    gen_conf = {
        "temperature": 0.7,
        "top_p": 0.95,
        "presence_penalty": 0.1,
        "frequency_penalty": 0.2,
    }
    cleaned = _make_litellm()._clean_conf(dict(gen_conf))
    assert cleaned == gen_conf


def test_litellm_preserves_thinking_param():
    """``thinking`` is injected by the model-family policy for reasoning
    models and must survive the whitelist (it is a valid LiteLLM param)."""
    cleaned = _make_litellm()._clean_conf({"thinking": {"type": "enabled"}, "temperature": 1.0})
    assert cleaned["thinking"] == {"type": "enabled"}


def test_max_tokens_is_dropped_on_both_backends():
    assert "max_tokens" not in _make_litellm()._clean_conf({"max_tokens": 100, "temperature": 0.3})
    assert "max_tokens" not in _make_base()._clean_conf({"max_tokens": 100, "temperature": 0.3})


# --------------------------------------------------------------------------- #
# Whitelist invariants.
# --------------------------------------------------------------------------- #
def test_litellm_whitelist_is_superset_of_base():
    assert ALLOWED_GEN_CONF_KEYS <= LITELLM_ALLOWED_GEN_CONF_KEYS


def test_model_type_not_whitelisted_anywhere():
    assert "model_type" not in ALLOWED_GEN_CONF_KEYS
    assert "model_type" not in LITELLM_ALLOWED_GEN_CONF_KEYS
