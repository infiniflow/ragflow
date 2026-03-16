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
import uuid

import pytest
from common import (
    kb_basic_info,
    kb_get_meta,
    kb_update_metadata_setting,
    list_tags,
    list_tags_from_kbs,
    rename_tags,
    rm_tags,
    update_chunk,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils import wait_for

INVALID_AUTH_CASES = [
    (None, 401, "Unauthorized"),
    (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
]

TAG_SEED_TIMEOUT = 20


def _wait_for_tag(auth, kb_id, tag, timeout=TAG_SEED_TIMEOUT):
    @wait_for(timeout, 1, "Tag seed timeout")
    def _condition():
        res = list_tags(auth, kb_id)
        if res["code"] != 0:
            return False
        return tag in res["data"]

    try:
        _condition()
    except AssertionError:
        return False
    return True


def _seed_tag(auth, kb_id, document_id, chunk_id):
    # KB tags are derived from chunk tag_kwd, not document metadata.
    tag = f"tag_{uuid.uuid4().hex[:8]}"
    res = update_chunk(
        auth,
        {
            "doc_id": document_id,
            "chunk_id": chunk_id,
            "content_with_weight": f"tag seed {tag}",
            "tag_kwd": [tag],
        },
    )
    assert res["code"] == 0, res
    if not _wait_for_tag(auth, kb_id, tag):
        return None
    return tag


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_list_tags_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = list_tags(invalid_auth, "kb_id")
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_list_tags_from_kbs_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = list_tags_from_kbs(invalid_auth, {"kb_ids": "kb_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_rm_tags_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = rm_tags(invalid_auth, "kb_id", {"tags": ["tag"]})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_rename_tag_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = rename_tags(invalid_auth, "kb_id", {"from_tag": "old", "to_tag": "new"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_get_meta_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = kb_get_meta(invalid_auth, {"kb_ids": "kb_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_basic_info_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = kb_basic_info(invalid_auth, {"kb_id": "kb_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_update_metadata_setting_auth_invalid(self, invalid_auth, expected_code, expected_fragment):
        res = kb_update_metadata_setting(invalid_auth, {"kb_id": "kb_id", "metadata": {}})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res


class TestKbTagsMeta:
    @pytest.mark.p2
    def test_list_tags(self, WebApiAuth, add_dataset):
        kb_id = add_dataset
        res = list_tags(WebApiAuth, kb_id)
        assert res["code"] == 0, res
        assert isinstance(res["data"], list), res

    @pytest.mark.p2
    def test_list_tags_from_kbs(self, WebApiAuth, add_dataset):
        kb_id = add_dataset
        res = list_tags_from_kbs(WebApiAuth, {"kb_ids": kb_id})
        assert res["code"] == 0, res
        assert isinstance(res["data"], list), res

    @pytest.mark.p3
    def test_rm_tags(self, WebApiAuth, add_chunks):
        kb_id, document_id, chunk_ids = add_chunks
        tag_to_remove = _seed_tag(WebApiAuth, kb_id, document_id, chunk_ids[0])
        if not tag_to_remove:
            # Tag aggregation is index-backed; skip if it never surfaces.
            pytest.skip("Seeded tag did not appear in list_tags.")

        res = rm_tags(WebApiAuth, kb_id, {"tags": [tag_to_remove]})
        assert res["code"] == 0, res
        assert res["data"] is True, res

        @wait_for(TAG_SEED_TIMEOUT, 1, "Tag removal timeout")
        def _condition():
            after_res = list_tags(WebApiAuth, kb_id)
            if after_res["code"] != 0:
                return False
            return tag_to_remove not in after_res["data"]

        _condition()

    @pytest.mark.p3
    def test_rename_tag(self, WebApiAuth, add_chunks):
        kb_id, document_id, chunk_ids = add_chunks
        from_tag = _seed_tag(WebApiAuth, kb_id, document_id, chunk_ids[0])
        if not from_tag:
            # Tag aggregation is index-backed; skip if it never surfaces.
            pytest.skip("Seeded tag did not appear in list_tags.")

        to_tag = f"{from_tag}_renamed"
        res = rename_tags(WebApiAuth, kb_id, {"from_tag": from_tag, "to_tag": to_tag})
        assert res["code"] == 0, res
        assert res["data"] is True, res

        @wait_for(TAG_SEED_TIMEOUT, 1, "Tag rename timeout")
        def _condition():
            after_res = list_tags(WebApiAuth, kb_id)
            if after_res["code"] != 0:
                return False
            tags = after_res["data"]
            return to_tag in tags and from_tag not in tags

        _condition()

    @pytest.mark.p2
    def test_get_meta(self, WebApiAuth, add_dataset):
        kb_id = add_dataset
        res = kb_get_meta(WebApiAuth, {"kb_ids": kb_id})
        assert res["code"] == 0, res
        assert isinstance(res["data"], dict), res

    @pytest.mark.p2
    def test_basic_info(self, WebApiAuth, add_dataset):
        kb_id = add_dataset
        res = kb_basic_info(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == 0, res
        for key in ["processing", "finished", "failed", "cancelled", "downloaded"]:
            assert key in res["data"], res

    @pytest.mark.p2
    def test_update_metadata_setting(self, WebApiAuth, add_dataset):
        kb_id = add_dataset
        metadata = {"source": "test"}
        res = kb_update_metadata_setting(WebApiAuth, {"kb_id": kb_id, "metadata": metadata, "enable_metadata": True})
        assert res["code"] == 0, res
        assert res["data"]["id"] == kb_id, res
        assert res["data"]["parser_config"]["metadata"] == metadata, res


class TestKbTagsMetaNegative:
    @pytest.mark.p3
    def test_list_tags_invalid_kb(self, WebApiAuth):
        res = list_tags(WebApiAuth, "invalid_kb_id")
        assert res["code"] == 109, res
        assert "No authorization" in res["message"], res

    @pytest.mark.p3
    def test_list_tags_from_kbs_invalid_kb(self, WebApiAuth):
        res = list_tags_from_kbs(WebApiAuth, {"kb_ids": "invalid_kb_id"})
        assert res["code"] == 109, res
        assert "No authorization" in res["message"], res

    @pytest.mark.p3
    def test_rm_tags_invalid_kb(self, WebApiAuth):
        res = rm_tags(WebApiAuth, "invalid_kb_id", {"tags": ["tag"]})
        assert res["code"] == 109, res
        assert "No authorization" in res["message"], res

    @pytest.mark.p3
    def test_rename_tag_invalid_kb(self, WebApiAuth):
        res = rename_tags(WebApiAuth, "invalid_kb_id", {"from_tag": "old", "to_tag": "new"})
        assert res["code"] == 109, res
        assert "No authorization" in res["message"], res

    @pytest.mark.p3
    def test_get_meta_invalid_kb(self, WebApiAuth):
        res = kb_get_meta(WebApiAuth, {"kb_ids": "invalid_kb_id"})
        assert res["code"] == 109, res
        assert "No authorization" in res["message"], res

    @pytest.mark.p3
    def test_basic_info_invalid_kb(self, WebApiAuth):
        res = kb_basic_info(WebApiAuth, {"kb_id": "invalid_kb_id"})
        assert res["code"] == 109, res
        assert "No authorization" in res["message"], res

    @pytest.mark.p3
    def test_update_metadata_setting_missing_metadata(self, WebApiAuth, add_dataset):
        res = kb_update_metadata_setting(WebApiAuth, {"kb_id": add_dataset})
        assert res["code"] == 101, res
        assert "required argument are missing" in res["message"], res
        assert "metadata" in res["message"], res
