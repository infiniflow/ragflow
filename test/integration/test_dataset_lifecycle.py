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
Dataset lifecycle integration tests.

Consolidates tests from:
- testcases/test_http_api/test_dataset_management/test_create_dataset.py
- testcases/test_http_api/test_dataset_management/test_list_datasets.py
- testcases/test_http_api/test_dataset_management/test_update_dataset.py
- testcases/test_http_api/test_dataset_management/test_delete_datasets.py

Tests the complete workflow: create → list → update → delete

Per AGENTS.md modularization: Tests service layer logic (kb_app)
not HTTP/SDK/Web endpoint structure.
"""

import pytest
from configs import INVALID_API_TOKEN
from hypothesis import example, given, settings
from libs.auth import RAGFlowHttpApiAuth
from testcases.test_http_api.common import (
    create_dataset,
    delete_datasets,
    list_datasets,
    update_dataset,
)
from utils.hypothesis_utils import valid_names


@pytest.mark.usefixtures("clear_datasets")
class TestDatasetAuthorization:
    """Test dataset authorization and authentication."""

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
        ids=["empty_auth", "invalid_api_token"],
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        """Test that invalid authentication is rejected."""
        res = create_dataset(invalid_auth, {"name": "auth_test"})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("clear_datasets")
class TestDatasetCRUD:
    """Test dataset CRUD operations (Create, Read, Update, Delete)."""

    @pytest.mark.p1
    def test_dataset_creation(self, api_client):
        """Test creating a dataset."""
        dataset_name = "test_dataset_create"

        # Create dataset
        res = create_dataset(api_client, {"name": dataset_name})
        assert res["code"] == 0, f"Failed to create dataset: {res}"

        # Verify response structure
        assert "data" in res
        assert "dataset_id" in res["data"]
        assert "name" in res["data"]
        dataset_id = res["data"]["dataset_id"]
        assert dataset_id is not None

    @pytest.mark.p1
    def test_dataset_list(self, api_client):
        """Test listing datasets."""
        # Create multiple datasets
        names = ["dataset_1", "dataset_2", "dataset_3"]
        created_ids = []

        for name in names:
            res = create_dataset(api_client, {"name": name})
            assert res["code"] == 0
            created_ids.append(res["data"]["dataset_id"])

        # List datasets
        res = list_datasets(api_client)
        assert res["code"] == 0
        assert "data" in res

        # Verify created datasets in list
        list_ids = [d["id"] for d in res["data"]]
        for dataset_id in created_ids:
            assert dataset_id in list_ids, f"Dataset {dataset_id} not found in list"

    @pytest.mark.p1
    def test_dataset_update(self, api_client):
        """Test updating dataset metadata."""
        # Create dataset
        original_name = "test_dataset_original"
        res = create_dataset(api_client, {"name": original_name})
        dataset_id = res["data"]["dataset_id"]

        # Update dataset
        new_name = "test_dataset_updated"
        new_description = "Updated description"
        res = update_dataset(api_client, dataset_id, {
            "name": new_name,
            "description": new_description
        })
        assert res["code"] == 0, f"Failed to update dataset: {res}"

        # Verify update
        res = list_datasets(api_client)
        assert res["code"] == 0
        updated_dataset = next(
            (d for d in res["data"] if d["id"] == dataset_id),
            None
        )
        assert updated_dataset is not None
        assert updated_dataset["name"] == new_name

    @pytest.mark.p1
    def test_dataset_delete(self, api_client):
        """Test deleting a dataset."""
        # Create dataset
        res = create_dataset(api_client, {"name": "test_dataset_delete"})
        dataset_id = res["data"]["dataset_id"]

        # Verify dataset exists
        res = list_datasets(api_client)
        list_ids = [d["id"] for d in res["data"]]
        assert dataset_id in list_ids

        # Delete dataset
        res = delete_datasets(api_client, [dataset_id])
        assert res["code"] == 0, f"Failed to delete dataset: {res}"

        # Verify dataset is deleted
        res = list_datasets(api_client)
        list_ids = [d["id"] for d in res["data"]]
        assert dataset_id not in list_ids

    @pytest.mark.p1
    def test_complete_lifecycle(self, api_client):
        """Test complete dataset lifecycle: create → list → update → delete."""
        dataset_name = "test_complete_lifecycle"

        # 1. Create
        res = create_dataset(api_client, {"name": dataset_name})
        assert res["code"] == 0
        dataset_id = res["data"]["dataset_id"]

        # 2. List and verify
        res = list_datasets(api_client)
        assert res["code"] == 0
        dataset_ids = [d["id"] for d in res["data"]]
        assert dataset_id in dataset_ids

        # 3. Update
        res = update_dataset(api_client, dataset_id, {"name": "updated_name"})
        assert res["code"] == 0

        # 4. List and verify update
        res = list_datasets(api_client)
        updated = next((d for d in res["data"] if d["id"] == dataset_id), None)
        assert updated is not None, f"Dataset {dataset_id} not found in list"
        assert updated["name"] == "updated_name"

        # 5. Delete
        res = delete_datasets(api_client, [dataset_id])
        assert res["code"] == 0

        # 6. List and verify deletion
        res = list_datasets(api_client)
        dataset_ids = [d["id"] for d in res["data"]]
        assert dataset_id not in dataset_ids


@pytest.mark.usefixtures("clear_datasets")
class TestDatasetNameValidation:
    """Test dataset name validation."""

    @pytest.mark.p2
    @given(name=valid_names())
    @example("a" * 128)
    @settings(max_examples=10)
    def test_valid_names(self, api_client, name):
        """Test creating datasets with valid names."""
        res = create_dataset(api_client, {"name": name})
        assert res["code"] == 0, f"Failed to create dataset with name: {name}"

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_name",
        [
            "",  # Empty name
            " " * 10,  # Only whitespace
        ],
        ids=["empty_name", "whitespace_only"],
    )
    def test_invalid_names(self, api_client, invalid_name):
        """Test that invalid names are rejected."""
        res = create_dataset(api_client, {"name": invalid_name})
        assert res["code"] != 0, f"Should reject invalid name: {invalid_name}"


class TestDatasetDocumentWorkflow:
    """Test dataset → document → chunk workflow."""

    @pytest.mark.p1
    @pytest.mark.skip(reason="Requires document parsing service")
    def test_document_upload_and_parse(self, api_client):
        """Test uploading and parsing documents in a dataset."""
        # TODO: Implement after document service stabilizes
        # - Create dataset
        # - Upload document
        # - Verify parse status
        # - Retrieve parsed chunks
        pass

    @pytest.mark.p2
    @pytest.mark.skip(reason="Requires chunk retrieval service")
    def test_chunk_retrieval(self, api_client):
        """Test retrieving chunks from parsed documents."""
        # TODO: Implement after chunk service stabilizes
        # - Create dataset with documents
        # - Parse documents
        # - Retrieve chunks with query
        # - Verify chunk relevance
        pass
