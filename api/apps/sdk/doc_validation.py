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
import math
import pathlib
import re

from api.constants import FILE_NAME_LEN_LIMIT
from api.db import FileType
from api.utils.validation_utils import UpdateDocumentReq
from common.constants import RetCode

def validate_immutable_fields(update_doc_req:UpdateDocumentReq, doc):
    """Validate that immutable fields have not been changed."""
    if update_doc_req.chunk_count and update_doc_req.chunk_count != int(getattr(doc, "chunk_num", -1)):
        return "Can't change `chunk_count`.", RetCode.DATA_ERROR

    if update_doc_req.token_count and update_doc_req.token_count != int(getattr(doc, "token_num", -1)):
        return "Can't change `token_num`.", RetCode.DATA_ERROR

    if update_doc_req.progress:
        progress_from_db = float(getattr(doc, "progress", -1.0))
        # should not use "==" to compare two float values
        if not math.isclose(update_doc_req.progress, progress_from_db):
            return "Can't change `progress`.", RetCode.DATA_ERROR

    return None, None


def validate_document_name(req_doc_name:str, doc, docs_from_name):
    """Validate document name update."""
    if len(req_doc_name.encode("utf-8")) > FILE_NAME_LEN_LIMIT:
        return f"File name must be {FILE_NAME_LEN_LIMIT} bytes or less.", RetCode.ARGUMENT_ERROR
    if pathlib.Path(req_doc_name.lower()).suffix != pathlib.Path(doc.name.lower()).suffix:
        return "The extension of file can't be changed", RetCode.ARGUMENT_ERROR

    for d in docs_from_name:
        if d.name == req_doc_name:
            return "Duplicated document name in the same dataset.", RetCode.DATA_ERROR
    return None, None

def validate_chunk_method(doc):
    """Validate chunk method update."""
    if doc.type == FileType.VISUAL or re.search(r"\.(ppt|pptx|pages)$", doc.name):
        return "Not supported yet!", RetCode.DATA_ERROR
    return None, None
