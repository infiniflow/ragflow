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
import pytest
from common import search_dataset, knowledge_graph
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p2
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowHttpApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = search_dataset(invalid_auth, "dataset_id", {"question": "test"})
        assert res["code"] == expected_code
        assert expected_message in res.get("message", "")


class TestDatasetSearch:
    @pytest.mark.p2
    def test_search_without_question(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = search_dataset(HttpApiAuth, dataset_id, {})
        assert res["code"] == 102, res

    @pytest.mark.p2
    def test_search_basic(self, HttpApiAuth, add_chunks):
        dataset_id, document_id, _ = add_chunks
        res = search_dataset(HttpApiAuth, dataset_id, {"question": "chunk"})
        assert res["code"] == 0, res
        assert "chunks" in res["data"], res

    @pytest.mark.p2
    def test_search_with_doc_ids(self, HttpApiAuth, add_chunks):
        dataset_id, document_id, _ = add_chunks
        res = search_dataset(HttpApiAuth, dataset_id, {"question": "chunk", "doc_ids": [document_id]})
        assert res["code"] == 0, res
        assert "chunks" in res["data"], res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code",
        [
            ({"question": "chunk", "page": 1, "size": 2}, 0),
            ({"question": "chunk", "similarity_threshold": 0.5}, 0),
            ({"question": "chunk", "vector_similarity_weight": 0.7}, 0),
            ({"question": "chunk", "top_k": 10}, 0),
        ],
    )
    def test_search_params(self, HttpApiAuth, add_chunks, payload, expected_code):
        dataset_id, _, _ = add_chunks
        res = search_dataset(HttpApiAuth, dataset_id, payload)
        assert res["code"] == expected_code, res


@pytest.mark.p2
class TestDatasetGraph:
    def test_graph_requires_auth(self):
        res = knowledge_graph(None, "dataset_id")
        assert res["code"] == 401

    def test_graph_basic(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = knowledge_graph(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res
