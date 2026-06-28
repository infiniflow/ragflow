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
    chat_completions,
    create_chat_assistant,
    create_session_with_chat_assistant,
    delete_all_chat_assistants,
    delete_all_sessions_with_chat_assistant,
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


class TestChatCompletions:
    @pytest.mark.p3
    def test_chat_completion_stream_false_with_session(self, HttpApiAuth, add_dataset_func, tmp_path, request):
        dataset_id = add_dataset_func
        document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0, res
        _parse_done(HttpApiAuth, dataset_id, document_ids)

        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_test", "dataset_ids": [dataset_id]})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_all_chat_assistants(HttpApiAuth))
        request.addfinalizer(lambda: delete_all_sessions_with_chat_assistant(HttpApiAuth, chat_id))

        res = create_session_with_chat_assistant(HttpApiAuth, chat_id, {"name": "session_for_completion"})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
                "session_id": session_id,
            },
        )
        assert res["code"] == 0, res
        assert isinstance(res["data"], dict), res
        for key in ["answer", "reference", "audio_binary", "id", "session_id"]:
            assert key in res["data"], res
        assert res["data"]["session_id"] == session_id, res

    @pytest.mark.p2
    def test_chat_completion_invalid_chat(self, HttpApiAuth):
        res = chat_completions(
            HttpApiAuth,
            "invalid_chat_id",
            {
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
                "session_id": "invalid_session",
            },
        )
        assert res["code"] == 109, res
        assert "No authorization." in res.get("message", ""), res

    @pytest.mark.p2
    def test_chat_completion_invalid_session(self, HttpApiAuth, request):
        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_invalid_session", "dataset_ids": []})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_all_chat_assistants(HttpApiAuth))
        request.addfinalizer(lambda: delete_all_sessions_with_chat_assistant(HttpApiAuth, chat_id))

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
                "session_id": "invalid_session",
            },
        )
        assert res["code"] == 102, res
        assert "Session not found!" in res.get("message", ""), res

    @pytest.mark.p2
    def test_chat_completion_stream_false_with_chat_without_session(self, HttpApiAuth, request):
        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_invalid_meta", "dataset_ids": []})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_all_chat_assistants(HttpApiAuth))
        request.addfinalizer(lambda: delete_all_sessions_with_chat_assistant(HttpApiAuth, chat_id))

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
            },
        )
        assert res["code"] == 0, res
        assert res["data"]["session_id"], res

    @pytest.mark.p2
    def test_chat_completion_stream_false_without_chat(self, HttpApiAuth):
        res = chat_completions(
            HttpApiAuth,
            None,
            {
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
            },
        )
        assert res["code"] == 0, res
        assert isinstance(res["data"], dict), res
        assert "answer" in res["data"], res
