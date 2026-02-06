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
from operator import attrgetter

import pytest
from configs import DATASET_NAME_LIMIT, DEFAULT_PARSER_CONFIG, HOST_ADDRESS, INVALID_API_TOKEN
from hypothesis import example, given, settings
from ragflow_sdk import DataSet, RAGFlow
from utils import encode_avatar
from utils.file_utils import create_image_file
from utils.hypothesis_utils import valid_names


@pytest.mark.usefixtures("clear_datasets")
class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_message",
        [
            (None, "Authentication error: API key is invalid!"),
            (INVALID_API_TOKEN, "Authentication error: API key is invalid!"),
        ],
        ids=["empty_auth", "invalid_api_token"],
    )
    def test_auth_invalid(self, invalid_auth, expected_message):
        client = RAGFlow(invalid_auth, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**{"name": "auth_test"})
        assert str(exception_info.value) == expected_message


@pytest.mark.usefixtures("clear_datasets")
class TestCapability:
    @pytest.mark.p3
    def test_create_dataset_1k(self, client):
        count = 1_000
        for i in range(count):
            payload = {"name": f"dataset_{i}"}
            client.create_dataset(**payload)
        assert len(client.list_datasets(page_size=2000)) == count

    @pytest.mark.p3
    def test_create_dataset_concurrent(self, client):
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.create_dataset, **{"name": f"dataset_{i}"}) for i in range(100)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses


@pytest.mark.usefixtures("clear_datasets")
class TestDatasetCreate:
    @pytest.mark.p1
    @given(name=valid_names())
    @example("a" * 128)
    @settings(max_examples=20)
    def test_name(self, client, name):
        dataset = client.create_dataset(**{"name": name})
        assert dataset.name == name, str(dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("", "String should have at least 1 character"),
            (" ", "String should have at least 1 character"),
            ("a" * (DATASET_NAME_LIMIT + 1), "String should have at most 128 characters"),
            (0, "not instance of"),
            (None, "not instance of"),
        ],
        ids=["empty_name", "space_name", "too_long_name", "invalid_name", "None_name"],
    )
    def test_name_invalid(self, client, name, expected_message):
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**{"name": name})
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_name_duplicated(self, client):
        name = "duplicated_name"
        payload = {"name": name}
        client.create_dataset(**payload)

        dataset = client.create_dataset(**payload)
        assert dataset.name == name + "(1)", str(dataset)

    @pytest.mark.p3
    def test_name_case_insensitive(self, client):
        name = "CaseInsensitive"
        payload = {"name": name.upper()}
        client.create_dataset(**payload)

        payload = {"name": name.lower()}
        dataset = client.create_dataset(**payload)
        assert dataset.name == name.lower() + "(1)", str(dataset)

    @pytest.mark.p2
    def test_avatar(self, client, tmp_path):
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {
            "name": "avatar",
            "avatar": f"data:image/png;base64,{encode_avatar(fn)}",
        }
        client.create_dataset(**payload)

    @pytest.mark.p3
    def test_avatar_exceeds_limit_length(self, client):
        payload = {"name": "avatar_exceeds_limit_length", "avatar": "a" * 65536}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert "String should have at most 65535 characters" in str(exception_info.value), str(exception_info.value)

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
    def test_avatar_invalid_prefix(self, client, tmp_path, name, prefix, expected_message):
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {
            "name": name,
            "avatar": f"{prefix}{encode_avatar(fn)}",
        }
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_avatar_unset(self, client):
        payload = {"name": "avatar_unset"}
        dataset = client.create_dataset(**payload)
        assert dataset.avatar is None, str(dataset)

    @pytest.mark.p2
    def test_description(self, client):
        payload = {"name": "description", "description": "description"}
        dataset = client.create_dataset(**payload)
        assert dataset.description == "description", str(dataset)

    @pytest.mark.p3
    def test_description_exceeds_limit_length(self, client):
        payload = {"name": "description_exceeds_limit_length", "description": "a" * 65536}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert "String should have at most 65535 characters" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_description_unset(self, client):
        payload = {"name": "description_unset"}
        dataset = client.create_dataset(**payload)
        assert dataset.description is None, str(dataset)

    @pytest.mark.p3
    def test_description_none(self, client):
        payload = {"name": "description_none", "description": None}
        dataset = client.create_dataset(**payload)
        assert dataset.description is None, str(dataset)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, embedding_model",
        [
            ("BAAI/bge-small-en-v1.5@Builtin", "BAAI/bge-small-en-v1.5@Builtin"),
            ("embedding-3@ZHIPU-AI", "embedding-3@ZHIPU-AI"),
        ],
        ids=["builtin_baai", "tenant_zhipu"],
    )
    def test_embedding_model(self, client, name, embedding_model):
        payload = {"name": name, "embedding_model": embedding_model}
        dataset = client.create_dataset(**payload)
        assert dataset.embedding_model == embedding_model, str(dataset)

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
    def test_embedding_model_invalid(self, client, name, embedding_model):
        payload = {"name": name, "embedding_model": embedding_model}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        if "tenant_no_auth" in name:
            assert str(exception_info.value) == f"Unauthorized model: <{embedding_model}>", str(exception_info.value)
        else:
            assert str(exception_info.value) == f"Unsupported model: <{embedding_model}>", str(exception_info.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, embedding_model",
        [
            ("empty", ""),
            ("space", " "),
            ("missing_at", "BAAI/bge-small-en-v1.5Builtin"),
            ("missing_model_name", "@Builtin"),
            ("missing_provider", "BAAI/bge-small-en-v1.5@"),
            ("whitespace_only_model_name", " @Builtin"),
            ("whitespace_only_provider", "BAAI/bge-small-en-v1.5@ "),
        ],
        ids=["empty", "space", "missing_at", "empty_model_name", "empty_provider", "whitespace_only_model_name", "whitespace_only_provider"],
    )
    def test_embedding_model_format(self, client, name, embedding_model):
        payload = {"name": name, "embedding_model": embedding_model}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        if name in ["empty", "space", "missing_at"]:
            assert "Embedding model identifier must follow <model_name>@<provider> format" in str(exception_info.value), str(exception_info.value)
        else:
            assert "Both model_name and provider must be non-empty strings" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_embedding_model_unset(self, client):
        payload = {"name": "embedding_model_unset"}
        dataset = client.create_dataset(**payload)
        assert dataset.embedding_model == "BAAI/bge-small-en-v1.5@Builtin", str(dataset)

    @pytest.mark.p2
    def test_embedding_model_none(self, client):
        payload = {"name": "embedding_model_none", "embedding_model": None}
        dataset = client.create_dataset(**payload)
        assert dataset.embedding_model == "BAAI/bge-small-en-v1.5@Builtin", str(dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, permission",
        [
            ("me", "me"),
            ("team", "team"),
        ],
        ids=["me", "team"],
    )
    def test_permission(self, client, name, permission):
        payload = {"name": name, "permission": permission}
        dataset = client.create_dataset(**payload)
        assert dataset.permission == permission.lower().strip(), str(dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, permission",
        [
            ("empty", ""),
            ("unknown", "unknown"),
            ("me_upercase", "ME"),
            ("team_upercase", "TEAM"),
            ("whitespace", " ME "),
        ],
        ids=["empty", "unknown", "me_upercase", "team_upercase", "whitespace"],
    )
    def test_permission_invalid(self, client, name, permission):
        payload = {"name": name, "permission": permission}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert "Input should be 'me' or 'team'" in str(exception_info.value)

    @pytest.mark.p2
    def test_permission_unset(self, client):
        payload = {"name": "permission_unset"}
        dataset = client.create_dataset(**payload)
        assert dataset.permission == "me", str(dataset)

    @pytest.mark.p3
    def test_permission_none(self, client):
        payload = {"name": "permission_none", "permission": None}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert "not instance of" in str(exception_info.value), str(exception_info.value)

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
    def test_chunk_method(self, client, name, chunk_method):
        payload = {"name": name, "chunk_method": chunk_method}
        dataset = client.create_dataset(**payload)
        assert dataset.chunk_method == chunk_method, str(dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, chunk_method",
        [
            ("empty", ""),
            ("unknown", "unknown"),
        ],
        ids=["empty", "unknown"],
    )
    def test_chunk_method_invalid(self, client, name, chunk_method):
        payload = {"name": name, "chunk_method": chunk_method}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'table' or 'tag'" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_chunk_method_unset(self, client):
        payload = {"name": "chunk_method_unset"}
        dataset = client.create_dataset(**payload)
        assert dataset.chunk_method == "naive", str(dataset)

    @pytest.mark.p3
    def test_chunk_method_none(self, client):
        payload = {"name": "chunk_method_none", "chunk_method": None}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert "not instance of" in str(exception_info.value), str(exception_info.value)

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
    def test_parser_config(self, client, name, parser_config):
        parser_config_o = DataSet.ParserConfig(client, parser_config)
        payload = {"name": name, "parser_config": parser_config_o}
        dataset = client.create_dataset(**payload)
        for k, v in parser_config.items():
            if isinstance(v, dict):
                for kk, vv in v.items():
                    assert attrgetter(f"{k}.{kk}")(dataset.parser_config) == vv, str(dataset)
            else:
                assert attrgetter(k)(dataset.parser_config) == v, str(dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, parser_config, expected_message",
        [
            ("auto_keywords_min_limit", {"auto_keywords": -1}, "Input should be greater than or equal to 0"),
            ("auto_keywords_max_limit", {"auto_keywords": 33}, "Input should be less than or equal to 32"),
            ("auto_keywords_float_not_allowed", {"auto_keywords": 3.14}, "Input should be a valid integer"),
            ("auto_keywords_type_invalid", {"auto_keywords": "string"}, "Input should be a valid integer"),
            ("auto_questions_min_limit", {"auto_questions": -1}, "Input should be greater than or equal to 0"),
            ("auto_questions_max_limit", {"auto_questions": 11}, "Input should be less than or equal to 10"),
            ("auto_questions_float_not_allowed", {"auto_questions": 3.14}, "Input should be a valid integer"),
            ("auto_questions_type_invalid", {"auto_questions": "string"}, "Input should be a valid integer"),
            ("chunk_token_num_min_limit", {"chunk_token_num": 0}, "Input should be greater than or equal to 1"),
            ("chunk_token_num_max_limit", {"chunk_token_num": 2049}, "Input should be less than or equal to 2048"),
            ("chunk_token_num_float_not_allowed", {"chunk_token_num": 3.14}, "Input should be a valid integer"),
            ("chunk_token_num_type_invalid", {"chunk_token_num": "string"}, "Input should be a valid integer"),
            ("delimiter_empty", {"delimiter": ""}, "String should have at least 1 character"),
            ("html4excel_type_invalid", {"html4excel": "string"}, "Input should be a valid boolean"),
            ("tag_kb_ids_not_list", {"tag_kb_ids": "1,2"}, "Input should be a valid list"),
            ("tag_kb_ids_int_in_list", {"tag_kb_ids": [1, 2]}, "Input should be a valid string"),
            ("topn_tags_min_limit", {"topn_tags": 0}, "Input should be greater than or equal to 1"),
            ("topn_tags_max_limit", {"topn_tags": 11}, "Input should be less than or equal to 10"),
            ("topn_tags_float_not_allowed", {"topn_tags": 3.14}, "Input should be a valid integer"),
            ("topn_tags_type_invalid", {"topn_tags": "string"}, "Input should be a valid integer"),
            ("filename_embd_weight_min_limit", {"filename_embd_weight": -1}, "Input should be greater than or equal to 0"),
            ("filename_embd_weight_max_limit", {"filename_embd_weight": 1.1}, "Input should be less than or equal to 1"),
            ("filename_embd_weight_type_invalid", {"filename_embd_weight": "string"}, "Input should be a valid number"),
            ("task_page_size_min_limit", {"task_page_size": 0}, "Input should be greater than or equal to 1"),
            ("task_page_size_float_not_allowed", {"task_page_size": 3.14}, "Input should be a valid integer"),
            ("task_page_size_type_invalid", {"task_page_size": "string"}, "Input should be a valid integer"),
            ("pages_not_list", {"pages": "1,2"}, "Input should be a valid list"),
            ("pages_not_list_in_list", {"pages": ["1,2"]}, "Input should be a valid list"),
            ("pages_not_int_list", {"pages": [["string1", "string2"]]}, "Input should be a valid integer"),
            ("graphrag_type_invalid", {"graphrag": {"use_graphrag": "string"}}, "Input should be a valid boolean"),
            ("graphrag_entity_types_not_list", {"graphrag": {"entity_types": "1,2"}}, "Input should be a valid list"),
            ("graphrag_entity_types_not_str_in_list", {"graphrag": {"entity_types": [1, 2]}}, "nput should be a valid string"),
            ("graphrag_method_unknown", {"graphrag": {"method": "unknown"}}, "Input should be 'light' or 'general'"),
            ("graphrag_method_none", {"graphrag": {"method": None}}, "Input should be 'light' or 'general'"),
            ("graphrag_community_type_invalid", {"graphrag": {"community": "string"}}, "Input should be a valid boolean"),
            ("graphrag_resolution_type_invalid", {"graphrag": {"resolution": "string"}}, "Input should be a valid boolean"),
            ("raptor_type_invalid", {"raptor": {"use_raptor": "string"}}, "Input should be a valid boolean"),
            ("raptor_prompt_empty", {"raptor": {"prompt": ""}}, "String should have at least 1 character"),
            ("raptor_prompt_space", {"raptor": {"prompt": " "}}, "String should have at least 1 character"),
            ("raptor_max_token_min_limit", {"raptor": {"max_token": 0}}, "Input should be greater than or equal to 1"),
            ("raptor_max_token_max_limit", {"raptor": {"max_token": 2049}}, "Input should be less than or equal to 2048"),
            ("raptor_max_token_float_not_allowed", {"raptor": {"max_token": 3.14}}, "Input should be a valid integer"),
            ("raptor_max_token_type_invalid", {"raptor": {"max_token": "string"}}, "Input should be a valid integer"),
            ("raptor_threshold_min_limit", {"raptor": {"threshold": -0.1}}, "Input should be greater than or equal to 0"),
            ("raptor_threshold_max_limit", {"raptor": {"threshold": 1.1}}, "Input should be less than or equal to 1"),
            ("raptor_threshold_type_invalid", {"raptor": {"threshold": "string"}}, "Input should be a valid number"),
            ("raptor_max_cluster_min_limit", {"raptor": {"max_cluster": 0}}, "Input should be greater than or equal to 1"),
            ("raptor_max_cluster_max_limit", {"raptor": {"max_cluster": 1025}}, "Input should be less than or equal to 1024"),
            ("raptor_max_cluster_float_not_allowed", {"raptor": {"max_cluster": 3.14}}, "Input should be a valid integer"),
            ("raptor_max_cluster_type_invalid", {"raptor": {"max_cluster": "string"}}, "Input should be a valid integer"),
            ("raptor_random_seed_min_limit", {"raptor": {"random_seed": -1}}, "Input should be greater than or equal to 0"),
            ("raptor_random_seed_float_not_allowed", {"raptor": {"random_seed": 3.14}}, "Input should be a valid integer"),
            ("raptor_random_seed_type_invalid", {"raptor": {"random_seed": "string"}}, "Input should be a valid integer"),
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
    def test_parser_config_invalid(self, client, name, parser_config, expected_message):
        parser_config_o = DataSet.ParserConfig(client, parser_config)
        payload = {"name": name, "parser_config": parser_config_o}
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_parser_config_empty(self, client):
        excepted_value = DataSet.ParserConfig(
            client,
            DEFAULT_PARSER_CONFIG,
        )
        parser_config_o = DataSet.ParserConfig(client, {})
        payload = {"name": "parser_config_empty", "parser_config": parser_config_o}
        dataset = client.create_dataset(**payload)
        assert str(dataset.parser_config) == str(excepted_value), str(dataset)

    @pytest.mark.p2
    def test_parser_config_unset(self, client):
        excepted_value = DataSet.ParserConfig(
            client,
            DEFAULT_PARSER_CONFIG,
        )
        payload = {"name": "parser_config_unset"}
        dataset = client.create_dataset(**payload)
        assert str(dataset.parser_config) == str(excepted_value), str(dataset)

    @pytest.mark.p3
    def test_parser_config_none(self, client):
        excepted_value = DataSet.ParserConfig(
            client,
            DEFAULT_PARSER_CONFIG,
        )
        payload = {"name": "parser_config_empty", "parser_config": None}
        dataset = client.create_dataset(**payload)
        assert str(dataset.parser_config) == str(excepted_value), str(dataset)

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
            {"name": "pagerank", "pagerank": 50},
            {"name": "unknown_field", "unknown_field": "unknown_field"},
        ],
    )
    def test_unsupported_field(self, client, payload):
        with pytest.raises(Exception) as exception_info:
            client.create_dataset(**payload)
        assert "got an unexpected keyword argument" in str(exception_info.value), str(exception_info.value)


@pytest.mark.usefixtures("clear_datasets")
class TestParserConfigBugFix:
    @pytest.mark.p1
    def test_parser_config_missing_raptor_and_graphrag(self, client):
        parser_config = DataSet.ParserConfig(client, {"chunk_token_num": 1024})
        payload = {"name": "test_parser_config_missing_fields_sdk", "parser_config": parser_config}
        dataset = client.create_dataset(**payload)

        config = dataset.parser_config
        assert hasattr(config, "raptor"), "raptor field should be present"
        assert hasattr(config, "graphrag"), "graphrag field should be present"
        assert config.raptor.use_raptor is False, "raptor.use_raptor should default to False"
        assert config.graphrag.use_graphrag is False, "graphrag.use_graphrag should default to False"
        assert config.chunk_token_num == 1024, "User-provided chunk_token_num should be preserved"

    @pytest.mark.p1
    def test_parser_config_with_only_raptor(self, client):
        parser_config = DataSet.ParserConfig(client, {"chunk_token_num": 1024, "raptor": {"use_raptor": True}})
        payload = {"name": "test_parser_config_only_raptor_sdk", "parser_config": parser_config}
        dataset = client.create_dataset(**payload)

        config = dataset.parser_config
        assert config.raptor.use_raptor is True, "User-provided raptor.use_raptor should be preserved"
        assert hasattr(config, "graphrag"), "graphrag field should be present"
        assert config.graphrag.use_graphrag is False, "graphrag.use_graphrag should default to False"

    @pytest.mark.p1
    def test_parser_config_with_only_graphrag(self, client):
        parser_config = DataSet.ParserConfig(client, {"chunk_token_num": 1024, "graphrag": {"use_graphrag": True}})
        payload = {"name": "test_parser_config_only_graphrag_sdk", "parser_config": parser_config}
        dataset = client.create_dataset(**payload)

        config = dataset.parser_config
        assert hasattr(config, "raptor"), "raptor field should be present"
        assert config.raptor.use_raptor is False, "raptor.use_raptor should default to False"
        assert config.graphrag.use_graphrag is True, "User-provided graphrag.use_graphrag should be preserved"

    @pytest.mark.p1
    def test_parser_config_with_both_fields(self, client):
        parser_config = DataSet.ParserConfig(client, {"chunk_token_num": 1024, "raptor": {"use_raptor": True}, "graphrag": {"use_graphrag": True}})
        payload = {"name": "test_parser_config_both_fields_sdk", "parser_config": parser_config}
        dataset = client.create_dataset(**payload)

        config = dataset.parser_config
        assert config.raptor.use_raptor is True, "User-provided raptor.use_raptor should be preserved"
        assert config.graphrag.use_graphrag is True, "User-provided graphrag.use_graphrag should be preserved"

    @pytest.mark.p2
    @pytest.mark.parametrize("chunk_method", ["qa", "manual", "paper", "book", "laws", "presentation"])
    def test_parser_config_different_chunk_methods(self, client, chunk_method):
        parser_config = DataSet.ParserConfig(client, {"chunk_token_num": 512})
        payload = {"name": f"test_parser_config_{chunk_method}_sdk", "chunk_method": chunk_method, "parser_config": parser_config}
        dataset = client.create_dataset(**payload)

        config = dataset.parser_config
        assert hasattr(config, "raptor"), f"raptor field should be present for {chunk_method}"
        assert hasattr(config, "graphrag"), f"graphrag field should be present for {chunk_method}"
        assert config.raptor.use_raptor is False, f"raptor.use_raptor should default to False for {chunk_method}"
        assert config.graphrag.use_graphrag is False, f"graphrag.use_graphrag should default to False for {chunk_method}"
