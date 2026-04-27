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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from common import create_chat_assistant, get_chat_assistant, update_chat_assistant


@pytest.mark.usefixtures("clear_chat_assistants")
class TestChatAssistantEdgeCases:
    @pytest.mark.p2
    def test_create_with_tavily_api_key(self, HttpApiAuth):
        """Test creating chat assistant with Tavily API key instead of dataset"""
        payload = {
            "name": "tavily_chat",
            "prompt_config": {
                "system": "You are a helpful assistant. Use this knowledge: {knowledge}",
                "parameters": [{"key": "knowledge", "optional": True}],
                "tavily_api_key": "test_tavily_key",
            },
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_create_with_extremely_long_system_prompt(self, HttpApiAuth):
        """Test creating chat assistant with very long system prompt"""
        long_prompt = "You are a helpful assistant. " * 1000
        payload = {
            "name": "long_prompt_chat",
            "prompt_config": {"system": long_prompt, "parameters": []},
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_create_with_unicode_characters(self, HttpApiAuth):
        """Test creating chat assistant with unicode characters in name"""
        payload = {
            "name": "unicode_test_中文_日本語_한국어_🎉",
            "prompt_config": {"system": "You are a helpful assistant.", "parameters": []},
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_create_with_negative_parameter_values(self, HttpApiAuth):
        """Test creating chat assistant with negative/zero parameter values"""
        payload = {
            "name": "negative_params_chat",
            "similarity_threshold": 0,
            "vector_similarity_weight": 0,
            "top_n": 0,
            "prompt_config": {"system": "You are a helpful assistant.", "parameters": []},
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_update_with_empty_dataset_ids(self, HttpApiAuth, add_chat_assistants_func):
        """Test updating chat assistant with empty dataset_ids list"""
        _, _, chat_assistant_ids = add_chat_assistants_func
        payload = {"dataset_ids": []}
        res = update_chat_assistant(HttpApiAuth, chat_assistant_ids[0], payload)
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_update_with_null_values(self, HttpApiAuth, add_chat_assistants_func):
        """Test updating chat assistant with null optional values"""
        _, _, chat_assistant_ids = add_chat_assistants_func
        payload = {"description": None}
        res = update_chat_assistant(HttpApiAuth, chat_assistant_ids[0], payload)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_with_complex_prompt_parameters(self, HttpApiAuth, add_dataset_func):
        """Test creating chat assistant with complex prompt parameters"""
        dataset_id = add_dataset_func
        payload = {
            "name": "complex_params_chat",
            "dataset_ids": [dataset_id],
            "prompt_config": {
                "system": "You are {role}. Use knowledge: {knowledge}. Context: {context}.",
                "parameters": [
                    {"key": "role", "optional": True},
                    {"key": "knowledge", "optional": False},
                    {"key": "context", "optional": True},
                ],
            },
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_with_special_ids(self, HttpApiAuth):
        """Test chat assistant operations with special ID formats"""
        special_ids = [
            "00000000-0000-0000-0000-000000000000",
            "ffffffff-ffff-ffff-ffff-ffffffffffff",
            "12345678-1234-1234-1234-123456789abc",
        ]

        for special_id in special_ids:
            res = get_chat_assistant(HttpApiAuth, special_id)
            assert res["code"] == 102, f"Should fail for ID: {special_id}"

    @pytest.mark.p3
    def test_with_extremely_large_llm_settings(self, HttpApiAuth):
        """Test chat assistant with very large LLM settings"""
        large_llm_setting = {
            "temperature": 0.7,
            "max_tokens": 999999,
        }
        payload = {
            "name": "large_llm_settings_chat",
            "llm_setting": large_llm_setting,
            "prompt_config": {"system": "You are a helpful assistant.", "parameters": []},
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p3
    def test_concurrent_operations(self, HttpApiAuth, add_chat_assistants_func):
        """Test concurrent operations on the same chat assistant"""
        _, _, chat_assistant_ids = add_chat_assistants_func
        chat_id = chat_assistant_ids[0]

        def update_operation(i):
            payload = {"name": f"concurrent_update_{i}"}
            return update_chat_assistant(HttpApiAuth, chat_id, payload)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(update_operation, i) for i in range(10)]

        responses = [future.result() for future in as_completed(futures)]

        successful_updates = sum(1 for response in responses if response["code"] == 0)
        assert successful_updates > 0, "No updates succeeded"

        res = get_chat_assistant(HttpApiAuth, chat_id)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_create_with_malformed_prompt_parameters(self, HttpApiAuth):
        """Test creating chat assistant with malformed prompt parameters"""
        payload = {
            "name": "malformed_params_chat",
            "prompt_config": {
                "system": "You are a helpful assistant.",
                "parameters": [
                    {"key": "valid_param", "optional": False},
                    {"optional": True},
                    {"key": "valid_param2"},
                ],
            },
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] in [0, 102], res

    @pytest.mark.p2
    def test_create_with_extreme_parameter_values(self, HttpApiAuth):
        """Test creating chat assistant with extreme parameter values"""
        payload = {
            "name": "extreme_values_chat",
            "similarity_threshold": 1.0,
            "vector_similarity_weight": 1.0,
            "top_n": 1000,
            "prompt_config": {"system": "You are a helpful assistant.", "parameters": []},
        }
        res = create_chat_assistant(HttpApiAuth, payload)
        assert res["code"] == 0, res
