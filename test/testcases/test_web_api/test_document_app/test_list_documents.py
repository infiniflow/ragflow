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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from test_common import list_documents
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils import is_sorted


@pytest.mark.p2
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = list_documents(invalid_auth, {"id": "dataset_id"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsList:
    @pytest.mark.p1
    def test_default(self, WebApiAuth, add_documents):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == 0, f", kb_id:{kb_id} +, res:{str(res)}"
        assert len(res["data"]["docs"]) == 5
        assert res["data"]["total"] == 5


    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page": None, "page_size": 5}, 0, 5, ""),
            ({"page": 0, "page_size": 5}, 0, 5, ""),
            ({"page": 2, "page_size": 2}, 0, 2, ""),
            ({"page": 3, "page_size": 2}, 0, 1, ""),
            ({"page": "3", "page_size": 2}, 0, 1, ""),
            pytest.param({"page": -1, "page_size": 2}, 100, 0, "1064", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"page": "a", "page_size": 2}, 100, 0, """ValueError("invalid literal for int() with base 10: 'a'")""", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_page(self, WebApiAuth, add_documents, params, expected_code, expected_page_size, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size, res
            assert res["data"]["total"] == 5, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page_size": None}, 0, 5, ""),
            ({"page_size": 5}, 0, 5, ""),
            ({"page_size": 1}, 0, 1, ""),
            ({"page_size": 6}, 0, 5, ""),
            ({"page_size": "1"}, 0, 1, ""),
            pytest.param({"page_size": -1}, 100, 0, "1064", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"page_size": "a"}, 100, 0, """ValueError("invalid literal for int() with base 10: 'a'")""", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_page_size(self, WebApiAuth, add_documents, params, expected_code, expected_page_size, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            ({"orderby": None}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"orderby": "create_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "update_time", True)), ""),
            pytest.param({"orderby": "name", "desc": "False"}, 0, lambda r: (is_sorted(r["data"]["docs"], "name", False)), "", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"orderby": "unknown"}, 102, 0, "orderby should be create_time or update_time", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_orderby(self, WebApiAuth, add_documents, params, expected_code, assertions, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            ({"desc": None}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": "true"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": "True"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": True}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            pytest.param({"desc": "false"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), "", marks=pytest.mark.skip(reason="issues/5851")),
            ({"desc": "False"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), ""),
            ({"desc": False}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), ""),
            ({"desc": "False", "orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "update_time", False)), ""),
            pytest.param({"desc": "unknown"}, 102, 0, "desc should be true or false", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_desc(self, WebApiAuth, add_documents, params, expected_code, assertions, expected_message):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_num",
        [
            ({"keywords": None}, 5),
            ({"keywords": ""}, 5),
            ({"keywords": "0"}, 1),
            ({"keywords": "ragflow_test_upload"}, 5),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, WebApiAuth, add_documents, params, expected_num):
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, **params})
        assert res["code"] == 0, res
        assert len(res["data"]["docs"]) == expected_num, res
        assert res["data"]["total"] == expected_num, res

    @pytest.mark.p3
    def test_concurrent_list(self, WebApiAuth, add_documents):
        kb_id, _ = add_documents
        count = 100

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_documents, WebApiAuth, {"kb_id": kb_id}) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures), responses

    # Tests moved from TestDocumentsListUnit
    @pytest.mark.p2
    def test_missing_kb_id(self, WebApiAuth):
        """Test missing KB ID returns error."""
        res = list_documents(WebApiAuth, {"kb_id": ""})
        assert res["code"] == 102
        assert res["message"]

    @pytest.mark.p2
    def test_unauthorized_dataset(self, WebApiAuth):
        """Test unauthorized dataset returns error."""
        res = list_documents(WebApiAuth, {"kb_id": "non_existent_kb_id"})
        assert res["code"] == 102
        assert res["message"]

    @pytest.mark.p3
    def test_invalid_run_status_filter(self, WebApiAuth, add_documents):
        """Test invalid run status filter returns error."""
        kb_id, _ = add_documents
        res = list_documents(WebApiAuth, {"kb_id": kb_id, "run": "INVALID"})
        assert res["code"] == 102
        assert "Invalid filter run status" in res["message"]

    @pytest.mark.p3
    def test_invalid_document_id_filter(self, WebApiAuth, add_documents):
        """Test invalid document ID filter returns error."""
        kb_id, _ = add_documents
        # Use a non-existent document ID
        res = list_documents(WebApiAuth, {"kb_id": kb_id, "id": "non_existent_doc_id"})
        assert res["code"] == 102
        assert "You don't own the document" in res["message"]

    @pytest.mark.p3
    def test_create_time_filter(self, WebApiAuth, add_documents):
        """Test create time range filter."""
        kb_id, _ = add_documents
        # Get current time range
        res = list_documents(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == 0
        if res["data"]["docs"]:
            create_time = res["data"]["docs"][0].get("create_time", 0)
            # Test with time range that should include the document
            res = list_documents(WebApiAuth, {"kb_id": kb_id, "create_time_from": 0, "create_time_to": create_time + 1000})
            assert res["code"] == 0
            assert len(res["data"]["docs"]) > 0
            # Test with time range that should not include the document
            res = list_documents(WebApiAuth, {"kb_id": kb_id, "create_time_from": create_time + 1000, "create_time_to": create_time + 2000})
            assert res["code"] == 0
            assert len(res["data"]["docs"]) == 0

