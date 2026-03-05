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

import pytest
from common import update_kb
from configs import DATASET_NAME_LIMIT, INVALID_API_TOKEN
from hypothesis import HealthCheck, example, given, settings
from libs.auth import RAGFlowWebApiAuth
from utils import encode_avatar
from utils.file_utils import create_image_file
from utils.hypothesis_utils import valid_names


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
        ids=["empty_auth", "invalid_api_token"],
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = update_kb(invalid_auth, "dataset_id")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestCapability:
    @pytest.mark.p3
    def test_update_dateset_concurrent(self, WebApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    update_kb,
                    WebApiAuth,
                    {
                        "kb_id": dataset_id,
                        "name": f"dataset_{i}",
                        "description": "",
                        "parser_id": "naive",
                    },
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)


class TestDatasetUpdate:
    @pytest.mark.p3
    def test_dataset_id_not_uuid(self, WebApiAuth):
        payload = {"name": "not uuid", "description": "", "parser_id": "naive", "kb_id": "not_uuid"}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 109, res
        assert "No authorization." in res["message"], res

    @pytest.mark.p1
    @given(name=valid_names())
    @example("a" * 128)
    # Network-bound API call; disable Hypothesis deadline to avoid flaky timeouts.
    @settings(max_examples=20, suppress_health_check=[HealthCheck.function_scoped_fixture], deadline=None)
    def test_name(self, WebApiAuth, add_dataset_func, name):
        dataset_id = add_dataset_func
        payload = {"name": name, "description": "", "parser_id": "naive", "kb_id": dataset_id}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == name, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("", "Dataset name can't be empty."),
            (" ", "Dataset name can't be empty."),
            ("a" * (DATASET_NAME_LIMIT + 1), "Dataset name length is 129 which is large than 128"),
            (0, "Dataset name must be string."),
            (None, "Dataset name must be string."),
        ],
        ids=["empty_name", "space_name", "too_long_name", "invalid_name", "None_name"],
    )
    def test_name_invalid(self, WebApiAuth, add_dataset_func, name, expected_message):
        kb_id = add_dataset_func
        payload = {"name": name, "description": "", "parser_id": "naive", "kb_id": kb_id}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert expected_message in res["message"], res

    @pytest.mark.p3
    def test_name_duplicated(self, WebApiAuth, add_datasets_func):
        kb_id = add_datasets_func[0]
        name = "kb_1"
        payload = {"name": name, "description": "", "parser_id": "naive", "kb_id": kb_id}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert res["message"] == "Duplicated dataset name.", res

    @pytest.mark.p3
    def test_name_case_insensitive(self, WebApiAuth, add_datasets_func):
        kb_id = add_datasets_func[0]
        name = "KB_1"
        payload = {"name": name, "description": "", "parser_id": "naive", "kb_id": kb_id}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert res["message"] == "Duplicated dataset name.", res

    @pytest.mark.p2
    def test_avatar(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fn = create_image_file(tmp_path / "ragflow_test.png")
        payload = {
            "name": "avatar",
            "description": "",
            "parser_id": "naive",
            "kb_id": kb_id,
            "avatar": f"data:image/png;base64,{encode_avatar(fn)}",
        }
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["avatar"] == f"data:image/png;base64,{encode_avatar(fn)}", res

    @pytest.mark.p2
    def test_description(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        payload = {"name": "description", "description": "description", "parser_id": "naive", "kb_id": kb_id}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["description"] == "description", res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "embedding_model",
        [
            "BAAI/bge-small-en-v1.5@Builtin",
            "embedding-3@ZHIPU-AI",
        ],
        ids=["builtin_baai", "tenant_zhipu"],
    )
    def test_embedding_model(self, WebApiAuth, add_dataset_func, embedding_model):
        kb_id = add_dataset_func
        payload = {"name": "embedding_model", "description": "", "parser_id": "naive", "kb_id": kb_id, "embd_id": embedding_model}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["embd_id"] == embedding_model, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "permission",
        [
            "me",
            "team",
        ],
        ids=["me", "team"],
    )
    def test_permission(self, WebApiAuth, add_dataset_func, permission):
        kb_id = add_dataset_func
        payload = {"name": "permission", "description": "", "parser_id": "naive", "kb_id": kb_id, "permission": permission}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["permission"] == permission.lower().strip(), res

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
            pytest.param("tag", marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="Infinity does not support parser_id=tag")),
        ],
        ids=["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table", "tag"],
    )
    def test_chunk_method(self, WebApiAuth, add_dataset_func, chunk_method):
        kb_id = add_dataset_func
        payload = {"name": "chunk_method", "description": "", "parser_id": chunk_method, "kb_id": kb_id}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["parser_id"] == chunk_method, res

    @pytest.mark.p1
    @pytest.mark.skipif(os.getenv("DOC_ENGINE") != "infinity", reason="Infinity does not support parser_id=tag")
    def test_chunk_method_tag_with_infinity(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        payload = {"name": "chunk_method", "description": "", "parser_id": "tag", "kb_id": kb_id}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 103, res
        assert res["message"] == "The chunking method Tag has not been supported by Infinity yet.", res

    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="#8208")
    @pytest.mark.p2
    @pytest.mark.parametrize("pagerank", [0, 50, 100], ids=["min", "mid", "max"])
    def test_pagerank(self, WebApiAuth, add_dataset_func, pagerank):
        kb_id = add_dataset_func
        payload = {"name": "pagerank", "description": "", "parser_id": "naive", "kb_id": kb_id, "pagerank": pagerank}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["pagerank"] == pagerank, res

    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="#8208")
    @pytest.mark.p2
    def test_pagerank_set_to_0(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        payload = {"name": "pagerank", "description": "", "parser_id": "naive", "kb_id": kb_id, "pagerank": 50}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["pagerank"] == 50, res

        payload = {"name": "pagerank", "description": "", "parser_id": "naive", "kb_id": kb_id, "pagerank": 0}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["pagerank"] == 0, res

    @pytest.mark.skipif(os.getenv("DOC_ENGINE") != "infinity", reason="#8208")
    @pytest.mark.p2
    def test_pagerank_infinity(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        payload = {"name": "pagerank", "description": "", "parser_id": "naive", "kb_id": kb_id, "pagerank": 50}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert res["message"] == "'pagerank' can only be set when doc_engine is elasticsearch", res

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
    def test_parser_config(self, WebApiAuth, add_dataset_func, parser_config):
        kb_id = add_dataset_func
        payload = {"name": "parser_config", "description": "", "parser_id": "naive", "kb_id": kb_id, "parser_config": parser_config}
        res = update_kb(WebApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["parser_config"] == parser_config, res

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
        ],
    )
    def test_field_unsupported(self, WebApiAuth, add_dataset_func, payload):
        kb_id = add_dataset_func
        full_payload = {"name": "field_unsupported", "description": "", "parser_id": "naive", "kb_id": kb_id, **payload}
        res = update_kb(WebApiAuth, full_payload)
        assert res["code"] == 101, res
        assert "isn't allowed" in res["message"], res
