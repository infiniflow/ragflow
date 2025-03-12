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
from common import delete_dataset
from libs.utils.file_utils import (
    create_docx_file,
    create_eml_file,
    create_excel_file,
    create_html_file,
    create_image_file,
    create_json_file,
    create_md_file,
    create_pdf_file,
    create_ppt_file,
    create_txt_file,
)


@pytest.fixture(scope="function", autouse=True)
def clear_datasets(get_http_api_auth):
    yield
    delete_dataset(get_http_api_auth)


@pytest.fixture
def generate_test_files(tmp_path):
    files = {}
    files["docx"] = tmp_path / "ragflow_test.docx"
    create_docx_file(files["docx"])

    files["excel"] = tmp_path / "ragflow_test.xlsx"
    create_excel_file(files["excel"])

    files["ppt"] = tmp_path / "ragflow_test.pptx"
    create_ppt_file(files["ppt"])

    files["image"] = tmp_path / "ragflow_test.png"
    create_image_file(files["image"])

    files["pdf"] = tmp_path / "ragflow_test.pdf"
    create_pdf_file(files["pdf"])

    files["txt"] = tmp_path / "ragflow_test.txt"
    create_txt_file(files["txt"])

    files["md"] = tmp_path / "ragflow_test.md"
    create_md_file(files["md"])

    files["json"] = tmp_path / "ragflow_test.json"
    create_json_file(files["json"])

    files["eml"] = tmp_path / "ragflow_test.eml"
    create_eml_file(files["eml"])

    files["html"] = tmp_path / "ragflow_test.html"
    create_html_file(files["html"])

    return files
