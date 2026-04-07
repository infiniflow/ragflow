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
from common import list_documents, update_document
from configs import DOCUMENT_NAME_LIMIT, INVALID_API_TOKEN, INVALID_ID_32
from libs.auth import RAGFlowHttpApiAuth
from configs import DEFAULT_PARSER_CONFIG

@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                401,
                "<Unauthorized '401: Unauthorized'>",
            ),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = update_document(invalid_auth, "dataset_id", "document_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsUpdated:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, expected_code, expected_message",
        [
            ("new_name.txt", 0, ""),
            (
                f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt",
                0,
                "",
            ),
            (
                0,
                100,
                """AttributeError(\'int\' object has no attribute \'encode\')""",
            ),
            (
                None,
                100,
                """AttributeError(\'NoneType\' object has no attribute \'encode\')""",
            ),
            (
                "",
                101,
                "The extension of file can't be changed",
            ),
            (
                "ragflow_test_upload_0",
                101,
                "The extension of file can't be changed",
            ),
            (
                "ragflow_test_upload_1.txt",
                102,
                "Duplicated document name in the same dataset.",
            ),
            (
                "RAGFLOW_TEST_UPLOAD_1.TXT",
                0,
                "",
            ),
        ],
    )
    def test_name(self, HttpApiAuth, add_documents, name, expected_code, expected_message):
        dataset_id, document_ids = add_documents
        res = update_document(HttpApiAuth, dataset_id, document_ids[0], {"name": name})
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_documents(HttpApiAuth, dataset_id, {"id": document_ids[0]})
            assert res["data"]["docs"][0]["name"] == name
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            (
                INVALID_ID_32,
                102,
                "The dataset doesn't own the document.",
            ),
        ],
    )
    def test_invalid_document_id(self, HttpApiAuth, add_documents, document_id, expected_code, expected_message):
        dataset_id, _ = add_documents
        res = update_document(HttpApiAuth, dataset_id, document_id, {"name": "new_name.txt"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            (
                INVALID_ID_32,
                102,
                "You don't own the dataset.",
            ),
        ],
    )
    def test_invalid_dataset_id(self, HttpApiAuth, add_documents, dataset_id, expected_code, expected_message):
        _, document_ids = add_documents
        res = update_document(HttpApiAuth, dataset_id, document_ids[0], {"name": "new_name.txt"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "meta_fields, expected_code, expected_message",
        [({"test": "test"}, 0, ""), ("test", 102, "Field: <meta_fields> - Message: <Input should be a valid dictionary> - Value: <test>")],
    )
    def test_meta_fields(self, HttpApiAuth, add_documents, meta_fields, expected_code, expected_message):
        dataset_id, document_ids = add_documents
        res = update_document(HttpApiAuth, dataset_id, document_ids[0], {"meta_fields": meta_fields})
        if expected_code == 0:
            res = list_documents(HttpApiAuth, dataset_id, {"id": document_ids[0]})
            assert res["data"]["docs"][0]["meta_fields"] == meta_fields
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
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
            ("", 102, "`chunk_method` (empty string) is not valid"),
            (
                "other_chunk_method",
                102,
                "Field: <chunk_method> - Message: <`chunk_method` other_chunk_method doesn't exist> - Value: <other_chunk_method>",
            ),
        ],
    )
    def test_chunk_method(self, HttpApiAuth, add_documents, chunk_method, expected_code, expected_message):
        dataset_id, document_ids = add_documents
        res = update_document(HttpApiAuth, dataset_id, document_ids[0], {"chunk_method": chunk_method})
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_documents(HttpApiAuth, dataset_id, {"id": document_ids[0]})
            if chunk_method == "":
                assert res["data"]["docs"][0]["chunk_method"] == "naive"
            else:
                assert res["data"]["docs"][0]["chunk_method"] == chunk_method
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"chunk_count": 1}, 102, "Can't change `chunk_count`."),
            pytest.param(
                {"create_date": "Fri, 14 Mar 2025 16:53:42 GMT"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"create_time": 1},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"created_by": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"dataset_id": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"id": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"location": "ragflow_test.txt"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"process_begin_at": 1},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"process_duration": 1.0},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param({"progress": 1.0}, 102, "Can't change `progress`."),
            pytest.param(
                {"progress_msg": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"run": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"size": 1},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"source_type": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"thumbnail": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            ({"token_count": 1}, 102, "Can't change `token_count`."),
            pytest.param(
                {"type": "ragflow_test"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"update_date": "Fri, 14 Mar 2025 16:33:17 GMT"},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"update_time": 1},
                102,
                "The input parameters are invalid.",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
        ],
    )
    def test_invalid_field(
        self,
        HttpApiAuth,
        add_documents,
        payload,
        expected_code,
        expected_message,
    ):
        dataset_id, document_ids = add_documents
        res = update_document(HttpApiAuth, dataset_id, document_ids[0], payload)
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"chunk_count": 100}, 102, "Can't change `chunk_count`."),
            ({"token_count": 100}, 102, "Can't change `token_count`."),
            ({"progress": 2.0}, 102, "Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <2.0>"),
            ({"progress": 1.0}, 102, "Can't change `progress`."),
            ({"meta_fields": []}, 102, "Field: <meta_fields> - Message: <Input should be a valid dictionary> - Value: <[]>"),
        ],
    )
    def test_update_doc_guards_and_error_paths(self, HttpApiAuth, add_documents, payload, expected_code, expected_message):
        """
        Test various guard conditions and error paths for document update functionality.
        This includes testing for invalid dataset ownership, document ownership,
        immutable fields, and validation errors.
        """
        dataset_id, document_ids = add_documents
        document_id = document_ids[0]

        res = update_document(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code
        if expected_message:
            assert expected_message in res["message"] or res["message"] == expected_message


DEFAULT_PARSER_CONFIG_FOR_TEST = {
    "layout_recognize": "DeepDOC",
    "chunk_token_num": 512,
    "delimiter": "\n",
    "auto_keywords": 0,
    "auto_questions": 0,
    "html4excel": False,
    "topn_tags": 3,
    "raptor": {
        "use_raptor": True,
        "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
        "max_token": 256,
        "threshold": 0.1,
        "max_cluster": 64,
        "random_seed": 0,
    },
    "graphrag": {
        "use_graphrag": True,
        "entity_types": [
            "organization",
            "person",
            "geo",
            "event",
            "category",
        ],
        "method": "light",
    },
}

class TestUpdateDocumentParserConfig:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "chunk_method, parser_config, expected_code, expected_message",
        [
            ("naive", {}, 0, ""),
            (
                "naive",
                DEFAULT_PARSER_CONFIG_FOR_TEST,
                0,
                "",
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": -1},
                102,
                "Field: <parser_config.chunk_token_num> - Message: <Input should be greater than or equal to 1> - Value: <-1>",
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": 0},
                102,
                "Field: <parser_config.chunk_token_num> - Message: <Input should be greater than or equal to 1> - Value: <0>",
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": 100000000},
                102,
                "Field: <parser_config.chunk_token_num> - Message: <Input should be less than or equal to 2048> - Value: <100000000>",
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": 3.14},
                102,
                "Field: <parser_config.chunk_token_num> - Message: <Input should be a valid integer> - Value: <3.14>",
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": "1024"},
                102,
                "Field: <parser_config.chunk_token_num> - Message: <Input should be a valid integer> - Value: <1024>",
            ),
            (
                "naive",
                {"layout_recognize": "DeepDOC"},
                0,
                "",
            ),
            (
                "naive",
                {"layout_recognize": "Naive"},
                0,
                "",
            ),
            ("naive", {"html4excel": True}, 0, ""),
            ("naive", {"html4excel": False}, 0, ""),
            pytest.param(
                "naive",
                {"html4excel": 1},
                102,
                "Field: <parser_config.html4excel> - Message: <Input should be a valid boolean> - Value: <1>",
            ),
            ("naive", {"delimiter": ""}, 102, "Field: <parser_config.delimiter> - Message: <String should have at least 1 character> - Value: <>"),
            ("naive", {"delimiter": "`##`"}, 0, ""),
            pytest.param(
                "naive",
                {"delimiter": 1},
                102,
                "Field: <parser_config.delimiter> - Message: <Input should be a valid string> - Value: <1>",
            ),
            pytest.param(
                "naive",
                {"task_page_size": -1},
                102,
                "Field: <parser_config.task_page_size> - Message: <Input should be greater than or equal to 1> - Value: <-1>",
            ),
            pytest.param(
                "naive",
                {"task_page_size": 0},
                102,
                "Field: <parser_config.task_page_size> - Message: <Input should be greater than or equal to 1> - Value: <0>",
            ),
            pytest.param(
                "naive",
                {"task_page_size": 100000000},
                0,
                "",
            ),
            pytest.param(
                "naive",
                {"task_page_size": 3.14},
                102,
                "Field: <parser_config.task_page_size> - Message: <Input should be a valid integer> - Value: <3.14>",
            ),
            pytest.param(
                "naive",
                {"task_page_size": "1024"},
                102,
                "Field: <parser_config.task_page_size> - Message: <Input should be a valid integer> - Value: <1024>",
            ),
            ("naive", {"raptor": {"use_raptor": {
                "a": "b"
            },}}, 102, "Field: <parser_config.raptor.use_raptor> - Message: <Input should be a valid boolean> - Value: <{'a': 'b'}>"),
            ("naive", {"raptor": {"use_raptor": False}}, 0, ""),
            pytest.param(
                "naive",
                {"invalid_key": "invalid_value"},
                102,
                "Field: <parser_config.invalid_key> - Message: <Extra inputs are not permitted> - Value: <invalid_value>",
            ),
            pytest.param(
                "naive",
                {"auto_keywords": -1},
                102,
                "Field: <parser_config.auto_keywords> - Message: <Input should be greater than or equal to 0> - Value: <-1>",
            ),
            pytest.param(
                "naive",
                {"auto_keywords": 32},
                0,
                "",
            ),
            pytest.param(
                "naive",
                {"auto_keywords": "1024"},
                102,
                "Field: <parser_config.auto_keywords> - Message: <Input should be a valid integer> - Value: <1024>",
            ),
            pytest.param(
                "naive",
                {"auto_keywords": 3.14},
                102,
                "Field: <parser_config.auto_keywords> - Message: <Input should be a valid integer> - Value: <3.14>",
            ),
            pytest.param(
                "naive",
                {"auto_questions": -1},
                102,
                "Field: <parser_config.auto_questions> - Message: <Input should be greater than or equal to 0> - Value: <-1>",
            ),
            pytest.param(
                "naive",
                {"auto_questions": 10},
                0,
                "",
            ),
            pytest.param(
                "naive",
                {"auto_questions": 3.14},
                102,
                "Field: <parser_config.auto_questions> - Message: <Input should be a valid integer> - Value: <3.14>",
            ),
            pytest.param(
                "naive",
                {"auto_questions": "1024"},
                102,
                "Field: <parser_config.auto_questions> - Message: <Input should be a valid integer> - Value: <1024>",
            ),
            pytest.param(
                "naive",
                {"topn_tags": -1},
                102,
                "Field: <parser_config.topn_tags> - Message: <Input should be greater than or equal to 1> - Value: <-1>",
            ),
            pytest.param(
                "naive",
                {"topn_tags": 10},
                0,
                "",
            ),
            pytest.param(
                "naive",
                {"topn_tags": 3.14},
                102,
                "Field: <parser_config.topn_tags> - Message: <Input should be a valid integer> - Value: <3.14>",
            ),
            pytest.param(
                "naive",
                {"topn_tags": "1024"},
                102,
                "Field: <parser_config.topn_tags> - Message: <Input should be a valid integer> - Value: <1024>",
            ),
        ],
    )
    def test_parser_config(
        self,
        HttpApiAuth,
        add_documents,
        chunk_method,
        parser_config,
        expected_code,
        expected_message,
    ):
        dataset_id, document_ids = add_documents
        res = update_document(
            HttpApiAuth,
            dataset_id,
            document_ids[0],
            {"chunk_method": chunk_method, "parser_config": parser_config},
        )
        assert res["code"] == expected_code
        if expected_code == 0:
            res = list_documents(HttpApiAuth, dataset_id, {"id": document_ids[0]})
            if parser_config == {}:
                assert res["data"]["docs"][0]["parser_config"] == DEFAULT_PARSER_CONFIG
            else:
                for k, v in parser_config.items():
                    assert res["data"]["docs"][0]["parser_config"][k] == v
        if expected_code != 0 or expected_message:
            assert res["message"] == expected_message
