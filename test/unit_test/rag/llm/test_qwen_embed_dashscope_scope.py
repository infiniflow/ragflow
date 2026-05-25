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

import threading

import pytest

from rag.llm import dashscope_utils


def test_dashscope_native_api_url_scope_restores_previous_endpoint(monkeypatch):
    original_url = "https://dashscope.aliyuncs.com/api/v1"
    custom_url = "https://dashscope-intl.aliyuncs.com/api/v1"
    monkeypatch.setattr(dashscope_utils.dashscope, "base_http_api_url", original_url, raising=False)

    with dashscope_utils.dashscope_native_api_url_scope(custom_url):
        assert dashscope_utils.dashscope.base_http_api_url == custom_url

    assert dashscope_utils.dashscope.base_http_api_url == original_url


def test_dashscope_default_scope_waits_for_custom_endpoint_to_restore(monkeypatch):
    original_url = "https://dashscope.aliyuncs.com/api/v1"
    custom_url = "https://dashscope-intl.aliyuncs.com/api/v1"
    monkeypatch.setattr(dashscope_utils.dashscope, "base_http_api_url", original_url, raising=False)

    custom_entered = threading.Event()
    release_custom = threading.Event()
    default_ready = threading.Event()
    default_may_enter = threading.Event()
    default_entered = threading.Event()
    observed_urls = []
    errors = []

    def custom_call():
        try:
            with dashscope_utils.dashscope_native_api_url_scope(custom_url):
                custom_entered.set()
                if not release_custom.wait(2):
                    errors.append("custom scope was not released")
        except Exception as exc:
            errors.append(exc)

    def default_call():
        try:
            if not custom_entered.wait(2):
                errors.append("custom scope was not entered")
                return
            default_ready.set()
            if not default_may_enter.wait(2):
                errors.append("default scope was not released to enter")
                return
            with dashscope_utils.dashscope_native_api_url_scope(None):
                observed_urls.append(dashscope_utils.dashscope.base_http_api_url)
                default_entered.set()
        except Exception as exc:
            errors.append(exc)

    custom_thread = threading.Thread(target=custom_call)
    default_thread = threading.Thread(target=default_call)

    custom_thread.start()
    assert custom_entered.wait(2)

    default_thread.start()
    assert default_ready.wait(2)
    default_may_enter.set()
    assert not default_entered.wait(0.2)

    release_custom.set()
    custom_thread.join(2)
    default_thread.join(2)

    assert not custom_thread.is_alive()
    assert not default_thread.is_alive()
    assert errors == []
    assert observed_urls == [original_url]
    assert dashscope_utils.dashscope.base_http_api_url == original_url


def test_dashscope_text_embedding_call_rejects_unsupported_text_type():
    with pytest.raises(ValueError, match="unsupported DashScope embedding text_type"):
        dashscope_utils.dashscope_text_embedding_call(None, "text-embedding-v2", "query", "api-key", "invalid")
