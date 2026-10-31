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
from beartype.roar import BeartypeCallHintParamViolation
from ragflow_sdk import RAGFlow


class _ListDatasetsResponse:
    def json(self):
        return {"code": 0, "data": []}


class TestListDatasetsArgumentValidation:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params",
        [{"id": "dataset-id", "ids": ["dataset-id"]}, {"id": "dataset-id", "ids": [""]}],
    )
    def test_rejects_conflicting_id_and_ids_before_get(self, monkeypatch, params):
        client = RAGFlow("token", "http://sdk.invalid")
        get_calls = []
        monkeypatch.setattr(client, "get", lambda *_args, **_kwargs: get_calls.append((_args, _kwargs)))

        with pytest.raises(ValueError, match="^Cannot use both 'id' and 'ids' parameters at the same time\\.$"):
            client.list_datasets(**params)

        assert get_calls == []

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params",
        [{"id": "dataset-id"}, {"ids": ["dataset-id"]}, {"id": None, "ids": None}, {"id": "dataset-id", "ids": []}, {"id": "", "ids": ["dataset-id"]}],
    )
    def test_preserves_nonconflicting_id_and_ids(self, monkeypatch, params):
        client = RAGFlow("token", "http://sdk.invalid")
        get_calls = []

        def _get(path, params=None, json=None):
            get_calls.append((path, params, json))
            return _ListDatasetsResponse()

        monkeypatch.setattr(client, "get", _get)
        assert client.list_datasets(**params) == []
        assert get_calls == [
            (
                "/datasets",
                {"page": 1, "page_size": 30, "orderby": "create_time", "desc": True, "id": params.get("id"), "ids": params.get("ids"), "name": None},
                None,
            )
        ]

    @pytest.mark.p2
    def test_get_dataset_not_found_raises(self, monkeypatch):
        client = RAGFlow("token", "http://sdk.invalid")
        monkeypatch.setattr(client, "list_datasets", lambda **_: [])
        with pytest.raises(Exception, match="^Dataset missing-name-for-coverage not found$"):
            client.get_dataset(name="missing-name-for-coverage")

    @pytest.mark.p3
    def test_field_unsupported(self):
        client = RAGFlow("token", "http://sdk.invalid")
        with pytest.raises(TypeError, match="unexpected keyword argument 'unknown_field'"):
            client.list_datasets(unknown_field="unknown_field")

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params",
        [{"page": "a"}, {"page": None}, {"page_size": "a"}, {"page_size": None}, {"orderby": None}, {"desc": 3.14}, {"desc": "unknown"}, {"desc": None}],
    )
    def test_invalid_parameter_types_before_get(self, monkeypatch, params):
        client = RAGFlow("token", "http://sdk.invalid")
        get_calls = []
        monkeypatch.setattr(client, "get", lambda *args, **kwargs: get_calls.append((args, kwargs)))
        with pytest.raises(BeartypeCallHintParamViolation, match="not instance of"):
            client.list_datasets(**params)
        assert get_calls == []


@pytest.mark.integration
class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth", [None, "invalid-api-key"])
    def test_auth_invalid(self, integration_settings, invalid_auth):
        client = RAGFlow(invalid_auth, integration_settings["base_url"])
        with pytest.raises(Exception, match="^Invalid access token$"):
            client.list_datasets()

    @pytest.mark.p2
    def test_auth_missing(self, integration_settings):
        client = RAGFlow("unused", integration_settings["base_url"])
        client.authorization_header = {}
        with pytest.raises(Exception, match="^Missing Authorization header$"):
            client.list_datasets()


@pytest.mark.integration
class TestCapability:
    @pytest.mark.p3
    def test_concurrent_list(self, client, add_datasets):
        expected_ids = {dataset.id for dataset in add_datasets}
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.list_datasets) for _ in range(100)]
            for future in as_completed(futures):
                assert {dataset.id for dataset in future.result()} == expected_ids


@pytest.mark.integration
@pytest.mark.usefixtures("add_datasets")
class TestDatasetsList:
    @pytest.mark.p2
    def test_params_unset(self, client, add_datasets):
        datasets = client.list_datasets()
        assert {dataset.id for dataset in datasets} == {dataset.id for dataset in add_datasets}
        assert len(datasets) == len(add_datasets)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [({"page": 2, "page_size": 2}, 2), ({"page": 3, "page_size": 2}, 1), ({"page": 4, "page_size": 2}, 0), ({"page": 1, "page_size": 10}, 5)],
        ids=["middle", "last_partial", "beyond_last", "single_page"],
    )
    def test_page(self, client, add_datasets, params, expected_page_size):
        datasets = client.list_datasets(**params)
        assert len(datasets) == expected_page_size
        assert len({dataset.id for dataset in datasets}) == len(datasets)
        assert {dataset.id for dataset in datasets} <= {dataset.id for dataset in add_datasets}

    @pytest.mark.p1
    def test_pages_partition_results(self, client, add_datasets):
        datasets = [dataset for page in range(1, 4) for dataset in client.list_datasets(page=page, page_size=2)]
        assert len(datasets) == len(add_datasets)
        assert {dataset.id for dataset in datasets} == {dataset.id for dataset in add_datasets}

    @pytest.mark.p2
    def test_page_invalid(self, client):
        with pytest.raises(Exception, match="greater than or equal to 1"):
            client.list_datasets(page=0)

    @pytest.mark.p1
    @pytest.mark.parametrize("page_size, expected_size", [(1, 1), (3, 3), (5, 5), (6, 5)])
    def test_page_size(self, client, add_datasets, page_size, expected_size):
        datasets = client.list_datasets(page_size=page_size)
        assert len(datasets) == expected_size
        assert len({dataset.id for dataset in datasets}) == len(datasets)
        assert {dataset.id for dataset in datasets} <= {dataset.id for dataset in add_datasets}

    @pytest.mark.p2
    def test_page_size_invalid(self, client):
        with pytest.raises(Exception, match="greater than or equal to 1"):
            client.list_datasets(page_size=0)

    @pytest.mark.p3
    @pytest.mark.parametrize("orderby", ["create_time", "update_time"])
    @pytest.mark.parametrize("desc", [True, False])
    def test_orderby(self, client, add_datasets, orderby, desc):
        # DataSet omits timestamps, so read persisted values through the detail API.
        times = {}
        for dataset in add_datasets:
            response = client.get(f"/datasets/{dataset.id}").json()
            assert response["code"] == 0, response
            times[dataset.id] = response["data"][orderby]
        datasets = client.list_datasets(orderby=orderby, desc=desc)
        assert {dataset.id for dataset in datasets} == set(times)
        values = [times[dataset.id] for dataset in datasets]
        assert values == sorted(values, reverse=desc)

    @pytest.mark.p3
    @pytest.mark.parametrize("orderby", ["", "unknown", "CREATE_TIME", "UPDATE_TIME", " create_time "])
    def test_orderby_invalid(self, client, orderby):
        with pytest.raises(Exception, match="Input should be 'create_time' or 'update_time'"):
            client.list_datasets(orderby=orderby)

    @pytest.mark.p1
    def test_name(self, client, add_datasets):
        target = add_datasets[1]
        datasets = client.list_datasets(name=target.name)
        assert [(dataset.id, dataset.name) for dataset in datasets] == [(target.id, target.name)]

    @pytest.mark.p2
    def test_name_wrong(self, client):
        assert client.list_datasets(name=f"missing-{uuid.uuid4().hex}") == []

    @pytest.mark.p2
    @pytest.mark.parametrize("name", ["", None])
    def test_name_empty(self, client, add_datasets, name):
        assert {dataset.id for dataset in client.list_datasets(name=name)} == {dataset.id for dataset in add_datasets}

    @pytest.mark.p1
    def test_id(self, client, add_datasets):
        datasets = client.list_datasets(id=add_datasets[0].id)
        assert [dataset.id for dataset in datasets] == [add_datasets[0].id]

    @pytest.mark.p2
    def test_id_not_uuid(self, client):
        with pytest.raises(Exception, match="^Invalid UUID format$"):
            client.list_datasets(id="not_uuid")

    @pytest.mark.p2
    @pytest.mark.parametrize("dataset_id", [uuid.uuid4().hex, "d94a8dc02c9711f0930f7fbc369eab6d"], ids=["uuid4", "missing_uuid1"])
    def test_id_wrong_uuid(self, client, dataset_id):
        with pytest.raises(Exception, match="^Resource not found$"):
            client.list_datasets(id=dataset_id)

    @pytest.mark.p2
    def test_id_empty(self, client):
        with pytest.raises(Exception, match="^Invalid UUID format$"):
            client.list_datasets(id="")

    @pytest.mark.p2
    def test_id_none(self, client, add_datasets):
        assert {dataset.id for dataset in client.list_datasets(id=None)} == {dataset.id for dataset in add_datasets}

    @pytest.mark.p1
    @pytest.mark.parametrize("matching_name", [True, False])
    def test_name_and_id(self, client, add_datasets, matching_name):
        target = add_datasets[0]
        name = target.name if matching_name else add_datasets[1].name
        datasets = client.list_datasets(id=target.id, name=name)
        assert [dataset.id for dataset in datasets] == ([target.id] if matching_name else [])

    @pytest.mark.p3
    @pytest.mark.parametrize("missing_id", [False, True])
    def test_name_and_id_wrong(self, client, add_datasets, missing_id):
        if missing_id:
            with pytest.raises(Exception, match="^Resource not found$"):
                client.list_datasets(id=uuid.uuid4().hex, name=add_datasets[0].name)
        else:
            assert client.list_datasets(id=add_datasets[0].id, name=f"missing-{uuid.uuid4().hex}") == []

    @pytest.mark.p1
    def test_ids_filter(self, client, add_datasets):
        expected = {dataset.id for dataset in add_datasets[:2]}
        datasets = client.list_datasets(ids=list(expected))
        assert len(datasets) == len(expected)
        assert {dataset.id for dataset in datasets} == expected

    @pytest.mark.p2
    def test_ids_missing_are_filtered(self, client, add_datasets):
        datasets = client.list_datasets(ids=[add_datasets[0].id, uuid.uuid4().hex])
        assert [dataset.id for dataset in datasets] == [add_datasets[0].id]
        assert client.list_datasets(ids=[uuid.uuid4().hex]) == []

    @pytest.mark.p2
    def test_ids_invalid(self, client):
        with pytest.raises(Exception, match="^Invalid UUID format$"):
            client.list_datasets(ids=["not_uuid"])

    @pytest.mark.p2
    def test_ids_duplicates(self, client, add_datasets):
        dataset_id = add_datasets[0].id
        with pytest.raises(Exception, match="Duplicate ids:"):
            client.list_datasets(ids=[dataset_id, dataset_id])
