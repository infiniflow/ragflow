#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from api.utils.model_id_utils import normalize_model_id_for_response, normalize_model_ids_for_response


def test_normalize_model_id_for_response_adds_default_instance():
    assert normalize_model_id_for_response("qwen-max@Tongyi-Qianwen") == "qwen-max@default@Tongyi-Qianwen"


def test_normalize_model_id_for_response_keeps_canonical_and_empty_values():
    assert normalize_model_id_for_response("qwen-max@custom@Tongyi-Qianwen") == "qwen-max@custom@Tongyi-Qianwen"
    assert normalize_model_id_for_response("") == ""
    assert normalize_model_id_for_response(None) is None


def test_normalize_model_ids_for_response_recurses_known_model_fields_only():
    payload = {
        "llm_id": "qwen-max@Tongyi-Qianwen",
        "chat_id": "chat-session-id",
        "parser_config": {
            "vlm": {"llm_id": "qwen-vl-plus@Tongyi-Qianwen"},
            "rerank_id": "bge-reranker@Builtin",
        },
        "search_config": {
            "chat_id": "qwen-plus@Tongyi-Qianwen",
        },
        "dsl": {
            "components": {
                "begin": {
                    "obj": {
                        "component_name": "Begin",
                        "params": {"embd_id": "bge-m3@Ollama"},
                    }
                }
            }
        },
    }

    assert normalize_model_ids_for_response(payload) == {
        "llm_id": "qwen-max@default@Tongyi-Qianwen",
        "chat_id": "chat-session-id",
        "parser_config": {
            "vlm": {"llm_id": "qwen-vl-plus@default@Tongyi-Qianwen"},
            "rerank_id": "bge-reranker@default@Builtin",
        },
        "search_config": {
            "chat_id": "qwen-plus@default@Tongyi-Qianwen",
        },
        "dsl": {
            "components": {
                "begin": {
                    "obj": {
                        "component_name": "Begin",
                        "params": {"embd_id": "bge-m3@default@Ollama"},
                    }
                }
            }
        },
    }
