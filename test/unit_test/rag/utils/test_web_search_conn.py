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

from rag.utils import web_search_conn


def test_create_web_search_provider_uses_existing_tavily_config_without_provider_field(monkeypatch):
    created_with = []
    provider = object()

    monkeypatch.setattr(web_search_conn, "Tavily", lambda api_key: created_with.append(api_key) or provider)

    result = web_search_conn.create_web_search_provider({"tavily_api_key": "tvly-test"})

    assert result is provider
    assert created_with == ["tvly-test"]


def test_create_web_search_provider_uses_selected_querit_config(monkeypatch):
    created_with = []
    provider = object()

    monkeypatch.setattr(web_search_conn, "Querit", lambda api_key: created_with.append(api_key) or provider)

    result = web_search_conn.create_web_search_provider(
        {
            "web_search_provider": "querit",
            "querit_api_key": "querit-test",
            "tavily_api_key": "tvly-test",
        }
    )

    assert result is provider
    assert created_with == ["querit-test"]


def test_create_web_search_provider_trims_selected_key(monkeypatch):
    created_with = []
    provider = object()

    monkeypatch.setattr(web_search_conn, "Querit", lambda api_key: created_with.append(api_key) or provider)

    result = web_search_conn.create_web_search_provider(
        {
            "web_search_provider": "querit",
            "querit_api_key": "  querit-test  ",
        }
    )

    assert result is provider
    assert created_with == ["querit-test"]


def test_create_web_search_provider_requires_key_for_selected_provider():
    assert web_search_conn.create_web_search_provider({}) is None
    assert web_search_conn.create_web_search_provider(None) is None
    assert web_search_conn.create_web_search_provider({"web_search_provider": "tavily"}) is None
    assert web_search_conn.create_web_search_provider({"web_search_provider": "querit"}) is None
    assert web_search_conn.create_web_search_provider({"tavily_api_key": "   "}) is None
    assert (
        web_search_conn.create_web_search_provider(
            {
                "web_search_provider": "querit",
                "querit_api_key": "   ",
            }
        )
        is None
    )


def test_has_web_search_provider_follows_selected_provider():
    assert web_search_conn.has_web_search_provider({"tavily_api_key": "tvly-test"})
    assert not web_search_conn.has_web_search_provider({"tavily_api_key": ""})
    assert web_search_conn.has_web_search_provider({"web_search_provider": "querit", "querit_api_key": "querit-test"})
    assert not web_search_conn.has_web_search_provider(
        {
            "web_search_provider": "querit",
            "querit_api_key": "",
            "tavily_api_key": "tvly-test",
        }
    )
    assert not web_search_conn.has_web_search_provider(
        {
            "web_search_provider": "unsupported",
            "querit_api_key": "querit-test",
            "tavily_api_key": "tvly-test",
        }
    )
