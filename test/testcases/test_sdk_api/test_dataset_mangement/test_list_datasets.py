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
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from configs import HOST_ADDRESS, INVALID_API_TOKEN
from ragflow_sdk import RAGFlow


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_message",
        [
            (None, "Authentication error: API key is invalid!"),
            (INVALID_API_TOKEN, "Authentication error: API key is invalid!"),
        ],
    )
    def test_auth_invalid(self, invalid_auth, expected_message):
        client = RAGFlow(invalid_auth, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            client.list_datasets()
        assert expected_message in str(exception_info.value)


class TestCapability:
    @pytest.mark.p3
    def test_concurrent_list(self, client):
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    client.list_datasets,
                )
                for _ in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses


@pytest.mark.usefixtures("add_datasets")
class TestDatasetsList:
    @pytest.mark.p2
    def test_params_unset(self, client):
        datasets = client.list_datasets()
        assert len(datasets) == 5, str(datasets)

    @pytest.mark.p2
    def test_params_empty(self, client):
        datasets = client.list_datasets(**{})
        assert len(datasets) == 5, str(datasets)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page": 2, "page_size": 2}, 2),
            ({"page": 3, "page_size": 2}, 1),
            ({"page": 4, "page_size": 2}, 0),
            ({"page": 1, "page_size": 10}, 5),
        ],
        ids=["normal_middle_page", "normal_last_partial_page", "beyond_max_page", "full_data_single_page"],
    )
    def test_page(self, client, params, expected_page_size):
        datasets = client.list_datasets(**params)
        assert len(datasets) == expected_page_size, str(datasets)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_message",
        [
            ({"page": 0}, "Input should be greater than or equal to 1"),
            ({"page": "a"}, "not instance of"),
        ],
        ids=["page_0", "page_a"],
    )
    def test_page_invalid(self, client, params, expected_message):
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_page_none(self, client):
        params = {"page": None}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "not instance of" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page_size": 1}, 1),
            ({"page_size": 3}, 3),
            ({"page_size": 5}, 5),
            ({"page_size": 6}, 5),
        ],
        ids=["min_valid_page_size", "medium_page_size", "page_size_equals_total", "page_size_exceeds_total"],
    )
    def test_page_size(self, client, params, expected_page_size):
        datasets = client.list_datasets(**params)
        assert len(datasets) == expected_page_size, str(datasets)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_message",
        [
            ({"page_size": 0}, "Input should be greater than or equal to 1"),
            ({"page_size": "a"}, "not instance of"),
        ],
    )
    def test_page_size_invalid(self, client, params, expected_message):
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert expected_message in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_page_size_none(self, client):
        params = {"page_size": None}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "not instance of" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params",
        [
            {"orderby": "create_time"},
            {"orderby": "update_time"},
        ],
        ids=["orderby_create_time", "orderby_update_time"],
    )
    def test_orderby(self, client, params):
        client.list_datasets(**params)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params",
        [
            {"orderby": ""},
            {"orderby": "unknown"},
            {"orderby": "CREATE_TIME"},
            {"orderby": "UPDATE_TIME"},
            {"orderby": " create_time "},
        ],
        ids=["empty", "unknown", "orderby_create_time_upper", "orderby_update_time_upper", "whitespace"],
    )
    def test_orderby_invalid(self, client, params):
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "Input should be 'create_time' or 'update_time'" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_orderby_none(self, client):
        params = {"orderby": None}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "not instance of" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params",
        [
            {"desc": True},
            {"desc": False},
        ],
        ids=["desc=True", "desc=False"],
    )
    def test_desc(self, client, params):
        client.list_datasets(**params)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params",
        [
            {"desc": 3.14},
            {"desc": "unknown"},
        ],
        ids=["float_value", "invalid_string"],
    )
    def test_desc_invalid(self, client, params):
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "not instance of" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_desc_none(self, client):
        params = {"desc": None}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "not instance of" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p1
    def test_name(self, client):
        params = {"name": "dataset_1"}
        datasets = client.list_datasets(**params)
        assert len(datasets) == 1, str(datasets)
        assert datasets[0].name == "dataset_1", str(datasets)

    @pytest.mark.p2
    def test_name_wrong(self, client):
        params = {"name": "wrong name"}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "lacks permission for dataset" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_name_empty(self, client):
        params = {"name": ""}
        datasets = client.list_datasets(**params)
        assert len(datasets) == 5, str(datasets)

    @pytest.mark.p2
    def test_name_none(self, client):
        params = {"name": None}
        datasets = client.list_datasets(**params)
        assert len(datasets) == 5, str(datasets)

    @pytest.mark.p1
    def test_id(self, client, add_datasets):
        dataset_ids = [dataset.id for dataset in add_datasets]
        params = {"id": dataset_ids[0]}
        datasets = client.list_datasets(**params)
        assert len(datasets) == 1, str(datasets)
        assert datasets[0].id == dataset_ids[0], str(datasets)

    @pytest.mark.p2
    def test_id_not_uuid(self, client):
        params = {"id": "not_uuid"}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "Invalid UUID1 format" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_id_not_uuid1(self, client):
        params = {"id": uuid.uuid4().hex}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "Invalid UUID1 format" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_id_wrong_uuid(self, client):
        params = {"id": "d94a8dc02c9711f0930f7fbc369eab6d"}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "lacks permission for dataset" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_id_empty(self, client):
        params = {"id": ""}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "Invalid UUID1 format" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p2
    def test_id_none(self, client):
        params = {"id": None}
        datasets = client.list_datasets(**params)
        assert len(datasets) == 5, str(datasets)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "func, name, expected_num",
        [
            (lambda r: r[0].id, "dataset_0", 1),
            (lambda r: r[0].id, "dataset_1", 0),
        ],
        ids=["name_and_id_match", "name_and_id_mismatch"],
    )
    def test_name_and_id(self, client, add_datasets, func, name, expected_num):
        params = None
        if callable(func):
            params = {"id": func(add_datasets), "name": name}
        datasets = client.list_datasets(**params)
        assert len(datasets) == expected_num, str(datasets)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "dataset_id, name",
        [
            (lambda r: r[0].id, "wrong_name"),
            (uuid.uuid1().hex, "dataset_0"),
        ],
        ids=["name", "id"],
    )
    def test_name_and_id_wrong(self, client, add_datasets, dataset_id, name):
        if callable(dataset_id):
            params = {"id": dataset_id(add_datasets), "name": name}
        else:
            params = {"id": dataset_id, "name": name}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "lacks permission for dataset" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_field_unsupported(self, client):
        params = {"unknown_field": "unknown_field"}
        with pytest.raises(Exception) as exception_info:
            client.list_datasets(**params)
        assert "got an unexpected keyword argument" in str(exception_info.value), str(exception_info.value)
