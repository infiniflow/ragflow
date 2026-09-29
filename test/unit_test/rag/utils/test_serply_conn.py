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

from rag.utils import serply_conn


class _Response:
    status_code = 200

    def raise_for_status(self):
        return None

    def json(self):
        return {
            "results": [
                {
                    "title": "RAGFlow",
                    "link": "https://example.com/ragflow",
                    "description": " \tRAGFlow is an open-source RAG engine.\n",
                },
                {
                    "title": "No snippet",
                    "link": "https://example.com/empty",
                    "description": "",
                },
                {
                    "title": "Blank snippet",
                    "link": "https://example.com/blank",
                    "description": " \t\n",
                },
            ]
        }


def test_serply_search_uses_chat_defaults_and_normalizes_results(monkeypatch):
    request = {}

    def fake_get(url, *, headers, params, timeout):
        request.update(url=url, headers=headers, params=params, timeout=timeout)
        return _Response()

    monkeypatch.setattr(serply_conn.requests, "get", fake_get)

    results = serply_conn.Serply("serply-test").search("What is RAGFlow?")

    assert request["url"] == "https://api.serply.io/v1/search/"
    assert request["headers"]["X-Api-Key"] == "serply-test"
    assert request["headers"]["User-Agent"]
    assert request["params"] == {
        "q": "What is RAGFlow?",
        "num": 6,
    }
    assert results == [
        {
            "url": "https://example.com/ragflow",
            "title": "RAGFlow",
            "content": "RAGFlow is an open-source RAG engine.",
            "score": 1.0,
        }
    ]


def test_serply_search_rejects_malformed_results_container(monkeypatch):
    class _MalformedResponse:
        def raise_for_status(self):
            return None

        def json(self):
            return {"results": {}}

    monkeypatch.setattr(serply_conn.requests, "get", lambda *_args, **_kwargs: _MalformedResponse())

    assert serply_conn.Serply("serply-test").search("RAGFlow") == []


def test_serply_retrieve_chunks_returns_ragflow_reference_shape(monkeypatch):
    monkeypatch.setattr(
        serply_conn.Serply,
        "search",
        lambda _self, _question: [
            {
                "url": "https://example.com/ragflow",
                "title": "RAGFlow",
                "content": "RAGFlow is an open-source RAG engine.",
                "score": 1.0,
            }
        ],
    )
    monkeypatch.setattr(serply_conn, "get_uuid", lambda: "chunk-1")
    monkeypatch.setattr(serply_conn.rag_tokenizer, "tokenize", lambda content: f"tokens:{content}")

    result = serply_conn.Serply("serply-test").retrieve_chunks("What is RAGFlow?")

    assert result["chunks"] == [
        {
            "chunk_id": "chunk-1",
            "content_ltks": "tokens:RAGFlow is an open-source RAG engine.",
            "content_with_weight": "RAGFlow is an open-source RAG engine.",
            "doc_id": "chunk-1",
            "docnm_kwd": "RAGFlow",
            "kb_id": [],
            "important_kwd": [],
            "image_id": "",
            "similarity": 1.0,
            "vector_similarity": 1.0,
            "term_similarity": 0,
            "vector": [],
            "positions": [],
            "url": "https://example.com/ragflow",
        }
    ]
    assert result["doc_aggs"] == [
        {
            "doc_name": "RAGFlow",
            "doc_id": "chunk-1",
            "count": 1,
            "url": "https://example.com/ragflow",
        }
    ]


def test_serply_search_keeps_the_key_out_of_failure_logs(monkeypatch, caplog):
    class _FailedResponse:
        def raise_for_status(self):
            raise ValueError("request failed with serply-secret")

    monkeypatch.setattr(serply_conn.requests, "get", lambda *_args, **_kwargs: _FailedResponse())

    with caplog.at_level("ERROR"):
        assert serply_conn.Serply("serply-secret").search("RAGFlow") == []

    assert "serply-secret" not in caplog.text
    assert "ValueError" in caplog.text


def test_serply_search_never_logs_the_query_from_the_request_url(monkeypatch, caplog):
    """Serply passes the query as a URL parameter, so the requests error
    message contains it."""

    class _ErrorResponse:
        status_code = 429
        url = "https://api.serply.io/v1/search/?q=my%20private%20query&num=6"

        def raise_for_status(self):
            raise serply_conn.requests.HTTPError(
                f"429 Client Error: Too Many Requests for url: {self.url}",
                response=self,
            )

    monkeypatch.setattr(serply_conn.requests, "get", lambda *_args, **_kwargs: _ErrorResponse())

    with caplog.at_level("ERROR"):
        assert serply_conn.Serply("serply-secret").search("my private query") == []

    assert "my private query" not in caplog.text
    assert "my%20private%20query" not in caplog.text
    assert "serply-secret" not in caplog.text
    assert "429" in caplog.text
