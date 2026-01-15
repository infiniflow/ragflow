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
"""
Tests for dataset API contract validation.
"""
import pytest


@pytest.mark.usefixtures("clear_datasets")
class TestDatasetApiContract:
    """Validate dataset endpoint contracts."""

    @pytest.mark.p1
    def test_create_dataset_response_schema(self, api_client):
        """Verify create_dataset response has required fields."""
        res = api_client.create_dataset({"name": "contract_test"})

        # Response schema contract
        assert "code" in res
        assert "message" in res
        assert "data" in res

        # Assert success before checking success fields
        assert res["code"] == 0, f"Dataset creation failed: {res}"

        # Success response contract
        data = res["data"]
        assert "dataset_id" in data
        assert "name" in data
        assert "description" in data

    @pytest.mark.p1
    def test_list_datasets_response_schema(self, api_client):
        """Verify list_datasets response has required fields."""
        res = api_client.list_datasets()

        # Response schema contract
        assert "code" in res
        assert "message" in res
        assert "data" in res

        # Assert success before checking success fields
        assert res["code"] == 0, f"List datasets failed: {res}"

        # Success response contract
        data = res["data"]
        assert isinstance(data, list)
        if data:
            dataset = data[0]
            assert "id" in dataset
            assert "name" in dataset
