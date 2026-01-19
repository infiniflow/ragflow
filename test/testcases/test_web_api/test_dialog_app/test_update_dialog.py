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
from common import update_dialog
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.usefixtures("clear_dialogs")
class TestAuthorization:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
        ids=["empty_auth", "invalid_api_token"],
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message, add_dialog_func):
        _, dialog_id = add_dialog_func
        payload = {"dialog_id": dialog_id, "name": "updated_name", "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(invalid_auth, payload)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDialogUpdate:
    @pytest.mark.p1
    def test_update_name(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        new_name = "updated_dialog_name"
        payload = {"dialog_id": dialog_id, "name": new_name, "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == new_name, res

    @pytest.mark.p1
    def test_update_description(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        new_description = "Updated description"
        payload = {"dialog_id": dialog_id, "description": new_description, "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["description"] == new_description, res

    @pytest.mark.p1
    def test_update_prompt_config(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        new_prompt_config = {"system": "You are an updated helpful assistant with {param1}.", "parameters": [{"key": "param1", "optional": False}]}
        payload = {"dialog_id": dialog_id, "prompt_config": new_prompt_config}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["prompt_config"]["system"] == new_prompt_config["system"], res

    @pytest.mark.p1
    def test_update_kb_ids(self, WebApiAuth, add_dialog_func, add_dataset_func):
        _, dialog_id = add_dialog_func
        new_dataset_id = add_dataset_func
        payload = {
            "dialog_id": dialog_id,
            "kb_ids": [new_dataset_id],
            "prompt_config": {"system": "You are a helpful assistant with knowledge: {knowledge}", "parameters": [{"key": "knowledge", "optional": True}]},
        }
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert new_dataset_id in res["data"]["kb_ids"], res

    @pytest.mark.p1
    def test_update_llm_settings(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        new_llm_setting = {"model": "gpt-4", "temperature": 0.9, "max_tokens": 2000}
        payload = {"dialog_id": dialog_id, "llm_setting": new_llm_setting, "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["llm_setting"]["model"] == "gpt-4", res
        assert res["data"]["llm_setting"]["temperature"] == 0.9, res

    @pytest.mark.p1
    def test_update_retrieval_settings(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        payload = {
            "dialog_id": dialog_id,
            "top_n": 15,
            "top_k": 4096,
            "similarity_threshold": 0.3,
            "vector_similarity_weight": 0.7,
            "prompt_config": {"system": "You are a helpful assistant.", "parameters": []},
        }
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["top_n"] == 15, res
        assert res["data"]["top_k"] == 4096, res
        assert res["data"]["similarity_threshold"] == 0.3, res
        assert res["data"]["vector_similarity_weight"] == 0.7, res

    @pytest.mark.p2
    def test_update_nonexistent_dialog(self, WebApiAuth):
        fake_dialog_id = "nonexistent_dialog_id"
        payload = {"dialog_id": fake_dialog_id, "name": "updated_name", "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert "Dialog not found" in res["message"], res

    @pytest.mark.p2
    def test_update_with_invalid_prompt_config(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        payload = {"dialog_id": dialog_id, "prompt_config": {"system": "You are a helpful assistant.", "parameters": [{"key": "unused_param", "optional": False}]}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert "Parameter 'unused_param' is not used" in res["message"], res

    @pytest.mark.p2
    def test_update_with_knowledge_but_no_kb(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        payload = {"dialog_id": dialog_id, "kb_ids": [], "prompt_config": {"system": "You are a helpful assistant with knowledge: {knowledge}", "parameters": [{"key": "knowledge", "optional": True}]}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert "Please remove `{knowledge}` in system prompt" in res["message"], res

    @pytest.mark.p2
    def test_update_icon(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        new_icon = "🚀"
        payload = {"dialog_id": dialog_id, "icon": new_icon, "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["icon"] == new_icon, res

    @pytest.mark.p2
    def test_update_rerank_id(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        payload = {"dialog_id": dialog_id, "rerank_id": "test_rerank_model", "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["rerank_id"] == "test_rerank_model", res

    @pytest.mark.p3
    def test_update_multiple_fields(self, WebApiAuth, add_dialog_func):
        _, dialog_id = add_dialog_func
        payload = {
            "dialog_id": dialog_id,
            "name": "multi_update_dialog",
            "description": "Updated with multiple fields",
            "icon": "🔄",
            "top_n": 20,
            "similarity_threshold": 0.4,
            "prompt_config": {"system": "You are a multi-updated assistant.", "parameters": []},
        }
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        data = res["data"]
        assert data["name"] == "multi_update_dialog", res
        assert data["description"] == "Updated with multiple fields", res
        assert data["icon"] == "🔄", res
        assert data["top_n"] == 20, res
        assert data["similarity_threshold"] == 0.4, res
