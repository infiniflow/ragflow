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

from common import create_dataset, list_dataset, rm_dataset, update_dataset, DATASET_NAME_LIMIT
import re
import random
import string


def test_dataset(get_auth):
    # create dataset
    res = create_dataset(get_auth, "test_create_dataset")
    assert res.get("code") == 0, f"{res.get('message')}"

    # list dataset
    page_number = 1
    dataset_list = []
    while True:
        res = list_dataset(get_auth, page_number)
        data = res.get("data").get("kbs")
        for item in data:
            dataset_id = item.get("id")
            dataset_list.append(dataset_id)
        if len(dataset_list) < page_number * 150:
            break
        page_number += 1

    print(f"found {len(dataset_list)} datasets")
    # delete dataset
    for dataset_id in dataset_list:
        res = rm_dataset(get_auth, dataset_id)
        assert res.get("code") == 0, f"{res.get('message')}"
    print(f"{len(dataset_list)} datasets are deleted")


def test_dataset_1k_dataset(get_auth):
    # create dataset
    for i in range(1000):
        res = create_dataset(get_auth, f"test_create_dataset_{i}")
        assert res.get("code") == 0, f"{res.get('message')}"

    # list dataset
    page_number = 1
    dataset_list = []
    while True:
        res = list_dataset(get_auth, page_number)
        data = res.get("data").get("kbs")
        for item in data:
            dataset_id = item.get("id")
            dataset_list.append(dataset_id)
        if len(dataset_list) < page_number * 150:
            break
        page_number += 1

    print(f"found {len(dataset_list)} datasets")
    # delete dataset
    for dataset_id in dataset_list:
        res = rm_dataset(get_auth, dataset_id)
        assert res.get("code") == 0, f"{res.get('message')}"
    print(f"{len(dataset_list)} datasets are deleted")


def test_duplicated_name_dataset(get_auth):
    # create dataset
    for i in range(20):
        res = create_dataset(get_auth, "test_create_dataset")
        assert res.get("code") == 0, f"{res.get('message')}"

    # list dataset
    res = list_dataset(get_auth, 1)
    data = res.get("data").get("kbs")
    dataset_list = []
    pattern = r'^test_create_dataset.*'
    for item in data:
        dataset_name = item.get("name")
        dataset_id = item.get("id")
        dataset_list.append(dataset_id)
        match = re.match(pattern, dataset_name)
        assert match is not None

    for dataset_id in dataset_list:
        res = rm_dataset(get_auth, dataset_id)
        assert res.get("code") == 0, f"{res.get('message')}"
    print(f"{len(dataset_list)} datasets are deleted")


def test_invalid_name_dataset(get_auth):
    # create dataset
    # with pytest.raises(Exception) as e:
    res = create_dataset(get_auth, 0)
    assert res['code'] == 102

    res = create_dataset(get_auth, "")
    assert res['code'] == 102

    long_string = ""

    while len(long_string.encode("utf-8")) <= DATASET_NAME_LIMIT:
        long_string += random.choice(string.ascii_letters + string.digits)

    res = create_dataset(get_auth, long_string)
    assert res['code'] == 102
    print(res)


def test_update_different_params_dataset_success(get_auth):
    # create dataset
    res = create_dataset(get_auth, "test_create_dataset")
    assert res.get("code") == 0, f"{res.get('message')}"

    # list dataset
    page_number = 1
    dataset_list = []
    while True:
        res = list_dataset(get_auth, page_number)
        data = res.get("data").get("kbs")
        for item in data:
            dataset_id = item.get("id")
            dataset_list.append(dataset_id)
        if len(dataset_list) < page_number * 150:
            break
        page_number += 1

    print(f"found {len(dataset_list)} datasets")
    dataset_id = dataset_list[0]

    json_req = {"kb_id": dataset_id, "name": "test_update_dataset", "description": "test", "permission": "me",
                "parser_id": "presentation",
                "language": "spanish"}
    res = update_dataset(get_auth, json_req)
    assert res.get("code") == 0, f"{res.get('message')}"

    # delete dataset
    for dataset_id in dataset_list:
        res = rm_dataset(get_auth, dataset_id)
        assert res.get("code") == 0, f"{res.get('message')}"
    print(f"{len(dataset_list)} datasets are deleted")


# update dataset with different parameters
def test_update_different_params_dataset_fail(get_auth):
    # create dataset
    res = create_dataset(get_auth, "test_create_dataset")
    assert res.get("code") == 0, f"{res.get('message')}"

    # list dataset
    page_number = 1
    dataset_list = []
    while True:
        res = list_dataset(get_auth, page_number)
        data = res.get("data").get("kbs")
        for item in data:
            dataset_id = item.get("id")
            dataset_list.append(dataset_id)
        if len(dataset_list) < page_number * 150:
            break
        page_number += 1

    print(f"found {len(dataset_list)} datasets")
    dataset_id = dataset_list[0]

    json_req = {"kb_id": dataset_id, "id": "xxx"}
    res = update_dataset(get_auth, json_req)
    assert res.get("code") == 101

    # delete dataset
    for dataset_id in dataset_list:
        res = rm_dataset(get_auth, dataset_id)
        assert res.get("code") == 0, f"{res.get('message')}"
    print(f"{len(dataset_list)} datasets are deleted")
