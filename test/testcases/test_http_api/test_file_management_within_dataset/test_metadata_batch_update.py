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
End-to-end tests for metadata batch update API.

This test file converts the unit test test_metadata_batch_update from test_doc_sdk_routes_unit.py
to end-to-end tests that call the actual HTTP API.
"""
import pytest
from common import (
    update_documents_metadata,
    list_documents,
    delete_documents,
    upload_documents,
    create_dataset,
    delete_datasets,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


def _create_and_upload_in_batches(auth, dataset_id, num_docs, tmp_path, batch_size=100):
    """Create and upload documents in batches to avoid too many open files."""
    document_ids = []
    for batch_start in range(0, num_docs, batch_size):
        batch_end = min(batch_start + batch_size, num_docs)
        fps = []
        for i in range(batch_start, batch_end):
            fp = tmp_path / f"ragflow_test_upload_{i}.txt"
            fp.write_text(f"Test document content {i}\n" * 10)
            fps.append(fp)
        res = upload_documents(auth, dataset_id, fps)
        for doc in res["data"]:
            document_ids.append(doc["id"])
    return document_ids


@pytest.fixture(scope="class")
def dataset_with_docs(request, HttpApiAuth, add_dataset, ragflow_tmp_dir):
    """Create a dataset with test documents and clean up after test class."""
    dataset_id = add_dataset

    # Upload test documents
    fps = []
    for i in range(5):
        fp = ragflow_tmp_dir / f"test_doc_{i}.txt"
        fp.write_text(f"Test document content {i}\n" * 10)
        fps.append(fp)

    upload_res = upload_documents(HttpApiAuth, dataset_id, fps)
    assert upload_res["code"] == 0, f"Failed to upload documents: {upload_res}"

    document_ids = [doc["id"] for doc in upload_res["data"]]

    def cleanup():
        delete_documents(HttpApiAuth, dataset_id, {"ids": document_ids})

    request.addfinalizer(cleanup)

    return dataset_id, document_ids


@pytest.mark.p3
class TestMetadataBatchUpdate:
    def test_batch_update_metadata(self, HttpApiAuth, add_dataset, ragflow_tmp_dir):
        """
        Test batch_update_metadata via HTTP API.
        This test calls the real batch_update_metadata on the server.
        """
        dataset_id = add_dataset

        # Upload documents in batches to avoid too many open files
        document_ids = _create_and_upload_in_batches(HttpApiAuth, dataset_id, 1010, ragflow_tmp_dir)

        # Update metadata via batch update API
        updates = [{"key": "author", "value": "new_author"}, {"key": "status", "value": "processed"}]
        res = update_documents_metadata(HttpApiAuth, dataset_id, {"selector": {"document_ids": document_ids}, "updates": updates})

        # Verify the API call succeeded
        assert res["code"] == 0, f"Expected code 0, got {res.get('code')}: {res.get('message')}"
        assert res["data"]["updated"] == 1010, f"Expected 1100 documents updated, got {res['data']['updated']}"

        # Verify metadata was updated for first and last few sample documents
        sample_ids = document_ids[:5] + document_ids[-5:]
        list_res = list_documents(HttpApiAuth, dataset_id, {"ids": sample_ids})
        assert list_res["code"] == 0

        for doc in list_res["data"]["docs"]:
            assert doc["meta_fields"].get("author") == "new_author", f"Expected author='new_author', got {doc['meta_fields'].get('author')}"
            assert doc["meta_fields"].get("status") == "processed", f"Expected status='processed', got {doc['meta_fields'].get('status')}"

        # Cleanup
        delete_documents(HttpApiAuth, dataset_id, {"ids": document_ids})


@pytest.mark.p2
class TestMetadataBatchUpdateValidation:
    """Test validation scenarios for metadata batch update API."""

    def test_invalid_auth(self):
        """Test that invalid authentication returns 401."""
        res = update_documents_metadata(
            RAGFlowHttpApiAuth(INVALID_API_TOKEN),
            "dataset_id",
            {"selector": {"document_ids": []}, "updates": [], "deletes": []},
        )
        assert res["code"] == 401

    def test_invalid_dataset_id(self, HttpApiAuth):
        """Test that invalid dataset ID returns error."""
        res = update_documents_metadata(
            HttpApiAuth,
            "invalid_dataset_id",
            {"selector": {"document_ids": []}, "updates": [], "deletes": []},
        )
        assert res["code"] == 102
        assert "You don't own the dataset" in res["message"]

    def test_selector_not_object(self, HttpApiAuth, dataset_with_docs):
        """Test that selector must be an object."""
        dataset_id, _ = dataset_with_docs

        # Pass selector as a list instead of object
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": [1], "updates": [], "deletes": []},
        )
        assert res["code"] == 100
        assert "selector must be an object" in res["message"]

    def test_updates_and_deletes_must_be_lists(self, HttpApiAuth, dataset_with_docs):
        """Test that updates and deletes must be lists."""
        dataset_id, _ = dataset_with_docs

        # Pass updates and deletes as objects instead of lists
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {}, "updates": {"key": "value"}, "deletes": []},
        )
        assert res["code"] == 100
        assert "updates and deletes must be lists" in res["message"]

    def test_metadata_condition_must_be_object(self, HttpApiAuth, dataset_with_docs):
        """Test that metadata_condition must be an object."""
        dataset_id, _ = dataset_with_docs

        # Pass metadata_condition as a list instead of object
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"metadata_condition": [1]}, "updates": [], "deletes": []},
        )
        assert res["code"] == 100
        assert "metadata_condition must be an object" in res["message"]

    def test_document_ids_must_be_list(self, HttpApiAuth, dataset_with_docs):
        """Test that document_ids must be a list."""
        dataset_id, _ = dataset_with_docs

        # Pass document_ids as a string instead of list
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"document_ids": "doc-1"}, "updates": [], "deletes": []},
        )
        assert res["code"] == 100
        assert "document_ids must be a list" in res["message"]

    def test_each_update_requires_key_and_value(self, HttpApiAuth, dataset_with_docs):
        """Test that each update requires key and value."""
        dataset_id, _ = dataset_with_docs

        # Pass update without key
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {}, "updates": [{"key": ""}], "deletes": []},
        )
        assert res["code"] == 100
        assert "Each update requires key and value" in res["message"]

    def test_each_delete_requires_key(self, HttpApiAuth, dataset_with_docs):
        """Test that each delete requires key."""
        dataset_id, _ = dataset_with_docs

        # Pass delete without key
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {}, "updates": [], "deletes": [{"x": "y"}]},
        )
        assert res["code"] == 100
        assert "Each delete requires key" in res["message"]

    def test_documents_not_belong_to_dataset(self, HttpApiAuth, dataset_with_docs):
        """Test that documents must belong to the dataset."""
        dataset_id, _ = dataset_with_docs

        # Pass document IDs that don't belong to the dataset
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {
                "selector": {"document_ids": ["doc-does-not-exist-1", "doc-does-not-exist-2"]},
                "updates": [{"key": "author", "value": "test"}],
                "deletes": [],
            },
        )
        assert res["code"] == 100
        assert "do not belong to dataset" in res["message"]


@pytest.mark.p2
class TestMetadataBatchUpdateSuccess:
    """Test successful scenarios for metadata batch update API."""

    def test_batch_update_by_document_ids(self, HttpApiAuth, dataset_with_docs):
        """Test batch update metadata by document IDs."""
        dataset_id, document_ids = dataset_with_docs

        # Update metadata for specific documents
        updates = [{"key": "author", "value": "test_author"}, {"key": "status", "value": "processed"}]
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"document_ids": document_ids}, "updates": updates, "deletes": []},
        )

        assert res["code"] == 0, f"Expected code 0, got {res.get('code')}: {res.get('message')}"
        assert res["data"]["updated"] == 5
        assert res["data"]["matched_docs"] == 5

        # Verify metadata was updated
        list_res = list_documents(HttpApiAuth, dataset_id, {"ids": document_ids})
        assert list_res["code"] == 0

        for doc in list_res["data"]["docs"]:
            assert doc["meta_fields"].get("author") == "test_author"
            assert doc["meta_fields"].get("status") == "processed"

    def test_batch_update_with_metadata_condition(self, HttpApiAuth, dataset_with_docs):
        """Test batch update metadata using metadata_condition filter."""
        dataset_id, document_ids = dataset_with_docs

        # First, set initial metadata
        updates = [{"key": "category", "value": "test_category"}]
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"document_ids": document_ids}, "updates": updates, "deletes": []},
        )
        assert res["code"] == 0

        # Now update only documents with category="test_category"
        updates = [{"key": "author", "value": "filtered_author"}]
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {
                "selector": {
                    "document_ids": document_ids,
                    "metadata_condition": {"conditions": [{"key": "category", "value": ["test_category"]}]},
                },
                "updates": updates,
                "deletes": [],
            },
        )

        assert res["code"] == 0, f"Expected code 0, got {res.get('code')}: {res.get('message')}"
        assert res["data"]["updated"] == 5
        assert res["data"]["matched_docs"] == 5

    def test_batch_delete_metadata(self, HttpApiAuth, dataset_with_docs):
        """Test batch delete metadata keys."""
        dataset_id, document_ids = dataset_with_docs

        # First, set some metadata
        updates = [{"key": "author", "value": "test_author"}, {"key": "status", "value": "processed"}]
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"document_ids": document_ids}, "updates": updates, "deletes": []},
        )
        assert res["code"] == 0

        # Now delete the "author" key
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"document_ids": document_ids}, "updates": [], "deletes": [{"key": "author"}]},
        )

        assert res["code"] == 0, f"Expected code 0, got {res.get('code')}: {res.get('message')}"
        assert res["data"]["updated"] == 5

        # Verify author was deleted but status remains
        list_res = list_documents(HttpApiAuth, dataset_id, {"ids": document_ids})
        assert list_res["code"] == 0

        for doc in list_res["data"]["docs"]:
            assert "author" not in doc["meta_fields"] or doc["meta_fields"].get("author") is None
            assert doc["meta_fields"].get("status") == "processed"

    def test_batch_update_and_delete_combined(self, HttpApiAuth, dataset_with_docs):
        """Test batch update and delete metadata in same request."""
        dataset_id, document_ids = dataset_with_docs

        # First, set initial metadata
        updates = [{"key": "author", "value": "old_author"}, {"key": "status", "value": "old_status"}]
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"document_ids": document_ids}, "updates": updates, "deletes": []},
        )
        assert res["code"] == 0

        # Now update and delete in same request
        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {
                "selector": {"document_ids": document_ids},
                "updates": [{"key": "author", "value": "new_author"}],
                "deletes": [{"key": "status"}],
            },
        )

        assert res["code"] == 0, f"Expected code 0, got {res.get('code')}: {res.get('message')}"
        assert res["data"]["updated"] == 5

        # Verify the changes
        list_res = list_documents(HttpApiAuth, dataset_id, {"ids": document_ids})
        assert list_res["code"] == 0

        for doc in list_res["data"]["docs"]:
            assert doc["meta_fields"].get("author") == "new_author"
            assert "status" not in doc["meta_fields"] or doc["meta_fields"].get("status") is None

    def test_update_with_empty_document_ids(self, HttpApiAuth, dataset_with_docs):
        """Test that empty document_ids returns success with 0 matched."""
        dataset_id, _ = dataset_with_docs

        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {"selector": {"document_ids": []}, "updates": [{"key": "author", "value": "test"}], "deletes": []},
        )

        assert res["code"] == 0
        assert res["data"]["updated"] == 0
        assert res["data"]["matched_docs"] == 0

    def test_update_with_no_matching_metadata_condition(self, HttpApiAuth, dataset_with_docs):
        """Test that non-matching metadata_condition returns 0 matched."""
        dataset_id, document_ids = dataset_with_docs

        res = update_documents_metadata(
            HttpApiAuth,
            dataset_id,
            {
                "selector": {
                    "document_ids": document_ids,
                    "metadata_condition": {"conditions": [{"key": "nonexistent_key", "value": ["nonexistent_value"]}]},
                },
                "updates": [{"key": "author", "value": "test"}],
                "deletes": [],
            },
        )

        assert res["code"] == 0
        assert res["data"]["updated"] == 0
        assert res["data"]["matched_docs"] == 0
