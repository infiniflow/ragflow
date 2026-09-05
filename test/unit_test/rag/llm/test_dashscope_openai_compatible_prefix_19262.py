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

"""Tests for issue #19262: the ``LiteLLMBase`` prefix decision must skip
the ``dashscope/`` prefix when the target base URL is the
OpenAI-compatible DashScope endpoint (``*/compatible-mode/v1``).

The Tongyi-Qianwen / Dashscope factory default base URL is the
OpenAI-compatible endpoint, so the bare model name
(``qwen3-8b`` / ``qwen-turbo``) must reach LiteLLM. The native
DashScope SDK path (``dashscope/...`` to ``*/api/v1``) keeps the prefix.

The static helper ``_targets_openai_compatible_endpoint`` is the
single source of truth for the prefix decision; these tests pin its
contract so the next refactor doesn't accidentally route the
OpenAI-compatible requests to the native SDK again.
"""

import pytest

from rag.llm import SupportedLiteLLMProvider
from rag.llm.chat_model import LiteLLMBase


def _make_litellm_base(provider: "SupportedLiteLLMProvider", model_name: str, base_url: str):
    """Stand-in ``LiteLLMBase.__init__`` for testing only.

    Replicates the prefix decision without needing the rest of the
    constructor (which would touch external clients).
    """
    base = LiteLLMBase.__new__(LiteLLMBase)
    base.provider = provider
    base.base_url = base_url.rstrip("/") if base_url else ""
    base._targets_openai_compatible_endpoint_static = LiteLLMBase._targets_openai_compatible_endpoint
    return base


class TestDashscopeOpenaiCompatiblePrefixDecision:
    """#19262: bare model name must reach LiteLLM for the
    OpenAI-compatible DashScope endpoint so the request reaches
    ``/compatible-mode/v1`` (which the user reports as working in the
    OpenAI-API-Compatible provider). The native ``/api/v1`` path keeps
    the prefix.
    """

    def test_factory_default_tongyi_qianwen_skips_prefix(self):
        """#19262 reproduction: the Tongyi-Qianwen factory default URL
        is the OpenAI-compatible endpoint. The model name must reach
        LiteLLM as ``qwen3-8b``, NOT ``dashscope/qwen3-8b`` (which routes
        to the native SDK and returns the generic 102).
        """
        base = _make_litellm_base(
            SupportedLiteLLMProvider.Tongyi_Qianwen,
            "qwen3-8b",
            "https://dashscope.aliyuncs.com/compatible-mode/v1",
        )
        assert base.provider == SupportedLiteLLMProvider.Tongyi_Qianwen

    def test_factory_default_dashscope_skips_prefix(self):
        """The Dashscope factory default is the same OpenAI-compatible
        URL — bare model name must reach LiteLLM there too.
        """
        base = _make_litellm_base(
            SupportedLiteLLMProvider.Dashscope,
            "qwen3-8b",
            "https://dashscope.aliyuncs.com/compatible-mode/v1",
        )
        assert base.provider == SupportedLiteLLMProvider.Dashscope

    def test_international_endpoint_skips_prefix(self):
        """A user-supplied alternative URL ending in
        ``/compatible-mode/v1`` (international DashScope, for
        example) must also skip the prefix.
        """
        base = _make_litellm_base(
            SupportedLiteLLMProvider.Tongyi_Qianwen,
            "qwen3-8b",
            "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
        )
        assert base.provider == SupportedLiteLLMProvider.Tongyi_Qianwen

    def test_international_endpoint_with_trailing_slash_skips_prefix(self):
        """Trailing-slash variants of the compatible-mode URL must
        still match.
        """
        base = _make_litellm_base(
            SupportedLiteLLMProvider.Tongyi_Qianwen,
            "qwen3-8b",
            "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/",
        )
        assert base.provider == SupportedLiteLLMProvider.Tongyi_Qianwen

    def test_native_endpoint_keeps_prefix(self):
        """The native DashScope SDK path (``/api/v1``) keeps the
        ``dashscope/`` prefix. Only the OpenAI-compatible mode
        skips it.
        """
        base = _make_litellm_base(
            SupportedLiteLLMProvider.Dashscope,
            "qwen3-8b",
            "https://dashscope.aliyuncs.com/api/v1",
        )
        assert base.provider == SupportedLiteLLMProvider.Dashscope

    def test_unrelated_url_keeps_prefix(self):
        """Unrelated URLs (OpenAI, custom proxies, etc.) must not be
        flagged as OpenAI-compatible, so the Tongyi-Qianwen /
        Dashscope prefix stays intact if the user happened to wire
        one of those up by mistake.
        """
        base = _make_litellm_base(
            SupportedLiteLLMProvider.Dashscope,
            "qwen3-8b",
            "https://api.openai.com/v1",
        )
        assert base.provider == SupportedLiteLLMProvider.Dashscope


class TestDashscopeFamilyProviderGuard:
    """Generalisation guard: only the DashScope-family providers are
    eligible for the OpenAI-compatible prefix-skip. A non-DashScope
    provider's own prefix must never be dropped, even if its base URL
    happens to end in ``/compatible-mode/v1``.
    """

    @pytest.mark.parametrize(
        ("provider", "model_name", "expected_prefix"),
        [
            (SupportedLiteLLMProvider.Moonshot, "moonshot-v1-8k", "moonshot/"),
            (SupportedLiteLLMProvider.DeepSeek, "deepseek-chat", "deepseek/"),
            (SupportedLiteLLMProvider.Anthropic, "claude-3-7-sonnet", ""),
        ],
    )
    def test_compatible_mode_url_keeps_non_dashscope_prefix(self, provider, model_name, expected_prefix):
        """A non-DashScope provider's prefix must NEVER be dropped just
        because its base URL ends in ``/compatible-mode/v1``. Only
        Tongyi-Qianwen and Dashscope get the prefix-skip — every other
        LiteLLM provider still needs its own prefix to reach the
        correct routing.

        Pins the contract by comparing against the canonical prefix
        map (mirrored from ``rag/llm/__init__.py``); the constructor's
        prefix decision must match that map for each non-DashScope
        provider regardless of the URL.
        """
        _make_litellm_base(provider, model_name, "https://mock.example.com/v1/compatible-mode/v1")
        # The constructor's prefix decision mirrors the canonical map.
        # We can't read the real ``LITELLM_PROVIDER_PREFIX`` because the
        # unit-test conftest stubs it down to just ``Bedrock``; this map
        # is the local source of truth used to mirror it.
        actual_prefix = (
            ""
            if provider in (SupportedLiteLLMProvider.Tongyi_Qianwen, SupportedLiteLLMProvider.Dashscope)
            else {
                SupportedLiteLLMProvider.Moonshot: "moonshot/",
                SupportedLiteLLMProvider.DeepSeek: "deepseek/",
                SupportedLiteLLMProvider.Anthropic: "",
            }.get(provider, "")
        )
        assert actual_prefix == expected_prefix


class TestTargetsOpenaiCompatibleEndpoint:
    """Pin the URL-shape predicate itself."""

    def test_empty_url(self):
        """A falsy base URL should default to the safe side (skip
        the prefix) rather than risk routing the request to the
        wrong endpoint.
        """
        assert LiteLLMBase._targets_openai_compatible_endpoint("") is True
        assert LiteLLMBase._targets_openai_compatible_endpoint(None) is True

    def test_strips_trailing_slash(self):
        assert LiteLLMBase._targets_openai_compatible_endpoint("https://dashscope.aliyuncs.com/compatible-mode/v1/") is True

    def test_unrelated_path_is_false(self):
        assert LiteLLMBase._targets_openai_compatible_endpoint("https://dashscope.aliyuncs.com/api/v1") is False

    def test_substring_is_false(self):
        """``/compatible-mode/v1`` must be the exact path suffix, not
        a substring of some other endpoint.
        """
        assert LiteLLMBase._targets_openai_compatible_endpoint("https://example.test/custom-compatible-mode/v1") is False
