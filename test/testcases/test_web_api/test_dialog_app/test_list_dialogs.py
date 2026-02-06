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
from common import batch_create_dialogs, create_dialog, list_dialogs
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.usefixtures("clear_dialogs")
class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
        ids=["empty_auth", "invalid_api_token"],
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = list_dialogs(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDialogList:
    @pytest.mark.p1
    @pytest.mark.usefixtures("add_dialogs_func")
    def test_list_empty_dialogs(self, WebApiAuth):
        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p1
    def test_list_multiple_dialogs(self, WebApiAuth, add_dialogs_func):
        _, dialog_ids = add_dialogs_func
        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

        returned_ids = [dialog["id"] for dialog in res["data"]]
        for dialog_id in dialog_ids:
            assert dialog_id in returned_ids, res

    @pytest.mark.p2
    @pytest.mark.usefixtures("add_dialogs_func")
    def test_list_dialogs_data_structure(self, WebApiAuth):
        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

        dialog = res["data"][0]
        required_fields = [
            "id",
            "name",
            "description",
            "kb_ids",
            "kb_names",
            "prompt_config",
            "llm_setting",
            "top_n",
            "top_k",
            "similarity_threshold",
            "vector_similarity_weight",
            "create_time",
            "update_time",
        ]
        for field in required_fields:
            assert field in dialog, f"Missing field: {field}"

        assert isinstance(dialog["id"], str), res
        assert isinstance(dialog["name"], str), res
        assert isinstance(dialog["kb_ids"], list), res
        assert isinstance(dialog["kb_names"], list), res
        assert isinstance(dialog["prompt_config"], dict), res
        assert isinstance(dialog["top_n"], int), res
        assert isinstance(dialog["top_k"], int), res
        assert isinstance(dialog["similarity_threshold"], (int, float)), res
        assert isinstance(dialog["vector_similarity_weight"], (int, float)), res

    @pytest.mark.p2
    @pytest.mark.usefixtures("add_dialogs_func")
    def test_list_dialogs_with_kb_names(self, WebApiAuth):
        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res

        dialog = res["data"][0]
        assert isinstance(dialog["kb_ids"], list), res
        assert isinstance(dialog["kb_names"], list), res
        assert len(dialog["kb_ids"]) == len(dialog["kb_names"]), res

    @pytest.mark.p2
    @pytest.mark.usefixtures("add_dialogs_func")
    def test_list_dialogs_ordering(self, WebApiAuth):
        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

        dialogs = res["data"]
        for i in range(len(dialogs) - 1):
            current_time = dialogs[i]["create_time"]
            next_time = dialogs[i + 1]["create_time"]
            assert current_time >= next_time, f"Dialogs not properly ordered: {current_time} should be >= {next_time}"

    @pytest.mark.p3
    @pytest.mark.usefixtures("clear_dialogs")
    def test_list_dialogs_with_invalid_kb(self, WebApiAuth):
        payload = {
            "name": "invalid_kb_dialog",
            "kb_ids": ["invalid_kb_id"],
            "prompt_config": {"system": "You are a helpful assistant with knowledge: {knowledge}", "parameters": [{"key": "knowledge", "optional": True}]},
        }
        create_res = create_dialog(WebApiAuth, payload)
        assert create_res["code"] == 0, create_res

        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res

        dialog = res["data"][0]

        assert len(dialog["kb_ids"]) == 0, res
        assert len(dialog["kb_names"]) == 0, res

    @pytest.mark.p3
    @pytest.mark.usefixtures("clear_dialogs")
    def test_list_dialogs_with_multiple_kbs(self, WebApiAuth, add_dataset_func):
        dataset_id1 = add_dataset_func
        dataset_id2 = add_dataset_func

        payload = {
            "name": "multi_kb_dialog",
            "kb_ids": [dataset_id1, dataset_id2],
            "prompt_config": {"system": "You are a helpful assistant with knowledge: {knowledge}", "parameters": [{"key": "knowledge", "optional": True}]},
        }
        create_res = create_dialog(WebApiAuth, payload)
        assert create_res["code"] == 0, create_res

        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res

        dialog = res["data"][0]
        assert len(dialog["kb_ids"]) == 2, res
        assert len(dialog["kb_names"]) == 2, res
        assert dataset_id1 in dialog["kb_ids"], res
        assert dataset_id2 in dialog["kb_ids"], res

    @pytest.mark.p3
    @pytest.mark.usefixtures("add_dialogs_func")
    def test_list_dialogs_prompt_config_structure(self, WebApiAuth):
        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res

        dialog = res["data"][0]
        prompt_config = dialog["prompt_config"]
        assert "system" in prompt_config, res
        assert "parameters" in prompt_config, res
        assert isinstance(prompt_config["system"], str), res
        assert isinstance(prompt_config["parameters"], list), res

    @pytest.mark.p3
    @pytest.mark.usefixtures("clear_dialogs")
    def test_list_dialogs_performance(self, WebApiAuth, add_document):
        dataset_id, _ = add_document
        dialog_ids = batch_create_dialogs(WebApiAuth, 100, [dataset_id])
        assert len(dialog_ids) == 100, "Failed to create 100 dialogs"

        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 100, res

        returned_ids = [dialog["id"] for dialog in res["data"]]
        for dialog_id in dialog_ids:
            assert dialog_id in returned_ids, f"Dialog {dialog_id} not found in list"

    @pytest.mark.p3
    @pytest.mark.usefixtures("clear_dialogs")
    def test_list_dialogs_with_mixed_kb_states(self, WebApiAuth, add_dataset_func):
        valid_dataset_id = add_dataset_func

        payload = {
            "name": "mixed_kb_dialog",
            "kb_ids": [valid_dataset_id, "invalid_kb_id"],
            "prompt_config": {"system": "You are a helpful assistant with knowledge: {knowledge}", "parameters": [{"key": "knowledge", "optional": True}]},
        }
        create_res = create_dialog(WebApiAuth, payload)
        assert create_res["code"] == 0, create_res

        res = list_dialogs(WebApiAuth)
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res

        dialog = res["data"][0]
        assert len(dialog["kb_ids"]) == 1, res
        assert dialog["kb_ids"][0] == valid_dataset_id, res
        assert len(dialog["kb_names"]) == 1, res
