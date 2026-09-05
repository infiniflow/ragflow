import json
from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError

from test.playwright.helpers import _next_apps_helpers as chat
from test.playwright.e2e.test_next_apps_search import _assert_search_result, _is_search_response


def stream(*events, status=200):
    body = "".join(f"data: {json.dumps(event)}\n\n" for event in events)
    return SimpleNamespace(status=status, text=lambda: body)


ANSWER = {"code": 0, "data": {"answer": "A real answer"}}
DONE = {"code": 0, "data": True}


def test_chat_accepts_successful_answer_and_terminal_event():
    chat._assert_successful_chat_stream(stream(ANSWER, DONE))


@pytest.mark.parametrize("response", [
    stream(status=503),
    stream(DONE),
    stream(ANSWER),
    stream({"code": 500, "data": {"answer": "**ERROR**"}}, DONE),
    stream(ANSWER, {"code": 500, "message": "failed after partial answer"}, DONE),
    stream({"code": 0, "data": {"answer": "  "}}, DONE),
])
def test_chat_rejects_failed_empty_or_truncated_stream(response):
    with pytest.raises(AssertionError):
        chat._assert_successful_chat_stream(response)


@pytest.mark.parametrize("url,method,messages,expected", [
    ("/api/v1/chat/completions", "POST", [{"role": "user", "content": "question"}], True),
    ("/api/v1/chat/completions", "GET", [{"role": "user", "content": "question"}], False),
    ("/api/v1/chat/completions", "POST", [{"role": "user", "content": "old question"}], False),
    ("/api/v1/searches/x/completions", "POST", [{"role": "user", "content": "question"}], False),
    ("/api/v1/chat/completions", "POST", [], False),
])
def test_chat_correlates_only_the_submitted_prompt(url, method, messages, expected):
    request = SimpleNamespace(url="http://test" + url, method=method, post_data_json={"messages": messages})
    assert chat._is_chat_completion_request(request, "question") is expected


def test_idle_ui_without_completion_cannot_pass(monkeypatch):
    page = MagicMock()
    page.locator.return_value.count.return_value = 1
    page.locator.return_value.evaluate.return_value = "TEXTAREA"
    page.locator.return_value.input_value.return_value = "question"
    page.locator.return_value.locator.return_value.count.return_value = 0
    page.expect_event.side_effect = PlaywrightTimeoutError("No completion request finished")
    monkeypatch.setattr(chat, "expect", lambda locator: MagicMock())
    with pytest.raises(PlaywrightTimeoutError, match="No completion"):
        chat._send_chat_and_wait_done(page, "question", timeout_ms=10)


def result(data, code=0, status=200):
    return SimpleNamespace(status=status, json=lambda: {"code": code, "data": data})


def test_search_accepts_seeded_document_and_content():
    _assert_search_result(result({"total": 1, "chunks": [{"doc_id": "seed", "content_with_weight": "known snippet"}]}), "seed", "known snippet")


@pytest.mark.parametrize("response", [
    result({"total": 0, "chunks": []}),
    result({"total": 1, "chunks": [{"doc_id": "other", "content_with_weight": "known snippet"}]}),
    result({"total": 1, "chunks": [{"doc_id": "seed", "content_with_weight": "wrong content"}]}),
    result({}, code=102),
    result({}, status=500),
])
def test_search_rejects_empty_failed_or_unrelated_results(response):
    with pytest.raises(AssertionError):
        _assert_search_result(response, "seed", "known snippet")


@pytest.mark.parametrize("query,datasets,expected", [
    ("known query", ["seed"], True),
    ("old query", ["seed"], False),
    ("known query", ["other"], False),
])
def test_search_correlates_query_and_dataset(query, datasets, expected):
    response = SimpleNamespace(
        url="http://test/api/v1/datasets/search",
        request=SimpleNamespace(method="POST", post_data_json={"question": query, "dataset_ids": datasets}),
    )
    assert _is_search_response(response, "known query", "seed") is expected
