#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from ragflow_sdk.modules.document import Document


class _DownloadResponse:
    def __init__(self, content, payload, headers=None):
        self.content = content
        self._payload = payload
        self.headers = headers or {}

    def json(self):
        if isinstance(self._payload, Exception):
            raise self._payload
        return self._payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "content,payload",
    [
        (b"[]", []),
        (b"[1, 2]", [1, 2]),
        (b"null", None),
        (b'"hello"', "hello"),
        (b"42", 42),
        (
            b'{"code": 0, "message": "file content"}',
            {"code": 0, "message": "file content"},
        ),
    ],
)
def test_download_preserves_attached_json_bytes(monkeypatch, content, payload):
    response = _DownloadResponse(
        content,
        payload,
        {"Content-Disposition": 'attachment; filename="sample.json"'},
    )
    document = Document(None, {"id": "doc", "dataset_id": "dataset"})
    monkeypatch.setattr(document, "get", lambda *_args, **_kwargs: response)

    assert document.download() == content


@pytest.mark.p2
def test_download_still_raises_api_error_without_attachment(monkeypatch):
    response = _DownloadResponse(
        b'{"code": 102, "message": "document not found"}',
        {"code": 102, "message": "document not found"},
        {"Content-Type": "application/json"},
    )
    document = Document(None, {"id": "doc", "dataset_id": "dataset"})
    monkeypatch.setattr(document, "get", lambda *_args, **_kwargs: response)

    with pytest.raises(Exception, match="document not found"):
        document.download()


@pytest.mark.p2
def test_download_raises_api_error_with_extra_response_fields(monkeypatch):
    response = _DownloadResponse(
        b'{"code": 102, "message": "document not found", "data": null}',
        {"code": 102, "message": "document not found", "data": None},
        {"Content-Type": "application/json"},
    )
    document = Document(None, {"id": "doc", "dataset_id": "dataset"})
    monkeypatch.setattr(document, "get", lambda *_args, **_kwargs: response)

    with pytest.raises(Exception, match="document not found"):
        document.download()


@pytest.mark.p2
@pytest.mark.parametrize(
    "content,payload",
    [
        (b'{"code": 0, "message": "success"}', {"code": 0, "message": "success"}),
        (b"plain text", ValueError("not JSON")),
    ],
)
def test_download_preserves_non_error_content_without_attachment(monkeypatch, content, payload):
    response = _DownloadResponse(content, payload, {"Content-Type": "application/json"})
    document = Document(None, {"id": "doc", "dataset_id": "dataset"})
    monkeypatch.setattr(document, "get", lambda *_args, **_kwargs: response)

    assert document.download() == content


@pytest.mark.p1
@pytest.mark.integration
@pytest.mark.parametrize(
    "generate_test_files",
    [
        "docx",
        "excel",
        "ppt",
        "image",
        "pdf",
        "txt",
        "md",
        "json",
        "eml",
        "html",
    ],
    indirect=True,
)
def test_file_type_validation(add_dataset_func, generate_test_files, request):
    dataset = add_dataset_func
    fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
    with fp.open("rb") as f:
        blob = f.read()

    documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])

    assert len(documents) == 1, str(documents)
    document = documents[0]
    assert document.dataset_id == dataset.id
    assert document.name == fp.name
    with fp.with_stem("ragflow_test_download").open("wb") as f:
        f.write(document.download())

    assert fp.read_bytes() == fp.with_stem("ragflow_test_download").read_bytes()


@pytest.mark.integration
class TestDocumentDownload:
    @pytest.mark.p3
    def test_download_empty_uploaded_file(self, add_dataset_func):
        dataset = add_dataset_func
        documents = dataset.upload_documents([{"display_name": "empty.txt", "blob": b""}])

        assert len(documents) == 1
        assert documents[0].download() == b""

    @pytest.mark.p3
    def test_same_file_repeat(self, add_document_func):
        num = 5
        _, document = add_document_func

        expected = b"RAGFlow document SDK integration test.\n"
        for _ in range(num):
            assert document.download() == expected

    @pytest.mark.p3
    def test_download_error_json_raises(self, add_document_func):
        dataset, document = add_document_func
        invalid_document = document.__class__(
            document.rag,
            {"id": "missing-document-id-for-download", "dataset_id": dataset.id},
        )
        with pytest.raises(Exception, match="Resource not found"):
            invalid_document.download()


@pytest.mark.p3
@pytest.mark.integration
def test_concurrent_download(add_dataset_func, tmp_path):
    count = 20
    dataset = add_dataset_func
    document_infos = []
    for i in range(count):
        fp = tmp_path / f"ragflow_test_upload_{i}.txt"
        fp.write_text("This is the content of a test TXT file.", encoding="utf-8")
        document_infos.append({"display_name": fp.name, "blob": fp.read_bytes()})
    documents = dataset.upload_documents(document_infos)
    assert len(documents) == count

    def download_doc(document, i):
        download_path = tmp_path / f"ragflow_test_download_{i}.txt"
        with download_path.open("wb") as f:
            f.write(document.download())
        return download_path

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(download_doc, documents[i], i) for i in range(count)]
        responses = [future.result() for future in as_completed(futures)]
    assert len(responses) == count

    for i in range(count):
        assert (tmp_path / f"ragflow_test_upload_{i}.txt").read_bytes() == (tmp_path / f"ragflow_test_download_{i}.txt").read_bytes()
