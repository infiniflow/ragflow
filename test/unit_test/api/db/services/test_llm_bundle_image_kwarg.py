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
"""
Regression tests for issue #15966: attaching images to a chat backed by a base
(OpenAI-compatible) chat model raised
``AsyncCompletions.create() got an unexpected keyword argument 'images'``.

Base chat models only have ``**kwargs`` and forward it verbatim to the provider
SDK, so an ``images=`` kwarg leaked into ``chat.completions.create()``. The fix
folds the images into the last user message as multimodal content (and drops the
kwarg) when the target model does not declare an explicit ``images`` parameter,
while leaving CV models that consume ``images`` natively untouched.
"""

import pytest

from api.db.services.llm_service import LLMBundle


def _base_chat_async_chat(system, history, gen_conf=None, **kwargs):
    """Mirror of chat_model.Base.async_chat: only **kwargs, no images param."""


def _cv_async_chat(system, history, gen_conf, images=None, **kwargs):
    """Mirror of cv_model.Base.async_chat: explicit images parameter."""


@pytest.mark.p2
def test_images_to_b64_encodes_bytes_and_passes_strings():
    assert LLMBundle._images_to_b64([b"hello"]) == ["aGVsbG8="]
    assert LLMBundle._images_to_b64(["data:image/png;base64,aGVsbG8="]) == ["data:image/png;base64,aGVsbG8="]


@pytest.mark.p2
def test_base_chat_model_folds_images_and_drops_kwarg():
    history = [{"role": "user", "content": "summarize this"}]
    kwargs = {"images": [b"hello"]}

    LLMBundle._fold_images_if_unsupported(_base_chat_async_chat, history, kwargs, "")

    # images must NOT reach the provider SDK
    assert "images" not in kwargs
    # ...they are folded into the last user message as multimodal content
    content = history[0]["content"]
    assert isinstance(content, list)
    assert [part["type"] for part in content] == ["text", "image_url"]
    assert content[1]["image_url"]["url"] == "data:image/png;base64,aGVsbG8="


@pytest.mark.p2
def test_cv_model_with_native_images_param_is_untouched():
    history = [{"role": "user", "content": "describe"}]
    kwargs = {"images": [b"x"]}

    LLMBundle._fold_images_if_unsupported(_cv_async_chat, history, kwargs, "")

    # CV models consume images natively, so leave both kwargs and history as-is
    assert kwargs["images"] == [b"x"]
    assert history[0]["content"] == "describe"


@pytest.mark.p2
def test_no_images_is_a_noop():
    kwargs = {}
    LLMBundle._fold_images_if_unsupported(_base_chat_async_chat, [], kwargs, "")
    assert kwargs == {}

    # an empty images list is dropped without touching history
    kwargs = {"images": []}
    LLMBundle._fold_images_if_unsupported(_base_chat_async_chat, [], kwargs, "")
    assert "images" not in kwargs
