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
from common import DATASET_NAME_LIMIT, INVALID_API_TOKEN, create_dataset
from hypothesis import example, given, settings
from libs.auth import RAGFlowHttpApiAuth
from libs.utils import encode_avatar
from libs.utils.file_utils import create_image_file
from libs.utils.hypothesis_utils import valid_names


@pytest.mark.usefixtures("clear_datasets")
class TestAuthorization:
    @pytest.mark.p1
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
        ids=["empty_auth", "invalid_api_token"],
    )
    def test_auth_invalid(self, auth, expected_code, expected_message):
        res = create_dataset(auth, {"name": "auth_test"})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestRquest:
    @pytest.mark.p3
    def test_content_type_bad(self, get_http_api_auth):
        BAD_CONTENT_TYPE = "text/xml"
        res = create_dataset(get_http_api_auth, {"name": "bad_content_type"}, headers={"Content-Type": BAD_CONTENT_TYPE})
        assert res["code"] == 101, res
        assert res["message"] == f"Unsupported content type: Expected application/json, got {BAD_CONTENT_TYPE}", res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ("a", "Malformed JSON syntax: Missing commas/brackets or invalid encoding"),
            ('"a"', "Invalid request payload: expected object, got str"),
        ],
        ids=["malformed_json_syntax", "invalid_request_payload_type"],
    )
    def test_payload_bad(self, get_http_api_auth, payload, expected_message):
        res = create_dataset(get_http_api_auth, data=payload)
        assert res["code"] == 101, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("clear_datasets")
class TestCapability:
    @pytest.mark.p3
    def test_create_dataset_1k(self, get_http_api_auth):
        for i in range(1_000):
            payload = {"name": f"dataset_{i}"}
            res = create_dataset(get_http_api_auth, payload)
            assert res["code"] == 0, f"Failed to create dataset {i}"

    @pytest.mark.p3
    def test_create_dataset_concurrent(self, get_http_api_auth):
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(create_dataset, get_http_api_auth, {"name": f"dataset_{i}"}) for i in range(100)]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses), responses


@pytest.mark.usefixtures("clear_datasets")
class TestDatasetCreate:
    @pytest.mark.p1
    @given(name=valid_names())
    @example("a" * 128)
    @settings(max_examples=20)
    def test_name(self, get_http_api_auth, name):
        res = create_dataset(get_http_api_auth, {"name": name})
        assert res["code"] == 0, res
        assert res["data"]["name"] == name, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("", "String should have at least 1 character"),
            (" ", "String should have at least 1 character"),
            ("a" * (DATASET_NAME_LIMIT + 1), "String should have at most 128 characters"),
            (0, "Input should be a valid string"),
            (None, "Input should be a valid string"),
        ],
        ids=["empty_name", "space_name", "too_long_name", "invalid_name", "None_name"],
    )
    def test_name_invalid(self, get_http_api_auth, name, expected_message):
        payload = {"name": name}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert expected_message in res["message"], res

    @pytest.mark.p3
    def test_name_duplicated(self, get_http_api_auth):
        name = "duplicated_name"
        payload = {"name": name}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res

        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 103, res
        assert res["message"] == f"Dataset name '{name}' already exists", res

    @pytest.mark.p3
    def test_name_case_insensitive(self, get_http_api_auth):
        name = "CaseInsensitive"
        payload = {"name": name.upper()}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res

        payload = {"name": name.lower()}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 103, res
        assert res["message"] == f"Dataset name '{name.lower()}' already exists", res

    @pytest.mark.p2
    def test_avatar(self, get_http_api_auth, tmp_path):
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {
            "name": "avatar",
            "avatar": f"data:image/png;base64,{encode_avatar(fn)}",
        }
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_avatar_exceeds_limit_length(self, get_http_api_auth):
        payload = {"name": "avatar_exceeds_limit_length", "avatar": "a" * 65536}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "String should have at most 65535 characters" in res["message"], res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "name, prefix, expected_message",
        [
            ("empty_prefix", "", "Missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
            ("missing_comma", "data:image/png;base64", "Missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
            ("unsupported_mine_type", "invalid_mine_prefix:image/png;base64,", "Invalid MIME prefix format. Must start with 'data:'"),
            ("invalid_mine_type", "data:unsupported_mine_type;base64,", "Unsupported MIME type. Allowed: ['image/jpeg', 'image/png']"),
        ],
        ids=["empty_prefix", "missing_comma", "unsupported_mine_type", "invalid_mine_type"],
    )
    def test_avatar_invalid_prefix(self, get_http_api_auth, tmp_path, name, prefix, expected_message):
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {
            "name": name,
            "avatar": f"{prefix}{encode_avatar(fn)}",
        }
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert expected_message in res["message"], res

    @pytest.mark.p3
    def test_avatar_unset(self, get_http_api_auth):
        payload = {"name": "avatar_unset"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["avatar"] is None, res

    @pytest.mark.p3
    def test_avatar_none(self, get_http_api_auth):
        payload = {"name": "avatar_none", "avatar": None}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["avatar"] is None, res

    @pytest.mark.p2
    def test_description(self, get_http_api_auth):
        payload = {"name": "description", "description": "description"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["description"] == "description", res

    @pytest.mark.p2
    def test_description_exceeds_limit_length(self, get_http_api_auth):
        payload = {"name": "description_exceeds_limit_length", "description": "a" * 65536}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "String should have at most 65535 characters" in res["message"], res

    @pytest.mark.p3
    def test_description_unset(self, get_http_api_auth):
        payload = {"name": "description_unset"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["description"] is None, res

    @pytest.mark.p3
    def test_description_none(self, get_http_api_auth):
        payload = {"name": "description_none", "description": None}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["description"] is None, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, embedding_model",
        [
            ("BAAI/bge-large-zh-v1.5@BAAI", "BAAI/bge-large-zh-v1.5@BAAI"),
            ("maidalun1020/bce-embedding-base_v1@Youdao", "maidalun1020/bce-embedding-base_v1@Youdao"),
            ("embedding-3@ZHIPU-AI", "embedding-3@ZHIPU-AI"),
        ],
        ids=["builtin_baai", "builtin_youdao", "tenant_zhipu"],
    )
    def test_embedding_model(self, get_http_api_auth, name, embedding_model):
        payload = {"name": name, "embedding_model": embedding_model}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["embedding_model"] == embedding_model, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, embedding_model",
        [
            ("unknown_llm_name", "unknown@ZHIPU-AI"),
            ("unknown_llm_factory", "embedding-3@unknown"),
            ("tenant_no_auth_default_tenant_llm", "text-embedding-v3@Tongyi-Qianwen"),
            ("tenant_no_auth", "text-embedding-3-small@OpenAI"),
        ],
        ids=["unknown_llm_name", "unknown_llm_factory", "tenant_no_auth_default_tenant_llm", "tenant_no_auth"],
    )
    def test_embedding_model_invalid(self, get_http_api_auth, name, embedding_model):
        payload = {"name": name, "embedding_model": embedding_model}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        if "tenant_no_auth" in name:
            assert res["message"] == f"Unauthorized model: <{embedding_model}>", res
        else:
            assert res["message"] == f"Unsupported model: <{embedding_model}>", res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, embedding_model",
        [
            ("missing_at", "BAAI/bge-large-zh-v1.5BAAI"),
            ("missing_model_name", "@BAAI"),
            ("missing_provider", "BAAI/bge-large-zh-v1.5@"),
            ("whitespace_only_model_name", " @BAAI"),
            ("whitespace_only_provider", "BAAI/bge-large-zh-v1.5@ "),
        ],
        ids=["missing_at", "empty_model_name", "empty_provider", "whitespace_only_model_name", "whitespace_only_provider"],
    )
    def test_embedding_model_format(self, get_http_api_auth, name, embedding_model):
        payload = {"name": name, "embedding_model": embedding_model}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        if name == "missing_at":
            assert "Embedding model identifier must follow <model_name>@<provider> format" in res["message"], res
        else:
            assert "Both model_name and provider must be non-empty strings" in res["message"], res

    @pytest.mark.p2
    def test_embedding_model_unset(self, get_http_api_auth):
        payload = {"name": "embedding_model_unset"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["embedding_model"] == "BAAI/bge-large-zh-v1.5@BAAI", res

    @pytest.mark.p2
    def test_embedding_model_none(self, get_http_api_auth):
        payload = {"name": "embedding_model_none", "embedding_model": None}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Input should be a valid string" in res["message"], res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, permission",
        [
            ("me", "me"),
            ("team", "team"),
            ("me_upercase", "ME"),
            ("team_upercase", "TEAM"),
            ("whitespace", " ME "),
        ],
        ids=["me", "team", "me_upercase", "team_upercase", "whitespace"],
    )
    def test_permission(self, get_http_api_auth, name, permission):
        payload = {"name": name, "permission": permission}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["permission"] == permission.lower().strip(), res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, permission",
        [
            ("empty", ""),
            ("unknown", "unknown"),
            ("type_error", list()),
        ],
        ids=["empty", "unknown", "type_error"],
    )
    def test_permission_invalid(self, get_http_api_auth, name, permission):
        payload = {"name": name, "permission": permission}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101
        assert "Input should be 'me' or 'team'" in res["message"]

    @pytest.mark.p2
    def test_permission_unset(self, get_http_api_auth):
        payload = {"name": "permission_unset"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["permission"] == "me", res

    @pytest.mark.p3
    def test_permission_none(self, get_http_api_auth):
        payload = {"name": "permission_none", "permission": None}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Input should be 'me' or 'team'" in res["message"], res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, chunk_method",
        [
            ("naive", "naive"),
            ("book", "book"),
            ("email", "email"),
            ("laws", "laws"),
            ("manual", "manual"),
            ("one", "one"),
            ("paper", "paper"),
            ("picture", "picture"),
            ("presentation", "presentation"),
            ("qa", "qa"),
            ("table", "table"),
            ("tag", "tag"),
        ],
        ids=["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table", "tag"],
    )
    def test_chunk_method(self, get_http_api_auth, name, chunk_method):
        payload = {"name": name, "chunk_method": chunk_method}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["chunk_method"] == chunk_method, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, chunk_method",
        [
            ("empty", ""),
            ("unknown", "unknown"),
            ("type_error", list()),
        ],
        ids=["empty", "unknown", "type_error"],
    )
    def test_chunk_method_invalid(self, get_http_api_auth, name, chunk_method):
        payload = {"name": name, "chunk_method": chunk_method}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'table' or 'tag'" in res["message"], res

    @pytest.mark.p2
    def test_chunk_method_unset(self, get_http_api_auth):
        payload = {"name": "chunk_method_unset"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["chunk_method"] == "naive", res

    @pytest.mark.p3
    def test_chunk_method_none(self, get_http_api_auth):
        payload = {"name": "chunk_method_none", "chunk_method": None}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'table' or 'tag'" in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, pagerank",
        [
            ("pagerank_min", 0),
            ("pagerank_mid", 50),
            ("pagerank_max", 100),
        ],
        ids=["min", "mid", "max"],
    )
    def test_pagerank(self, get_http_api_auth, name, pagerank):
        payload = {"name": name, "pagerank": pagerank}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["pagerank"] == pagerank, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "name, pagerank, expected_message",
        [
            ("pagerank_min_limit", -1, "Input should be greater than or equal to 0"),
            ("pagerank_max_limit", 101, "Input should be less than or equal to 100"),
        ],
        ids=["min_limit", "max_limit"],
    )
    def test_pagerank_invalid(self, get_http_api_auth, name, pagerank, expected_message):
        payload = {"name": name, "pagerank": pagerank}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert expected_message in res["message"], res

    @pytest.mark.p3
    def test_pagerank_unset(self, get_http_api_auth):
        payload = {"name": "pagerank_unset"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["pagerank"] == 0, res

    @pytest.mark.p3
    def test_pagerank_none(self, get_http_api_auth):
        payload = {"name": "pagerank_unset", "pagerank": None}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Input should be a valid integer" in res["message"], res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, parser_config",
        [
            ("auto_keywords_min", {"auto_keywords": 0}),
            ("auto_keywords_mid", {"auto_keywords": 16}),
            ("auto_keywords_max", {"auto_keywords": 32}),
            ("auto_questions_min", {"auto_questions": 0}),
            ("auto_questions_mid", {"auto_questions": 5}),
            ("auto_questions_max", {"auto_questions": 10}),
            ("chunk_token_num_min", {"chunk_token_num": 1}),
            ("chunk_token_num_mid", {"chunk_token_num": 1024}),
            ("chunk_token_num_max", {"chunk_token_num": 2048}),
            ("delimiter", {"delimiter": "\n"}),
            ("delimiter_space", {"delimiter": " "}),
            ("html4excel_true", {"html4excel": True}),
            ("html4excel_false", {"html4excel": False}),
            ("layout_recognize_DeepDOC", {"layout_recognize": "DeepDOC"}),
            ("layout_recognize_navie", {"layout_recognize": "Plain Text"}),
            ("tag_kb_ids", {"tag_kb_ids": ["1", "2"]}),
            ("topn_tags_min", {"topn_tags": 1}),
            ("topn_tags_mid", {"topn_tags": 5}),
            ("topn_tags_max", {"topn_tags": 10}),
            ("filename_embd_weight_min", {"filename_embd_weight": 0.1}),
            ("filename_embd_weight_mid", {"filename_embd_weight": 0.5}),
            ("filename_embd_weight_max", {"filename_embd_weight": 1.0}),
            ("task_page_size_min", {"task_page_size": 1}),
            ("task_page_size_None", {"task_page_size": None}),
            ("pages", {"pages": [[1, 100]]}),
            ("pages_none", {"pages": None}),
            ("graphrag_true", {"graphrag": {"use_graphrag": True}}),
            ("graphrag_false", {"graphrag": {"use_graphrag": False}}),
            ("graphrag_entity_types", {"graphrag": {"entity_types": ["age", "sex", "height", "weight"]}}),
            ("graphrag_method_general", {"graphrag": {"method": "general"}}),
            ("graphrag_method_light", {"graphrag": {"method": "light"}}),
            ("graphrag_community_true", {"graphrag": {"community": True}}),
            ("graphrag_community_false", {"graphrag": {"community": False}}),
            ("graphrag_resolution_true", {"graphrag": {"resolution": True}}),
            ("graphrag_resolution_false", {"graphrag": {"resolution": False}}),
            ("raptor_true", {"raptor": {"use_raptor": True}}),
            ("raptor_false", {"raptor": {"use_raptor": False}}),
            ("raptor_prompt", {"raptor": {"prompt": "Who are you?"}}),
            ("raptor_max_token_min", {"raptor": {"max_token": 1}}),
            ("raptor_max_token_mid", {"raptor": {"max_token": 1024}}),
            ("raptor_max_token_max", {"raptor": {"max_token": 2048}}),
            ("raptor_threshold_min", {"raptor": {"threshold": 0.0}}),
            ("raptor_threshold_mid", {"raptor": {"threshold": 0.5}}),
            ("raptor_threshold_max", {"raptor": {"threshold": 1.0}}),
            ("raptor_max_cluster_min", {"raptor": {"max_cluster": 1}}),
            ("raptor_max_cluster_mid", {"raptor": {"max_cluster": 512}}),
            ("raptor_max_cluster_max", {"raptor": {"max_cluster": 1024}}),
            ("raptor_random_seed_min", {"raptor": {"random_seed": 0}}),
        ],
        ids=[
            "auto_keywords_min",
            "auto_keywords_mid",
            "auto_keywords_max",
            "auto_questions_min",
            "auto_questions_mid",
            "auto_questions_max",
            "chunk_token_num_min",
            "chunk_token_num_mid",
            "chunk_token_num_max",
            "delimiter",
            "delimiter_space",
            "html4excel_true",
            "html4excel_false",
            "layout_recognize_DeepDOC",
            "layout_recognize_navie",
            "tag_kb_ids",
            "topn_tags_min",
            "topn_tags_mid",
            "topn_tags_max",
            "filename_embd_weight_min",
            "filename_embd_weight_mid",
            "filename_embd_weight_max",
            "task_page_size_min",
            "task_page_size_None",
            "pages",
            "pages_none",
            "graphrag_true",
            "graphrag_false",
            "graphrag_entity_types",
            "graphrag_method_general",
            "graphrag_method_light",
            "graphrag_community_true",
            "graphrag_community_false",
            "graphrag_resolution_true",
            "graphrag_resolution_false",
            "raptor_true",
            "raptor_false",
            "raptor_prompt",
            "raptor_max_token_min",
            "raptor_max_token_mid",
            "raptor_max_token_max",
            "raptor_threshold_min",
            "raptor_threshold_mid",
            "raptor_threshold_max",
            "raptor_max_cluster_min",
            "raptor_max_cluster_mid",
            "raptor_max_cluster_max",
            "raptor_random_seed_min",
        ],
    )
    def test_parser_config(self, get_http_api_auth, name, parser_config):
        payload = {"name": name, "parser_config": parser_config}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        for k, v in parser_config.items():
            if isinstance(v, dict):
                for kk, vv in v.items():
                    assert res["data"]["parser_config"][k][kk] == vv, res
            else:
                assert res["data"]["parser_config"][k] == v, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, parser_config, expected_message",
        [
            ("auto_keywords_min_limit", {"auto_keywords": -1}, "Input should be greater than or equal to 0"),
            ("auto_keywords_max_limit", {"auto_keywords": 33}, "Input should be less than or equal to 32"),
            ("auto_keywords_float_not_allowed", {"auto_keywords": 3.14}, "Input should be a valid integer, got a number with a fractional part"),
            ("auto_keywords_type_invalid", {"auto_keywords": "string"}, "Input should be a valid integer, unable to parse string as an integer"),
            ("auto_questions_min_limit", {"auto_questions": -1}, "Input should be greater than or equal to 0"),
            ("auto_questions_max_limit", {"auto_questions": 11}, "Input should be less than or equal to 10"),
            ("auto_questions_float_not_allowed", {"auto_questions": 3.14}, "Input should be a valid integer, got a number with a fractional part"),
            ("auto_questions_type_invalid", {"auto_questions": "string"}, "Input should be a valid integer, unable to parse string as an integer"),
            ("chunk_token_num_min_limit", {"chunk_token_num": 0}, "Input should be greater than or equal to 1"),
            ("chunk_token_num_max_limit", {"chunk_token_num": 2049}, "Input should be less than or equal to 2048"),
            ("chunk_token_num_float_not_allowed", {"chunk_token_num": 3.14}, "Input should be a valid integer, got a number with a fractional part"),
            ("chunk_token_num_type_invalid", {"chunk_token_num": "string"}, "Input should be a valid integer, unable to parse string as an integer"),
            ("delimiter_empty", {"delimiter": ""}, "String should have at least 1 character"),
            ("html4excel_type_invalid", {"html4excel": "string"}, "Input should be a valid boolean, unable to interpret input"),
            ("tag_kb_ids_not_list", {"tag_kb_ids": "1,2"}, "Input should be a valid list"),
            ("tag_kb_ids_int_in_list", {"tag_kb_ids": [1, 2]}, "Input should be a valid string"),
            ("topn_tags_min_limit", {"topn_tags": 0}, "Input should be greater than or equal to 1"),
            ("topn_tags_max_limit", {"topn_tags": 11}, "Input should be less than or equal to 10"),
            ("topn_tags_float_not_allowed", {"topn_tags": 3.14}, "Input should be a valid integer, got a number with a fractional part"),
            ("topn_tags_type_invalid", {"topn_tags": "string"}, "Input should be a valid integer, unable to parse string as an integer"),
            ("filename_embd_weight_min_limit", {"filename_embd_weight": -1}, "Input should be greater than or equal to 0"),
            ("filename_embd_weight_max_limit", {"filename_embd_weight": 1.1}, "Input should be less than or equal to 1"),
            ("filename_embd_weight_type_invalid", {"filename_embd_weight": "string"}, "Input should be a valid number, unable to parse string as a number"),
            ("task_page_size_min_limit", {"task_page_size": 0}, "Input should be greater than or equal to 1"),
            ("task_page_size_float_not_allowed", {"task_page_size": 3.14}, "Input should be a valid integer, got a number with a fractional part"),
            ("task_page_size_type_invalid", {"task_page_size": "string"}, "Input should be a valid integer, unable to parse string as an integer"),
            ("pages_not_list", {"pages": "1,2"}, "Input should be a valid list"),
            ("pages_not_list_in_list", {"pages": ["1,2"]}, "Input should be a valid list"),
            ("pages_not_int_list", {"pages": [["string1", "string2"]]}, "Input should be a valid integer, unable to parse string as an integer"),
            ("graphrag_type_invalid", {"graphrag": {"use_graphrag": "string"}}, "Input should be a valid boolean, unable to interpret input"),
            ("graphrag_entity_types_not_list", {"graphrag": {"entity_types": "1,2"}}, "Input should be a valid list"),
            ("graphrag_entity_types_not_str_in_list", {"graphrag": {"entity_types": [1, 2]}}, "nput should be a valid string"),
            ("graphrag_method_unknown", {"graphrag": {"method": "unknown"}}, "Input should be 'light' or 'general'"),
            ("graphrag_method_none", {"graphrag": {"method": None}}, "Input should be 'light' or 'general'"),
            ("graphrag_community_type_invalid", {"graphrag": {"community": "string"}}, "Input should be a valid boolean, unable to interpret input"),
            ("graphrag_resolution_type_invalid", {"graphrag": {"resolution": "string"}}, "Input should be a valid boolean, unable to interpret input"),
            ("raptor_type_invalid", {"raptor": {"use_raptor": "string"}}, "Input should be a valid boolean, unable to interpret input"),
            ("raptor_prompt_empty", {"raptor": {"prompt": ""}}, "String should have at least 1 character"),
            ("raptor_prompt_space", {"raptor": {"prompt": " "}}, "String should have at least 1 character"),
            ("raptor_max_token_min_limit", {"raptor": {"max_token": 0}}, "Input should be greater than or equal to 1"),
            ("raptor_max_token_max_limit", {"raptor": {"max_token": 2049}}, "Input should be less than or equal to 2048"),
            ("raptor_max_token_float_not_allowed", {"raptor": {"max_token": 3.14}}, "Input should be a valid integer, got a number with a fractional part"),
            ("raptor_max_token_type_invalid", {"raptor": {"max_token": "string"}}, "Input should be a valid integer, unable to parse string as an integer"),
            ("raptor_threshold_min_limit", {"raptor": {"threshold": -0.1}}, "Input should be greater than or equal to 0"),
            ("raptor_threshold_max_limit", {"raptor": {"threshold": 1.1}}, "Input should be less than or equal to 1"),
            ("raptor_threshold_type_invalid", {"raptor": {"threshold": "string"}}, "Input should be a valid number, unable to parse string as a number"),
            ("raptor_max_cluster_min_limit", {"raptor": {"max_cluster": 0}}, "Input should be greater than or equal to 1"),
            ("raptor_max_cluster_max_limit", {"raptor": {"max_cluster": 1025}}, "Input should be less than or equal to 1024"),
            ("raptor_max_cluster_float_not_allowed", {"raptor": {"max_cluster": 3.14}}, "Input should be a valid integer, got a number with a fractional par"),
            ("raptor_max_cluster_type_invalid", {"raptor": {"max_cluster": "string"}}, "Input should be a valid integer, unable to parse string as an integer"),
            ("raptor_random_seed_min_limit", {"raptor": {"random_seed": -1}}, "Input should be greater than or equal to 0"),
            ("raptor_random_seed_float_not_allowed", {"raptor": {"random_seed": 3.14}}, "Input should be a valid integer, got a number with a fractional part"),
            ("raptor_random_seed_type_invalid", {"raptor": {"random_seed": "string"}}, "Input should be a valid integer, unable to parse string as an integer"),
            ("parser_config_type_invalid", {"delimiter": "a" * 65536}, "Parser config exceeds size limit (max 65,535 characters)"),
        ],
        ids=[
            "auto_keywords_min_limit",
            "auto_keywords_max_limit",
            "auto_keywords_float_not_allowed",
            "auto_keywords_type_invalid",
            "auto_questions_min_limit",
            "auto_questions_max_limit",
            "auto_questions_float_not_allowed",
            "auto_questions_type_invalid",
            "chunk_token_num_min_limit",
            "chunk_token_num_max_limit",
            "chunk_token_num_float_not_allowed",
            "chunk_token_num_type_invalid",
            "delimiter_empty",
            "html4excel_type_invalid",
            "tag_kb_ids_not_list",
            "tag_kb_ids_int_in_list",
            "topn_tags_min_limit",
            "topn_tags_max_limit",
            "topn_tags_float_not_allowed",
            "topn_tags_type_invalid",
            "filename_embd_weight_min_limit",
            "filename_embd_weight_max_limit",
            "filename_embd_weight_type_invalid",
            "task_page_size_min_limit",
            "task_page_size_float_not_allowed",
            "task_page_size_type_invalid",
            "pages_not_list",
            "pages_not_list_in_list",
            "pages_not_int_list",
            "graphrag_type_invalid",
            "graphrag_entity_types_not_list",
            "graphrag_entity_types_not_str_in_list",
            "graphrag_method_unknown",
            "graphrag_method_none",
            "graphrag_community_type_invalid",
            "graphrag_resolution_type_invalid",
            "raptor_type_invalid",
            "raptor_prompt_empty",
            "raptor_prompt_space",
            "raptor_max_token_min_limit",
            "raptor_max_token_max_limit",
            "raptor_max_token_float_not_allowed",
            "raptor_max_token_type_invalid",
            "raptor_threshold_min_limit",
            "raptor_threshold_max_limit",
            "raptor_threshold_type_invalid",
            "raptor_max_cluster_min_limit",
            "raptor_max_cluster_max_limit",
            "raptor_max_cluster_float_not_allowed",
            "raptor_max_cluster_type_invalid",
            "raptor_random_seed_min_limit",
            "raptor_random_seed_float_not_allowed",
            "raptor_random_seed_type_invalid",
            "parser_config_type_invalid",
        ],
    )
    def test_parser_config_invalid(self, get_http_api_auth, name, parser_config, expected_message):
        payload = {"name": name, "parser_config": parser_config}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert expected_message in res["message"], res

    @pytest.mark.p2
    def test_parser_config_empty(self, get_http_api_auth):
        payload = {"name": "parser_config_empty", "parser_config": {}}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["parser_config"] == {
            "chunk_token_num": 128,
            "delimiter": r"\n",
            "html4excel": False,
            "layout_recognize": "DeepDOC",
            "raptor": {"use_raptor": False},
        }, res

    @pytest.mark.p2
    def test_parser_config_unset(self, get_http_api_auth):
        payload = {"name": "parser_config_unset"}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["parser_config"] == {
            "chunk_token_num": 128,
            "delimiter": r"\n",
            "html4excel": False,
            "layout_recognize": "DeepDOC",
            "raptor": {"use_raptor": False},
        }, res

    @pytest.mark.p3
    def test_parser_config_none(self, get_http_api_auth):
        payload = {"name": "parser_config_none", "parser_config": None}
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["parser_config"] == {
            "chunk_token_num": 128,
            "delimiter": "\\n",
            "html4excel": False,
            "layout_recognize": "DeepDOC",
            "raptor": {"use_raptor": False},
        }, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload",
        [
            {"name": "id", "id": "id"},
            {"name": "tenant_id", "tenant_id": "e57c1966f99211efb41e9e45646e0111"},
            {"name": "created_by", "created_by": "created_by"},
            {"name": "create_date", "create_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
            {"name": "create_time", "create_time": 1741671443322},
            {"name": "update_date", "update_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
            {"name": "update_time", "update_time": 1741671443339},
            {"name": "document_count", "document_count": 1},
            {"name": "chunk_count", "chunk_count": 1},
            {"name": "token_num", "token_num": 1},
            {"name": "status", "status": "1"},
            {"name": "unknown_field", "unknown_field": "unknown_field"},
        ],
    )
    def test_unsupported_field(self, get_http_api_auth, payload):
        res = create_dataset(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Extra inputs are not permitted" in res["message"], res
