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

import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from ragflow_sdk import RAGFlow


@pytest.mark.integration
class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth", [None, "invalid-api-key"])
    def test_auth_invalid(self, integration_settings, invalid_auth):
        client = RAGFlow(invalid_auth, integration_settings["base_url"])
        with pytest.raises(Exception, match="^Invalid access token$"):
            client.delete_datasets()

    @pytest.mark.p2
    def test_auth_missing(self, integration_settings):
        client = RAGFlow("unused", integration_settings["base_url"])
        client.authorization_header = {}
        with pytest.raises(Exception, match="^Missing Authorization header$"):
            client.delete_datasets()


@pytest.mark.integration
class TestCapability:
    @pytest.mark.p3
    def test_delete_dataset_1k(self, client, dataset_factory):
        datasets = [dataset_factory() for _ in range(1_000)]
        assert client.delete_datasets(ids=[dataset.id for dataset in datasets]) is None
        assert client.list_datasets() == []

    @pytest.mark.p3
    def test_concurrent_deletion(self, client, dataset_factory):
        datasets = [dataset_factory() for _ in range(20)]
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.delete_datasets, ids=[dataset.id]) for dataset in datasets]
            for future in as_completed(futures):
                assert future.result() is None
        assert client.list_datasets() == []


@pytest.mark.integration
class TestDatasetsDelete:
    @pytest.mark.p1
    @pytest.mark.parametrize("delete_count", [1, 3], ids=["single_dataset", "multiple_datasets"])
    def test_ids(self, client, add_datasets_func, delete_count):
        ids = [dataset.id for dataset in add_datasets_func]
        assert client.delete_datasets(ids=ids[:delete_count]) is None
        assert {dataset.id for dataset in client.list_datasets()} == set(ids[delete_count:])

    @pytest.mark.p2
    def test_ids_empty(self, client, add_dataset_func):
        client.delete_datasets(ids=[])
        assert [dataset.id for dataset in client.list_datasets()] == [add_dataset_func.id]

    @pytest.mark.p3
    def test_ids_none(self, client, add_datasets_func):
        client.delete_datasets(ids=None)
        assert {dataset.id for dataset in client.list_datasets()} == {dataset.id for dataset in add_datasets_func}

    @pytest.mark.p2
    def test_ids_omitted(self, client, add_dataset_func):
        client.delete_datasets()
        assert [dataset.id for dataset in client.list_datasets()] == [add_dataset_func.id]

    @pytest.mark.p2
    def test_id_not_uuid(self, client, add_dataset_func):
        with pytest.raises(Exception, match="^Invalid UUID format$"):
            client.delete_datasets(ids=["not_uuid"])
        assert [dataset.id for dataset in client.list_datasets()] == [add_dataset_func.id]

    @pytest.mark.p2
    @pytest.mark.parametrize("dataset_id", [uuid.uuid4().hex, "d94a8dc02c9711f0930f7fbc369eab6d"], ids=["uuid4", "missing_uuid1"])
    def test_id_wrong_uuid(self, client, add_dataset_func, dataset_id):
        with pytest.raises(Exception, match="^Permission denied$"):
            client.delete_datasets(ids=[dataset_id])
        assert [dataset.id for dataset in client.list_datasets()] == [add_dataset_func.id]

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_index", [0, 1, 3], ids=["first", "middle", "last"])
    def test_ids_partial_invalid(self, client, add_datasets_func, invalid_index):
        ids = [dataset.id for dataset in add_datasets_func]
        payload = ids.copy()
        payload.insert(invalid_index, uuid.uuid4().hex)
        with pytest.raises(Exception, match="^Permission denied$"):
            client.delete_datasets(ids=payload)
        assert {dataset.id for dataset in client.list_datasets()} == set(ids)

    @pytest.mark.p2
    def test_ids_duplicate(self, client, add_datasets_func):
        ids = [dataset.id for dataset in add_datasets_func]
        with pytest.raises(Exception, match="Duplicate ids:"):
            client.delete_datasets(ids=ids + ids)
        assert {dataset.id for dataset in client.list_datasets()} == set(ids)

    @pytest.mark.p2
    def test_repeated_delete(self, client, add_datasets_func):
        ids = [dataset.id for dataset in add_datasets_func]
        client.delete_datasets(ids=ids)
        with pytest.raises(Exception, match="^Permission denied$"):
            client.delete_datasets(ids=ids)
        assert client.list_datasets() == []


class TestDeleteDatasetsArgumentValidation:
    @pytest.mark.p3
    def test_field_unsupported(self):
        client = RAGFlow("token", "http://sdk.invalid")
        with pytest.raises(TypeError, match="unexpected keyword argument 'unknown_field'"):
            client.delete_datasets(unknown_field="unknown_field")
