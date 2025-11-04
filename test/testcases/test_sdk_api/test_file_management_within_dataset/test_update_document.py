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
            with pytest.raises(Exception) as excinfo:
                document.update({"name": name})
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            document.update({"name": name})
            updated_doc = dataset.list_documents(id=document.id)[0]
            assert updated_doc.name == name, str(updated_doc)

    @pytest.mark.p3
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
            with pytest.raises(Exception) as excinfo:
                document.update({"meta_fields": meta_fields})
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            document.update({"meta_fields": meta_fields})

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
            ("", "`chunk_method`  doesn't exist"),
            ("other_chunk_method", "`chunk_method` other_chunk_method doesn't exist"),
        ],
    )
    def test_chunk_method(self, add_documents, chunk_method, expected_message):
        dataset, documents = add_documents
        document = documents[0]

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                document.update({"chunk_method": chunk_method})
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            document.update({"chunk_method": chunk_method})
            updated_doc = dataset.list_documents(id=document.id)[0]
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

        with pytest.raises(Exception) as excinfo:
            document.update(payload)
        assert expected_message in str(excinfo.value), str(excinfo.value)


class TestUpdateDocumentParserConfig:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "chunk_method, parser_config, expected_message",
        [
            ("naive", {}, ""),
            (
                "naive",
                DEFAULT_PARSER_CONFIG,
                "",
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": -1},
                "chunk_token_num should be in range from 1 to 100000000",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": 0},
                "chunk_token_num should be in range from 1 to 100000000",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": 100000000},
                "chunk_token_num should be in range from 1 to 100000000",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": 3.14},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"chunk_token_num": "1024"},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            ("naive", {"layout_recognize": "DeepDOC"}, ""),
            ("naive", {"layout_recognize": "Naive"}, ""),
            ("naive", {"html4excel": True}, ""),
            ("naive", {"html4excel": False}, ""),
            pytest.param(
                "naive",
                {"html4excel": 1},
                "html4excel should be True or False",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            ("naive", {"delimiter": ""}, ""),
            ("naive", {"delimiter": "`##`"}, ""),
            pytest.param(
                "naive",
                {"delimiter": 1},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"task_page_size": -1},
                "task_page_size should be in range from 1 to 100000000",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"task_page_size": 0},
                "task_page_size should be in range from 1 to 100000000",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"task_page_size": 100000000},
                "task_page_size should be in range from 1 to 100000000",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"task_page_size": 3.14},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"task_page_size": "1024"},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            ("naive", {"raptor": {"use_raptor": True,                 
                                "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
                                "max_token": 256,
                                "threshold": 0.1,
                                "max_cluster": 64,
                                "random_seed": 0,}}, ""),
            ("naive", {"raptor": {"use_raptor": False}}, ""),
            pytest.param(
                "naive",
                {"invalid_key": "invalid_value"},
                "Abnormal 'parser_config'. Invalid key: invalid_key",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_keywords": -1},
                "auto_keywords should be in range from 0 to 32",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_keywords": 32},
                "auto_keywords should be in range from 0 to 32",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_keywords": 3.14},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_keywords": "1024"},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_questions": -1},
                "auto_questions should be in range from 0 to 10",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_questions": 10},
                "auto_questions should be in range from 0 to 10",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_questions": 3.14},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"auto_questions": "1024"},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"topn_tags": -1},
                "topn_tags should be in range from 0 to 10",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"topn_tags": 10},
                "topn_tags should be in range from 0 to 10",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"topn_tags": 3.14},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
            pytest.param(
                "naive",
                {"topn_tags": "1024"},
                "",
                marks=pytest.mark.skip(reason="issues/6098"),
            ),
        ],
    )
    def test_parser_config(self, client, add_documents, chunk_method, parser_config, expected_message):
        dataset, documents = add_documents
        document = documents[0]
        from operator import attrgetter

        update_data = {"chunk_method": chunk_method, "parser_config": parser_config}

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                document.update(update_data)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            document.update(update_data)
            updated_doc = dataset.list_documents(id=document.id)[0]
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
