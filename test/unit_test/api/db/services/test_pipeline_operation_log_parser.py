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

"""Regression tests for the PipelineOperationLog parser extraction (issue #18306).

When a Pipeline's Parser component is configured with ``parse_method =
"Docling"`` (or any non-default parser) for PDFs, the resulting dataflow
task should be logged with the actual pipeline parser, not the
``document.parser_id`` which may carry the KB default (typically
"DeepDOC"). These tests pin the helper used to extract that value.
"""

import json

from api.db.services.pipeline_operation_log_service import (
    _PARSER_SETUP_KEY_BY_SUFFIX,
    _parser_for_document_from_dsl,
)


def _dsl_with_parser(setup_key: str, parse_method: str) -> str:
    return json.dumps(
        {
            "components": {
                "parser-node": {
                    "obj": {
                        "component_name": "Parser",
                        "params": {
                            "setups": {
                                setup_key: {"parse_method": parse_method},
                            },
                        },
                    },
                },
            },
            "path": [],
        }
    )


def test_extracts_docling_for_pdf_when_pipeline_parser_is_docling():
    """Regression for #18306: PDF in a pipeline with Docling parser should
    be logged as "docling", not "DeepDOC"."""
    dsl = _dsl_with_parser("pdf", "docling")
    assert _parser_for_document_from_dsl(dsl, "pdf") == "docling"


def test_extracts_docling_case_insensitively_for_uppercase_suffix():
    assert _parser_for_document_from_dsl(_dsl_with_parser("pdf", "docling"), "PDF") == "docling"


def test_extracts_mineru_for_pdf_when_pipeline_parser_is_mineru():
    dsl = _dsl_with_parser("pdf", "MinerU")
    assert _parser_for_document_from_dsl(dsl, "pdf") == "MinerU"


def test_extracts_deepdoc_for_spreadsheet_when_pipeline_parser_is_deepdoc():
    """Non-PDF families fall back to their own setup key (issue #18306)."""
    dsl = _dsl_with_parser("spreadsheet", "deepdoc")
    assert _parser_for_document_from_dsl(dsl, "xlsx") == "deepdoc"


def test_extracts_paddleocr_for_image_family():
    dsl = _dsl_with_parser("image", "paddleocr")
    assert _parser_for_document_from_dsl(dsl, "png") == "paddleocr"


def test_returns_none_when_dsl_is_empty():
    assert _parser_for_document_from_dsl("{}", "pdf") is None
    assert _parser_for_document_from_dsl("", "pdf") is None
    assert _parser_for_document_from_dsl(None, "pdf") is None  # type: ignore[arg-type]


def test_returns_none_when_dsl_is_not_valid_json():
    assert _parser_for_document_from_dsl("not-json", "pdf") is None


def test_returns_none_when_parser_component_is_missing():
    dsl = json.dumps(
        {
            "components": {
                "begin": {
                    "obj": {
                        "component_name": "Begin",
                        "params": {"setups": {"pdf": {"parse_method": "docling"}}},
                    },
                },
            },
            "path": [],
        }
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_returns_none_when_parser_setup_for_family_is_missing():
    """If the pipeline's Parser component has no setup for the document's
    file family, we don't guess — fall back to document.parser_id via
    the caller."""
    dsl = _dsl_with_parser("image", "paddleocr")
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_returns_none_when_suffix_is_unknown():
    dsl = _dsl_with_parser("pdf", "docling")
    assert _parser_for_document_from_dsl(dsl, "unknown-ext") is None
    assert _parser_for_document_from_dsl(dsl, "") is None


def test_setup_key_map_covers_common_families():
    """Guards against accidentally dropping a family from the suffix map
    (which would silently re-introduce #18306 for that family)."""
    expected = {
        "pdf", "xls", "xlsx", "csv",
        "doc", "docx",
        "ppt", "pptx", "pages",
        "md", "markdown",
        "html", "htm",
        "jpg", "jpeg", "png", "bmp", "tif", "tiff", "webp", "gif",
        "mp3", "wav", "m4a", "flac", "ogg", "opus",
        "mp4", "mov", "avi", "webm", "mkv",
        "eml", "epub",
        "txt", "json", "log",
    }
    assert set(_PARSER_SETUP_KEY_BY_SUFFIX.keys()) >= expected


def test_extracts_deepdoc_when_pipeline_parser_is_deepdoc_for_pdf():
    """If the operator explicitly chose DeepDOC in the pipeline, the log
    should reflect that — not be silently overwritten."""
    dsl = _dsl_with_parser("pdf", "deepdoc")
    assert _parser_for_document_from_dsl(dsl, "pdf") == "deepdoc"


def test_extracts_docling_when_multiple_components_present():
    """Robustness: only the Parser component's setup matters, not Begin
    or other components with the same family key."""
    dsl = json.dumps(
        {
            "components": {
                "begin": {
                    "obj": {
                        "component_name": "Begin",
                        "params": {"setups": {"pdf": {"parse_method": "docling"}}},
                    },
                },
                "parser": {
                    "obj": {
                        "component_name": "Parser",
                        "params": {
                            "setups": {
                                "pdf": {"parse_method": "docling"},
                            },
                        },
                    },
                },
                "chunker": {
                    "obj": {
                        "component_name": "TokenChunker",
                        "params": {},
                    },
                },
            },
            "path": [],
        }
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") == "docling"
