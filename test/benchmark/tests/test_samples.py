"""Request failures must become samples, not successful or missing measurements."""

import json
from unittest.mock import Mock

import pytest
import requests

from test.benchmark.chat import stream_chat_completion
from test.benchmark.http_client import HttpClient
from test.benchmark.retrieval import run_retrieval


def stream_response(events, status=200):
    response = Mock(spec=requests.Response)
    response.status_code = status
    response.headers = {"Content-Type": "text/event-stream"}

    def lines(**kwargs):
        for event in events:
            if isinstance(event, Exception):
                raise event
            yield "data: " + (event if isinstance(event, str) else json.dumps(event))

    response.iter_lines.side_effect = lines
    return response


CONTENT = {"choices": [{"delta": {"content": "partial answer"}}]}


def test_chat_eof_after_content_is_failure():
    client = Mock(spec=HttpClient)
    client.request.return_value = stream_response([CONTENT])
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error
    assert sample.response_text == "partial answer"
    client.request.return_value.close.assert_called_once()


@pytest.mark.parametrize("failure", [requests.ConnectTimeout(), requests.ConnectionError()])
def test_connection_failure_returns_chat_sample(failure):
    client = Mock(spec=HttpClient)
    client.request.side_effect = failure
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error
    assert sample.first_token_latency is None


def test_midstream_timeout_retains_partial_response():
    client = Mock(spec=HttpClient)
    client.request.return_value = stream_response([CONTENT, requests.ReadTimeout()])
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error
    assert sample.response_text == "partial answer"
    assert sample.first_token_latency is not None
    client.request.return_value.close.assert_called_once()


@pytest.mark.parametrize("failure", [requests.ReadTimeout(), requests.ConnectionError()])
def test_connection_failure_returns_retrieval_sample(failure):
    client = Mock(spec=HttpClient)
    client.request.side_effect = failure
    sample = run_retrieval(client, {"question": "question", "dataset_ids": ["dataset"]})
    assert sample.error


def test_retrieval_error_without_message_is_failure():
    response = requests.Response()
    response.status_code = 200
    response._content = b'{"code": 102}'
    client = HttpClient("http://unused.invalid")
    client.request = Mock(return_value=response)
    sample = run_retrieval(client, {})
    assert sample.error


@pytest.mark.parametrize("ending", ["[DONE]", {"choices": [{"delta": {}, "finish_reason": "stop"}]}])
def test_completed_chat_remains_successful(ending):
    client = Mock(spec=HttpClient)
    client.request.return_value = stream_response([CONTENT, ending])
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error is None
    assert sample.response_text == "partial answer"
    assert sample.total_latency >= sample.first_token_latency >= 0
    client.request.return_value.close.assert_called_once()


@pytest.mark.parametrize("event", ["not-json", [], None, {"choices": [None]}, {"choices": {"delta": {}}}, {"choices": [{"delta": "bad"}]}, {"error": {"message": "unavailable"}}, {"code": 102}])
def test_malformed_or_error_event_after_content_is_failure(event):
    client = Mock(spec=HttpClient)
    client.request.return_value = stream_response([CONTENT, event, "[DONE]"])
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error
    assert sample.response_text == "partial answer"
    client.request.return_value.close.assert_called_once()


def test_chat_http_error_cannot_be_hidden_by_valid_sse_body():
    client = Mock(spec=HttpClient)
    response = stream_response([CONTENT, "[DONE]"], status=503)
    client.request.return_value = response
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error == "HTTP 503"
    response.iter_lines.assert_not_called()
    response.close.assert_called_once()


@pytest.mark.parametrize("payload", [[], None, {"code": 102}, {"code": 102, "message": ""}])
def test_non_stream_chat_errors_do_not_escape(payload):
    client = Mock(spec=HttpClient)
    response = Mock(spec=requests.Response)
    response.status_code = 200
    response.headers = {"Content-Type": "application/json"}
    response.json.return_value = payload
    client.request.return_value = response
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error
    response.close.assert_called_once()


@pytest.mark.parametrize("status,payload", [(503, {"code": 0}), (200, []), (200, None), (200, {}), (200, {"code": 102, "message": ""})])
def test_invalid_retrieval_response_is_failure(status, payload):
    response = requests.Response()
    response.status_code = status
    response._content = json.dumps(payload).encode()
    client = HttpClient("http://unused.invalid")
    client.request = Mock(return_value=response)
    sample = run_retrieval(client, {})
    assert sample.error


def test_successful_empty_retrieval_is_not_a_transport_failure():
    response = requests.Response()
    response.status_code = 200
    response._content = b'{"code": 0, "data": {"chunks": []}}'
    client = HttpClient("http://unused.invalid")
    client.request = Mock(return_value=response)
    sample = run_retrieval(client, {})
    assert sample.error is None
    assert sample.response["data"]["chunks"] == []


def test_empty_completed_chat_still_fails():
    client = Mock(spec=HttpClient)
    client.request.return_value = stream_response(["[DONE]"])
    sample = stream_chat_completion(client, "chat", "model", [])
    assert sample.error == "No assistant content received"


def test_programming_errors_are_not_swallowed():
    client = Mock(spec=HttpClient)
    client.request.side_effect = RuntimeError("unexpected bug")
    with pytest.raises(RuntimeError, match="unexpected bug"):
        stream_chat_completion(client, "chat", "model", [])
    with pytest.raises(RuntimeError, match="unexpected bug"):
        run_retrieval(client, {})
