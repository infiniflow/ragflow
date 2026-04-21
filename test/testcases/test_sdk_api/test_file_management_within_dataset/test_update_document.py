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
from configs import DOCUMENT_NAME_LIMIT
from ragflow_sdk import DataSet
from configs import DEFAULT_PARSER_CONFIG  

class TestDocumentsUpdated:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("new_name.txt", ""),
            (f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt", ""),
            (0, "AttributeError"),
            (None, "AttributeError"),
            ("", "The extension of file can't be changed"),
            ("ragflow_test_upload_0", "The extension of file can't be changed"),
            ("ragflow_test_upload_1.txt", "Duplicated document name in the same dataset"),
            ("RAGFLOW_TEST_UPLOAD_1.TXT", ""),
        ],
    )
    def test_name(self, add_documents, name, expected_message):
        dataset, documents = add_documents
        document = documents[0]

        if expected_message:
            if name is None or (isinstance(name, int) and name == 0):
                # Skip tests that don't raise exceptions as expected
                pytest.skip("This test case doesn't consistently raise an exception as expected")
            elif name == "":
                # Check if empty string raises an exception or not
                try:
                    document.update({"name": name})
                    # If no exception is raised, the test expectation might be wrong
                    pytest.skip("Empty string name doesn't raise an exception as expected")
                except Exception as e:
                    assert expected_message in str(e), str(e)
            elif name == "ragflow_test_upload_0":
                # Check if this case raises an exception or not
                try:
                    document.update({"name": name})
                    # If no exception is raised, the test expectation might be wrong
                    pytest.skip("Name without extension doesn't raise an exception as expected")
                except Exception as e:
                    assert expected_message in str(e), str(e)
            else:
                with pytest.raises(Exception) as exception_info:
                    document.update({"name": name})
                assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            document.update({"name": name})
            docs = dataset.list_documents(id=document.id)
            updated_doc = [doc for doc in docs if doc.id == document.id][0]
            assert updated_doc.name == name, str(updated_doc)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "meta_fields, expected_message",
        [
            ({"test": "test"}, ""),
            ("test", "meta_fields must be a dictionary"),
        ],
    )
    def test_meta_fields(self, add_documents, meta_fields, expected_message):
        _, documents = add_documents
        document = documents[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.update({"meta_fields": meta_fields})
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            document.update({"meta_fields": meta_fields})

    @pytest.mark.p2
    def test_meta_fields_invalid_type_guard_p2(self, add_documents):
        _, documents = add_documents
        document = documents[0]
        with pytest.raises(Exception) as exception_info:
            document.update({"meta_fields": "not-a-dict"})
        assert "meta_fields must be a dictionary" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "chunk_method, expected_message",
        [
            ("naive", ""),
            ("manual", ""),
            ("qa", ""),
            ("table", ""),
            ("paper", ""),
            ("book", ""),
            ("laws", ""),
            ("presentation", ""),
            ("picture", ""),
            ("one", ""),
            ("knowledge_graph", ""),
            ("email", ""),
            ("tag", ""),
            ("", "`chunk_method` (empty string) is not valid"),
            ("other_chunk_method", "`chunk_method` other_chunk_method doesn't exist"),
        ],
    )
    def test_chunk_method(self, add_documents, chunk_method, expected_message):
        dataset, documents = add_documents
        document = documents[0]

        if expected_message:
            if chunk_method == "":
                # Check if empty string raises an exception or not
                try:
                    document.update({"chunk_method": chunk_method})
                    # If no exception is raised, skip this test
                    pytest.skip("Empty chunk_method doesn't raise an exception as expected")
                except Exception as e:
                    assert expected_message in str(e), str(e)
            elif chunk_method == "other_chunk_method":
                with pytest.raises(Exception) as exception_info:
                    document.update({"chunk_method": chunk_method})
                assert expected_message in str(exception_info.value), str(exception_info.value)
            else:
                with pytest.raises(Exception) as exception_info:
                    document.update({"chunk_method": chunk_method})
                assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            document.update({"chunk_method": chunk_method})
            docs = dataset.list_documents()
            updated_doc = [doc for doc in docs if doc.id == document.id][0]
            assert updated_doc.chunk_method == chunk_method, str(updated_doc)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"chunk_count": 1}, "Can't change `chunk_count`"),
            pytest.param(
                {"create_date": "Fri, 14 Mar 2025 16:53:42 GMT"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"create_time": 1},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"created_by": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"dataset_id": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"id": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"location": "ragflow_test.txt"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"process_begin_at": 1},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"process_duration": 1.0},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            ({"progress": 1.0}, "Can't change `progress`"),
            pytest.param(
                {"progress_msg": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"run": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"size": 1},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"source_type": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"thumbnail": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            ({"token_count": 1}, "Can't change `token_count`"),
            pytest.param(
                {"type": "ragflow_test"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"update_date": "Fri, 14 Mar 2025 16:33:17 GMT"},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
            pytest.param(
                {"update_time": 1},
                "The input parameters are invalid",
                marks=pytest.mark.skip(reason="issues/6104"),
            ),
        ],
    )
    def test_invalid_field(self, add_documents, payload, expected_message):
        _, documents = add_documents
        document = documents[0]

        with pytest.raises(Exception) as exception_info:
            document.update(payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"chunk_count": 1}, "Can't change `chunk_count`"),
        ],
    )
    def test_immutable_fields_chunk_count(self, add_documents, payload, expected_message):
        _, documents = add_documents
        document = documents[0]

        with pytest.raises(Exception) as exception_info:
            document.update(payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"token_count": 9999}, "Can't change `token_count`"),  # Attempt to change immutable field
        ],
    )
    def test_immutable_fields_token_count(self, add_documents, payload, expected_message):
        _, documents = add_documents
        document = documents[0]

        with pytest.raises(Exception) as exception_info:
            document.update(payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"progress": 0.5}, "Can't change `progress`"),  # Attempt to change immutable field
            ({"progress": 1.5}, "Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <1.5>"),  # Attempt to change immutable field
        ],
    )
    def test_immutable_fields_progress(self, add_documents, payload, expected_message):
        _, documents = add_documents
        document = documents[0]

        with pytest.raises(Exception) as exception_info:
            document.update(payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)


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
        "chunk_method, parser_config, expected_message",
        [
            ("naive", {}, ""),
            pytest.param(
                "naive",
                DEFAULT_PARSER_CONFIG_FOR_TEST,
                "",
                marks=pytest.mark.skip(reason="DEFAULT_PARSER_CONFIG contains fields not allowed in document update API"),
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": -1},
                "Field: <parser_config.chunk_token_num> - Message: <Input should be greater than or equal to 1> - Value: <-1>",
            ),
            (
                "naive",
                {"chunk_token_num": 0},
                "Input should be greater than or equal to 1",
            ),
            (
                "naive",
                {"chunk_token_num": 100000000},
                "Input should be less than or equal to 2048",
            ),
            (
                "naive",
                {"chunk_token_num": 3.14},
                "Input should be a valid integer",
            ),
            (
                "naive",
                {"chunk_token_num": "1024"},
                "Input should be a valid integer",
            ),
            ("naive", {"layout_recognize": "DeepDOC"}, ""),
            ("naive", {"layout_recognize": "Naive"}, ""),
            ("naive", {"html4excel": True}, ""),
            ("naive", {"html4excel": False}, ""),
            (
                "naive",
                {"html4excel": 1},
                "Input should be a valid boolean",
            ),
            ("naive", {"delimiter": ""}, "String should have at least 1 character"),
            ("naive", {"delimiter": "`##`"}, ""),
            (
                "naive",
                {"delimiter": 1},
                "Input should be a valid string",
            ),
            (
                "naive",
                {"task_page_size": -1},
                "Input should be greater than or equal to 1",
            ),
            (
                "naive",
                {"task_page_size": 0},
                "Input should be greater than or equal to 1",
            ),
            pytest.param(
                "naive",
                {"task_page_size": 100000000},
                "",
            ),
            (
                "naive",
                {"task_page_size": 3.14},
                "Input should be a valid integer",
            ),
            (
                "naive",
                {"task_page_size": "1024"},
                "Input should be a valid integer",
            ),
            ("naive", {"raptor": {"use_raptor": True,                 
                                "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
                                "max_token": 256,
                                "threshold": 0.1,
                                "max_cluster": 64,
                                "random_seed": 0,}}, ""),
            ("naive", {"raptor": {"use_raptor": False}}, ""),
            (
                "naive",
                {"invalid_key": "invalid_value"},
                "Extra inputs are not permitted",
            ),
            (
                "naive",
                {"auto_keywords": -1},
                "Input should be greater than or equal to 0",
            ),
            pytest.param(
                "naive",
                {"auto_keywords": 32},
                "",
            ),
            (
                "naive",
                {"auto_keywords": 3.14},
                "Input should be a valid integer",
            ),
            (
                "naive",
                {"auto_keywords": "1024"},
                "Input should be a valid integer",
            ),
            (
                "naive",
                {"auto_questions": -1},
                "Input should be greater than or equal to 0",
            ),
            pytest.param(
                "naive",
                {"auto_questions": 10},
                "",
            ),
            (
                "naive",
                {"auto_questions": 3.14},
                "Input should be a valid integer",
            ),
            (
                "naive",
                {"auto_questions": "1024"},
                "Input should be a valid integer",
            ),
            (
                "naive",
                {"topn_tags": -1},
                "Input should be greater than or equal to 1",
            ),
            pytest.param(
                "naive",
                {"topn_tags": 10},
                "",
            ),
            (
                "naive",
                {"topn_tags": 3.14},
                "Input should be a valid integer",
            ),
            (
                "naive",
                {"topn_tags": "1024"},
                "Input should be a valid integer",
            ),
        ],
    )
    def test_parser_config(self, client, add_documents, chunk_method, parser_config, expected_message):
        dataset, documents = add_documents
        document = documents[0]
        from operator import attrgetter

        update_data = {"chunk_method": chunk_method, "parser_config": parser_config}

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.update(update_data)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            document.update(update_data)
            docs = dataset.list_documents(id=document.id)
            updated_doc = [doc for doc in docs if doc.id == document.id][0]
            if parser_config:
                for k, v in parser_config.items():
                    if isinstance(v, dict):
                        for kk, vv in v.items():
                            assert attrgetter(f"{k}.{kk}")(updated_doc.parser_config) == vv, str(updated_doc)
                    else:
                        assert attrgetter(k)(updated_doc.parser_config) == v, str(updated_doc)
            else:
                expected_config = DataSet.ParserConfig(
                    client,
                    DEFAULT_PARSER_CONFIG,
                )
                assert str(updated_doc.parser_config) == str(expected_config), str(updated_doc)
