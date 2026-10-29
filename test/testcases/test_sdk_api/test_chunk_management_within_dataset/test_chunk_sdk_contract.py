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
from unittest.mock import Mock

import pytest
from conftest import add_document, raw_ids, wait_until
from ragflow_sdk import Chunk, Document, RAGFlow
from ragflow_sdk.modules.chunk import ChunkUpdateError


@pytest.fixture
def sdk():
    return RAGFlow("test-token", "http://unused.invalid")


@pytest.fixture
def document(sdk):
    return Document(sdk, {"id": "document", "dataset_id": "dataset"})


def response(body):
    return Mock(json=Mock(return_value=body))


def test_add_chunk_payload_and_response(sdk, document, monkeypatch):
    body = {"content": "text", "important_keywords": ["key"], "questions": ["why?"], "tag_kwd": ["tag"], "image_base64": "aGVsbG8="}
    row = {**{key: value for key, value in body.items() if key != "image_base64"}, "id": "chunk", "dataset_id": "dataset", "document_id": "document", "image_id": "image"}
    post = Mock(return_value=response({"code": 0, "data": {"chunk": row}}))
    monkeypatch.setattr(sdk, "post", post)
    chunk = document.add_chunk(**body)
    post.assert_called_once_with("/datasets/dataset/documents/document/chunks", body, stream=False, files=None)
    for key, value in row.items():
        assert getattr(chunk, key) == value


def test_add_chunk_defaults(sdk, document, monkeypatch):
    post = Mock(return_value=response({"code": 0, "data": {"chunk": {"id": "chunk"}}}))
    monkeypatch.setattr(sdk, "post", post)
    document.add_chunk("text")
    assert post.call_args.args[1] == {"content": "text", "important_keywords": [], "questions": [], "tag_kwd": []}


@pytest.mark.parametrize(
    "payload,expected",
    [({}, {"chunk_ids": None, "delete_all": False}), ({"ids": ["chunk"]}, {"chunk_ids": ["chunk"], "delete_all": False}), ({"delete_all": True}, {"chunk_ids": None, "delete_all": True})],
)
def test_delete_chunk_payload(sdk, document, monkeypatch, payload, expected):
    delete = Mock(return_value=response({"code": 0}))
    monkeypatch.setattr(sdk, "delete", delete)
    assert document.delete_chunks(**payload) is None
    delete.assert_called_once_with("/datasets/dataset/documents/document/chunks", expected)


def test_list_chunk_query_and_response(sdk, document, monkeypatch):
    get = Mock(return_value=response({"code": 0, "data": {"chunks": [{"id": "chunk", "content": "text"}]}}))
    monkeypatch.setattr(sdk, "get", get)
    rows = document.list_chunks(page=2, page_size=3, keywords="key", id="chunk")
    get.assert_called_once_with("/datasets/dataset/documents/document/chunks", {"page": 2, "page_size": 3, "keywords": "key", "id": "chunk"})
    assert [(row.id, row.content) for row in rows] == [("chunk", "text")]


@pytest.mark.parametrize("body", [{"code": 0}, {"code": 102, "message": "invalid", "details": {"field": "tag_kwd"}}])
def test_chunk_update_result(sdk, monkeypatch, body):
    patch = Mock(return_value=response(body))
    monkeypatch.setattr(sdk, "patch", patch)
    chunk = Chunk(sdk, {"id": "chunk", "dataset_id": "dataset", "document_id": "document", "content": "old"})
    if body["code"]:
        with pytest.raises(ChunkUpdateError) as error:
            chunk.update({"content": "new"})
        assert (error.value.code, error.value.message, error.value.details) == (102, "invalid", {"field": "tag_kwd"})
    else:
        assert chunk.update({"content": "new"}) is None
    patch.assert_called_once_with("/datasets/dataset/documents/document/chunks/chunk", {"content": "new"})
    assert chunk.content == "old"  # Updates require a fresh read; the SDK does not mutate this object.


@pytest.mark.parametrize("operation", ["add", "delete", "list", "retrieve"])
def test_sdk_propagates_api_error(sdk, document, monkeypatch, operation):
    error = response({"code": 102, "message": "contract failure"})
    for method in ("post", "get", "delete"):
        monkeypatch.setattr(sdk, method, Mock(return_value=error))
    with pytest.raises(Exception, match="^contract failure$"):
        if operation == "add":
            document.add_chunk("text")
        elif operation == "delete":
            document.delete_chunks(ids=["chunk"])
        elif operation == "list":
            document.list_chunks()
        else:
            sdk.retrieve(dataset_ids=["dataset"], question="text")


@pytest.mark.parametrize(
    "options",
    [
        {},
        {
            "page": 2,
            "page_size": 1,
            "top_k": 10,
            "vector_similarity_weight": 0.5,
            "similarity_threshold": 0,
            "document_ids": ["document"],
            "rerank_id": "reranker",
            "keyword": True,
            "cross_languages": ["English"],
            "metadata_condition": {"logic": "and", "conditions": []},
            "use_kg": True,
        },
    ],
)
def test_retrieve_payload_and_response(sdk, monkeypatch, options):
    post = Mock(return_value=response({"code": 0, "data": {"chunks": [{"id": "chunk", "content": "text", "similarity": 0.75, "document_id": "document", "dataset_id": "dataset"}]}}))
    monkeypatch.setattr(sdk, "post", post)
    rows = sdk.retrieve(dataset_ids=["dataset"], question="text", **options)
    expected = {
        "dataset_ids": ["dataset"],
        "question": "text",
        "document_ids": [],
        "page": 1,
        "page_size": 30,
        "top_k": 1024,
        "similarity_threshold": 0.2,
        "vector_similarity_weight": 0.3,
        "rerank_id": None,
        "keyword": False,
        "cross_languages": None,
        "metadata_condition": None,
        "use_kg": False,
        **options,
    }
    post.assert_called_once_with("/retrieval", json=expected)
    assert [(row.id, row.content, row.similarity, row.document_id, row.dataset_id) for row in rows] == [("chunk", "text", 0.75, "document", "dataset")]


def test_missing_datasets_is_python_argument_error(sdk):
    with pytest.raises(TypeError, match="dataset_ids"):
        sdk.retrieve(question="text")


def test_invalid_kwargs_is_python_argument_error(document):
    with pytest.raises(TypeError, match="mapping"):
        document.delete_chunks(**"not json")


def test_wait_only_retries_visibility():
    probe = Mock(side_effect=[False, True])
    assert wait_until(probe, timeout=1) is True
    assert probe.call_count == 2


def test_wait_propagates_errors():
    with pytest.raises(RuntimeError, match="backend unavailable"):
        wait_until(Mock(side_effect=RuntimeError("backend unavailable")))


def test_wait_deadline():
    with pytest.raises(pytest.fail.Exception, match="deadline exceeded"):
        wait_until(lambda: False, timeout=0.001)


def test_pagination_detects_repeated_pages(document, monkeypatch):
    monkeypatch.setattr("conftest.raw_chunks", lambda *args, **kwargs: [{"chunk_id": str(i)} for i in range(100)])
    with pytest.raises(AssertionError, match="pagination repeated IDs"):
        raw_ids(document)


@pytest.mark.parametrize("delete_fails", [False, True])
def test_fixture_cleanup_is_owned_and_errors_visible(delete_fails):
    client = Mock()
    dataset = client.create_dataset.return_value
    dataset.id = "owned-dataset"
    dataset.upload_documents.return_value = [Mock(id="owned-document")]
    client.list_datasets.return_value = []
    fixture = add_document.__wrapped__(client)
    assert next(fixture) == (dataset, dataset.upload_documents.return_value[0])
    if delete_fails:
        client.delete_datasets.side_effect = RuntimeError("cleanup failure")
        with pytest.raises(RuntimeError, match="cleanup failure"):
            next(fixture)
    else:
        with pytest.raises(StopIteration):
            next(fixture)
        client.list_datasets.assert_called_once_with(id="owned-dataset")
    client.delete_datasets.assert_called_once_with(ids=["owned-dataset"])


def test_fixture_cleans_up_after_upload_error():
    client = Mock()
    client.create_dataset.return_value.id = "owned-dataset"
    client.create_dataset.return_value.upload_documents.side_effect = RuntimeError("upload failure")
    client.list_datasets.return_value = []
    with pytest.raises(RuntimeError, match="upload failure"):
        next(add_document.__wrapped__(client))
    client.delete_datasets.assert_called_once_with(ids=["owned-dataset"])
