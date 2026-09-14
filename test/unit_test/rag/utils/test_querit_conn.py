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

from rag.utils import querit_conn


class _Response:
    status_code = 200

    def raise_for_status(self):
        return None

    def json(self):
        return {
            "results": {
                "result": [
                    {
                        "title": "RAGFlow",
                        "url": "https://example.com/ragflow",
                        "snippet": "RAGFlow is an open-source RAG engine.",
                    }
                ]
            }
        }


def test_querit_search_uses_chat_defaults_and_normalizes_results(monkeypatch):
    request = {}

    def fake_post(url, *, headers, json, timeout):
        request.update(url=url, headers=headers, json=json, timeout=timeout)
        return _Response()

    monkeypatch.setattr(querit_conn.requests, "post", fake_post)

    results = querit_conn.Querit("querit-test").search("What is RAGFlow?")

    assert request["url"] == "https://api.querit.ai/v1/search"
    assert request["headers"]["Authorization"] == "Bearer querit-test"
    assert request["json"] == {
        "query": "What is RAGFlow?",
        "count": 6,
        "chunksPerDoc": 1,
    }
    assert results == [
        {
            "url": "https://example.com/ragflow",
            "title": "RAGFlow",
            "content": "RAGFlow is an open-source RAG engine.",
            "score": 1.0,
        }
    ]


def test_querit_retrieve_chunks_returns_ragflow_reference_shape(monkeypatch):
    monkeypatch.setattr(
        querit_conn.Querit,
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
    monkeypatch.setattr(querit_conn, "get_uuid", lambda: "chunk-1")
    monkeypatch.setattr(querit_conn.rag_tokenizer, "tokenize", lambda content: f"tokens:{content}")

    result = querit_conn.Querit("querit-test").retrieve_chunks("What is RAGFlow?")

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


def test_querit_search_keeps_the_key_out_of_failure_logs(monkeypatch, caplog):
    class _FailedResponse:
        def raise_for_status(self):
            raise ValueError("request failed with querit-secret")

    monkeypatch.setattr(querit_conn.requests, "post", lambda *_args, **_kwargs: _FailedResponse())

    with caplog.at_level("ERROR"):
        assert querit_conn.Querit("querit-secret").search("RAGFlow") == []

    assert "querit-secret" not in caplog.text
    # Only the exception type survives into the log.
    assert "ValueError" in caplog.text


def test_querit_search_logs_only_the_status_on_http_errors(monkeypatch, caplog):
    class _ErrorResponse:
        status_code = 402
        url = "https://api.querit.ai/v1/search"

        def raise_for_status(self):
            raise querit_conn.requests.HTTPError(
                f"402 Client Error: Payment Required for url: {self.url}",
                response=self,
            )

    monkeypatch.setattr(querit_conn.requests, "post", lambda *_args, **_kwargs: _ErrorResponse())

    with caplog.at_level("ERROR"):
        assert querit_conn.Querit("querit-secret").search("my private query") == []

    assert "my private query" not in caplog.text
    assert "querit-secret" not in caplog.text
    assert "402" in caplog.text


def test_querit_retrieve_chunks_never_logs_the_query_or_page_text(monkeypatch, caplog):
    monkeypatch.setattr(
        querit_conn.Querit,
        "search",
        lambda self, question: [
            {
                "url": "https://example.com/a",
                "title": "A",
                "content": "secret page body text",
                "score": 1.0,
            }
        ],
    )

    with caplog.at_level("INFO"):
        retrieved = querit_conn.Querit("k").retrieve_chunks("my private query")

    assert len(retrieved["chunks"]) == 1
    assert "my private query" not in caplog.text
    assert "secret page body text" not in caplog.text
    assert "1" in caplog.text
