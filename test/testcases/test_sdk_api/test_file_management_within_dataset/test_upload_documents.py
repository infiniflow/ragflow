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
import string
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

import pytest

DOCUMENT_NAME_MAX_BYTES = 255
TEXT_FILE_CONTENT = "This is the content of a test TXT file."


def _create_text_file(path: Path) -> Path:
    path.write_text(TEXT_FILE_CONTENT, encoding="utf-8")
    return path


@pytest.mark.integration
class TestDocumentsUpload:
    @pytest.mark.p1
    def test_valid_single_upload(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        fp = _create_text_file(tmp_path / "ragflow_test.txt")
        with fp.open("rb") as f:
            blob = f.read()

        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        assert len(documents) == 1, str(documents)
        document = documents[0]
        assert document.dataset_id == dataset.id, str(document)
        assert document.name == fp.name, str(document)
        assert [item.id for item in dataset.list_documents(id=document.id)] == [document.id]

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
    def test_file_type_validation(self, add_dataset_func, generate_test_files, request):
        dataset = add_dataset_func
        fp = generate_test_files[request.node.callspec.params["generate_test_files"]]

        with fp.open("rb") as f:
            blob = f.read()

        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        assert len(documents) == 1, str(documents)
        document = documents[0]
        assert document.dataset_id == dataset.id, str(document)
        assert document.name == fp.name, str(document)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "file_type",
        ["exe", "unknown"],
    )
    def test_unsupported_file_type(self, add_dataset_func, tmp_path, file_type):
        dataset = add_dataset_func
        fp = tmp_path / f"ragflow_test.{file_type}"
        fp.touch()

        with fp.open("rb") as f:
            blob = f.read()

        with pytest.raises(Exception) as exception_info:
            dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        assert str(exception_info.value) == f"ragflow_test.{file_type}: This type of file has not been supported yet!", str(exception_info.value)

    @pytest.mark.p2
    def test_missing_file(self, add_dataset_func):
        dataset = add_dataset_func
        with pytest.raises(Exception) as exception_info:
            dataset.upload_documents([])
        assert str(exception_info.value) == "No file part!", str(exception_info.value)

    @pytest.mark.p3
    def test_empty_file(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        fp = tmp_path / "empty.txt"
        fp.touch()

        with fp.open("rb") as f:
            blob = f.read()

        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        assert len(documents) == 1, str(documents)
        document = documents[0]
        assert document.size == 0, str(document)
        assert [item.id for item in dataset.list_documents(id=document.id)] == [document.id]

    @pytest.mark.p3
    def test_filename_empty(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        fp = _create_text_file(tmp_path / "ragflow_test.txt")

        with fp.open("rb") as f:
            blob = f.read()

        with pytest.raises(Exception) as exception_info:
            dataset.upload_documents([{"display_name": "", "blob": blob}])
        assert str(exception_info.value) == "No file part!", str(exception_info.value)

    @pytest.mark.p2
    def test_filename_max_length(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        fp = _create_text_file(tmp_path / f"{'a' * (DOCUMENT_NAME_MAX_BYTES - len('.txt'))}.txt")

        with fp.open("rb") as f:
            blob = f.read()

        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        assert len(documents) == 1, str(documents)
        document = documents[0]
        assert document.dataset_id == dataset.id, str(document)
        assert document.name == fp.name, str(document)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "display_name",
        [
            "a" * 252 + ".txt",
            "界" * 84 + ".txt",
        ],
    )
    def test_filename_over_limit(self, add_dataset_func, display_name):
        assert len(display_name.encode("utf-8")) == DOCUMENT_NAME_MAX_BYTES + 1

        with pytest.raises(Exception, match="File name must be 255 bytes or less\\."):
            add_dataset_func.upload_documents([{"display_name": display_name, "blob": b"content"}])

    @pytest.mark.p2
    def test_partial_upload_persists_successful_file(self, add_dataset_func):
        dataset = add_dataset_func
        content = b"valid document in a mixed upload"

        with pytest.raises(Exception, match="rejected\\.exe: This type of file has not been supported yet!"):
            dataset.upload_documents(
                [
                    {"display_name": "accepted.txt", "blob": content},
                    {"display_name": "rejected.exe", "blob": b"not an executable"},
                ]
            )

        documents = dataset.list_documents(page_size=10)
        assert len(documents) == 1
        assert documents[0].name == "accepted.txt"
        assert documents[0].download() == content

    @pytest.mark.p2
    def test_duplicate_files(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        fp = _create_text_file(tmp_path / "ragflow_test.txt")

        with fp.open("rb") as f:
            blob = f.read()

        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}, {"display_name": fp.name, "blob": blob}])

        assert len(documents) == 2, str(documents)
        for i, document in enumerate(documents):
            assert document.dataset_id == dataset.id, str(document)
            expected_name = fp.name if i == 0 else f"{fp.stem}({i}){fp.suffix}"
            assert document.name == expected_name, str(document)

    @pytest.mark.p3
    def test_same_file_repeat(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        fp = _create_text_file(tmp_path / "ragflow_test.txt")

        with fp.open("rb") as f:
            blob = f.read()

        for i in range(3):
            documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
            assert len(documents) == 1, str(documents)
            document = documents[0]
            assert document.dataset_id == dataset.id, str(document)
            expected_name = fp.name if i == 0 else f"{fp.stem}({i}){fp.suffix}"
            assert document.name == expected_name, str(document)

    @pytest.mark.p3
    def test_filename_special_characters(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        illegal_chars = '<>:"/\\|?*'
        translation_table = str.maketrans({char: "_" for char in illegal_chars})
        safe_filename = string.punctuation.translate(translation_table)
        fp = tmp_path / f"{safe_filename}.txt"
        fp.write_text("Sample text content")

        with fp.open("rb") as f:
            blob = f.read()

        documents = dataset.upload_documents([{"display_name": fp.name, "blob": blob}])
        assert len(documents) == 1, str(documents)
        document = documents[0]
        assert document.dataset_id == dataset.id, str(document)
        assert document.name == fp.name, str(document)

    @pytest.mark.p3
    def test_multiple_files(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        expected_document_count = 20
        document_infos = []
        for i in range(expected_document_count):
            fp = _create_text_file(tmp_path / f"ragflow_test_upload_{i}.txt")
            with fp.open("rb") as f:
                blob = f.read()
            document_infos.append({"display_name": fp.name, "blob": blob})
        documents = dataset.upload_documents(document_infos)
        assert len(documents) == expected_document_count, str(documents)

        retrieved_documents = dataset.list_documents(page_size=expected_document_count)
        assert {document.id for document in retrieved_documents} == {document.id for document in documents}

    @pytest.mark.p3
    def test_concurrent_upload(self, add_dataset_func, tmp_path):
        dataset = add_dataset_func
        count = 20
        fps = [_create_text_file(tmp_path / f"ragflow_test_{i}.txt") for i in range(count)]

        def upload_file(fp):
            with fp.open("rb") as f:
                blob = f.read()
            return dataset.upload_documents([{"display_name": fp.name, "blob": blob}])

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(upload_file, fp) for fp in fps]
        uploaded = []
        for future in as_completed(futures):
            result = future.result()
            assert len(result) == 1, result
            uploaded.extend(result)

        assert len(uploaded) == count
        retrieved_documents = dataset.list_documents(page_size=count)
        assert {document.id for document in retrieved_documents} == {document.id for document in uploaded}
