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
"DeepDOC"). These tests pin the helpers used to extract that value
without re-decoding the DSL twice.
"""

import json

from api.db.services.pipeline_operation_log_service import (
    _PARSER_SETUP_KEY_BY_SUFFIX,
    _load_dsl_mapping,
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


# --------------------------------------------------------------------------- #
# _load_dsl_mapping — decode + validate JSON DSL into a mapping
# --------------------------------------------------------------------------- #


def test_load_dsl_mapping_returns_mapping_for_valid_json():
    dsl = _dsl_with_parser("pdf", "docling")
    parsed = _load_dsl_mapping(dsl)
    assert isinstance(parsed, dict)
    assert "components" in parsed


def test_load_dsl_mapping_returns_none_for_invalid_json():
    """The PipelineOperationLog.create path used to crash on malformed DSL
    by re-running json.loads — see CodeRabbit review for #18306."""
    assert _load_dsl_mapping("not-json") is None


def test_load_dsl_mapping_returns_none_for_empty_string():
    assert _load_dsl_mapping("") is None


def test_load_dsl_mapping_returns_none_for_none_input():
    assert _load_dsl_mapping(None) is None  # type: ignore[arg-type]


def test_load_dsl_mapping_returns_none_for_json_array():
    """JSON arrays are valid JSON but not mappings — must be rejected so
    the caller falls back to document.parser_id instead of crashing
    downstream on ``dsl.get("components")``."""
    assert _load_dsl_mapping("[]") is None


def test_load_dsl_mapping_returns_none_for_json_string():
    assert _load_dsl_mapping('"just a string"') is None


def test_load_dsl_mapping_returns_none_for_json_number():
    assert _load_dsl_mapping("42") is None


def test_load_dsl_mapping_returns_dict_for_empty_object():
    """``{}`` is a valid mapping — should decode, even if it has no Parser
    component for the helper to extract."""
    assert _load_dsl_mapping("{}") == {}


def test_load_dsl_mapping_returns_none_for_empty_components_dict():
    """The create path falls back gracefully on ``{"components": []}``
    because the helper can't find a Parser component — the test asserts
    the mapping IS decoded (it's a valid mapping) but the parser lookup
    returns None."""
    parsed = _load_dsl_mapping(json.dumps({"components": []}))
    assert parsed == {"components": []}
    assert _parser_for_document_from_dsl(parsed, "pdf") is None


# --------------------------------------------------------------------------- #
# _parser_for_document_from_dsl — extract parse_method for the document family
# --------------------------------------------------------------------------- #


def test_extracts_docling_for_pdf_when_pipeline_parser_is_docling():
    """Regression for #18306: PDF in a pipeline with Docling parser should
    be logged as "docling", not "DeepDOC"."""
    dsl = _load_dsl_mapping(_dsl_with_parser("pdf", "docling"))
    assert _parser_for_document_from_dsl(dsl, "pdf") == "docling"


def test_extracts_docling_case_insensitively_for_uppercase_suffix():
    dsl = _load_dsl_mapping(_dsl_with_parser("pdf", "docling"))
    assert _parser_for_document_from_dsl(dsl, "PDF") == "docling"


def test_extracts_mineru_for_pdf_when_pipeline_parser_is_mineru():
    dsl = _load_dsl_mapping(_dsl_with_parser("pdf", "MinerU"))
    assert _parser_for_document_from_dsl(dsl, "pdf") == "MinerU"


def test_extracts_deepdoc_for_spreadsheet_when_pipeline_parser_is_deepdoc():
    """Non-PDF families fall back to their own setup key (issue #18306)."""
    dsl = _load_dsl_mapping(_dsl_with_parser("spreadsheet", "deepdoc"))
    assert _parser_for_document_from_dsl(dsl, "xlsx") == "deepdoc"


def test_extracts_paddleocr_for_image_family():
    dsl = _load_dsl_mapping(_dsl_with_parser("image", "paddleocr"))
    assert _parser_for_document_from_dsl(dsl, "png") == "paddleocr"


def test_extracts_deepdoc_when_pipeline_parser_is_deepdoc_for_pdf():
    """If the operator explicitly chose DeepDOC in the pipeline, the log
    should reflect that — not be silently overwritten."""
    dsl = _load_dsl_mapping(_dsl_with_parser("pdf", "deepdoc"))
    assert _parser_for_document_from_dsl(dsl, "pdf") == "deepdoc"


def test_returns_none_when_dsl_is_none():
    assert _parser_for_document_from_dsl(None, "pdf") is None


def test_returns_none_when_dsl_is_empty_mapping():
    assert _parser_for_document_from_dsl({}, "pdf") is None


def test_returns_none_when_parser_component_is_missing():
    dsl = _load_dsl_mapping(
        json.dumps(
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
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_returns_none_when_parser_setup_for_family_is_missing():
    """If the pipeline's Parser component has no setup for the document's
    file family, we don't guess — fall back to document.parser_id via
    the caller."""
    dsl = _load_dsl_mapping(_dsl_with_parser("image", "paddleocr"))
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_returns_none_when_suffix_is_unknown():
    dsl = _load_dsl_mapping(_dsl_with_parser("pdf", "docling"))
    assert _parser_for_document_from_dsl(dsl, "unknown-ext") is None
    assert _parser_for_document_from_dsl(dsl, "") is None


def test_returns_none_when_components_is_a_list():
    """Robustness: if the DSL has a non-mapping ``components`` value
    (e.g. a JSON array or a string), the helper must not crash."""
    dsl = _load_dsl_mapping(
        json.dumps(
            {
                "components": ["not", "a", "dict"],
                "path": [],
            }
        )
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_returns_none_when_a_component_is_a_non_dict():
    """Robustness: a non-mapping component entry must not crash."""
    dsl = _load_dsl_mapping(
        json.dumps(
            {
                "components": {"p": "not-a-dict"},
                "path": [],
            }
        )
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_returns_none_when_params_is_non_dict():
    """Robustness: a Parser component with non-mapping params must not
    crash (defends against future DSL schema changes)."""
    dsl = _load_dsl_mapping(
        json.dumps(
            {
                "components": {
                    "p": {
                        "obj": {
                            "component_name": "Parser",
                            "params": "not-a-dict",
                        },
                    },
                },
                "path": [],
            }
        )
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_returns_none_when_setup_value_is_non_dict():
    """Robustness: a Parser component whose setup entry for the family is
    not a mapping (e.g. a string, number, or list) must not crash."""
    dsl = _load_dsl_mapping(
        json.dumps(
            {
                "components": {
                    "p": {
                        "obj": {
                            "component_name": "Parser",
                            "params": {"setups": {"pdf": "not-a-dict"}},
                        },
                    },
                },
                "path": [],
            }
        )
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") is None


def test_extracts_docling_when_multiple_components_present():
    """Robustness: only the Parser component's setup matters, not Begin
    or other components with the same family key."""
    dsl = _load_dsl_mapping(
        json.dumps(
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
    )
    assert _parser_for_document_from_dsl(dsl, "pdf") == "docling"


# --------------------------------------------------------------------------- #
# Setup-key exhaustive guard
# --------------------------------------------------------------------------- #


def test_setup_key_map_covers_common_families():
    """Guards against accidentally dropping a family from the suffix map
    (which would silently re-introduce #18306 for that family), and
    against a suffix being mapped to the wrong setup key (which would
    record the parser for a different file family)."""
    expected = {
        "pdf": "pdf",
        "xls": "spreadsheet",
        "xlsx": "spreadsheet",
        "csv": "spreadsheet",
        "doc": "doc",
        "docx": "docx",
        "ppt": "slides",
        "pptx": "slides",
        "pages": "slides",
        "md": "markdown",
        "markdown": "markdown",
        "html": "html",
        "htm": "html",
        "jpg": "image",
        "jpeg": "image",
        "png": "image",
        "bmp": "image",
        "tif": "image",
        "tiff": "image",
        "webp": "image",
        "gif": "image",
        "mp3": "audio",
        "wav": "audio",
        "m4a": "audio",
        "flac": "audio",
        "ogg": "audio",
        "opus": "audio",
        "mp4": "video",
        "mov": "video",
        "avi": "video",
        "webm": "video",
        "mkv": "video",
        "eml": "email",
        "epub": "epub",
        "txt": "text&code",
        "json": "text&code",
        "log": "text&code",
    }
    # Verify both keys AND values — a suffix mapped to the wrong setup
    # key would silently log the parser for a different file family.
    for suffix, expected_setup_key in expected.items():
        assert _PARSER_SETUP_KEY_BY_SUFFIX[suffix] == expected_setup_key, (
            f"suffix {suffix!r} maps to {_PARSER_SETUP_KEY_BY_SUFFIX[suffix]!r}, "
            f"expected {expected_setup_key!r}"
        )
