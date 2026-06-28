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
Tests for issue #15981: model-level custom request headers and default
generation parameters.

Covers, for both the OpenAI-compatible (``Base``) and ``LiteLLMBase`` backends:
- header parsing/validation (``parse_custom_headers``),
- default gen-param validation (``validate_default_gen_conf``),
- merge precedence (``merge_gen_conf`` — a request's gen_conf overrides the
  model-level defaults),
- that ``_clean_conf`` layers model defaults under the request, and
- that custom headers reach the underlying client / completion call.
"""

import pytest

from rag.llm.chat_model import (
    Base,
    LiteLLMBase,
    merge_gen_conf,
    parse_custom_headers,
    validate_default_gen_conf,
)


class _ConcreteBase(Base):
    pass


class _ConcreteLiteLLM(LiteLLMBase):
    pass


def _make_base(model_name="gpt-4o", default_gen_conf=None):
    inst = _ConcreteBase.__new__(_ConcreteBase)
    inst.model_name = model_name
    if default_gen_conf is not None:
        inst.default_gen_conf = default_gen_conf
    return inst


def _make_litellm(model_name="gpt-4o", provider="", default_gen_conf=None, custom_headers=None):
    inst = _ConcreteLiteLLM.__new__(_ConcreteLiteLLM)
    inst.model_name = model_name
    inst.provider = provider
    inst.api_key = "k"
    inst.max_retries = 1
    inst.base_url = ""
    inst.tools = []
    inst.is_tools = False
    if default_gen_conf is not None:
        inst.default_gen_conf = default_gen_conf
    if custom_headers is not None:
        inst.custom_headers = custom_headers
    return inst


# --------------------------------------------------------------------------- #
# parse_custom_headers
# --------------------------------------------------------------------------- #
def test_parse_headers_from_dict():
    assert parse_custom_headers({"X-Project-ID": "abc", "X-Workspace": "prod"}) == {
        "X-Project-ID": "abc",
        "X-Workspace": "prod",
    }


def test_parse_headers_from_json_string():
    assert parse_custom_headers('{"X-A": "1"}') == {"X-A": "1"}


def test_parse_headers_stringifies_scalars_and_drops_bad_entries():
    out = parse_custom_headers({"X-Int": 5, "X-Float": 1.5, "X-Bool": True, "": "skip", "X-List": [1]})
    assert out == {"X-Int": "5", "X-Float": "1.5"}  # bool, empty-key, list all dropped


@pytest.mark.parametrize("bad", [None, "", "not json", "[1,2]", 42, ["a"]])
def test_parse_headers_bad_input_returns_empty(bad):
    assert parse_custom_headers(bad) == {}


# --------------------------------------------------------------------------- #
# validate_default_gen_conf
# --------------------------------------------------------------------------- #
def test_validate_keeps_supported_params():
    out = validate_default_gen_conf(
        {"temperature": 0.2, "top_p": 0.9, "presence_penalty": 0.5, "frequency_penalty": -0.5}
    )
    assert out == {"temperature": 0.2, "top_p": 0.9, "presence_penalty": 0.5, "frequency_penalty": -0.5}


def test_validate_max_tokens_must_be_positive_int():
    assert validate_default_gen_conf({"max_tokens": 2048}) == {"max_tokens": 2048}
    assert validate_default_gen_conf({"max_tokens": 0}) == {}
    assert validate_default_gen_conf({"max_tokens": 1.5}) == {}


def test_validate_drops_unsupported_and_out_of_range():
    out = validate_default_gen_conf(
        {"temperature": 9, "top_p": 2.0, "model_type": "chat", "stream": True, "frequency_penalty": 0.1}
    )
    assert out == {"frequency_penalty": 0.1}  # temp>2, top_p>1, unsupported keys all dropped


def test_validate_rejects_bool_for_numeric_param():
    assert validate_default_gen_conf({"temperature": True}) == {}


def test_validate_from_json_string():
    assert validate_default_gen_conf('{"temperature": 0.3}') == {"temperature": 0.3}


# --------------------------------------------------------------------------- #
# merge_gen_conf precedence
# --------------------------------------------------------------------------- #
def test_merge_request_overrides_defaults():
    assert merge_gen_conf({"temperature": 0.2, "top_p": 0.9}, {"temperature": 0.8}) == {
        "temperature": 0.8,
        "top_p": 0.9,
    }


def test_merge_handles_none():
    assert merge_gen_conf(None, {"temperature": 0.5}) == {"temperature": 0.5}
    assert merge_gen_conf({"top_p": 0.9}, None) == {"top_p": 0.9}


# --------------------------------------------------------------------------- #
# _clean_conf layers model defaults under the request (both backends)
# --------------------------------------------------------------------------- #
def test_base_clean_conf_applies_defaults():
    cleaned = _make_base(default_gen_conf={"temperature": 0.2, "top_p": 0.9})._clean_conf({})
    assert cleaned["temperature"] == 0.2
    assert cleaned["top_p"] == 0.9


def test_base_clean_conf_request_overrides_default():
    cleaned = _make_base(default_gen_conf={"temperature": 0.2})._clean_conf({"temperature": 0.8})
    assert cleaned["temperature"] == 0.8


def test_base_clean_conf_without_defaults_is_unchanged_behaviour():
    # No default_gen_conf attribute at all (e.g. legacy construction) must not error.
    cleaned = _make_base()._clean_conf({"temperature": 0.5})
    assert cleaned["temperature"] == 0.5


def test_litellm_clean_conf_applies_defaults():
    cleaned = _make_litellm(default_gen_conf={"temperature": 0.2})._clean_conf({})
    assert cleaned["temperature"] == 0.2


def test_litellm_clean_conf_request_overrides_default():
    cleaned = _make_litellm(default_gen_conf={"temperature": 0.2})._clean_conf({"temperature": 0.7})
    assert cleaned["temperature"] == 0.7


# --------------------------------------------------------------------------- #
# Custom headers reach the client / completion call
# --------------------------------------------------------------------------- #
def test_base_init_forwards_default_headers_to_client(monkeypatch):
    captured = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        return object()

    import rag.llm.chat_model as cm

    monkeypatch.setattr(cm, "OpenAI", _capture)
    monkeypatch.setattr(cm, "AsyncOpenAI", lambda **kw: object())
    _ConcreteBase(key="k", model_name="gpt-4o", base_url="http://x", default_headers={"X-A": "1"})
    assert captured.get("default_headers") == {"X-A": "1"}


def test_base_init_omits_default_headers_when_none(monkeypatch):
    captured = {}
    import rag.llm.chat_model as cm

    monkeypatch.setattr(cm, "OpenAI", lambda **kw: captured.update(kw) or object())
    monkeypatch.setattr(cm, "AsyncOpenAI", lambda **kw: object())
    _ConcreteBase(key="k", model_name="gpt-4o", base_url="http://x")
    assert "default_headers" not in captured  # default client behavior preserved


def test_litellm_construct_args_includes_headers():
    inst = _make_litellm(custom_headers={"X-A": "1"})
    args = inst._construct_completion_args(history=[{"role": "user", "content": "hi"}], stream=False, tools=False)
    assert args["headers"] == {"X-A": "1"}


def test_litellm_construct_args_omits_headers_when_none():
    inst = _make_litellm(custom_headers={})
    args = inst._construct_completion_args(history=[{"role": "user", "content": "hi"}], stream=False, tools=False)
    assert "headers" not in args
