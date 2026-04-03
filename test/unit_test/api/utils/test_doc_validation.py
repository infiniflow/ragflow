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

"""Unit tests for api.apps.sdk.doc_validation module."""

from unittest.mock import Mock
from api.utils.validation_utils import (
    validate_immutable_fields,
    validate_document_name,
    validate_chunk_method
)
from api.constants import FILE_NAME_LEN_LIMIT
from api.db import FileType
from common.constants import RetCode
from api.utils.validation_utils import UpdateDocumentReq


def test_validate_immutable_fields_no_changes():
    """Test when no immutable fields are present in request."""
    update_doc_req = UpdateDocumentReq()
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg is None
    assert error_code is None


def test_validate_immutable_fields_chunk_count_matches():
    """Test when chunk_count matches the document's chunk_num."""
    update_doc_req = UpdateDocumentReq(chunk_count=10)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg is None
    assert error_code is None


def test_validate_immutable_fields_token_count_matches():
    """Test when token_count matches the document's token_num."""
    update_doc_req = UpdateDocumentReq(token_count=100)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg is None
    assert error_code is None


def test_validate_immutable_fields_progress_matches():
    """Test when progress matches the document's progress."""
    update_doc_req = UpdateDocumentReq(progress=0.5)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg is None
    assert error_code is None


def test_validate_immutable_fields_chunk_count_mismatch():
    """Test when chunk_count doesn't match the document's chunk_num."""
    update_doc_req = UpdateDocumentReq(chunk_count=15)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg == "Can't change `chunk_count`."
    assert error_code == RetCode.DATA_ERROR


def test_validate_immutable_fields_token_count_mismatch():
    """Test when token_count doesn't match the document's token_num."""
    update_doc_req = UpdateDocumentReq(token_count=150)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg == "Can't change `token_count`."
    assert error_code == RetCode.DATA_ERROR


def test_validate_immutable_fields_progress_mismatch():
    """Test when progress doesn't match the document's progress."""
    update_doc_req = UpdateDocumentReq(progress=0.75)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg == "Can't change `progress`."
    assert error_code == RetCode.DATA_ERROR


def test_validate_immutable_fields_progress_boundary_values():
    """Test progress with boundary values (0.0 and 1.0)."""
    # Test with 0.0
    update_doc_req = UpdateDocumentReq(progress=0.0)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.0
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg is None
    assert error_code is None
    
    # Test with 1.0
    update_doc_req = UpdateDocumentReq(progress=1.0)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 1.0
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg is None
    assert error_code is None


def test_validate_immutable_fields_none_values():
    """Test when request fields are None."""
    update_doc_req = UpdateDocumentReq(chunk_count=None, token_count=None, progress=None)
    doc = Mock()
    doc.chunk_num = 10
    doc.token_num = 100
    doc.progress = 0.5
    
    error_msg, error_code = validate_immutable_fields(update_doc_req, doc)
    assert error_msg is None
    assert error_code is None


def test_validate_document_name_valid():
    """Test valid document name update."""
    req_doc_name = "new_document.pdf"
    doc = Mock()
    doc.name = "old_document.pdf"

    docs_from_name = []

    error_msg, error_code = validate_document_name(req_doc_name, doc, docs_from_name)
    assert error_msg is None
    assert error_code is None

def test_validate_document_name_attr_error():
    """Test valid document name update."""
    req_doc_name = 0
    doc = Mock()
    doc.name = "old_document.pdf"

    docs_from_name = []

    error_msg, error_code = validate_document_name(req_doc_name, doc, docs_from_name)
    assert error_msg == f"AttributeError('{type(req_doc_name).__name__}' object has no attribute 'encode')"
    assert error_code == RetCode.EXCEPTION_ERROR


def test_validate_document_name_exceeds_byte_limit():
    """Test when name exceeds byte limit."""
    long_name = "a" * (FILE_NAME_LEN_LIMIT + 1)
    doc = Mock()
    doc.name = "old_document.pdf"

    docs_from_name = []

    error_msg, error_code = validate_document_name(long_name, doc, docs_from_name)
    assert f"File name must be {FILE_NAME_LEN_LIMIT} bytes or less." in error_msg
    assert error_code == RetCode.ARGUMENT_ERROR


def test_validate_document_name_different_extension():
    """Test when extension is different from original."""
    req_doc_name = "new_document.docx"
    doc = Mock()
    doc.name = "old_document.pdf"

    docs_from_name = []

    error_msg, error_code = validate_document_name(req_doc_name, doc, docs_from_name)
    assert "The extension of file can't be changed" in error_msg
    assert error_code == RetCode.ARGUMENT_ERROR


def test_validate_document_name_duplicate():
    """Test when name already exists in the same dataset."""
    req_doc_name = "duplicate.pdf"
    doc = Mock()
    doc.name = "original.pdf"

    duplicate_doc = Mock()
    duplicate_doc.name = "duplicate.pdf"
    docs_from_name = [duplicate_doc]

    error_msg, error_code = validate_document_name(req_doc_name, doc, docs_from_name)
    assert "Duplicated document name in the same dataset." in error_msg
    assert error_code == RetCode.DATA_ERROR


def test_validate_document_name_case_insensitive_extension():
    """Test that extension check is case-insensitive."""
    req_doc_name = "new_document.PDF"
    doc = Mock()
    doc.name = "old_document.pdf"

    docs_from_name = []

    error_msg, error_code = validate_document_name(req_doc_name, doc, docs_from_name)
    assert error_msg is None
    assert error_code is None


def test_validate_chunk_method_valid():
    """Test with a valid chunk method."""
    doc = Mock()
    doc.type = FileType.PDF
    doc.name = "document.pdf"
    
    error_msg, error_code = validate_chunk_method(doc)
    assert error_msg is None
    assert error_code is None


def test_validate_chunk_method_visual_not_supported():
    """Test that visual file types are not supported."""
    doc = Mock()
    doc.type = FileType.VISUAL
    doc.name = "image.jpg"
    
    error_msg, error_code = validate_chunk_method(doc)
    assert "Not supported yet!" in error_msg
    assert error_code == RetCode.DATA_ERROR


def test_validate_chunk_method_ppt_not_supported():
    """Test that PPT files are not supported."""
    doc = Mock()
    doc.type = FileType.PDF
    doc.name = "presentation.ppt"
    
    error_msg, error_code = validate_chunk_method(doc)
    assert "Not supported yet!" in error_msg
    assert error_code == RetCode.DATA_ERROR


def test_validate_chunk_method_pptx_not_supported():
    """Test that PPTX files are not supported."""
    doc = Mock()
    doc.type = FileType.PDF
    doc.name = "presentation.pptx"
    
    error_msg, error_code = validate_chunk_method(doc)
    assert "Not supported yet!" in error_msg
    assert error_code == RetCode.DATA_ERROR


def test_validate_chunk_method_pages_not_supported():
    """Test that Pages files are not supported."""
    doc = Mock()
    doc.type = FileType.PDF
    doc.name = "document.pages"
    
    error_msg, error_code = validate_chunk_method(doc)
    assert "Not supported yet!" in error_msg
    assert error_code == RetCode.DATA_ERROR


def test_validate_chunk_method_other_extensions_still_valid():
    """Test that other file extensions are still valid."""
    doc = Mock()
    doc.type = FileType.PDF
    doc.name = "document.docx"
    
    error_msg, error_code = validate_chunk_method(doc)
    assert error_msg is None
    assert error_code is None