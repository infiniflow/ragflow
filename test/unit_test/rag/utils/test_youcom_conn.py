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

from rag.utils import youcom_conn


class _Response:
    status_code = 200

    def __init__(self, payload=None):
        self._payload = payload if payload is not None else _default_payload()

    def raise_for_status(self):
        return None

    def json(self):
        return self._payload


def _default_payload():
    return {
        "results": {
            "web": [
                {
                    "title": "RAGFlow",
                    "url": "https://example.com/ragflow",
                    "description": "Meta description.",
                    "snippets": ["RAGFlow is an open-source RAG engine.", "It does deep document understanding."],
                }
            ],
            "news": [
                {
                    "title": "RAGFlow ships",
                    "url": "https://news.example.com/ragflow",
                    "description": "News description only.",
                }
            ],
        }
    }


def _capture_get(monkeypatch, response=None):
    request = {}

    def fake_get(url, *, headers, params, timeout):
        request.update(url=url, headers=headers, params=params, timeout=timeout)
        return response if response is not None else _Response()

    monkeypatch.setattr(youcom_conn.requests, "get", fake_get)
    return request


def test_youcom_search_uses_keyless_endpoint_without_a_key(monkeypatch):
    request = _capture_get(monkeypatch)

    results = youcom_conn.YouCom("").search("What is RAGFlow?")

    assert request["url"] == "https://api.you.com/v1/agents/search"
    # The keyless endpoint rejects an auth header, so none may be sent.
    assert "X-API-Key" not in request["headers"]
    assert request["params"] == {"query": "What is RAGFlow?", "count": 6}
    assert len(results) == 2


def test_youcom_search_uses_keyed_endpoint_when_a_key_is_set(monkeypatch):
    request = _capture_get(monkeypatch)

    youcom_conn.YouCom("ydc-test").search("What is RAGFlow?")

    assert request["url"] == "https://api.you.com/v1/search"
    assert request["headers"]["X-API-Key"] == "ydc-test"


def test_youcom_search_trims_the_key_and_identifies_ragflow(monkeypatch):
    request = _capture_get(monkeypatch)

    youcom_conn.YouCom("  ydc-test  ").search("What is RAGFlow?")

    assert request["headers"]["X-API-Key"] == "ydc-test"
    assert request["headers"]["User-Agent"] == "RAGFlow youdotcom-integration/infiniflow-ragflow"


def test_youcom_search_prefers_passages_and_falls_back_to_description(monkeypatch):
    _capture_get(monkeypatch)

    results = youcom_conn.YouCom("").search("What is RAGFlow?")

    assert results == [
        {
            "url": "https://example.com/ragflow",
            "title": "RAGFlow",
            "content": "RAGFlow is an open-source RAG engine.\nIt does deep document understanding.",
            "score": 1.0,
        },
        {
            "url": "https://news.example.com/ragflow",
            "title": "RAGFlow ships",
            "content": "News description only.",
            "score": 1.0,
        },
    ]


def test_youcom_search_skips_results_without_content(monkeypatch):
    payload = {"results": {"web": [{"title": "No content", "url": "https://example.com/empty", "snippets": ["  "]}]}}
    _capture_get(monkeypatch, _Response(payload))

    assert youcom_conn.YouCom("").search("What is RAGFlow?") == []


def test_youcom_search_caps_the_merged_sections(monkeypatch):
    payload = {
        "results": {
            "web": [{"title": f"w{i}", "url": f"https://example.com/w{i}", "description": "d"} for i in range(6)],
            "news": [{"title": f"n{i}", "url": f"https://example.com/n{i}", "description": "d"} for i in range(6)],
        }
    }
    _capture_get(monkeypatch, _Response(payload))

    results = youcom_conn.YouCom("").search("What is RAGFlow?")

    # `count` applies per section, so the merged list is trimmed back to 6.
    assert len(results) == 6
    assert [result["title"] for result in results] == ["w0", "w1", "w2", "w3", "w4", "w5"]


def test_youcom_search_returns_empty_on_malformed_payloads(monkeypatch):
    _capture_get(monkeypatch, _Response(["not", "an", "object"]))
    assert youcom_conn.YouCom("").search("q") == []

    _capture_get(monkeypatch, _Response({"results": "not-an-object"}))
    assert youcom_conn.YouCom("").search("q") == []

    _capture_get(monkeypatch, _Response({"results": {"web": "not-an-array"}}))
    assert youcom_conn.YouCom("").search("q") == []


class _ErrorResponse:
    """A response whose raise_for_status() carries the full request URL, the way
    requests builds it — including the query string."""

    status_code = 402
    url = "https://api.you.com/v1/agents/search?query=my%20private%20query&count=6"

    def raise_for_status(self):
        raise youcom_conn.requests.HTTPError(
            f"402 Client Error: Payment Required for url: {self.url}",
            response=self,
        )

    def json(self):  # pragma: no cover - never reached
        return {}


def test_youcom_search_never_logs_the_query_or_key_on_http_errors(monkeypatch, caplog):
    """The query is a URL parameter, so the requests error message contains it."""
    monkeypatch.setattr(youcom_conn.requests, "get", lambda *a, **k: _ErrorResponse())

    with caplog.at_level("ERROR"):
        assert youcom_conn.YouCom("ydc-secret").search("my private query") == []

    assert "my private query" not in caplog.text
    assert "my%20private%20query" not in caplog.text
    assert "ydc-secret" not in caplog.text
    # Only the status code is useful and safe to record.
    assert "402" in caplog.text


def test_youcom_search_logs_only_the_exception_type_on_transport_errors(monkeypatch, caplog):
    def fake_get(url, *, headers, params, timeout):
        raise youcom_conn.requests.ConnectionError(f"failed connecting to {url}?query=my%20private%20query")

    monkeypatch.setattr(youcom_conn.requests, "get", fake_get)

    with caplog.at_level("ERROR"):
        assert youcom_conn.YouCom("ydc-secret").search("my private query") == []

    assert "my%20private%20query" not in caplog.text
    assert "ydc-secret" not in caplog.text
    assert "ConnectionError" in caplog.text


def test_youcom_retrieve_chunks_returns_ragflow_chunk_shape(monkeypatch):
    _capture_get(monkeypatch)

    retrieved = youcom_conn.YouCom("").retrieve_chunks("What is RAGFlow?")

    assert set(retrieved) == {"chunks", "doc_aggs"}
    assert len(retrieved["chunks"]) == 2
    chunk = retrieved["chunks"][0]
    assert chunk["url"] == "https://example.com/ragflow"
    assert chunk["docnm_kwd"] == "RAGFlow"
    assert chunk["content_with_weight"].startswith("RAGFlow is an open-source RAG engine.")
    assert chunk["similarity"] == 1.0
    doc_agg = retrieved["doc_aggs"][0]
    assert doc_agg["doc_name"] == "RAGFlow"
    assert doc_agg["doc_id"] == chunk["chunk_id"]
