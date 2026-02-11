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
import requests
import pytest
from common import (
    bulk_upload_documents,
    chat_completions,
    create_chat_assistant,
    create_session_with_chat_assistant,
    delete_chat_assistants,
    delete_session_with_chat_assistants,
    HEADERS,
    list_documents,
    parse_documents,
)
from configs import HOST_ADDRESS, VERSION
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
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = create_session_with_chat_assistant(HttpApiAuth, chat_id, {"name": "session_for_completion"})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {"question": "hello", "stream": False, "session_id": session_id},
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
            {"question": "hello", "stream": False, "session_id": "invalid_session"},
        )
        assert res["code"] == 102, res
        assert "You don't own the chat" in res.get("message", ""), res

    @pytest.mark.p2
    def test_chat_completion_invalid_session(self, HttpApiAuth, request):
        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_invalid_session", "dataset_ids": []})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {"question": "hello", "stream": False, "session_id": "invalid_session"},
        )
        assert res["code"] == 102, res
        assert "You don't own the session" in res.get("message", ""), res

    @pytest.mark.p2
    def test_chat_completion_invalid_metadata_condition(self, HttpApiAuth, request):
        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_invalid_meta", "dataset_ids": []})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = create_session_with_chat_assistant(HttpApiAuth, chat_id, {"name": "session_for_meta"})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        res = chat_completions(
            HttpApiAuth,
            chat_id,
            {
                "question": "hello",
                "stream": False,
                "session_id": session_id,
                "metadata_condition": "invalid",
            },
        )
        assert res["code"] == 102, res
        assert "metadata_condition" in res.get("message", ""), res

    @pytest.mark.p3
    def test_chat_completion_stream_minimal(self, HttpApiAuth, add_dataset_func, tmp_path, request):
        dataset_id = add_dataset_func
        document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0, res
        _parse_done(HttpApiAuth, dataset_id, document_ids)

        res = create_chat_assistant(HttpApiAuth, {"name": "chat_completion_stream_test", "dataset_ids": [dataset_id]})
        assert res["code"] == 0, res
        chat_id = res["data"]["id"]
        request.addfinalizer(lambda: delete_session_with_chat_assistants(HttpApiAuth, chat_id))
        request.addfinalizer(lambda: delete_chat_assistants(HttpApiAuth))

        res = create_session_with_chat_assistant(HttpApiAuth, chat_id, {"name": "session_for_stream"})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        url = f"{HOST_ADDRESS}/api/{VERSION}/chats/{chat_id}/completions"
        stream_res = requests.post(
            url=url,
            headers=HEADERS,
            auth=HttpApiAuth,
            json={"question": "hello", "stream": True, "session_id": session_id},
            stream=True,
        )
        data_events = []
        terminal = False
        try:
            content_type = stream_res.headers.get("Content-Type", "")
            assert "text/event-stream" in content_type, content_type
            for line in stream_res.iter_lines(decode_unicode=True):
                if not line:
                    continue
                if line.startswith("data:"):
                    data_events.append(line)
                    if '"data": true' in line or '"data":true' in line:
                        terminal = True
                        break
        finally:
            stream_res.close()

        assert data_events, "Expected at least one data event"
        assert terminal, "Expected terminal data event"

    @pytest.mark.p3
    def test_sequence2txt_and_tts_validation_only(self, HttpApiAuth):
        sequence_url = f"{HOST_ADDRESS}/api/{VERSION}/sequence2txt"
        res = requests.post(url=sequence_url, auth=HttpApiAuth, data={"stream": "false"})
        body = res.json()
        assert body.get("code") != 0, body
        assert "Missing 'file'" in body.get("message", ""), body

        res = requests.post(
            url=sequence_url,
            auth=HttpApiAuth,
            files={"file": ("bad.txt", b"invalid")},
            data={"stream": "false"},
        )
        body = res.json()
        assert body.get("code") != 0, body
        assert "Unsupported audio format" in body.get("message", ""), body

        tts_url = f"{HOST_ADDRESS}/api/{VERSION}/tts"
        res = requests.post(url=tts_url, headers=HEADERS, auth=HttpApiAuth, json={"text": "hello"})
        body = res.json()
        assert body.get("code") != 0, body
        assert "No default TTS model is set" in body.get("message", ""), body
