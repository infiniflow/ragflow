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
from common import bulk_upload_documents, list_documents, parse_documents, run_graphrag, trace_graphrag
from utils import wait_for


@wait_for(200, 1, "Document parsing timeout")
def _parse_done(auth, dataset_id, document_ids=None):
    res = list_documents(auth, dataset_id)
    target_docs = res["data"]["docs"]
    if document_ids is None:
        return all(doc.get("run") == "DONE" for doc in target_docs)
    target_ids = set(document_ids)
    for doc in target_docs:
        if doc.get("id") in target_ids and doc.get("run") != "DONE":
            return False
    return True


class TestGraphRAGTasks:
    @pytest.mark.p2
    def test_trace_graphrag_before_run(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = trace_graphrag(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res
        assert res["data"] == {}, res

    @pytest.mark.p2
    def test_run_graphrag_no_documents(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = run_graphrag(HttpApiAuth, dataset_id)
        assert res["code"] == 102, res
        assert "No documents in Dataset" in res.get("message", ""), res

    @pytest.mark.p3
    def test_run_graphrag_returns_task_id(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = run_graphrag(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res
        assert res["data"].get("graphrag_task_id"), res

    @pytest.mark.p3
    def test_trace_graphrag_until_complete(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = parse_documents(HttpApiAuth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0, res
        _parse_done(HttpApiAuth, dataset_id, document_ids)

        res = run_graphrag(HttpApiAuth, dataset_id)
        assert res["code"] == 0, res

        last_res = {}

        @wait_for(200, 1, "GraphRAG task timeout")
        def condition():
            res = trace_graphrag(HttpApiAuth, dataset_id)
            if res["code"] != 0:
                return False
            data = res.get("data") or {}
            if not data:
                return False
            if data.get("task_type") != "graphrag":
                return False
            progress = data.get("progress")
            if progress in (-1, 1, -1.0, 1.0):
                last_res["res"] = res
                return True
            return False

        condition()
        res = last_res["res"]
        assert res["data"]["task_type"] == "graphrag", res
        assert res["data"].get("progress") in (-1, 1, -1.0, 1.0), res
