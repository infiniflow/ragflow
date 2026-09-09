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
from common import get_dataset
from libs.auth import RAGFlowHttpApiAuth
from configs import INVALID_API_TOKEN


@pytest.mark.usefixtures("clear_datasets")
class TestGetDataset:
    @pytest.mark.p2
    def test_get_dataset_success(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = get_dataset(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res
        assert res["data"]["id"] == dataset_id, res

    @pytest.mark.p2
    def test_get_dataset_invalid_id(self, HttpApiAuth):
        res = get_dataset(HttpApiAuth, "invalid_dataset_id")
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_get_dataset_unauthorized(self, add_dataset_func):
        dataset_id = add_dataset_func
        res = get_dataset(RAGFlowHttpApiAuth(INVALID_API_TOKEN), dataset_id)
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_get_dataset_nonexistent(self, HttpApiAuth):
        res = get_dataset(HttpApiAuth, "0" * 32)
        assert res["code"] != 0, res
