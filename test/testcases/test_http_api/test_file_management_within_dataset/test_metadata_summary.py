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
from common import metadata_summary, update_document


def _summary_to_counts(summary):
    counts = {}
    for key, field_data in summary.items():
        # New format: {key: {"type": "...", "values": [[value, count], ...]}}
        pairs = field_data["values"]
        counts[key] = {str(k): v for k, v in pairs}
    return counts


class TestMetadataSummary:
    @pytest.mark.p2
    def test_metadata_summary_missing_kb_id(self, HttpApiAuth, add_document_func):
        """
        Call with non-existent dataset
        :param HttpApiAuth:
        :param add_document_func:
        :return:
        """
        res = metadata_summary(HttpApiAuth, "")
        assert res["code"] == 404, res
        assert res["message"] == "Not Found: /api/v1/datasets//metadata/summary", res

    @pytest.mark.p2
    def test_metadata_summary_invalid_kb_id(self, HttpApiAuth, add_document_func):
        """Test metadata summary when user doesn't have access to the dataset."""
        kb_id, doc_id = add_document_func
        invalid_kb_id = "invalid_" + kb_id
        # Call with a dataset that the user doesn't have access to
        res = metadata_summary(HttpApiAuth, invalid_kb_id)
        # Should succeed with valid access
        assert res["code"] == 102, res
        assert res["message"] == f"You don't own the dataset {invalid_kb_id}. "

    @pytest.mark.p2
    def test_metadata_summary_success_and_exception(self, HttpApiAuth, add_document_func):
        """Test metadata summary success case and exception handling."""
        kb_id, doc_id = add_document_func
        # Test successful case
        res = metadata_summary(HttpApiAuth, kb_id)
        assert res["code"] == 0, res
        assert "summary" in res["data"], res

    @pytest.mark.p2
    def test_metadata_summary_counts(self, HttpApiAuth, add_documents_func):
        """
        test normal cases
        :param HttpApiAuth:
        :param add_documents_func:
        :return:
        """
        dataset_id, document_ids = add_documents_func
        payloads = [
            {"tags": ["foo", "bar"], "author": "alice"},
            {"tags": ["foo"], "author": "bob"},
            {"tags": ["bar", "baz"], "author": ""},
        ]
        for doc_id, meta_fields in zip(document_ids, payloads):
            res = update_document(HttpApiAuth, dataset_id, doc_id, {"meta_fields": meta_fields})
            assert res["code"] == 0, res

        res = metadata_summary(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res

        summary = res["data"]["summary"]
        counts = _summary_to_counts(summary)
        assert counts["tags"]["foo"] == 2, counts
        assert counts["tags"]["bar"] == 2, counts
        assert counts["tags"]["baz"] == 1, counts
        assert counts["author"]["alice"] == 1, counts
        assert counts["author"]["bob"] == 1, counts
        assert "None" not in counts["author"], counts
