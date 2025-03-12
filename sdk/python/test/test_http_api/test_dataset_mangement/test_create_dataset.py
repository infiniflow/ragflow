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
from common import DATASET_NAME_LIMIT, INVALID_API_TOKEN, create_dataset
from libs.auth import RAGFlowHttpApiAuth
from libs.utils import encode_avatar
from libs.utils.file_utils import create_image_file


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
        res = create_dataset(auth, {"name": "auth_test"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDatasetCreation:
    @pytest.mark.parametrize(
        "payload, expected_code",
        [
            ({"name": "valid_name"}, 0),
            ({"name": "a" * (DATASET_NAME_LIMIT + 1)}, 102),
            ({"name": 0}, 100),
            ({"name": ""}, 102),
            ({"name": "duplicated_name"}, 102),
            ({"name": "case_insensitive"}, 102),
        ],
    )
    def test_basic_scenarios(self, get_http_api_auth, payload, expected_code):
        if payload["name"] == "duplicated_name":
            create_dataset(get_http_api_auth, payload)
        elif payload["name"] == "case_insensitive":
            create_dataset(get_http_api_auth, {"name": payload["name"].upper()})

        res = create_dataset(get_http_api_auth, payload)

        assert res["code"] == expected_code
        if expected_code == 0:
            assert res["data"]["name"] == payload["name"]

        if payload["name"] in ["duplicated_name", "case_insensitive"]:
            assert res["message"] == "Duplicated dataset name in creating dataset."

    @pytest.mark.slow
    def test_dataset_10k(self, get_http_api_auth):
        for i in range(10_000):
            payload = {"name": f"dataset_{i}"}
            res = create_dataset(get_http_api_auth, payload)
            assert res["code"] == 0, f"Failed to create dataset {i}"


class TestAdvancedConfigurations:
    def test_avatar(self, get_http_api_auth, tmp_path):
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {
            "name": "avatar_test",
            "avatar": encode_avatar(fn),
        }
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0

    def test_description(self, get_http_api_auth):
        payload = {"name": "description_test", "description": "a" * 65536}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0

    @pytest.mark.parametrize(
        "name, permission, expected_code",
        [
            ("me", "me", 0),
            ("team", "team", 0),
            ("empty_permission", "", 0),
            ("me_upercase", "ME", 102),
            ("team_upercase", "TEAM", 102),
            ("other_permission", "other_permission", 102),
        ],
    )
    def test_permission(self, get_http_api_auth, name, permission, expected_code):
        payload = {"name": name, "permission": permission}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0 and permission != "":
            assert res["data"]["permission"] == permission
        if permission == "":
            assert res["data"]["permission"] == "me"

    @pytest.mark.parametrize(
        "name, chunk_method, expected_code",
        [
            ("naive", "naive", 0),
            ("manual", "manual", 0),
            ("qa", "qa", 0),
            ("table", "table", 0),
            ("paper", "paper", 0),
            ("book", "book", 0),
            ("laws", "laws", 0),
            ("presentation", "presentation", 0),
            ("picture", "picture", 0),
            ("one", "one", 0),
            ("picknowledge_graphture", "knowledge_graph", 0),
            ("email", "email", 0),
            ("tag", "tag", 0),
            ("empty_chunk_method", "", 0),
            ("other_chunk_method", "other_chunk_method", 102),
        ],
    )
    def test_chunk_method(self, get_http_api_auth, name, chunk_method, expected_code):
        payload = {"name": name, "chunk_method": chunk_method}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0 and chunk_method != "":
            assert res["data"]["chunk_method"] == chunk_method
        if chunk_method == "":
            assert res["data"]["chunk_method"] == "naive"

    @pytest.mark.parametrize(
        "name, embedding_model, expected_code",
        [
            ("BAAI/bge-large-zh-v1.5", "BAAI/bge-large-zh-v1.5", 0),
            ("BAAI/bge-base-en-v1.5", "BAAI/bge-base-en-v1.5", 0),
            ("BAAI/bge-large-en-v1.5", "BAAI/bge-large-en-v1.5", 0),
            ("BAAI/bge-small-en-v1.5", "BAAI/bge-small-en-v1.5", 0),
            ("BAAI/bge-small-zh-v1.5", "BAAI/bge-small-zh-v1.5", 0),
            (
                "jinaai/jina-embeddings-v2-base-en",
                "jinaai/jina-embeddings-v2-base-en",
                0,
            ),
            (
                "jinaai/jina-embeddings-v2-small-en",
                "jinaai/jina-embeddings-v2-small-en",
                0,
            ),
            ("nomic-ai/nomic-embed-text-v1.5", "nomic-ai/nomic-embed-text-v1.5", 0),
            (
                "sentence-transformers/all-MiniLM-L6-v2",
                "sentence-transformers/all-MiniLM-L6-v2",
                0,
            ),
            ("text-embedding-v2", "text-embedding-v2", 0),
            ("text-embedding-v3", "text-embedding-v3", 0),
            (
                "maidalun1020/bce-embedding-base_v1",
                "maidalun1020/bce-embedding-base_v1",
                0,
            ),
            ("other_embedding_model", "other_embedding_model", 102),
        ],
    )
    def test_embedding_model(
        self, get_http_api_auth, name, embedding_model, expected_code
    ):
        payload = {"name": name, "embedding_model": embedding_model}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert res["data"]["embedding_model"] == embedding_model

    @pytest.mark.parametrize(
        "name, chunk_method, parser_config, expected_code, expected_message",
        [
            (
                "naive_default",
                "naive",
                {
                    "chunk_token_num": 128,
                    "layout_recognize": "DeepDOC",
                    "html4excel": False,
                    "delimiter": "\n!?。；！？",
                    "task_page_size": 12,
                    "raptor": {"use_raptor": False},
                },
                0,
                "",
            ),
            ("naive_empty", "naive", {}, 0, ""),
            (
                "naive_chunk_token_num_negative",
                "naive",
                {"chunk_token_num": -1},
                100,
                "AssertionError('chunk_token_num should be in range from 1 to 100000000')",
            ),
            (
                "naive_chunk_token_num_zero",
                "naive",
                {"chunk_token_num": 0},
                100,
                "AssertionError('chunk_token_num should be in range from 1 to 100000000')",
            ),
            (
                "naive_chunk_token_num_max",
                "naive",
                {"chunk_token_num": 100000000},
                100,
                "AssertionError('chunk_token_num should be in range from 1 to 100000000')",
            ),
            pytest.param(
                "naive_chunk_token_num_float",
                "naive",
                {"chunk_token_num": 3.14},
                102,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            pytest.param(
                "naive_chunk_token_num_str",
                "naive",
                {"chunk_token_num": "1024"},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            (
                "naive_layout_recognize_DeepDOC",
                "naive",
                {"layout_recognize": "DeepDOC"},
                0,
                "",
            ),
            (
                "naive_layout_recognize_Naive",
                "naive",
                {"layout_recognize": "Naive"},
                0,
                "",
            ),
            ("naive_html4excel_true", "naive", {"html4excel": True}, 0, ""),
            ("naive_html4excel_false", "naive", {"html4excel": False}, 0, ""),
            (
                "naive_html4excel_not_bool",
                "naive",
                {"html4excel": 1},
                100,
                "AssertionError('html4excel should be True or False')",
            ),
            ("naive_delimiter_empty", "naive", {"delimiter": ""}, 0, ""),
            ("naive_delimiter_backticks", "naive", {"delimiter": "`##`"}, 0, ""),
            pytest.param(
                "naive_delimiter_not_str",
                "naive",
                {"delimiter": 1},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            (
                "naive_task_page_size_negative",
                "naive",
                {"task_page_size": -1},
                100,
                "AssertionError('task_page_size should be in range from 1 to 100000000')",
            ),
            (
                "naive_task_page_size_zero",
                "naive",
                {"task_page_size": 0},
                100,
                "AssertionError('task_page_size should be in range from 1 to 100000000')",
            ),
            (
                "naive_task_page_size_max",
                "naive",
                {"task_page_size": 100000000},
                100,
                "AssertionError('task_page_size should be in range from 1 to 100000000')",
            ),
            pytest.param(
                "naive_task_page_size_float",
                "naive",
                {"task_page_size": 3.14},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            pytest.param(
                "naive_task_page_size_str",
                "naive",
                {"task_page_size": "1024"},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            ("naive_raptor_true", "naive", {"raptor": {"use_raptor": True}}, 0, ""),
            ("naive_raptor_false", "naive", {"raptor": {"use_raptor": False}}, 0, ""),
            (
                "invalid_key",
                "naive",
                {"invalid_key": "invalid_value"},
                100,
                """AssertionError("Abnormal \'parser_config\'. Invalid key: invalid_key")""",
            ),
            (
                "naive_auto_keywords_negative",
                "naive",
                {"auto_keywords": -1},
                100,
                "AssertionError('auto_keywords should be in range from 0 to 32')",
            ),
            (
                "naive_auto_keywords_max",
                "naive",
                {"auto_keywords": 32},
                100,
                "AssertionError('auto_keywords should be in range from 0 to 32')",
            ),
            pytest.param(
                "naive_auto_keywords_float",
                "naive",
                {"auto_questions": 3.14},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            pytest.param(
                "naive_auto_keywords_str",
                "naive",
                {"auto_keywords": "1024"},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            (
                "naive_auto_questions_negative",
                "naive",
                {"auto_questions": -1},
                100,
                "AssertionError('auto_questions should be in range from 0 to 10')",
            ),
            (
                "naive_auto_questions_max",
                "naive",
                {"auto_questions": 10},
                100,
                "AssertionError('auto_questions should be in range from 0 to 10')",
            ),
            pytest.param(
                "naive_auto_questions_float",
                "naive",
                {"auto_questions": 3.14},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            pytest.param(
                "naive_auto_questions_str",
                "naive",
                {"auto_questions": "1024"},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            (
                "naive_topn_tags_negative",
                "naive",
                {"topn_tags": -1},
                100,
                "AssertionError('topn_tags should be in range from 0 to 10')",
            ),
            (
                "naive_topn_tags_max",
                "naive",
                {"topn_tags": 10},
                100,
                "AssertionError('topn_tags should be in range from 0 to 10')",
            ),
            pytest.param(
                "naive_topn_tags_float",
                "naive",
                {"topn_tags": 3.14},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
            pytest.param(
                "naive_topn_tags_str",
                "naive",
                {"topn_tags": "1024"},
                100,
                "",
                marks=pytest.mark.xfail(reason="issue#5719"),
            ),
        ],
    )
    def test_parser_configs(
        self,
        get_http_api_auth,
        name,
        chunk_method,
        parser_config,
        expected_code,
        expected_message,
    ):
        payload = {
            "name": name,
            "chunk_method": chunk_method,
            "parser_config": parser_config,
        }
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if expected_code == 0 and parser_config != {}:
            for k, v in parser_config.items():
                assert res["data"]["parser_config"][k] == v
        if expected_code != 0 or expected_message:
            assert res["message"] == expected_message
        if parser_config == {}:
            assert res["data"]["parser_config"] == {
                "chunk_token_num": 128,
                "delimiter": "\\n!?;。；！？",
                "html4excel": False,
                "layout_recognize": "DeepDOC",
                "raptor": {"use_raptor": False},
            }
