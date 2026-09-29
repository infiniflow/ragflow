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
from common import get_flattened_metadata


@pytest.mark.usefixtures("clear_datasets")
class TestFlattenedMetadata:
    @pytest.mark.p2
    def test_get_flattened_metadata_success(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = get_flattened_metadata(HttpApiAuth, [dataset_id])
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_get_flattened_metadata_multiple_datasets(self, HttpApiAuth, add_datasets_func):
        dataset_ids = add_datasets_func
        res = get_flattened_metadata(HttpApiAuth, dataset_ids)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_get_flattened_metadata_empty_ids(self, HttpApiAuth):
        res = get_flattened_metadata(HttpApiAuth, [])
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_get_flattened_metadata_invalid_id(self, HttpApiAuth):
        res = get_flattened_metadata(HttpApiAuth, ["invalid_id"])
        assert res["code"] != 0, res
