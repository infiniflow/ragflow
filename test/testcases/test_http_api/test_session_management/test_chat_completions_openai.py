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
