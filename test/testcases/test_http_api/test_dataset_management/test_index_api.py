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
from common import (
    bulk_upload_documents,
    list_documents,
    parse_documents,
    run_index,
    trace_index,
    delete_index,
)
from utils import wait_for


@wait_for(200, 1, "Document parsing timeout")
def _parse_done(auth, dataset_id, document_ids=None):
    res = list_documents(auth, dataset_id)
    if res.get("code") != 0:
        return False
    target_docs = res.get("data", {}).get("docs", [])
    if not target_docs:
        return False
    if document_ids is None:
        return all(doc.get("run") == "DONE" for doc in target_docs)
    target_ids = set(document_ids)
    seen_ids = set()
    for doc in target_docs:
        doc_id = doc.get("id")
        if doc_id in target_ids:
            seen_ids.add(doc_id)
            if doc.get("run") != "DONE":
                return False
    return seen_ids == target_ids


@pytest.mark.usefixtures("clear_datasets")
class TestRunIndex:
    @pytest.mark.p2
    def test_run_index_graph(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = run_index(HttpApiAuth, dataset_id, "graph")
        assert res["code"] == 0, res
        assert res["data"].get("task_id"), res

    @pytest.mark.p2
    def test_run_index_raptor(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = run_index(HttpApiAuth, dataset_id, "raptor")
        assert res["code"] == 0, res
        assert res["data"].get("task_id"), res

    @pytest.mark.p2
    def test_run_index_mindmap(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = run_index(HttpApiAuth, dataset_id, "mindmap")
        assert res["code"] == 0, res
        assert res["data"].get("task_id"), res

    @pytest.mark.p2
    def test_run_index_invalid_type(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        res = run_index(HttpApiAuth, dataset_id, "invalid_type")
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_run_index_no_documents(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = run_index(HttpApiAuth, dataset_id, "raptor")
        assert res["code"] == 102, res


@pytest.mark.usefixtures("clear_datasets")
class TestDeleteIndex:
    @pytest.mark.p2
    def test_delete_graph(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = delete_index(HttpApiAuth, dataset_id, "graph")
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_delete_raptor(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = delete_index(HttpApiAuth, dataset_id, "raptor")
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_delete_mindmap(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = delete_index(HttpApiAuth, dataset_id, "mindmap")
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_delete_invalid_type(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = delete_index(HttpApiAuth, dataset_id, "invalid_type")
        assert res["code"] != 0, res


@pytest.mark.usefixtures("clear_datasets")
class TestTraceIndex:
    @pytest.mark.p2
    def test_trace_index_graph(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        parse_documents(HttpApiAuth, dataset_id)
        _parse_done(HttpApiAuth, dataset_id)
        res = run_index(HttpApiAuth, dataset_id, "graph")
        assert res["code"] == 0, res
        res = trace_index(HttpApiAuth, dataset_id, "graph")
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_trace_index_raptor(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        parse_documents(HttpApiAuth, dataset_id)
        _parse_done(HttpApiAuth, dataset_id)
        res = run_index(HttpApiAuth, dataset_id, "raptor")
        assert res["code"] == 0, res
        res = trace_index(HttpApiAuth, dataset_id, "raptor")
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_trace_index_mindmap(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        bulk_upload_documents(HttpApiAuth, dataset_id, 1, tmp_path)
        parse_documents(HttpApiAuth, dataset_id)
        _parse_done(HttpApiAuth, dataset_id)
        res = run_index(HttpApiAuth, dataset_id, "mindmap")
        assert res["code"] == 0, res
        res = trace_index(HttpApiAuth, dataset_id, "mindmap")
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_trace_index_invalid_type(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = trace_index(HttpApiAuth, dataset_id, "invalid_type")
        assert res["code"] != 0, res

    @pytest.mark.p2
    def test_trace_index_no_task(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = trace_index(HttpApiAuth, dataset_id, "graph")
        assert res["code"] == 0, res
        assert res["data"] == {}
