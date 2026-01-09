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
import os
import re
import tempfile

import pytest

from common import (
    chat_completions,
    create_chat_assistant,
    create_session_with_chat_assistant,
    delete_chat_assistants,
    delete_documents,
    list_documents,
    upload_documents,
    parse_documents,
)
from utils import wait_for


# Test constants
FIELD_NAME_SUFFIXES = ["_tks", "_long", "_kwd"]


@wait_for(30, 1, "Document parsing timeout")
def wait_for_parsing_completion(auth, dataset_id, document_id=None):
    """
    Wait for document parsing to complete.

    Args:
        auth: Authentication object
        dataset_id: Dataset ID
        document_id: Optional specific document ID to wait for

    Returns:
        bool: True if parsing is complete, False otherwise
    """
    res = list_documents(auth, dataset_id)
    docs = res["data"]["docs"]

    if document_id is None:
        # Wait for all documents to complete
        for doc in docs:
            if doc.get("run") != "DONE":
                return False
        return True
    else:
        # Wait for specific document
        for doc in docs:
            if doc["id"] == document_id:
                status = doc.get("run", "UNKNOWN")
                if status == "DONE":
                    print(f"[Parsing] ✓ Document {document_id[:8]}... parsed successfully")
                    return True
                elif status == "FAILED":
                    pytest.fail(f"Document parsing failed: {doc}")
                return False
        return False

# Test data
TEST_EXCEL_DATA = [
    ["url", "title2", "body"],
    ["https://example1.com", "Title 1", "Body content1"],
    ["https://example2.com", "Title 2", "Body content2"],
    ["https://example3.com", "Title 3", "Body content3"],
    ["https://example4.com", "Title 4", "Body content4"],
    ["https://example5.com", "Title 5", "Body content5"],
    ["https://example6.com", "Title 6", "Body content6"],
    ["https://example7.com", "Title 7", "Body content7"],
]

DEFAULT_CHAT_PROMPT = (
    "You are a helpful assistant that answers questions about table data using SQL queries.\n\n"
    "Here is the knowledge base:\n{knowledge}\n\n"
    "Use this information to answer questions."
)


@pytest.mark.usefixtures("add_table_parser_dataset")
class TestTableParserDatasetChat:
    """
    Test table parser dataset chat functionality with Infinity backend.

    Verifies that:
    1. Excel files are uploaded and parsed correctly into table parser datasets
    2. Chat assistants can query the parsed table data via SQL
    3. Field names are displayed correctly (e.g., 'body' not 'body_tks')
    4. Different types of queries work (aggregate, simple select, multiple fields)
    """

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "question, expected_answer_pattern",
        [
            # Test simple field display - should show all titles
            ("Show me the body column", r"url|title2|body"),
            # Test COUNT query - should show count(*)
            ("How many rows are there?", r"count\(\*\)"),
            # Test WHERE query - should find matching rows
            ("Which rows have body containing 'content3'", r"Rows with body containing 'content3'"),
        ],
    )
    def test_table_parser_dataset_chat(
        self, HttpApiAuth, add_table_parser_dataset, question, expected_answer_pattern
    ):
        """
        Test that table parser dataset chat works correctly.

        This test ensures that when querying table data via chat:
        - Field names are displayed as 'body' not 'body_tks'
        - Field names are displayed as 'title2' not 'title2_long'
        - The answers are user-friendly and don't expose internal field naming
        - Chat assistants can successfully query table parser datasets
        """
        dataset_id = add_table_parser_dataset

        # Step 1: Upload and parse Excel file
        self._upload_and_parse_excel(HttpApiAuth, dataset_id)

        # Step 2: Create chat assistant with session
        chat_id, session_id = self._create_chat_assistant_with_session(
            HttpApiAuth, dataset_id
        )

        try:
            # Step 3: Send question and verify response
            answer = self._ask_question(HttpApiAuth, chat_id, session_id, question)

            # Step 4: Verify field name display
            self._assert_no_internal_field_names(answer)
            self._assert_answer_matches_pattern(answer, expected_answer_pattern)

        finally:
            # Cleanup
            delete_chat_assistants(HttpApiAuth, {"ids": [chat_id]})
            # Cleanup documents to ensure clean state for next test
            self._cleanup_documents(HttpApiAuth, dataset_id)

    def _upload_and_parse_excel(self, auth, dataset_id):
        """
        Upload an Excel file and wait for parsing to complete.

        Returns:
            str: The document ID of the uploaded file

        Raises:
            AssertionError: If upload or parsing fails
        """
        excel_file_path = None
        try:
            # Create temporary Excel file
            excel_file_path = self._create_temp_excel_file()

            # Upload document
            res = upload_documents(auth, dataset_id, [excel_file_path])
            assert res["code"] == 0, f"Failed to upload document: {res}"
            document_id = res["data"][0]["id"]

            # Start parsing
            parse_payload = {"document_ids": [document_id]}
            res = parse_documents(auth, dataset_id, parse_payload)
            assert res["code"] == 0, f"Failed to start parsing: {res}"

            # Wait for parsing completion
            wait_for_parsing_completion(auth, dataset_id, document_id)

            return document_id

        finally:
            # Clean up temporary file
            if excel_file_path:
                os.unlink(excel_file_path)

    def _cleanup_documents(self, auth, dataset_id):
        """
        Delete all documents from the dataset to ensure clean state for each test.

        This is important because the dataset is shared across all test cases in the class.
        """
        try:
            res = list_documents(auth, dataset_id)
            if res["code"] == 0 and "docs" in res["data"]:
                doc_ids = [doc["id"] for doc in res["data"]["docs"]]
                if doc_ids:
                    delete_documents(auth, dataset_id, {"document_ids": doc_ids})
        except Exception as e:
            print(f"Warning: Failed to cleanup documents: {e}")

    def _create_temp_excel_file(self):
        """Create a temporary Excel file with table test data."""
        from openpyxl import Workbook

        f = tempfile.NamedTemporaryFile(mode="wb", suffix=".xlsx", delete=False)
        f.close()

        wb = Workbook()
        ws = wb.active

        # Write test data to the worksheet
        for row_idx, row_data in enumerate(TEST_EXCEL_DATA, start=1):
            for col_idx, value in enumerate(row_data, start=1):
                ws.cell(row=row_idx, column=col_idx, value=value)

        wb.save(f.name)
        return f.name

    def _create_chat_assistant_with_session(self, auth, dataset_id):
        """
        Create a chat assistant and session for testing.

        Returns:
            tuple: (chat_id, session_id)
        """
        import uuid

        chat_payload = {
            "name": f"test_table_parser_dataset_chat_{uuid.uuid4().hex[:8]}",
            "dataset_ids": [dataset_id],
            "prompt_config": {
                "system": DEFAULT_CHAT_PROMPT,
                "parameters": [
                    {
                        "key": "knowledge",
                        "optional": True,
                        "value": "Use the table data to answer questions with SQL queries.",
                    }
                ],
            },
        }

        res = create_chat_assistant(auth, chat_payload)
        assert res["code"] == 0, f"Failed to create chat assistant: {res}"
        chat_id = res["data"]["id"]

        res = create_session_with_chat_assistant(auth, chat_id, {"name": f"test_session_{uuid.uuid4().hex[:8]}"})
        assert res["code"] == 0, f"Failed to create session: {res}"
        session_id = res["data"]["id"]

        return chat_id, session_id

    def _ask_question(self, auth, chat_id, session_id, question):
        """
        Send a question to the chat assistant and return the answer.

        Returns:
            str: The assistant's answer
        """
        payload = {
            "question": question,
            "stream": False,
            "session_id": session_id,
        }

        res_json = chat_completions(auth, chat_id, payload)
        assert res_json["code"] == 0, f"Chat completion failed: {res_json}"

        return res_json["data"]["answer"]

    def _assert_no_internal_field_names(self, answer):
        """
        Assert that the answer doesn't contain internal field name suffixes.

        Internal field names have suffixes like '_tks', '_long', '_kwd' which should
        not be exposed to end users in chat responses.
        """
        for suffix in FIELD_NAME_SUFFIXES:
            assert suffix not in answer, (
                f"Answer should not contain internal field names with '{suffix}' suffix.\n"
                f"This suggests the system is exposing technical implementation details.\n"
                f"Answer: {answer}"
            )

    def _assert_answer_matches_pattern(self, answer, pattern):
        """
        Assert that the answer matches the expected pattern.

        Args:
            answer: The actual answer from the chat assistant
            pattern: Regular expression pattern to match
        """
        assert re.search(pattern, answer, re.IGNORECASE), (
            f"Answer does not match expected pattern '{pattern}'.\n"
            f"Answer: {answer}"
        )
