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

import pytest


class TestDocumentsList:
    @pytest.mark.p1
    def test_default(self, add_documents):
        dataset, _ = add_documents
        documents = dataset.list_documents()
        assert len(documents) == 5, str(documents)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size, expected_message",
        [
            ({"page": None, "page_size": 2}, 2, "not instance of"),
            ({"page": 0, "page_size": 2}, 2, ""),
            ({"page": 2, "page_size": 2}, 2, ""),
            ({"page": 3, "page_size": 2}, 1, ""),
            ({"page": "3", "page_size": 2}, 1, "not instance of"),
            pytest.param(
                {"page": -1, "page_size": 2},
                0,
                "Invalid page number",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
            pytest.param(
                {"page": "a", "page_size": 2},
                0,
                "Invalid page value",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
        ],
    )
    def test_page(self, add_documents, params, expected_page_size, expected_message):
        dataset, _ = add_documents
        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.list_documents(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            documents = dataset.list_documents(**params)
            assert len(documents) == expected_page_size, str(documents)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size, expected_message",
        [
            ({"page_size": None}, 5, "not instance of"),
            ({"page_size": 0}, 0, ""),
            ({"page_size": 1}, 1, ""),
            ({"page_size": 6}, 5, ""),
            ({"page_size": "1"}, 1, "not instance of"),
            pytest.param(
                {"page_size": -1},
                0,
                "Invalid page size",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
            pytest.param(
                {"page_size": "a"},
                0,
                "Invalid page size value",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
        ],
    )
    def test_page_size(self, add_documents, params, expected_page_size, expected_message):
        dataset, _ = add_documents
        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.list_documents(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            documents = dataset.list_documents(**params)
            assert len(documents) == expected_page_size, str(documents)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_message",
        [
            ({"orderby": None}, "not instance of"),
            ({"orderby": "create_time"}, ""),
            ({"orderby": "update_time"}, ""),
            pytest.param({"orderby": "name", "desc": "False"}, "", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"orderby": "unknown"}, "orderby should be create_time or update_time", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_orderby(self, add_documents, params, expected_message):
        dataset, _ = add_documents
        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.list_documents(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            dataset.list_documents(**params)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_message",
        [
            ({"desc": None}, "not instance of"),
            ({"desc": "true"}, "not instance of"),
            ({"desc": "True"}, "not instance of"),
            ({"desc": True}, ""),
            pytest.param({"desc": "false"}, "", marks=pytest.mark.skip(reason="issues/5851")),
            ({"desc": "False"}, "not instance of"),
            ({"desc": False}, ""),
            ({"desc": "False", "orderby": "update_time"}, "not instance of"),
            pytest.param({"desc": "unknown"}, "desc should be true or false", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_desc(self, add_documents, params, expected_message):
        dataset, _ = add_documents
        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.list_documents(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            dataset.list_documents(**params)

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
            ({"name": "unknown.txt"}, 0, "You don't own the document unknown.txt"),
        ],
    )
    def test_name(self, add_documents, params, expected_num, expected_message):
        dataset, _ = add_documents
        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.list_documents(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
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
            ("unknown.txt", 0, "You don't own the document unknown.txt"),
        ],
    )
    def test_id(self, add_documents, document_id, expected_num, expected_message):
        dataset, documents = add_documents
        if callable(document_id):
            params = {"id": document_id(documents)}
        else:
            params = {"id": document_id}

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.list_documents(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            documents = dataset.list_documents(**params)
            assert len(documents) == expected_num, str(documents)
            if params["id"] not in [None, ""]:
                assert documents[0].id == params["id"], str(documents)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, name, expected_num, expected_message",
        [
            (lambda docs: docs[0].id, "ragflow_test_upload_0.txt", 1, ""),
            (lambda docs: docs[0].id, "ragflow_test_upload_1.txt", 0, ""),
            (lambda docs: docs[0].id, "unknown", 0, "You don't own the document unknown"),
            ("invalid_id", "ragflow_test_upload_0.txt", 0, "You don't own the document invalid_id"),
        ],
    )
    def test_name_and_id(self, add_documents, document_id, name, expected_num, expected_message):
        dataset, documents = add_documents
        params = {"id": document_id(documents) if callable(document_id) else document_id, "name": name}

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.list_documents(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
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

    @pytest.mark.p3
    def test_invalid_params(self, add_documents):
        dataset, _ = add_documents
        params = {"a": "b"}
        with pytest.raises(TypeError) as excinfo:
            dataset.list_documents(**params)
        assert "got an unexpected keyword argument" in str(excinfo.value), str(excinfo.value)
