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
from concurrent.futures import ThreadPoolExecutor, as_completed
from operator import attrgetter

import pytest
from configs import DATASET_NAME_LIMIT
from hypothesis import HealthCheck, example, given, settings
from ragflow_sdk import DataSet
from utils import encode_avatar
from utils.file_utils import create_image_file
from utils.hypothesis_utils import valid_names


class TestRquest:
    @pytest.mark.p2
    def test_payload_empty(self, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({})
        assert "No properties were modified" in str(excinfo.value), str(excinfo.value)


class TestCapability:
    @pytest.mark.p3
    def test_update_dateset_concurrent(self, add_dataset_func):
        dataset = add_dataset_func
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(dataset.update, {"name": f"dataset_{i}"}) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses


class TestDatasetUpdate:
    @pytest.mark.p1
    @given(name=valid_names())
    @example("a" * 128)
    @settings(max_examples=20, suppress_health_check=[HealthCheck.function_scoped_fixture])
    def test_name(self, client, add_dataset_func, name):
        dataset = add_dataset_func
        payload = {"name": name}
        dataset.update(payload)
        assert dataset.name == name, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.name == name, str(retrieved_dataset)

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
    def test_name_invalid(self, add_dataset_func, name, expected_message):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"name": name})
        assert expected_message in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    def test_name_duplicated(self, add_datasets_func):
        datasets = add_datasets_func
        name = "dataset_1"
        with pytest.raises(Exception) as excinfo:
            datasets[0].update({"name": name})
        assert f"Dataset name '{name}' already exists" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    def test_name_case_insensitive(self, add_datasets_func):
        dataset = add_datasets_func[0]
        name = "DATASET_1"
        with pytest.raises(Exception) as excinfo:
            dataset.update({"name": name})
        assert f"Dataset name '{name}' already exists" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p2
    def test_avatar(self, client, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        fn = create_image_file(tmp_path / "ragflow_test.png")
        avatar_data = f"data:image/png;base64,{encode_avatar(fn)}"
        dataset.update({"avatar": avatar_data})
        assert dataset.avatar == avatar_data, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.avatar == avatar_data, str(retrieved_dataset)

    @pytest.mark.p2
    def test_avatar_exceeds_limit_length(self, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"avatar": "a" * 65536})
        assert "String should have at most 65535 characters" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "avatar_prefix, expected_message",
        [
            ("", "Missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
            ("data:image/png;base64", "Missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
            ("invalid_mine_prefix:image/png;base64,", "Invalid MIME prefix format. Must start with 'data:'"),
            ("data:unsupported_mine_type;base64,", "Unsupported MIME type. Allowed: ['image/jpeg', 'image/png']"),
        ],
        ids=["empty_prefix", "missing_comma", "unsupported_mine_type", "invalid_mine_type"],
    )
    def test_avatar_invalid_prefix(self, add_dataset_func, tmp_path, avatar_prefix, expected_message):
        dataset = add_dataset_func
        fn = create_image_file(tmp_path / "ragflow_test.png")
        with pytest.raises(Exception) as excinfo:
            dataset.update({"avatar": f"{avatar_prefix}{encode_avatar(fn)}"})
        assert expected_message in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    def test_avatar_none(self, client, add_dataset_func):
        dataset = add_dataset_func
        dataset.update({"avatar": None})
        assert dataset.avatar is None, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.avatar is None, str(retrieved_dataset)

    @pytest.mark.p2
    def test_description(self, client, add_dataset_func):
        dataset = add_dataset_func
        dataset.update({"description": "description"})
        assert dataset.description == "description", str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.description == "description", str(retrieved_dataset)

    @pytest.mark.p2
    def test_description_exceeds_limit_length(self, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"description": "a" * 65536})
        assert "String should have at most 65535 characters" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    def test_description_none(self, client, add_dataset_func):
        dataset = add_dataset_func
        dataset.update({"description": None})
        assert dataset.description is None, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.description is None, str(retrieved_dataset)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "embedding_model",
        [
            "BAAI/bge-large-zh-v1.5@BAAI",
            "maidalun1020/bce-embedding-base_v1@Youdao",
            "embedding-3@ZHIPU-AI",
        ],
        ids=["builtin_baai", "builtin_youdao", "tenant_zhipu"],
    )
    def test_embedding_model(self, client, add_dataset_func, embedding_model):
        dataset = add_dataset_func
        dataset.update({"embedding_model": embedding_model})
        assert dataset.embedding_model == embedding_model, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.embedding_model == embedding_model, str(retrieved_dataset)

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
    def test_embedding_model_invalid(self, add_dataset_func, name, embedding_model):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"name": name, "embedding_model": embedding_model})
        error_msg = str(excinfo.value)
        if "tenant_no_auth" in name:
            assert error_msg == f"Unauthorized model: <{embedding_model}>", error_msg
        else:
            assert error_msg == f"Unsupported model: <{embedding_model}>", error_msg

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, embedding_model",
        [
            ("empty", ""),
            ("space", " "),
            ("missing_at", "BAAI/bge-large-zh-v1.5BAAI"),
            ("missing_model_name", "@BAAI"),
            ("missing_provider", "BAAI/bge-large-zh-v1.5@"),
            ("whitespace_only_model_name", " @BAAI"),
            ("whitespace_only_provider", "BAAI/bge-large-zh-v1.5@ "),
        ],
        ids=["empty", "space", "missing_at", "empty_model_name", "empty_provider", "whitespace_only_model_name", "whitespace_only_provider"],
    )
    def test_embedding_model_format(self, add_dataset_func, name, embedding_model):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"name": name, "embedding_model": embedding_model})
        error_msg = str(excinfo.value)
        if name in ["empty", "space", "missing_at"]:
            assert "Embedding model identifier must follow <model_name>@<provider> format" in error_msg, error_msg
        else:
            assert "Both model_name and provider must be non-empty strings" in error_msg, error_msg

    @pytest.mark.p2
    def test_embedding_model_none(self, client, add_dataset_func):
        dataset = add_dataset_func
        dataset.update({"embedding_model": None})
        assert dataset.embedding_model == "BAAI/bge-large-zh-v1.5@BAAI", str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.embedding_model == "BAAI/bge-large-zh-v1.5@BAAI", str(retrieved_dataset)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "permission",
        [
            "me",
            "team",
        ],
        ids=["me", "team"],
    )
    def test_permission(self, client, add_dataset_func, permission):
        dataset = add_dataset_func
        dataset.update({"permission": permission})
        assert dataset.permission == permission.lower().strip(), str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.permission == permission.lower().strip(), str(retrieved_dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "permission",
        [
            "",
            "unknown",
            list(),
            "ME",
            "TEAM",
            " ME ",
        ],
        ids=["empty", "unknown", "type_error", "me_upercase", "team_upercase", "whitespace"],
    )
    def test_permission_invalid(self, add_dataset_func, permission):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"permission": permission})
        assert "Input should be 'me' or 'team'" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    def test_permission_none(self, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"permission": None})
        assert "Input should be 'me' or 'team'" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "chunk_method",
        [
            "naive",
            "book",
            "email",
            "laws",
            "manual",
            "one",
            "paper",
            "picture",
            "presentation",
            "qa",
            "table",
            "tag",
        ],
        ids=["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table", "tag"],
    )
    def test_chunk_method(self, client, add_dataset_func, chunk_method):
        dataset = add_dataset_func
        dataset.update({"chunk_method": chunk_method})
        assert dataset.chunk_method == chunk_method, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.chunk_method == chunk_method, str(retrieved_dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "chunk_method",
        [
            "",
            "unknown",
            list(),
        ],
        ids=["empty", "unknown", "type_error"],
    )
    def test_chunk_method_invalid(self, add_dataset_func, chunk_method):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"chunk_method": chunk_method})
        assert "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'table' or 'tag'" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    def test_chunk_method_none(self, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"chunk_method": None})
        assert "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'table' or 'tag'" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="#8208")
    @pytest.mark.p2
    @pytest.mark.parametrize("pagerank", [0, 50, 100], ids=["min", "mid", "max"])
    def test_pagerank(self, client, add_dataset_func, pagerank):
        dataset = add_dataset_func
        dataset.update({"pagerank": pagerank})
        assert dataset.pagerank == pagerank, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.pagerank == pagerank, str(retrieved_dataset)

    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="#8208")
    @pytest.mark.p2
    def test_pagerank_set_to_0(self, client, add_dataset_func):
        dataset = add_dataset_func
        dataset.update({"pagerank": 50})
        assert dataset.pagerank == 50, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.pagerank == 50, str(retrieved_dataset)

        dataset.update({"pagerank": 0})
        assert dataset.pagerank == 0, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert retrieved_dataset.pagerank == 0, str(retrieved_dataset)

    @pytest.mark.skipif(os.getenv("DOC_ENGINE") != "infinity", reason="#8208")
    @pytest.mark.p2
    def test_pagerank_infinity(self, client, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"pagerank": 50})
        assert "'pagerank' can only be set when doc_engine is elasticsearch" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "pagerank, expected_message",
        [
            (-1, "Input should be greater than or equal to 0"),
            (101, "Input should be less than or equal to 100"),
        ],
        ids=["min_limit", "max_limit"],
    )
    def test_pagerank_invalid(self, add_dataset_func, pagerank, expected_message):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"pagerank": pagerank})
        assert expected_message in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p3
    def test_pagerank_none(self, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"pagerank": None})
        assert "Input should be a valid integer" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "parser_config",
        [
            {"auto_keywords": 0},
            {"auto_keywords": 16},
            {"auto_keywords": 32},
            {"auto_questions": 0},
            {"auto_questions": 5},
            {"auto_questions": 10},
            {"chunk_token_num": 1},
            {"chunk_token_num": 1024},
            {"chunk_token_num": 2048},
            {"delimiter": "\n"},
            {"delimiter": " "},
            {"html4excel": True},
            {"html4excel": False},
            {"layout_recognize": "DeepDOC"},
            {"layout_recognize": "Plain Text"},
            {"tag_kb_ids": ["1", "2"]},
            {"topn_tags": 1},
            {"topn_tags": 5},
            {"topn_tags": 10},
            {"filename_embd_weight": 0.1},
            {"filename_embd_weight": 0.5},
            {"filename_embd_weight": 1.0},
            {"task_page_size": 1},
            {"task_page_size": None},
            {"pages": [[1, 100]]},
            {"pages": None},
            {"graphrag": {"use_graphrag": True}},
            {"graphrag": {"use_graphrag": False}},
            {"graphrag": {"entity_types": ["age", "sex", "height", "weight"]}},
            {"graphrag": {"method": "general"}},
            {"graphrag": {"method": "light"}},
            {"graphrag": {"community": True}},
            {"graphrag": {"community": False}},
            {"graphrag": {"resolution": True}},
            {"graphrag": {"resolution": False}},
            {"raptor": {"use_raptor": True}},
            {"raptor": {"use_raptor": False}},
            {"raptor": {"prompt": "Who are you?"}},
            {"raptor": {"max_token": 1}},
            {"raptor": {"max_token": 1024}},
            {"raptor": {"max_token": 2048}},
            {"raptor": {"threshold": 0.0}},
            {"raptor": {"threshold": 0.5}},
            {"raptor": {"threshold": 1.0}},
            {"raptor": {"max_cluster": 1}},
            {"raptor": {"max_cluster": 512}},
            {"raptor": {"max_cluster": 1024}},
            {"raptor": {"random_seed": 0}},
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
    def test_parser_config(self, client, add_dataset_func, parser_config):
        dataset = add_dataset_func
        dataset.update({"parser_config": parser_config})
        for k, v in parser_config.items():
            if isinstance(v, dict):
                for kk, vv in v.items():
                    assert attrgetter(f"{k}.{kk}")(dataset.parser_config) == vv, str(dataset)
            else:
                assert attrgetter(k)(dataset.parser_config) == v, str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        for k, v in parser_config.items():
            if isinstance(v, dict):
                for kk, vv in v.items():
                    assert attrgetter(f"{k}.{kk}")(retrieved_dataset.parser_config) == vv, str(retrieved_dataset)
            else:
                assert attrgetter(k)(retrieved_dataset.parser_config) == v, str(retrieved_dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "parser_config, expected_message",
        [
            ({"auto_keywords": -1}, "Input should be greater than or equal to 0"),
            ({"auto_keywords": 33}, "Input should be less than or equal to 32"),
            ({"auto_keywords": 3.14}, "Input should be a valid integer"),
            ({"auto_keywords": "string"}, "Input should be a valid integer"),
            ({"auto_questions": -1}, "Input should be greater than or equal to 0"),
            ({"auto_questions": 11}, "Input should be less than or equal to 10"),
            ({"auto_questions": 3.14}, "Input should be a valid integer"),
            ({"auto_questions": "string"}, "Input should be a valid integer"),
            ({"chunk_token_num": 0}, "Input should be greater than or equal to 1"),
            ({"chunk_token_num": 2049}, "Input should be less than or equal to 2048"),
            ({"chunk_token_num": 3.14}, "Input should be a valid integer"),
            ({"chunk_token_num": "string"}, "Input should be a valid integer"),
            ({"delimiter": ""}, "String should have at least 1 character"),
            ({"html4excel": "string"}, "Input should be a valid boolean"),
            ({"tag_kb_ids": "1,2"}, "Input should be a valid list"),
            ({"tag_kb_ids": [1, 2]}, "Input should be a valid string"),
            ({"topn_tags": 0}, "Input should be greater than or equal to 1"),
            ({"topn_tags": 11}, "Input should be less than or equal to 10"),
            ({"topn_tags": 3.14}, "Input should be a valid integer"),
            ({"topn_tags": "string"}, "Input should be a valid integer"),
            ({"filename_embd_weight": -1}, "Input should be greater than or equal to 0"),
            ({"filename_embd_weight": 1.1}, "Input should be less than or equal to 1"),
            ({"filename_embd_weight": "string"}, "Input should be a valid number"),
            ({"task_page_size": 0}, "Input should be greater than or equal to 1"),
            ({"task_page_size": 3.14}, "Input should be a valid integer"),
            ({"task_page_size": "string"}, "Input should be a valid integer"),
            ({"pages": "1,2"}, "Input should be a valid list"),
            ({"pages": ["1,2"]}, "Input should be a valid list"),
            ({"pages": [["string1", "string2"]]}, "Input should be a valid integer"),
            ({"graphrag": {"use_graphrag": "string"}}, "Input should be a valid boolean"),
            ({"graphrag": {"entity_types": "1,2"}}, "Input should be a valid list"),
            ({"graphrag": {"entity_types": [1, 2]}}, "nput should be a valid string"),
            ({"graphrag": {"method": "unknown"}}, "Input should be 'light' or 'general'"),
            ({"graphrag": {"method": None}}, "Input should be 'light' or 'general'"),
            ({"graphrag": {"community": "string"}}, "Input should be a valid boolean"),
            ({"graphrag": {"resolution": "string"}}, "Input should be a valid boolean"),
            ({"raptor": {"use_raptor": "string"}}, "Input should be a valid boolean"),
            ({"raptor": {"prompt": ""}}, "String should have at least 1 character"),
            ({"raptor": {"prompt": " "}}, "String should have at least 1 character"),
            ({"raptor": {"max_token": 0}}, "Input should be greater than or equal to 1"),
            ({"raptor": {"max_token": 2049}}, "Input should be less than or equal to 2048"),
            ({"raptor": {"max_token": 3.14}}, "Input should be a valid integer"),
            ({"raptor": {"max_token": "string"}}, "Input should be a valid integer"),
            ({"raptor": {"threshold": -0.1}}, "Input should be greater than or equal to 0"),
            ({"raptor": {"threshold": 1.1}}, "Input should be less than or equal to 1"),
            ({"raptor": {"threshold": "string"}}, "Input should be a valid number"),
            ({"raptor": {"max_cluster": 0}}, "Input should be greater than or equal to 1"),
            ({"raptor": {"max_cluster": 1025}}, "Input should be less than or equal to 1024"),
            ({"raptor": {"max_cluster": 3.14}}, "Input should be a valid integer"),
            ({"raptor": {"max_cluster": "string"}}, "Input should be a valid integer"),
            ({"raptor": {"random_seed": -1}}, "Input should be greater than or equal to 0"),
            ({"raptor": {"random_seed": 3.14}}, "Input should be a valid integer"),
            ({"raptor": {"random_seed": "string"}}, "Input should be a valid integer"),
            ({"delimiter": "a" * 65536}, "Parser config exceeds size limit (max 65,535 characters)"),
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
    def test_parser_config_invalid(self, add_dataset_func, parser_config, expected_message):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update({"parser_config": parser_config})
        assert expected_message in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p2
    def test_parser_config_empty(self, client, add_dataset_func):
        dataset = add_dataset_func
        expected_config = DataSet.ParserConfig(
            client,
            {
                "chunk_token_num": 512,
                "delimiter": r"\n",
                "html4excel": False,
                "layout_recognize": "DeepDOC",
                "raptor": {"use_raptor": False},
                "graphrag": {"use_graphrag": False},
            },
        )
        dataset.update({"parser_config": {}})
        assert str(dataset.parser_config) == str(expected_config), str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert str(retrieved_dataset.parser_config) == str(expected_config), str(retrieved_dataset)

    @pytest.mark.p3
    def test_parser_config_none(self, client, add_dataset_func):
        dataset = add_dataset_func
        expected_config = DataSet.ParserConfig(
            client,
            {
                "chunk_token_num": 512,
                "delimiter": r"\n",
                "html4excel": False,
                "layout_recognize": "DeepDOC",
                "raptor": {"use_raptor": False},
                "graphrag": {"use_graphrag": False},
            },
        )
        dataset.update({"parser_config": None})
        assert str(dataset.parser_config) == str(expected_config), str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert str(retrieved_dataset.parser_config) == str(expected_config), str(retrieved_dataset)

    @pytest.mark.p3
    def test_parser_config_empty_with_chunk_method_change(self, client, add_dataset_func):
        dataset = add_dataset_func
        expected_config = DataSet.ParserConfig(
            client,
            {
                "raptor": {"use_raptor": False},
                "graphrag": {"use_graphrag": False},
            },
        )
        dataset.update({"chunk_method": "qa", "parser_config": {}})
        assert str(dataset.parser_config) == str(expected_config), str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert str(retrieved_dataset.parser_config) == str(expected_config), str(retrieved_dataset)

    @pytest.mark.p3
    def test_parser_config_unset_with_chunk_method_change(self, client, add_dataset_func):
        dataset = add_dataset_func
        expected_config = DataSet.ParserConfig(
            client,
            {
                "raptor": {"use_raptor": False},
                "graphrag": {"use_graphrag": False},
            },
        )
        dataset.update({"chunk_method": "qa"})
        assert str(dataset.parser_config) == str(expected_config), str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert str(retrieved_dataset.parser_config) == str(expected_config), str(retrieved_dataset)

    @pytest.mark.p3
    def test_parser_config_none_with_chunk_method_change(self, client, add_dataset_func):
        dataset = add_dataset_func
        expected_config = DataSet.ParserConfig(
            client,
            {
                "raptor": {"use_raptor": False},
                "graphrag": {"use_graphrag": False},
            },
        )
        dataset.update({"chunk_method": "qa", "parser_config": None})
        assert str(dataset.parser_config) == str(expected_config), str(dataset)

        retrieved_dataset = client.get_dataset(name=dataset.name)
        assert str(retrieved_dataset.parser_config) == str(expected_config), str(retrieved_dataset)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload",
        [
            {"id": "id"},
            {"tenant_id": "e57c1966f99211efb41e9e45646e0111"},
            {"created_by": "created_by"},
            {"create_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
            {"create_time": 1741671443322},
            {"update_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
            {"update_time": 1741671443339},
            {"document_count": 1},
            {"chunk_count": 1},
            {"token_num": 1},
            {"status": "1"},
            {"unknown_field": "unknown_field"},
        ],
    )
    def test_field_unsupported(self, add_dataset_func, payload):
        dataset = add_dataset_func
        with pytest.raises(Exception) as excinfo:
            dataset.update(payload)
        assert "Extra inputs are not permitted" in str(excinfo.value), str(excinfo.value)

    @pytest.mark.p2
    def test_field_unset(self, client, add_dataset_func):
        dataset = add_dataset_func
        original_dataset = client.get_dataset(name=dataset.name)

        dataset.update({"name": "default_unset"})

        updated_dataset = client.get_dataset(name="default_unset")
        assert updated_dataset.avatar == original_dataset.avatar, str(updated_dataset)
        assert updated_dataset.description == original_dataset.description, str(updated_dataset)
        assert updated_dataset.embedding_model == original_dataset.embedding_model, str(updated_dataset)
        assert updated_dataset.permission == original_dataset.permission, str(updated_dataset)
        assert updated_dataset.chunk_method == original_dataset.chunk_method, str(updated_dataset)
        assert updated_dataset.pagerank == original_dataset.pagerank, str(updated_dataset)
        assert str(updated_dataset.parser_config) == str(original_dataset.parser_config), str(updated_dataset)
