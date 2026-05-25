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

from contextlib import contextmanager
from types import SimpleNamespace

import pytest

import rag.llm.cv_model as cv_model
from rag.llm import dashscope_utils
from rag.llm.cv_model import QWenCV


@pytest.mark.p2
def test_qwen_cv_video_fallback_uses_scoped_dashscope_endpoint(monkeypatch):
    original_url = dashscope_utils.DASHSCOPE_CN_NATIVE_API_URL
    observed_urls = []
    scoped_urls = []
    monkeypatch.setattr(dashscope_utils.dashscope, "base_http_api_url", original_url, raising=False)

    real_scope = cv_model.dashscope_native_api_url_scope

    @contextmanager
    def recording_scope(url):
        scoped_urls.append(url)
        with real_scope(url):
            yield

    monkeypatch.setattr(cv_model, "dashscope_native_api_url_scope", recording_scope)

    class FakeMultiModalConversation:
        @staticmethod
        def call(api_key, model, messages):
            observed_urls.append(dashscope_utils.dashscope.base_http_api_url)
            if len(observed_urls) == 1:
                return {"message": "default endpoint failed"}
            return {"output": {"choices": [{"message": SimpleNamespace(content=[{"text": "video summary"}])}]}}

    monkeypatch.setattr(dashscope_utils.dashscope, "MultiModalConversation", FakeMultiModalConversation, raising=False)

    qwen = QWenCV.__new__(QWenCV)
    qwen.api_key = "api-key"
    qwen.model_name = "qwen-vl"

    summary, token_count = qwen._process_video(b"video-bytes", "sample.mp4", "describe this video")

    assert summary == "video summary"
    assert token_count > 0
    assert scoped_urls == [None, dashscope_utils.DASHSCOPE_INTL_NATIVE_API_URL]
    assert observed_urls == [original_url, dashscope_utils.DASHSCOPE_INTL_NATIVE_API_URL]
    assert dashscope_utils.dashscope.base_http_api_url == original_url
