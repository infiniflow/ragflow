#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from unittest.mock import Mock

import pytest
from ragflow_sdk import Document


@pytest.mark.integration
class TestDocumentsUpdated:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("new_name.txt", ""),
            (f"{'a' * 251}.txt", ""),
            (0, "Field: <name> - Message: <Input should be a valid string> - Value: <0>"),
            (None, "Field: <name> - Message: <Input should be a valid string> - Value: <None>"),
            ("", "the extension of file can't be changed"),
            ("ragflow_test_upload_0", "the extension of file can't be changed"),
            ("ragflow_test_upload_1.txt", "duplicated document name in the same dataset"),
            ("RAGFLOW_TEST_UPLOAD_1.TXT", "duplicated document name in the same dataset"),
            ("RAGFLOW_TEST_UPLOAD_0.TXT", ""),
        ],
    )
    def test_name(self, add_documents, name, expected_message):
        dataset, documents = add_documents
        document = documents[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.update({"name": name})
            assert expected_message in str(exception_info.value), str(exception_info.value)
            return

        document.update({"name": name})
        docs = dataset.list_documents(id=document.id)
        updated_doc = next(doc for doc in docs if doc.id == document.id)
        assert updated_doc.name == name, str(updated_doc)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "meta_fields, expected_error",
        [
            ({"author": "Ada", "year": 2025, "tags": ["one", "two"]}, ""),
            ({"nested": {"x": 1}}, "The type is not supported"),
            ({"tags": [{"x": 1}]}, "The type is not supported in list"),
        ],
    )
    def test_meta_fields(self, add_documents, meta_fields, expected_error):
        dataset, documents = add_documents
        document = documents[0]
        original = dataset.list_documents(id=document.id)[0].meta_fields.to_json()
        if expected_error:
            with pytest.raises(Exception) as exception_info:
                document.update({"meta_fields": meta_fields})
            assert expected_error in str(exception_info.value)
            unchanged = dataset.list_documents(id=document.id)[0].meta_fields.to_json()
            assert unchanged == original
            return

        document.update({"meta_fields": meta_fields})

        docs = dataset.list_documents(id=document.id)
        updated_doc = next(doc for doc in docs if doc.id == document.id)
        assert updated_doc.meta_fields.to_json() == meta_fields


@pytest.mark.p2
def test_meta_fields_rejects_non_dictionary_without_backend():
    rag = Mock()
    document = Document(rag, {"dataset_id": "dataset-1", "id": "document-1"})

    with pytest.raises(Exception, match="meta_fields must be a dictionary"):
        document.update({"meta_fields": "not-a-dict"})
    rag.patch.assert_not_called()


@pytest.mark.integration
class TestDocumentParserSelection:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "chunk_method, expected_message",
        [
            ("naive", ""),
            ("general", ""),
            ("book", ""),
            ("laws", ""),
            ("one", ""),
            ("qa", ""),
            ("table", ""),
            ("manual", "selected pipeline does not support file type"),
            ("paper", "selected pipeline does not support file type"),
            ("presentation", "selected pipeline does not support file type"),
            ("picture", "selected pipeline does not support file type"),
            ("email", "selected pipeline does not support file type"),
            ("knowledge_graph", "unknown builtin parser_id: knowledge_graph"),
            ("tag", "unknown builtin parser_id: tag"),
            ("", "`chunk_method` (empty string) is not valid"),
            ("other_chunk_method", "unknown builtin parser_id: other_chunk_method"),
        ],
    )
    def test_chunk_method(self, add_documents, chunk_method, expected_message):
        dataset, documents = add_documents
        document = documents[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.update({"chunk_method": chunk_method})
            assert expected_message in str(exception_info.value), str(exception_info.value)
            return

        document.update({"chunk_method": chunk_method})
        docs = dataset.list_documents(id=document.id)
        updated_doc = next(doc for doc in docs if doc.id == document.id)
        assert updated_doc.chunk_method == chunk_method, str(updated_doc)


@pytest.mark.integration
class TestImmutableDocumentFields:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"chunk_count": 1}, "can't change `chunk_count`"),
        ],
    )
    def test_immutable_fields_chunk_count(self, add_documents, payload, expected_message):
        _, documents = add_documents
        with pytest.raises(Exception) as exception_info:
            documents[0].update(payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"token_count": 9999}, "can't change `token_count`"),
        ],
    )
    def test_immutable_fields_token_count(self, add_documents, payload, expected_message):
        _, documents = add_documents
        with pytest.raises(Exception) as exception_info:
            documents[0].update(payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"progress": 0.5}, "can't change `progress`"),
            ({"progress": 1.0}, "can't change `progress`"),
            (
                {"progress": 1.5},
                "Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <1.5>",
            ),
        ],
    )
    def test_immutable_fields_progress(self, add_documents, payload, expected_message):
        _, documents = add_documents
        with pytest.raises(Exception) as exception_info:
            documents[0].update(payload)
        assert expected_message in str(exception_info.value), str(exception_info.value)


@pytest.mark.integration
class TestUnknownUpdateFields:
    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload",
        [
            {"create_date": "Fri, 14 Mar 2025 16:53:42 GMT"},
            {"create_time": 1},
            {"created_by": "ragflow_test"},
            {"dataset_id": "ragflow_test"},
            {"id": "ragflow_test"},
            {"location": "ragflow_test.txt"},
            {"process_begin_at": 1},
            {"process_duration": 1.0},
            {"progress_msg": "ragflow_test"},
            {"run": "ragflow_test"},
            {"size": 1},
            {"source_type": "ragflow_test"},
            {"thumbnail": "ragflow_test"},
            {"type": "ragflow_test"},
            {"update_date": "Fri, 14 Mar 2025 16:33:17 GMT"},
            {"update_time": 1},
        ],
    )
    def test_unknown_fields_are_ignored_without_mutation(self, add_documents, payload):
        dataset, documents = add_documents
        document = documents[0]
        path = f"/datasets/{dataset.id}/documents"
        before = dataset.rag.get(path, params={"id": document.id}).json()["data"]["docs"][0]

        document.update(payload)
        after = dataset.rag.get(path, params={"id": document.id}).json()["data"]["docs"][0]
        for field in before.keys() - {"update_date", "update_time"}:
            assert after[field] == before[field], f"{field} changed for {payload}"
        for field in ("update_date", "update_time"):
            if field in payload:
                assert after[field] != payload[field], f"{field} was applied from {payload}"


@pytest.mark.integration
class TestUpdateDocumentParserConfig:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "parser_config, saved_path, expected_value, expected_message",
        [
            ({}, None, None, ""),
            (
                {"GeneralChunker:SixApplesFall": {"chunk_token_size": 256}},
                ("GeneralChunker:SixApplesFall", "chunk_token_size"),
                256,
                "",
            ),
            (
                {"GeneralChunker:SixApplesFall": {"delimiters": ["!"]}},
                ("GeneralChunker:SixApplesFall", "delimiters"),
                ["!"],
                "",
            ),
            (
                {"Extractor:AutoExtractDefault": {"keywords": {"top_n": 5}}},
                ("Extractor:AutoExtractDefault", "keywords", "top_n"),
                5,
                "",
            ),
            (
                {"Extractor:AutoExtractDefault": {"questions": {"top_n": 5}}},
                ("Extractor:AutoExtractDefault", "questions", "top_n"),
                5,
                "",
            ),
            (
                {"Extractor:AutoExtractDefault": {"tags": {"top_n": 3}}},
                ("Extractor:AutoExtractDefault", "tags", "top_n"),
                3,
                "",
            ),
            (
                {"GeneralChunker:SixApplesFall": {"delimiters": ["!"] * 33000}},
                None,
                None,
                "exceeds size limit",
            ),
            ([], None, None, "input should be a valid dictionary"),
            ({"Parser:HipSignsRhyme": {"pdf": {"pages": [[0, 5]]}}}, None, None, "page"),
        ],
    )
    def test_parser_config(self, add_documents, parser_config, saved_path, expected_value, expected_message):
        dataset, documents = add_documents
        document = documents[0]
        original_config = dataset.list_documents(id=document.id)[0].parser_config.to_json()

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.update({"parser_config": parser_config})
            assert expected_message in str(exception_info.value).lower()
            unchanged = dataset.list_documents(id=document.id)[0].parser_config.to_json()
            assert unchanged == original_config
            return

        document.update({"parser_config": parser_config})
        docs = dataset.list_documents(id=document.id)
        updated_doc = next(doc for doc in docs if doc.id == document.id)
        saved = updated_doc.parser_config.to_json()
        if saved_path:
            value = saved
            for key in saved_path:
                value = value[key]
            assert value == expected_value
        else:
            assert saved == original_config
