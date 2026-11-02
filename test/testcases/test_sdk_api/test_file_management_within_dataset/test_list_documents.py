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
from concurrent.futures import ThreadPoolExecutor, as_completed
from unittest.mock import Mock

import pytest
from beartype.roar import BeartypeCallHintParamViolation
from ragflow_sdk import DataSet, Document


def raw_document_values_by_id(dataset, field, desc):
    response = dataset.rag.get(
        f"/datasets/{dataset.id}/documents",
        params={"page": 1, "page_size": 100, "orderby": field, "desc": desc},
    )
    result = response.json()
    assert result["code"] == 0, result
    return {document["id"]: document[field] for document in result["data"]["docs"]}


class TestDocumentsList:
    pytestmark = pytest.mark.integration

    @pytest.mark.p1
    def test_default(self, add_documents):
        dataset, _ = add_documents
        documents = dataset.list_documents()
        assert len(documents) == 5, str(documents)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_ids",
        [
            ({"page": 1, "page_size": 2, "orderby": "name", "desc": False}, slice(0, 2)),
            ({"page": 2, "page_size": 2, "orderby": "name", "desc": False}, slice(2, 4)),
            ({"page": 3, "page_size": 2, "orderby": "name", "desc": False}, slice(4, 6)),
            ({"page": -1, "page_size": 2, "orderby": "name", "desc": False}, slice(0, 2)),
        ],
    )
    def test_page(self, add_documents, params, expected_page_ids):
        dataset, fixture_documents = add_documents
        expected = sorted(fixture_documents, key=lambda document: document.name)
        documents = dataset.list_documents(**params)
        assert [document.id for document in documents] == [document.id for document in expected[expected_page_ids]]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_size",
        [
            ({"page_size": 2, "orderby": "name", "desc": False}, 2),
            ({"page_size": 1, "orderby": "name", "desc": False}, 1),
            ({"page_size": 6, "orderby": "name", "desc": False}, 5),
            ({"page_size": -1, "orderby": "name", "desc": False}, 5),
        ],
    )
    def test_page_size(self, add_documents, params, expected_size):
        dataset, fixture_documents = add_documents
        expected = sorted(fixture_documents, key=lambda document: document.name)
        documents = dataset.list_documents(**params)
        assert [document.id for document in documents] == [document.id for document in expected[:expected_size]]

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_message",
        [
            ({"orderby": "create_time"}, ""),
            ({"orderby": "update_time"}, ""),
            ({"orderby": "name"}, ""),
            ({"orderby": "unknown"}, "invalid orderby field: unknown"),
        ],
    )
    def test_orderby(self, add_documents, params, expected_message):
        dataset, fixture_documents = add_documents
        if expected_message:
            with pytest.raises(Exception) as exception_info:
                dataset.list_documents(**params)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            field = params["orderby"]
            documents = dataset.list_documents(**params)
            assert {document.id for document in documents} == {doc.id for doc in fixture_documents}
            raw_values = raw_document_values_by_id(dataset, field, desc=True)
            assert set(raw_values) == {doc.id for doc in fixture_documents}
            values = [raw_values[document.id] for document in documents]
            assert values == sorted(values, reverse=True), values

    @pytest.mark.p3
    @pytest.mark.parametrize("desc", [True, False])
    def test_desc(self, add_documents, desc):
        dataset, fixture_documents = add_documents
        documents = dataset.list_documents(desc=desc)
        assert {document.id for document in documents} == {doc.id for doc in fixture_documents}
        raw_values = raw_document_values_by_id(dataset, "create_time", desc)
        assert set(raw_values) == {doc.id for doc in fixture_documents}
        create_times = [raw_values[document.id] for document in documents]
        assert create_times == sorted(create_times, reverse=desc), create_times

    @pytest.mark.p2
    @pytest.mark.parametrize("desc", [False, True])
    def test_name_ordering(self, add_documents, desc):
        dataset, fixture_documents = add_documents
        documents = dataset.list_documents(orderby="name", desc=desc)
        assert {document.id for document in documents} == {doc.id for doc in fixture_documents}
        names = [document.name for document in documents]
        assert names == sorted(names, reverse=desc), names

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_num",
        [
            ({"keywords": None}, 5),
            ({"keywords": ""}, 5),
            ({"keywords": "0"}, 1),
            ({"keywords": "ragflow_test_upload"}, 5),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, add_documents, params, expected_num):
        dataset, _ = add_documents
        documents = dataset.list_documents(**params)
        assert len(documents) == expected_num, str(documents)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_num, expected_message",
        [
            ({"name": None}, 5, ""),
            ({"name": ""}, 5, ""),
            ({"name": "ragflow_test_upload_0.txt"}, 1, ""),
            ({"name": "unknown.txt"}, 0, "you don't own the document unknown.txt"),
        ],
    )
    def test_name(self, add_documents, params, expected_num, expected_message):
        dataset, _ = add_documents
        if expected_message:
            with pytest.raises(Exception) as exception_info:
                dataset.list_documents(**params)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            documents = dataset.list_documents(**params)
            assert len(documents) == expected_num, str(documents)
            if params["name"] not in [None, ""]:
                assert documents[0].name == params["name"], str(documents)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "document_id, expected_num, expected_message",
        [
            (None, 5, ""),
            ("", 5, ""),
            (lambda docs: docs[0].id, 1, ""),
            ("unknown.txt", 0, "you don't own the document unknown.txt"),
        ],
    )
    def test_id(self, add_documents, document_id, expected_num, expected_message):
        dataset, documents = add_documents
        if callable(document_id):
            params = {"id": document_id(documents)}
        else:
            params = {"id": document_id}

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                dataset.list_documents(**params)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            documents = dataset.list_documents(**params)
            assert len(documents) == expected_num, str(documents)
            if params["id"] not in [None, ""]:
                assert documents[0].id == params["id"], str(documents)

    @pytest.mark.p1
    def test_ids(self, add_documents):
        dataset, fixture_documents = add_documents
        expected_ids = {document.id for document in fixture_documents[:2]}

        documents = dataset.list_documents(ids=list(expected_ids))

        assert len(documents) == 2, str(documents)
        assert {document.id for document in documents} == expected_ids, str(documents)

    @pytest.mark.p1
    def test_empty_ids_is_unfiltered(self, add_documents):
        dataset, fixture_documents = add_documents
        expected_ids = {document.id for document in fixture_documents}

        documents = dataset.list_documents(ids=[])

        assert len(documents) == 5, str(documents)
        assert {document.id for document in documents} == expected_ids, str(documents)

    @pytest.mark.p3
    def test_id_and_ids_raise_value_error(self, add_documents):
        dataset, documents = add_documents

        with pytest.raises(ValueError) as exception_info:
            dataset.list_documents(id=documents[0].id, ids=[documents[1].id])

        assert str(exception_info.value) == "Cannot use both 'id' and 'ids' parameters at the same time."

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, name, expected_num, expected_message",
        [
            (lambda docs: docs[0].id, "ragflow_test_upload_0.txt", 1, ""),
            (lambda docs: docs[0].id, "ragflow_test_upload_1.txt", 0, ""),
            (lambda docs: docs[0].id, "unknown", 0, "you don't own the document unknown"),
            ("invalid_id", "ragflow_test_upload_0.txt", 0, "you don't own the document invalid_id"),
        ],
    )
    def test_name_and_id(self, add_documents, document_id, name, expected_num, expected_message):
        dataset, documents = add_documents
        params = {"id": document_id(documents) if callable(document_id) else document_id, "name": name}

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                dataset.list_documents(**params)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            documents = dataset.list_documents(**params)
            assert len(documents) == expected_num, str(documents)

    @pytest.mark.p3
    def test_concurrent_list(self, add_documents):
        dataset, _ = add_documents
        count = 100

        def list_docs():
            return dataset.list_documents()

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_docs) for _ in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        for future in futures:
            docs = future.result()
            assert len(docs) == 5, str(docs)


class TestDocumentListSDKTypes:
    def test_timestamp_ordering_uses_raw_rows_by_sdk_document_id(self):
        rows = [
            {"id": "second", "name": "second.txt", "create_time": 20},
            {"id": "first", "name": "first.txt", "create_time": 10},
        ]
        rag = Mock()
        rag.get.return_value.json.return_value = {"code": 0, "data": {"docs": rows}}
        dataset = DataSet(rag, {"id": "dataset"})
        documents = [Document(rag, row.copy()) for row in rows]
        values = raw_document_values_by_id(dataset, "create_time", True)
        assert [values[document.id] for document in documents] == [20, 10]
        rag.get.assert_called_once_with(
            "/datasets/dataset/documents",
            params={"page": 1, "page_size": 100, "orderby": "create_time", "desc": True},
        )

    @pytest.mark.parametrize(
        "params",
        [
            {"page": None},
            {"page": "3"},
            {"page": "a"},
            {"page_size": None},
            {"page_size": "1"},
            {"page_size": "a"},
            {"orderby": None},
            {"desc": None},
            {"desc": "true"},
            {"desc": "True"},
            {"desc": "false"},
            {"desc": "False"},
            {"desc": "False", "orderby": "update_time"},
            {"desc": "unknown"},
        ],
    )
    def test_invalid_parameter_types_are_rejected_before_http(self, params):
        dataset = DataSet(None, {"id": "dataset-without-http-client"})
        with pytest.raises(BeartypeCallHintParamViolation):
            dataset.list_documents(**params)

    def test_invalid_params(self):
        dataset = DataSet(None, {"id": "dataset-without-http-client"})
        params = {"a": "b"}
        with pytest.raises(TypeError) as exception_info:
            dataset.list_documents(**params)
        assert "got an unexpected keyword argument" in str(exception_info.value), str(exception_info.value)
