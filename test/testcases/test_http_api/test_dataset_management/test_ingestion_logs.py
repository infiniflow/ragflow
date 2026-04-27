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
from common import list_ingestion_logs, get_ingestion_log


@pytest.mark.usefixtures("clear_datasets")
class TestListIngestionLogs:
    @pytest.mark.p2
    def test_list_ingestion_logs_success(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = list_ingestion_logs(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res
        assert "total" in res["data"], res
        assert "logs" in res["data"], res

    @pytest.mark.p2
    def test_list_ingestion_logs_with_pagination(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = list_ingestion_logs(HttpApiAuth, dataset_id, params={"page": 1, "page_size": 10})
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_list_ingestion_logs_invalid_id(self, HttpApiAuth):
        res = list_ingestion_logs(HttpApiAuth, "invalid_id")
        assert res["code"] != 0, res


@pytest.mark.usefixtures("clear_datasets")
class TestGetIngestionLog:
    @pytest.mark.p2
    def test_get_ingestion_log_not_found(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = get_ingestion_log(HttpApiAuth, dataset_id, "nonexistent_log_id")
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_get_ingestion_log_invalid_dataset(self, HttpApiAuth):
        res = get_ingestion_log(HttpApiAuth, "invalid_id", "some_log_id")
        assert res["code"] != 0, res


@pytest.mark.usefixtures("clear_datasets")
class TestListIngestionLogsEdgeCases:
    @pytest.mark.p3
    def test_list_ingestion_logs_missing_dataset_id(self, HttpApiAuth):
        """Test list ingestion logs without providing dataset_id"""
        res = list_ingestion_logs(HttpApiAuth, "")
        assert res["code"] != 0, res

    @pytest.mark.p3
    def test_list_ingestion_logs_abnormal_date_filter(self, HttpApiAuth, add_dataset_func):
        """Test list ingestion logs with date range that has no matching records.

        The API returns an error when the date filter produces an abnormal result
        (i.e., create_date_from > create_date_to or range with no data).
        """
        dataset_id = add_dataset_func
        res = list_ingestion_logs(
            HttpApiAuth,
            dataset_id,
            params={
                "desc": "false",
                "create_date_from": "2025-01-01",
                "create_date_to": "2025-02-01",
            },
        )
        assert res["code"] in [0, 102], res
