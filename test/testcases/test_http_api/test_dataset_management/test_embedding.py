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
from common import run_embedding


@pytest.mark.usefixtures("clear_datasets")
class TestRunEmbedding:
    @pytest.mark.p2
    def test_run_embedding_no_documents(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = run_embedding(HttpApiAuth, dataset_id)
        assert res["code"] == 102, res
        assert "No documents in Dataset" in res.get("message", ""), res

    @pytest.mark.p2
    def test_run_embedding_invalid_id(self, HttpApiAuth):
        res = run_embedding(HttpApiAuth, "invalid_id")
        assert res["code"] != 0, res
