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

from api.db.services.pipeline_operation_log_service import _remove_embedding_vectors


def test_remove_embedding_vectors_preserves_pipeline_runtime_outputs():
    dsl = {
        "components": {
            "Tokenizer:0": {
                "obj": {
                    "params": {
                        "outputs": {
                            "chunks": {
                                "value": [
                                    {
                                        "text": "content",
                                        "q_1024_vec": [0.1, 0.2],
                                        "metadata": {"q_3_vec": [1, 2, 3], "source": "test"},
                                    }
                                ]
                            },
                            "embedding_token_consumption": {"value": 12},
                        }
                    }
                }
            }
        },
        "path": ["Tokenizer:0"],
    }

    result = _remove_embedding_vectors(dsl)

    chunk = result["components"]["Tokenizer:0"]["obj"]["params"]["outputs"]["chunks"]["value"][0]
    assert chunk == {"text": "content", "metadata": {"source": "test"}}
    assert result["components"]["Tokenizer:0"]["obj"]["params"]["outputs"]["embedding_token_consumption"]["value"] == 12
    assert result["path"] == ["Tokenizer:0"]
