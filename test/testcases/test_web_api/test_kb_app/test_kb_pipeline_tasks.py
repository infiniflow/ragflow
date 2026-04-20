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
from test_common import (
    kb_delete_pipeline_logs,
    kb_list_pipeline_dataset_logs,
    kb_list_pipeline_logs,
    kb_pipeline_log_detail,
    list_documents,
    parse_documents,
)
from utils import wait_for

TASK_STATUS_DONE = "3"


def _assert_progress_in_scale(progress, payload):
    assert isinstance(progress, (int, float)), payload
    if progress < 0:
        assert False, f"Negative progress is not expected: {payload}"
    scale = 100 if progress > 1 else 1
    # Infer scale from observed payload (0..1 or 0..100).
    assert 0 <= progress <= scale, payload
    return scale


def _wait_for_docs_parsed(auth, kb_id, timeout=60):
    @wait_for(timeout, 2, "Document parsing timeout")
    def _condition():
        res = list_documents(auth, {"id": kb_id})
        if res["code"] != 0:
            return False
        for doc in res["data"]["docs"]:
            progress = doc.get("progress", 0)
            _assert_progress_in_scale(progress, doc)
            scale = 100 if progress > 1 else 1
            if doc.get("run") != TASK_STATUS_DONE or progress < scale:
                return False
        return True

    _condition()


def _wait_for_pipeline_logs(auth, kb_id, timeout=30):
    @wait_for(timeout, 1, "Pipeline log timeout")
    def _condition():
        res = kb_list_pipeline_logs(auth, params={"kb_id": kb_id}, payload={})
        if res["code"] != 0:
            return False
        return bool(res["data"]["logs"])

    _condition()


class TestKbPipelineLogs:
    @pytest.mark.p3
    def test_pipeline_log_lifecycle(self, WebApiAuth, add_document):
        kb_id, document_id = add_document
        parse_documents(WebApiAuth, {"doc_ids": [document_id], "run": "1"})
        _wait_for_docs_parsed(WebApiAuth, kb_id)
        _wait_for_pipeline_logs(WebApiAuth, kb_id)

        list_res = kb_list_pipeline_logs(WebApiAuth, params={"kb_id": kb_id}, payload={})
        assert list_res["code"] == 0, list_res
        assert "total" in list_res["data"], list_res
        assert isinstance(list_res["data"]["logs"], list), list_res
        assert list_res["data"]["logs"], list_res

        log_id = list_res["data"]["logs"][0]["id"]
        detail_res = kb_pipeline_log_detail(WebApiAuth, {"log_id": log_id})
        assert detail_res["code"] == 0, detail_res
        detail = detail_res["data"]
        assert detail["id"] == log_id, detail_res
        assert detail["kb_id"] == kb_id, detail_res
        for key in ["document_id", "task_type", "operation_status", "progress"]:
            assert key in detail, detail_res

        delete_res = kb_delete_pipeline_logs(WebApiAuth, params={"kb_id": kb_id}, payload={"log_ids": [log_id]})
        assert delete_res["code"] == 0, delete_res
        assert delete_res["data"] is True, delete_res

        @wait_for(30, 1, "Pipeline log delete timeout")
        def _condition():
            res = kb_list_pipeline_logs(WebApiAuth, params={"kb_id": kb_id}, payload={})
            if res["code"] != 0:
                return False
            return all(log.get("id") != log_id for log in res["data"]["logs"])

        _condition()

    @pytest.mark.p3
    def test_list_pipeline_dataset_logs(self, WebApiAuth, add_document):
        kb_id, _ = add_document
        res = kb_list_pipeline_dataset_logs(WebApiAuth, params={"kb_id": kb_id}, payload={})
        assert res["code"] == 0, res
        assert "total" in res["data"], res
        assert isinstance(res["data"]["logs"], list), res

    @pytest.mark.p3
    def test_pipeline_log_detail_missing_id(self, WebApiAuth):
        res = kb_pipeline_log_detail(WebApiAuth, {})
        assert res["code"] == 101, res
        assert "Pipeline log ID" in res["message"], res

    @pytest.mark.p3
    def test_delete_pipeline_logs_empty(self, WebApiAuth, add_document):
        kb_id, _ = add_document
        res = kb_delete_pipeline_logs(WebApiAuth, params={"kb_id": kb_id}, payload={"log_ids": []})
        assert res["code"] == 0, res
        assert res["data"] is True, res

    @pytest.mark.p3
    def test_list_pipeline_logs_missing_kb_id(self, WebApiAuth):
        res = kb_list_pipeline_logs(WebApiAuth, params={}, payload={})
        assert res["code"] == 101, res
        assert "KB ID" in res["message"], res

    @pytest.mark.p3
    def test_list_pipeline_logs_abnormal_date_filter(self, WebApiAuth, add_document):
        kb_id, _ = add_document
        res = kb_list_pipeline_logs(
            WebApiAuth,
            params={
                "kb_id": kb_id,
                "desc": "false",
                "create_date_from": "2025-01-01",
                "create_date_to": "2025-02-01",
            },
            payload={},
        )
        assert res["code"] == 102, res
        assert "Create data filter is abnormal." in res["message"], res
