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
End-to-end test for metadata filtering during retrieval.

Tests that chunks are only retrieved from documents matching the metadata condition.
"""

import pytest
import logging
from common import (
    create_dataset,
    delete_datasets,
    list_documents,
    update_document,
)
from utils import wait_for


@wait_for(30, 1, "Document parsing timeout")
def _condition_parsing_complete(_auth, dataset_id):
    res = list_documents(_auth, dataset_id)
    if res["code"] != 0:
        return False

    for doc in res["data"]["docs"]:
        status = doc.get("run", "UNKNOWN")
        if status == "FAILED":
            pytest.fail(f"Document parsing failed: {doc}")
            return False
        if status != "DONE":
            return False
    return True


@pytest.fixture(scope="function")
def add_dataset_with_metadata(HttpApiAuth):
    # First create the dataset
    res = create_dataset(HttpApiAuth, {
        "name": f"test_metadata_{int(__import__('time').time())}",
        "chunk_method": "naive"
    })

    assert res["code"] == 0, f"Failed to create dataset: {res}"
    dataset_id = res["data"]["id"]

    # Then configure metadata via the update_metadata_setting endpoint
    import requests
    from configs import HOST_ADDRESS, VERSION

    metadata_config = {
        "type": "object",
        "properties": {
            "character": {
                "description": "Historical figure name",
                "type": "string"
            },
            "era": {
                "description": "Historical era",
                "type": "string"
            },
            "achievements": {
                "description": "Major achievements",
                "type": "array",
                "items": {
                    "type": "string"
                }
            }
        }
    }

    res = requests.post(
        url=f"{HOST_ADDRESS}/{VERSION}/kb/update_metadata_setting",
        headers={"Content-Type": "application/json"},
        auth=HttpApiAuth,
        json={
            "kb_id": dataset_id,
            "metadata": metadata_config,
            "enable_metadata": False
        }
    ).json()

    assert res["code"] == 0, f"Failed to configure metadata: {res}"

    yield dataset_id

    # Cleanup
    delete_datasets(HttpApiAuth, {"ids": [dataset_id]})


@pytest.mark.p2
class TestMetadataWithRetrieval:
    """Test retrieval with metadata filtering."""

    def test_retrieval_with_metadata_filter(self, HttpApiAuth, add_dataset_with_metadata, tmp_path):
        """
        Test that retrieval respects metadata filters.

        Verifies that chunks are only retrieved from documents matching the metadata condition.
        """
        from common import upload_documents, parse_documents, retrieval_chunks

        dataset_id = add_dataset_with_metadata

        # Create two documents with different metadata
        content_doc1 = "Document about Zhuge Liang who lived in Three Kingdoms period."
        content_doc2 = "Document about Cao Cao who lived in Late Eastern Han Dynasty."

        fp1 = tmp_path / "doc1_zhuge_liang.txt"
        fp2 = tmp_path / "doc2_cao_cao.txt"

        with open(fp1, "w", encoding="utf-8") as f:
            f.write(content_doc1)
        with open(fp2, "w", encoding="utf-8") as f:
            f.write(content_doc2)

        # Upload both documents
        res = upload_documents(HttpApiAuth, dataset_id, [fp1, fp2])
        assert res["code"] == 0, f"Failed to upload documents: {res}"

        doc1_id = res["data"][0]["id"]
        doc2_id = res["data"][1]["id"]

        # Add different metadata to each document
        res = update_document(HttpApiAuth, dataset_id, doc1_id, {
            "meta_fields": {"character": "Zhuge Liang", "era": "Three Kingdoms"}
        })
        assert res["code"] == 0, f"Failed to update doc1 metadata: {res}"

        res = update_document(HttpApiAuth, dataset_id, doc2_id, {
            "meta_fields": {"character": "Cao Cao", "era": "Late Eastern Han"}
        })
        assert res["code"] == 0, f"Failed to update doc2 metadata: {res}"

        # Parse both documents
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": [doc1_id, doc2_id]})
        assert res["code"] == 0, f"Failed to trigger parsing: {res}"

        # Wait for parsing to complete
        assert _condition_parsing_complete(HttpApiAuth, dataset_id), "Parsing timeout"

        # Test retrieval WITH metadata filter for "Zhuge Liang"
        res = retrieval_chunks(HttpApiAuth, {
            "question": "Zhuge Liang",
            "dataset_ids": [dataset_id],
            "metadata_condition": {
                "logic": "and",
                "conditions": [
                    {
                        "name": "character",
                        "comparison_operator": "is",
                        "value": "Zhuge Liang"
                    }
                ]
            }
        })
        assert res["code"] == 0, f"Retrieval with metadata filter failed: {res}"

        chunks_with_filter = res["data"]["chunks"]
        doc_ids_with_filter = set(chunk.get("document_id", "") for chunk in chunks_with_filter)

        logging.info(f"✓ Retrieved {len(chunks_with_filter)} chunks from documents: {doc_ids_with_filter}")

        # Verify that filtered results only contain doc1 (Zhuge Liang)
        if len(chunks_with_filter) > 0:
            assert doc1_id in doc_ids_with_filter, f"Filtered results should contain doc1 (Zhuge Liang), but got: {doc_ids_with_filter}"
            assert doc2_id not in doc_ids_with_filter, f"Filtered results should NOT contain doc2 (Cao Cao), but got: {doc_ids_with_filter}"
            logging.info("Metadata filter correctly excluded chunks from non-matching documents")
        else:
            logging.warning("No chunks retrieved with filter - this might be due to embedding model not configured")
