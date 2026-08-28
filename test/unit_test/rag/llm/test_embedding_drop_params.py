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
"""Regression tests for issue #18584.

``OpenAIEmbed._call`` sent ``extra_body={"drop_params": True}`` to every
OpenAI-compatible endpoint. ``drop_params`` is an OpenRouter-specific
convention -- it tells the server to silently drop unknown parameters.
Together AI (and any other strict OpenAI-compatible provider) rejects the
unknown field with HTTP 400
"Error code: 400 - {'error': {'message': 'Unrecognized request arguments
supplied: drop_params'}}".

The fix removes ``drop_params`` from ``OpenAIEmbed._call``. ``OpenRouterEmbed``
already has its own ``_call`` override that includes ``drop_params`` (and the
optional ``provider.order`` block) -- that override is unchanged and
continues to send ``drop_params`` to OpenRouter.

These tests pin both contracts:

* ``OpenAIEmbed._call`` no longer sends ``drop_params`` in ``extra_body``
  (or in any other field -- only fields every OpenAI-compatible provider
  accepts are forwarded);
* ``OpenRouterEmbed._call`` still sends ``extra_body={"drop_params": True,
  ...}`` so the OpenRouter-specific behaviour is preserved.
"""

from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest

from rag.llm.embedding_model import OpenAIEmbed, OpenRouterEmbed


# ---------------------------------------------------------------------------
# Fakes
# ---------------------------------------------------------------------------


class _FakeResp:
    def __init__(self, vectors):
        self.data = [SimpleNamespace(embedding=list(v)) for v in vectors]
        self.usage = SimpleNamespace(total_tokens=0)


def _make_openai_embed(cls=OpenAIEmbed):
    """Construct an embedding client and capture the kwargs of every
    ``embeddings.create`` call into ``embed.client.embeddings.create.call_args_list``.
    """
    embed = cls("key", "text-embedding-3-small", base_url="https://example.invalid/v1")
    embed.client = MagicMock()
    embed.client.embeddings.create = MagicMock(side_effect=lambda input, model, **kwargs: _FakeResp([[float(i)] for i in range(len(input))]))
    return embed


def _make_openrouter_embed():
    """Construct an OpenRouterEmbed and capture the kwargs of every
    ``embeddings.create`` call. ``provider_order`` is empty so the
    ``extra_body`` is exactly the base case.
    """
    embed = OpenRouterEmbed("key", "openai/text-embedding-3-small", base_url="https://openrouter.ai/api/v1")
    embed.client = MagicMock()
    embed.client.embeddings.create = MagicMock(side_effect=lambda input, model, **kwargs: _FakeResp([[float(i)] for i in range(len(input))]))
    return embed


# ---------------------------------------------------------------------------
# Tests: OpenAIEmbed (and all its subclasses) must NOT send drop_params
# ---------------------------------------------------------------------------


def test_openai_embed_call_does_not_send_drop_params():
    """``OpenAIEmbed._call`` must not include ``drop_params`` in
    ``extra_body`` (or any other argument). Together AI rejects the field
    with HTTP 400; OpenAI itself silently ignores it. Either way, sending
    it is the wrong default for the OpenAI-compatible path.
    """
    embed = _make_openai_embed(OpenAIEmbed)
    embed._call(["hello", "world"])

    assert embed.client.embeddings.create.call_count == 1
    kwargs = embed.client.embeddings.create.call_args.kwargs
    assert "extra_body" not in kwargs, f"OpenAIEmbed._call must not pass extra_body to strict providers; got {kwargs.get('extra_body')!r}"
    assert "drop_params" not in kwargs, f"OpenAIEmbed._call must not pass drop_params to strict providers; got kwargs={kwargs!r}"


@pytest.mark.parametrize(
    "cls_name",
    [
        "AzureEmbed",
        "AstraflowEmbed",
        "AIMLAPIEmbed",
        "BaiChuanEmbed",
        "FuturMixEmbed",
        "TogetherAIEmbed",
        "PerfXCloudEmbed",
        "UpstageEmbed",
    ],
)
def test_openai_embed_subclass_inherits_no_drop_params(cls_name):
    """Subclasses of OpenAIEmbed inherit ``_call`` and must therefore also
    not send ``drop_params`` to their respective providers. Together AI is
    one of these subclasses, so the issue's reproduction case is covered
    here.
    """
    from rag.llm import embedding_model as em

    cls = getattr(em, cls_name)
    embed = _make_openai_embed(cls)
    embed._call(["hello"])

    assert embed.client.embeddings.create.call_count == 1
    kwargs = embed.client.embeddings.create.call_args.kwargs
    assert "drop_params" not in kwargs, f"{cls_name}._call inherited drop_params from OpenAIEmbed -- this regresses issue #18584. kwargs={kwargs!r}"


# ---------------------------------------------------------------------------
# Tests: OpenRouterEmbed still sends drop_params (regression guard)
# ---------------------------------------------------------------------------


def test_openrouter_embed_call_still_sends_drop_params():
    """``OpenRouterEmbed._call`` has its own override and must keep sending
    ``drop_params=True`` so OpenRouter's "drop unknown parameters"
    behaviour continues to apply.
    """
    embed = _make_openrouter_embed()
    embed._call(["hello"])

    assert embed.client.embeddings.create.call_count == 1
    kwargs = embed.client.embeddings.create.call_args.kwargs
    assert "extra_body" in kwargs, "OpenRouterEmbed._call must still pass extra_body"
    assert kwargs["extra_body"].get("drop_params") is True, f"OpenRouterEmbed._call must keep drop_params=True so OpenRouter silently drops unknown params; got extra_body={kwargs['extra_body']!r}"


def test_openrouter_embed_call_appends_provider_when_provider_order_set():
    """``OpenRouterEmbed._call`` adds the ``provider.order`` block to
    ``extra_body`` when ``provider_order`` is set on the instance, alongside
    the existing ``drop_params=True``. Pins the OpenRouter-specific
    behaviour that the fix must not disturb.
    """
    embed = _make_openrouter_embed()
    embed.provider_order = "Azure,OpenAI"
    embed._call(["hello"])

    kwargs = embed.client.embeddings.create.call_args.kwargs
    extra_body = kwargs.get("extra_body", {})
    assert extra_body.get("drop_params") is True
    assert extra_body.get("provider") == {"order": ["Azure", "OpenAI"], "allow_fallbacks": False}


# ---------------------------------------------------------------------------
# Sanity: the response shape is preserved
# ---------------------------------------------------------------------------


def test_openai_embed_call_returns_vectors_in_input_order():
    """Sanity: removing ``drop_params`` must not change the response
    handling -- the call still returns the embedding vectors from
    ``res.data`` and the total token count.
    """
    embed = _make_openai_embed(OpenAIEmbed)
    vectors, total_tokens = embed._call(["a", "b", "c"])

    assert len(vectors) == 3
    assert vectors[0] == [0.0]
    assert vectors[1] == [1.0]
    assert vectors[2] == [2.0]
    assert total_tokens == 0
