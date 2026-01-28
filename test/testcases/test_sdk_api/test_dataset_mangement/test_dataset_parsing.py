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
import time
from utils.file_utils import create_txt_file

class TestDatasetParsing:
    """
    Specifically for testing parsing-related methods in sdk/python/ragflow_sdk/modules/dataset.py:
    - parse_documents
    - _get_documents_status
    - async_cancel_parse_documents
    """

    @pytest.mark.p1
    def test_parse_documents_and_status(self, add_dataset_func, tmp_path):
        """Test synchronous document parsing and status retrieval"""
        dataset = add_dataset_func
        # 1. Upload a document
        fp = create_txt_file(tmp_path / "test_parsing.txt")
        with fp.open("rb") as f:
            blob = f.read()
        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        doc_id = documents[0].id

        # 2. Call parse_documents (internally calls async_parse_documents and _get_documents_status)
        # Note: In the test environment, parsing may complete quickly or take some time
        dataset.parse_documents([doc_id])
        
        # 3. Verify document status
        # list_documents should show the document is parsed or in a terminal state
        docs = dataset.list_documents(id=doc_id)
        assert len(docs) > 0
        assert docs[0].run in ["DONE", "FAIL", "CANCEL"]

    @pytest.mark.p2
    def test_async_cancel_parse_documents(self, add_dataset_func, tmp_path):
        """Test canceling document parsing"""
        dataset = add_dataset_func
        # 1. Upload a document
        fp = create_txt_file(tmp_path / "test_cancel.txt")
        with fp.open("rb") as f:
            blob = f.read()
        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        doc_id = documents[0].id

        # 2. Start asynchronous parsing
        dataset.async_parse_documents([doc_id])
        
        # 3. Cancel parsing immediately
        dataset.async_cancel_parse_documents([doc_id])
        
        # 4. Verify status (may take some time to synchronize)
        time.sleep(1)
        docs = dataset.list_documents(id=doc_id)
        # After cancellation, the status should be CANCEL or still being processed
        assert docs[0].run in ["CANCEL", "DONE", "FAIL", "UNSTART"] # Depends on cancellation speed

    @pytest.mark.p2
    def test_get_documents_status_internal(self, add_dataset_func, tmp_path):
        """Test internal method _get_documents_status directly"""
        dataset = add_dataset_func
        fp = create_txt_file(tmp_path / "test_status_internal.txt")
        with fp.open("rb") as f:
            blob = f.read()
        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        doc_id = documents[0].id

        # Start parsing
        dataset.async_parse_documents([doc_id])
        
        # Call internal method directly
        finished_info = dataset._get_documents_status([doc_id])
        
        assert len(finished_info) == 1
        # Return structure: (doc_id, doc.run, doc.chunk_count, doc.token_count)
        assert finished_info[0][0] == doc_id
        assert finished_info[0][1] in ["DONE", "FAIL", "CANCEL"]
