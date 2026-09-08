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

from rag.utils import tavily_conn


class _StubClient:
    def __init__(self, results=None, error=None):
        self._results = results or []
        self._error = error

    def search(self, **_kwargs):
        if self._error is not None:
            raise self._error
        return {"results": self._results}


def _tavily(client) -> tavily_conn.Tavily:
    instance = tavily_conn.Tavily.__new__(tavily_conn.Tavily)
    instance.tavily_client = client
    return instance


def test_tavily_search_logs_only_the_exception_type(monkeypatch, caplog):
    """The client's exception can carry both the query and the API key."""
    error = RuntimeError("401 for https://api.tavily.com/search?q=my%20private%20query key=tvly-secret")

    with caplog.at_level("ERROR"):
        assert _tavily(_StubClient(error=error)).search("my private query") == []

    assert "my%20private%20query" not in caplog.text
    assert "tvly-secret" not in caplog.text
    assert "RuntimeError" in caplog.text


def test_tavily_retrieve_chunks_never_logs_the_query_or_page_text(caplog):
    results = [
        {
            "url": "https://example.com/ragflow",
            "title": "RAGFlow",
            "content": "secret page body text",
            "score": 0.9,
        }
    ]

    with caplog.at_level("INFO"):
        retrieved = _tavily(_StubClient(results=results)).retrieve_chunks("my private query")

    assert len(retrieved["chunks"]) == 1
    assert retrieved["chunks"][0]["url"] == "https://example.com/ragflow"
    assert "my private query" not in caplog.text
    assert "secret page body text" not in caplog.text


def test_tavily_retrieve_chunks_still_returns_the_reference_shape(caplog):
    results = [
        {"url": "https://example.com/a", "title": "A", "content": "body a", "score": 0.5},
        {"url": "https://example.com/b", "title": "B", "content": "body b", "score": 0.4},
    ]

    retrieved = _tavily(_StubClient(results=results)).retrieve_chunks("q")

    assert [c["url"] for c in retrieved["chunks"]] == ["https://example.com/a", "https://example.com/b"]
    assert [a["doc_name"] for a in retrieved["doc_aggs"]] == ["A", "B"]
    assert retrieved["chunks"][0]["similarity"] == 0.5
