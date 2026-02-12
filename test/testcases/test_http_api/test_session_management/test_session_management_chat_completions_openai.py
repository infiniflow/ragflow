#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import pytest
from pathlib import Path
import sys
import types
import json

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module
from common import (
    bulk_upload_documents,
    chat_completions_openai,
    create_chat_assistant,
    delete_chat_assistants,
    list_documents,
    parse_documents,
)
from utils import wait_for


@wait_for(200, 1, "Document parsing timeout")
def _parse_done(auth, dataset_id, document_ids=None):
    res = list_documents(auth, dataset_id)
    target_docs = res["data"]["docs"]
    if document_ids is None:
        return all(doc.get("run") == "DONE" for doc in target_docs)
    target_ids = set(document_ids)
    for doc in target_docs:
        if doc.get("id") in target_ids and doc.get("run") != "DONE":
            return False
    return True


class TestChatCompletionsOpenAI:
    """Test cases for the OpenAI-compatible chat completions endpoint"""

    @pytest.mark.p2
    def test_openai_chat_completion_non_stream(self, HttpApiAuth, add_dataset_func, tmp_path, request):
        """Test OpenAI-compatible endpoint returns proper response with token usage"""
        dataset_id = add_dataset_func
        document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0, res
        _parse_done(HttpApiAuth, dataset_id, document_ids)

        res = create_chat_assistant(HttpApiAuth, {"name": "openai_endpoint_test", "dataset_ids": [dataset_id]})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = chat_completions_openai(
            HttpApiAuth,
            chat_id,
            {
                "model": "model",  # Required by OpenAI-compatible API, value is ignored by RAGFlow
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
            },
        )

        # Verify OpenAI-compatible response structure
        assert "choices" in res, f"Response should contain 'choices': {res}"
        assert len(res["choices"]) > 0, f"'choices' should not be empty: {res}"
        assert "message" in res["choices"][0], f"Choice should contain 'message': {res}"
        assert "content" in res["choices"][0]["message"], f"Message should contain 'content': {res}"

        # Verify token usage is present and uses actual token counts (not character counts)
        assert "usage" in res, f"Response should contain 'usage': {res}"
        usage = res["usage"]
        assert "prompt_tokens" in usage, f"'usage' should contain 'prompt_tokens': {usage}"
        assert "completion_tokens" in usage, f"'usage' should contain 'completion_tokens': {usage}"
        assert "total_tokens" in usage, f"'usage' should contain 'total_tokens': {usage}"
        assert usage["total_tokens"] == usage["prompt_tokens"] + usage["completion_tokens"], \
            f"total_tokens should equal prompt_tokens + completion_tokens: {usage}"

    @pytest.mark.p2
    def test_openai_chat_completion_token_count_reasonable(self, HttpApiAuth, add_dataset_func, tmp_path, request):
        """Test that token counts are reasonable (using tiktoken, not character counts)"""
        dataset_id = add_dataset_func
        document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0, res
        _parse_done(HttpApiAuth, dataset_id, document_ids)

        res = create_chat_assistant(HttpApiAuth, {"name": "openai_token_count_test", "dataset_ids": [dataset_id]})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        # Use a message with known token count
        # "hello" is 1 token in cl100k_base encoding
        res = chat_completions_openai(
            HttpApiAuth,
            chat_id,
            {
                "model": "model",  # Required by OpenAI-compatible API, value is ignored by RAGFlow
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
            },
        )

        assert "usage" in res, f"Response should contain 'usage': {res}"
        usage = res["usage"]

        # The prompt tokens should be reasonable for the message "hello" plus any system context
        # If using len() instead of tiktoken, a short response could have equal or fewer tokens
        # than characters, which would be incorrect
        # With tiktoken, "hello" = 1 token, so prompt_tokens should include that plus context
        assert usage["prompt_tokens"] > 0, f"prompt_tokens should be greater than 0: {usage}"
        assert usage["completion_tokens"] > 0, f"completion_tokens should be greater than 0: {usage}"

    @pytest.mark.p2
    def test_openai_chat_completion_invalid_chat(self, HttpApiAuth):
        """Test OpenAI endpoint returns error for invalid chat ID"""
        res = chat_completions_openai(
            HttpApiAuth,
            "invalid_chat_id",
            {
                "model": "model",  # Required by OpenAI-compatible API, value is ignored by RAGFlow
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
            },
        )
        # Should return an error (format may vary based on implementation)
        assert "error" in res or res.get("code") != 0, f"Should return error for invalid chat: {res}"

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"extra_body": "bad"}, "extra_body must be an object"),
            ({"extra_body": {"reference_metadata": "bad"}}, "reference_metadata must be an object"),
            ({"extra_body": {"reference_metadata": {"fields": "bad"}}}, "reference_metadata.fields must be an array"),
            ({"extra_body": {"metadata_condition": "bad"}}, "metadata_condition must be an object"),
            ({"messages": []}, "You have to provide messages"),
            ({"messages": [{"role": "assistant", "content": "hello"}]}, "last content of this conversation"),
        ],
    )
    def test_openai_chat_completion_extra_body_validation(self, HttpApiAuth, payload, expected_message, request):
        res = create_chat_assistant(HttpApiAuth, {"name": "openai_validation_test", "dataset_ids": []})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        base_payload = {
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        }
        base_payload.update(payload)
        res = chat_completions_openai(HttpApiAuth, chat_id, base_payload)
        assert res.get("code") != 0 or "error" in res, res
        assert expected_message in res.get("message", "") or expected_message in res.get("error", ""), res


@pytest.mark.asyncio
async def test_openai_chat_completion_metadata_condition_type(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
            "extra_body": {"metadata_condition": "bad"},
        }

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=[])])

    resp = await mod.chat_completion_openai_like("tenant", "chat")
    assert resp["code"] != 0
    assert "metadata_condition" in resp["message"]


@pytest.mark.asyncio
async def test_openai_chat_completion_metadata_condition_doc_ids(monkeypatch):
    mod = load_session_module(monkeypatch)
    captured = {}

    async def _get_request_json():
        return {
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": True,
            "extra_body": {"metadata_condition": {"conditions": [{"name": "author"}]}},
        }

    async def _async_chat(_dia, _msg, _stream, **kwargs):
        captured.update(kwargs)
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=["kb"])])
    mod.meta_filter = lambda _metas, _conditions, _logic: []
    mod.async_chat = _async_chat

    resp = await mod.chat_completion_openai_like("tenant", "chat")
    assert isinstance(resp, mod.Response)
    assert captured["doc_ids"] == "-999"


@pytest.mark.asyncio
async def test_openai_chat_completion_message_filtering(monkeypatch):
    mod = load_session_module(monkeypatch)
    captured = {}

    async def _get_request_json():
        return {
            "model": "model",
            "messages": [
                {"role": "system", "content": "sys"},
                {"role": "assistant", "content": "assistant"},
                {"role": "user", "content": "hello"},
            ],
            "stream": False,
        }

    async def _async_chat(_dia, msg, _stream, **_kwargs):
        captured["msg"] = msg
        yield {"answer": "ok"}

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=[])])
    mod.async_chat = _async_chat

    resp = await mod.chat_completion_openai_like("tenant", "chat")
    assert resp["choices"][0]["message"]["content"] == "ok"
    assert all(m["role"] != "system" for m in captured["msg"])
    assert captured["msg"][0]["role"] == "user"


@pytest.mark.asyncio
async def test_openai_chat_completion_stream_reasoning_and_reference(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {
            "model": "model",
            "messages": [{"role": "user", "content": "hi"}],
            "stream": True,
            "extra_body": {"reference": True},
        }

    async def _async_chat(_dia, _msg, _stream, **_kwargs):
        yield {"start_to_think": True}
        yield {"answer": "think"}
        yield {"end_to_think": True}
        yield {"answer": "hello"}
        yield {"final": True, "answer": "final", "reference": {"chunks": [{"chunk_id": "c1", "content": "ref"}]}}

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=[])])
    mod.async_chat = _async_chat

    resp = await mod.chat_completion_openai_like("tenant", "chat")
    payloads = []
    done = False
    async for line in resp.response:
        if not line.startswith("data:"):
            continue
        data = line[5:].strip()
        if data == "[DONE]":
            done = True
            continue
        payloads.append(json.loads(data))

    assert done is True
    last = payloads[-1]
    delta = last["choices"][0]["delta"]
    assert delta["reference"][0]["id"] == "c1"
    assert delta["final_content"] == "final"
    assert last["usage"]["total_tokens"] == last["usage"]["prompt_tokens"] + last["usage"]["completion_tokens"]


@pytest.mark.asyncio
async def test_openai_chat_completion_stream_error_chunk(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {
            "model": "model",
            "messages": [{"role": "user", "content": "hi"}],
            "stream": True,
        }

    async def _async_chat(_dia, _msg, _stream, **_kwargs):
        raise RuntimeError("boom")
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=[])])
    mod.async_chat = _async_chat

    resp = await mod.chat_completion_openai_like("tenant", "chat")
    lines = []
    async for line in resp.response:
        lines.append(line)
    assert any("**ERROR**" in line for line in lines)


@pytest.mark.asyncio
async def test_openai_chat_completion_non_stream_reference(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {
            "model": "model",
            "messages": [{"role": "user", "content": "hi"}],
            "stream": False,
            "extra_body": {"reference": True, "metadata_condition": {"conditions": [{"name": "author"}]}},
        }

    async def _async_chat(_dia, _msg, _stream, **kwargs):
        assert kwargs.get("doc_ids") == "-999"
        yield {"answer": "ok", "reference": {"chunks": [{"chunk_id": "c1", "content": "ref"}]}}

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(kb_ids=["kb"])])
    mod.meta_filter = lambda _metas, _conditions, _logic: []
    mod.async_chat = _async_chat

    resp = await mod.chat_completion_openai_like("tenant", "chat")
    assert resp["choices"][0]["message"]["reference"][0]["id"] == "c1"
