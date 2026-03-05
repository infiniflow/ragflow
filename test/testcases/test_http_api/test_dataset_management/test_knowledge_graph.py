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
from common import delete_knowledge_graph, knowledge_graph
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p2
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 0, "Authorization"),
            (RAGFlowHttpApiAuth(INVALID_API_TOKEN), 109, "API key is invalid"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = knowledge_graph(invalid_auth, "dataset_id")
        assert res["code"] == expected_code
        assert expected_message in res.get("message", "")


class TestKnowledgeGraph:
    @pytest.mark.p2
    def test_get_knowledge_graph_empty(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = knowledge_graph(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res
        assert "graph" in res["data"], res
        assert "mind_map" in res["data"], res
        assert isinstance(res["data"]["graph"], dict), res
        assert isinstance(res["data"]["mind_map"], dict), res

    @pytest.mark.p2
    def test_delete_knowledge_graph(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = delete_knowledge_graph(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res
        assert res["data"] is True, res
