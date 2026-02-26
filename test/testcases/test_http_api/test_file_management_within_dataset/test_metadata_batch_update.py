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
from common import metadata_batch_update, list_documents, delete_documents, upload_documents


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


@pytest.mark.p2
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
        res = metadata_batch_update(HttpApiAuth, dataset_id, {"selector": {"document_ids": document_ids}, "updates": updates})

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
        delete_documents(HttpApiAuth, dataset_id, {"ids": None})
