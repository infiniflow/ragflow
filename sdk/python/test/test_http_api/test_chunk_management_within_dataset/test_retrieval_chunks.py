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
import os

import pytest
from common import (
    INVALID_API_TOKEN,
    retrieval_chunks,
)
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, auth, expected_code, expected_message):
        res = retrieval_chunks(auth)
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChunksRetrieval:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"question": "chunk", "dataset_ids": None}, 0, 4, ""),
            ({"question": "chunk", "document_ids": None}, 102, 0, "`dataset_ids` is required."),
            ({"question": "chunk", "dataset_ids": None, "document_ids": None}, 0, 4, ""),
            ({"question": "chunk"}, 102, 0, "`dataset_ids` is required."),
        ],
    )
    def test_basic_scenarios(self, get_http_api_auth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        if "dataset_ids" in payload:
            payload["dataset_ids"] = [dataset_id]
        if "document_ids" in payload:
            payload["document_ids"] = [document_id]
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            pytest.param(
                {"page": None, "page_size": 2},
                100,
                2,
                """TypeError("int() argument must be a string, a bytes-like object or a real number, not \'NoneType\'")""",
                marks=pytest.mark.skip,
            ),
            pytest.param(
                {"page": 0, "page_size": 2},
                100,
                0,
                "ValueError('Search does not support negative slicing.')",
                marks=pytest.mark.skip,
            ),
            pytest.param({"page": 2, "page_size": 2}, 0, 2, "", marks=pytest.mark.skip(reason="issues/6646")),
            ({"page": 3, "page_size": 2}, 0, 0, ""),
            ({"page": "3", "page_size": 2}, 0, 0, ""),
            pytest.param(
                {"page": -1, "page_size": 2},
                100,
                0,
                "ValueError('Search does not support negative slicing.')",
                marks=pytest.mark.skip,
            ),
            pytest.param(
                {"page": "a", "page_size": 2},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_page(self, get_http_api_auth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            pytest.param(
                {"page_size": None},
                100,
                0,
                """TypeError("int() argument must be a string, a bytes-like object or a real number, not \'NoneType\'")""",
                marks=pytest.mark.skip,
            ),
            # ({"page_size": 0}, 0, 0, ""),
            ({"page_size": 1}, 0, 1, ""),
            ({"page_size": 5}, 0, 4, ""),
            ({"page_size": "1"}, 0, 1, ""),
            # ({"page_size": -1}, 0, 0, ""),
            pytest.param(
                {"page_size": "a"},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_page_size(self, get_http_api_auth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})

        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"vector_similarity_weight": 0}, 0, 4, ""),
            ({"vector_similarity_weight": 0.5}, 0, 4, ""),
            ({"vector_similarity_weight": 10}, 0, 4, ""),
            pytest.param(
                {"vector_similarity_weight": "a"},
                100,
                0,
                """ValueError("could not convert string to float: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_vector_similarity_weight(self, get_http_api_auth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"top_k": 10}, 0, 4, ""),
            pytest.param(
                {"top_k": 1},
                0,
                4,
                "",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in ["infinity", "opensearch"], reason="Infinity"),
            ),
            pytest.param(
                {"top_k": 1},
                0,
                1,
                "",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "opensearch", "elasticsearch"], reason="elasticsearch"),
            ),
            pytest.param(
                {"top_k": -1},
                100,
                4,
                "must be greater than 0",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in ["infinity", "opensearch"], reason="Infinity"),
            ),
            pytest.param(
                {"top_k": -1},
                100,
                4,
                "3014",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "opensearch", "elasticsearch"], reason="elasticsearch"),
            ),
            pytest.param(
                {"top_k": "a"},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_top_k(self, get_http_api_auth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert expected_message in res["message"]

    @pytest.mark.skip
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"rerank_id": "BAAI/bge-reranker-v2-m3"}, 0, ""),
            pytest.param({"rerank_id": "unknown"}, 100, "LookupError('Model(unknown) not authorized')", marks=pytest.mark.skip),
        ],
    )
    def test_rerank_id(self, get_http_api_auth, add_chunks, payload, expected_code, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) > 0
        else:
            assert expected_message in res["message"]

    @pytest.mark.skip
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"keyword": True}, 0, 5, ""),
            ({"keyword": "True"}, 0, 5, ""),
            ({"keyword": False}, 0, 5, ""),
            ({"keyword": "False"}, 0, 5, ""),
            ({"keyword": None}, 0, 5, ""),
        ],
    )
    def test_keyword(self, get_http_api_auth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk test", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_highlight, expected_message",
        [
            ({"highlight": True}, 0, True, ""),
            ({"highlight": "True"}, 0, True, ""),
            pytest.param({"highlight": False}, 0, False, "", marks=pytest.mark.skip(reason="issues/6648")),
            ({"highlight": "False"}, 0, False, ""),
            pytest.param({"highlight": None}, 0, False, "", marks=pytest.mark.skip(reason="issues/6648")),
        ],
    )
    def test_highlight(self, get_http_api_auth, add_chunks, payload, expected_code, expected_highlight, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_highlight:
            for chunk in res["data"]["chunks"]:
                assert "highlight" in chunk
        else:
            for chunk in res["data"]["chunks"]:
                assert "highlight" not in chunk

        if expected_code != 0:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_invalid_params(self, get_http_api_auth, add_chunks):
        dataset_id, _, _ = add_chunks
        payload = {"question": "chunk", "dataset_ids": [dataset_id], "a": "b"}
        res = retrieval_chunks(get_http_api_auth, payload)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == 4

    @pytest.mark.p3
    def test_concurrent_retrieval(self, get_http_api_auth, add_chunks):
        from concurrent.futures import ThreadPoolExecutor

        dataset_id, _, _ = add_chunks
        payload = {"question": "chunk", "dataset_ids": [dataset_id]}

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(retrieval_chunks, get_http_api_auth, payload) for i in range(100)]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)
