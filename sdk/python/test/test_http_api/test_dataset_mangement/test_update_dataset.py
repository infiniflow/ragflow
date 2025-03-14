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
from concurrent.futures import ThreadPoolExecutor

import pytest
from common import (
    DATASET_NAME_LIMIT,
    INVALID_API_TOKEN,
    create_datasets,
    list_dataset,
    update_dataset,
)
from libs.auth import RAGFlowHttpApiAuth
from libs.utils import encode_avatar
from libs.utils.file_utils import create_image_file

# TODO: Missing scenario for updating embedding_model with chunk_count != 0


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
    def test_invalid_auth(
        self, get_http_api_auth, auth, expected_code, expected_message
    ):
        ids = create_datasets(get_http_api_auth, 1)
        res = update_dataset(auth, ids[0], {"name": "new_name"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDatasetUpdate:
    @pytest.mark.parametrize(
        "name, expected_code, expected_message",
        [
            ("valid_name", 0, ""),
            (
                "a" * (DATASET_NAME_LIMIT + 1),
                102,
                "Dataset name should not be longer than 128 characters.",
            ),
            (0, 100, """AttributeError("\'int\' object has no attribute \'strip\'")"""),
            (
                None,
                100,
                """AttributeError("\'NoneType\' object has no attribute \'strip\'")""",
            ),
            pytest.param("", 102, "", marks=pytest.mark.xfail(reason="issue#5915")),
            ("dataset_1", 102, "Duplicated dataset name in updating dataset."),
            ("DATASET_1", 102, "Duplicated dataset name in updating dataset."),
        ],
    )
    def test_name(self, get_http_api_auth, name, expected_code, expected_message):
        ids = create_datasets(get_http_api_auth, 2)
        res = update_dataset(get_http_api_auth, ids[0], {"name": name})
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_dataset(get_http_api_auth, {"id": ids[0]})
            assert res["data"][0]["name"] == name
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "embedding_model, expected_code, expected_message",
        [
            ("BAAI/bge-large-zh-v1.5", 0, ""),
            ("BAAI/bge-base-en-v1.5", 0, ""),
            ("BAAI/bge-large-en-v1.5", 0, ""),
            ("BAAI/bge-small-en-v1.5", 0, ""),
            ("BAAI/bge-small-zh-v1.5", 0, ""),
            ("jinaai/jina-embeddings-v2-base-en", 0, ""),
            ("jinaai/jina-embeddings-v2-small-en", 0, ""),
            ("nomic-ai/nomic-embed-text-v1.5", 0, ""),
            ("sentence-transformers/all-MiniLM-L6-v2", 0, ""),
            ("text-embedding-v2", 0, ""),
            ("text-embedding-v3", 0, ""),
            ("maidalun1020/bce-embedding-base_v1", 0, ""),
            (
                "other_embedding_model",
                102,
                "`embedding_model` other_embedding_model doesn't exist",
            ),
            (None, 102, "`embedding_model` can't be empty"),
        ],
    )
    def test_embedding_model(
        self, get_http_api_auth, embedding_model, expected_code, expected_message
    ):
        ids = create_datasets(get_http_api_auth, 1)
        res = update_dataset(
            get_http_api_auth, ids[0], {"embedding_model": embedding_model}
        )
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_dataset(get_http_api_auth, {"id": ids[0]})
            assert res["data"][0]["embedding_model"] == embedding_model
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "chunk_method, expected_code, expected_message",
        [
            ("naive", 0, ""),
            ("manual", 0, ""),
            ("qa", 0, ""),
            ("table", 0, ""),
            ("paper", 0, ""),
            ("book", 0, ""),
            ("laws", 0, ""),
            ("presentation", 0, ""),
            ("picture", 0, ""),
            ("one", 0, ""),
            ("knowledge_graph", 0, ""),
            ("email", 0, ""),
            ("tag", 0, ""),
            ("", 0, ""),
            (
                "other_chunk_method",
                102,
                "'other_chunk_method' is not in ['naive', 'manual', 'qa', 'table',"
                " 'paper', 'book', 'laws', 'presentation', 'picture', 'one', "
                "'knowledge_graph', 'email', 'tag']",
            ),
        ],
    )
    def test_chunk_method(
        self, get_http_api_auth, chunk_method, expected_code, expected_message
    ):
        ids = create_datasets(get_http_api_auth, 1)
        res = update_dataset(get_http_api_auth, ids[0], {"chunk_method": chunk_method})
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_dataset(get_http_api_auth, {"id": ids[0]})
            if chunk_method != "":
                assert res["data"][0]["chunk_method"] == chunk_method
            else:
                assert res["data"][0]["chunk_method"] == "naive"
        else:
            assert res["message"] == expected_message

    def test_avatar(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {"avatar": encode_avatar(fn)}
        res = update_dataset(get_http_api_auth, ids[0], payload)
        assert res["code"] == 0

    def test_description(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        payload = {"description": "description"}
        res = update_dataset(get_http_api_auth, ids[0], payload)
        assert res["code"] == 0

        res = list_dataset(get_http_api_auth, {"id": ids[0]})
        assert res["data"][0]["description"] == "description"

    def test_pagerank(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        payload = {"pagerank": 1}
        res = update_dataset(get_http_api_auth, ids[0], payload)
        assert res["code"] == 0

        res = list_dataset(get_http_api_auth, {"id": ids[0]})
        assert res["data"][0]["pagerank"] == 1

    def test_similarity_threshold(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        payload = {"similarity_threshold": 1}
        res = update_dataset(get_http_api_auth, ids[0], payload)
        assert res["code"] == 0

        res = list_dataset(get_http_api_auth, {"id": ids[0]})
        assert res["data"][0]["similarity_threshold"] == 1

    @pytest.mark.parametrize(
        "permission, expected_code",
        [
            ("me", 0),
            ("team", 0),
            ("", 0),
            ("ME", 102),
            ("TEAM", 102),
            ("other_permission", 102),
        ],
    )
    def test_permission(self, get_http_api_auth, permission, expected_code):
        ids = create_datasets(get_http_api_auth, 1)
        payload = {"permission": permission}
        res = update_dataset(get_http_api_auth, ids[0], payload)
        assert res["code"] == expected_code

        res = list_dataset(get_http_api_auth, {"id": ids[0]})
        if expected_code == 0 and permission != "":
            assert res["data"][0]["permission"] == permission
        if permission == "":
            assert res["data"][0]["permission"] == "me"

    def test_vector_similarity_weight(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        payload = {"vector_similarity_weight": 1}
        res = update_dataset(get_http_api_auth, ids[0], payload)
        assert res["code"] == 0

        res = list_dataset(get_http_api_auth, {"id": ids[0]})
        assert res["data"][0]["vector_similarity_weight"] == 1

    def test_invalid_dataset_id(self, get_http_api_auth):
        create_datasets(get_http_api_auth, 1)
        res = update_dataset(
            get_http_api_auth, "invalid_dataset_id", {"name": "invalid_dataset_id"}
        )
        assert res["code"] == 102
        assert res["message"] == "You don't own the dataset"

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"chunk_count": 1}, 102, "Can't change `chunk_count`."),
            (
                {"create_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
                102,
                "The input parameters are invalid.",
            ),
            ({"create_time": 1741671443322}, 102, "The input parameters are invalid."),
            ({"created_by": "aa"}, 102, "The input parameters are invalid."),
            ({"document_count": 1}, 102, "Can't change `document_count`."),
            ({"id": "id"}, 102, "The input parameters are invalid."),
            ({"status": "1"}, 102, "The input parameters are invalid."),
            (
                {"tenant_id": "e57c1966f99211efb41e9e45646e0111"},
                102,
                "Can't change `tenant_id`.",
            ),
            ({"token_num": 1}, 102, "The input parameters are invalid."),
            (
                {"update_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
                102,
                "The input parameters are invalid.",
            ),
            ({"update_time": 1741671443339}, 102, "The input parameters are invalid."),
        ],
    )
    def test_modify_read_only_field(
        self, get_http_api_auth, payload, expected_code, expected_message
    ):
        ids = create_datasets(get_http_api_auth, 1)
        res = update_dataset(get_http_api_auth, ids[0], payload)
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    def test_modify_unknown_field(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        res = update_dataset(get_http_api_auth, ids[0], {"unknown_field": 0})
        assert res["code"] == 100

    def test_concurrent_update(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    update_dataset, get_http_api_auth, ids[0], {"name": f"dataset_{i}"}
                )
                for i in range(100)
            ]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)
