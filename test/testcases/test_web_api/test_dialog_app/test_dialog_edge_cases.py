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
from common import create_dialog, delete_dialog, get_dialog, update_dialog


@pytest.mark.usefixtures("clear_dialogs")
class TestDialogEdgeCases:
    @pytest.mark.p2
    def test_create_dialog_with_tavily_api_key(self, WebApiAuth):
        """Test creating dialog with Tavily API key instead of dataset"""
        payload = {
            "name": "tavily_dialog",
            "prompt_config": {"system": "You are a helpful assistant. Use this knowledge: {knowledge}", "parameters": [{"key": "knowledge", "optional": True}], "tavily_api_key": "test_tavily_key"},
        }
        res = create_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.skip
    @pytest.mark.p2
    def test_create_dialog_with_different_embedding_models(self, WebApiAuth):
        """Test creating dialog with knowledge bases that have different embedding models"""
        # This test would require creating datasets with different embedding models
        # For now, we'll test the error case with a mock scenario
        payload = {
            "name": "mixed_embedding_dialog",
            "kb_ids": ["kb_with_model_a", "kb_with_model_b"],
            "prompt_config": {"system": "You are a helpful assistant with knowledge: {knowledge}", "parameters": [{"key": "knowledge", "optional": True}]},
        }
        res = create_dialog(WebApiAuth, payload)
        # This should fail due to different embedding models
        assert res["code"] == 102, res
        assert "Datasets use different embedding models" in res["message"], res

    @pytest.mark.p2
    def test_create_dialog_with_extremely_long_system_prompt(self, WebApiAuth):
        """Test creating dialog with very long system prompt"""
        long_prompt = "You are a helpful assistant. " * 1000
        payload = {"name": "long_prompt_dialog", "prompt_config": {"system": long_prompt, "parameters": []}}
        res = create_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_create_dialog_with_unicode_characters(self, WebApiAuth):
        """Test creating dialog with Unicode characters in various fields"""
        payload = {
            "name": "Unicode测试对话🤖",
            "description": "测试Unicode字符支持 with émojis 🚀🌟",
            "icon": "🤖",
            "prompt_config": {"system": "你是一个有用的助手。You are helpful. Vous êtes utile. 🌍", "parameters": []},
        }
        res = create_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == "Unicode测试对话🤖", res
        assert res["data"]["description"] == "测试Unicode字符支持 with émojis 🚀🌟", res

    @pytest.mark.p2
    def test_create_dialog_with_extreme_parameter_values(self, WebApiAuth):
        """Test creating dialog with extreme parameter values"""
        payload = {
            "name": "extreme_params_dialog",
            "top_n": 0,
            "top_k": 1,
            "similarity_threshold": 0.0,
            "vector_similarity_weight": 1.0,
            "prompt_config": {"system": "You are a helpful assistant.", "parameters": []},
        }
        res = create_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["top_n"] == 0, res
        assert res["data"]["top_k"] == 1, res
        assert res["data"]["similarity_threshold"] == 0.0, res
        assert res["data"]["vector_similarity_weight"] == 1.0, res

    @pytest.mark.p2
    def test_create_dialog_with_negative_parameter_values(self, WebApiAuth):
        """Test creating dialog with negative parameter values"""
        payload = {
            "name": "negative_params_dialog",
            "top_n": -1,
            "top_k": -100,
            "similarity_threshold": -0.5,
            "vector_similarity_weight": -0.3,
            "prompt_config": {"system": "You are a helpful assistant.", "parameters": []},
        }
        res = create_dialog(WebApiAuth, payload)
        assert res["code"] in [0, 102], res

    @pytest.mark.p2
    def test_update_dialog_with_empty_kb_ids(self, WebApiAuth, add_dialog_func):
        """Test updating dialog to remove all knowledge bases"""
        dataset_id, dialog_id = add_dialog_func
        payload = {"dialog_id": dialog_id, "kb_ids": [], "prompt_config": {"system": "You are a helpful assistant without knowledge.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["kb_ids"] == [], res

    @pytest.mark.p2
    def test_update_dialog_with_null_values(self, WebApiAuth, add_dialog_func):
        """Test updating dialog with null/None values"""
        dataset_id, dialog_id = add_dialog_func
        payload = {"dialog_id": dialog_id, "description": None, "icon": None, "rerank_id": None, "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = update_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_dialog_with_complex_prompt_parameters(self, WebApiAuth, add_dataset_func):
        """Test dialog with complex prompt parameter configurations"""
        payload = {
            "name": "complex_params_dialog",
            "prompt_config": {
                "system": "You are {role} assistant. Use {knowledge} and consider {context}. Optional: {optional_param}",
                "parameters": [{"key": "role", "optional": False}, {"key": "knowledge", "optional": True}, {"key": "context", "optional": False}, {"key": "optional_param", "optional": True}],
            },
            "kb_ids": [add_dataset_func],
        }
        res = create_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_dialog_with_malformed_prompt_parameters(self, WebApiAuth):
        """Test dialog with malformed prompt parameter configurations"""
        payload = {
            "name": "malformed_params_dialog",
            "prompt_config": {
                "system": "You are a helpful assistant.",
                "parameters": [
                    {
                        "key": "",
                        "optional": False,
                    },
                    {"optional": True},
                    {
                        "key": "valid_param",
                    },
                ],
            },
        }
        res = create_dialog(WebApiAuth, payload)

        assert res["code"] in [0, 102], res

    @pytest.mark.p3
    def test_dialog_operations_with_special_ids(self, WebApiAuth):
        """Test dialog operations with special ID formats"""
        special_ids = [
            "00000000-0000-0000-0000-000000000000",
            "ffffffff-ffff-ffff-ffff-ffffffffffff",
            "12345678-1234-1234-1234-123456789abc",
        ]

        for special_id in special_ids:
            res = get_dialog(WebApiAuth, {"dialog_id": special_id})
            assert res["code"] == 102, f"Should fail for ID: {special_id}"

            res = delete_dialog(WebApiAuth, {"dialog_ids": [special_id]})
            assert res["code"] == 103, f"Should fail for ID: {special_id}"

    @pytest.mark.p3
    def test_dialog_with_extremely_large_llm_settings(self, WebApiAuth):
        """Test dialog with very large LLM settings"""
        large_llm_setting = {
            "model": "gpt-4",
            "temperature": 0.7,
            "max_tokens": 999999,
            "custom_param_" + "x" * 1000: "large_value_" + "y" * 1000,
        }
        payload = {"name": "large_llm_settings_dialog", "llm_setting": large_llm_setting, "prompt_config": {"system": "You are a helpful assistant.", "parameters": []}}
        res = create_dialog(WebApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_concurrent_dialog_operations(self, WebApiAuth, add_dialog_func):
        """Test concurrent operations on the same dialog"""
        from concurrent.futures import ThreadPoolExecutor, as_completed

        _, dialog_id = add_dialog_func

        def update_operation(i):
            payload = {"dialog_id": dialog_id, "name": f"concurrent_update_{i}", "prompt_config": {"system": f"You are assistant number {i}.", "parameters": []}}
            return update_dialog(WebApiAuth, payload)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(update_operation, i) for i in range(10)]

        responses = [future.result() for future in as_completed(futures)]

        successful_updates = sum(1 for response in responses if response["code"] == 0)
        assert successful_updates > 0, "No updates succeeded"

        res = get_dialog(WebApiAuth, {"dialog_id": dialog_id})
        assert res["code"] == 0, res
