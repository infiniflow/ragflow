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
from common import bulk_upload_documents
from utils import compare_by_hash


@pytest.mark.p1
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
def test_file_type_validation(add_dataset, generate_test_files, request):
    dataset = add_dataset
    fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
    with fp.open("rb") as f:
        blob = f.read()

    documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])

    for document in documents:
        with fp.with_stem("ragflow_test_download").open("wb") as f:
            f.write(document.download())

        assert compare_by_hash(fp, fp.with_stem("ragflow_test_download"))


class TestDocumentDownload:
    @pytest.mark.p3
    def test_same_file_repeat(self, add_documents, tmp_path, ragflow_tmp_dir):
        num = 5
        _, documents = add_documents

        for i in range(num):
            download_path = tmp_path / f"ragflow_test_download_{i}.txt"
            with download_path.open("wb") as f:
                f.write(documents[0].download())
            assert compare_by_hash(ragflow_tmp_dir / "ragflow_test_upload_0.txt", download_path), f"Downloaded file {i} does not match original"


@pytest.mark.p3
def test_concurrent_download(add_dataset, tmp_path):
    count = 20
    dataset = add_dataset
    documents = bulk_upload_documents(dataset, count, tmp_path)

    def download_doc(document, i):
        download_path = tmp_path / f"ragflow_test_download_{i}.txt"
        with download_path.open("wb") as f:
            f.write(document.download())
        # assert compare_by_hash(tmp_path / f"ragflow_test_upload_{i}.txt", download_path), str(download_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(download_doc, documents[i], i) for i in range(count)]
    responses = list(as_completed(futures))
    assert len(responses) == count, responses

    for i in range(count):
        assert compare_by_hash(
            tmp_path / f"ragflow_test_upload_{i}.txt",
            tmp_path / f"ragflow_test_download_{i}.txt",
        )
