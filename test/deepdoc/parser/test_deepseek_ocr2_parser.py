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
import sys
import os
import types

import pytest

# Add project root to path for direct module import
project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
sys.path.insert(0, project_root)

# Create minimal mock modules for dependencies before importing
deepdoc_module = types.ModuleType('deepdoc')
deepdoc_parser_module = types.ModuleType('deepdoc.parser')
deepdoc_parser_pdf_parser_module = types.ModuleType('deepdoc.parser.pdf_parser')
deepdoc_parser_pdf_parser_module.RAGFlowPdfParser = object

sys.modules['deepdoc'] = deepdoc_module
sys.modules['deepdoc.parser'] = deepdoc_parser_module
sys.modules['deepdoc.parser.pdf_parser'] = deepdoc_parser_pdf_parser_module

# Direct import from file to avoid triggering full deepdoc package dependencies
import importlib.util
spec = importlib.util.spec_from_file_location(
    "deepseek_ocr2_parser",
    os.path.join(project_root, "deepdoc/parser/deepseek_ocr2_parser.py")
)
deepseek_ocr2_parser = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deepseek_ocr2_parser)
DeepSeekOcr2Parser = deepseek_ocr2_parser.DeepSeekOcr2Parser


def test_parser_init():
    """Test parser initialization with default config."""
    parser = DeepSeekOcr2Parser()
    assert parser is not None
    assert parser.model_name == "deepseek-ai/DeepSeek-OCR-2"


def test_parser_init_custom_model():
    """Test parser initialization with custom model path."""
    parser = DeepSeekOcr2Parser(model_path="/custom/path")
    assert parser.model_path == "/custom/path"


def test_http_backend_init():
    """Test HTTP backend initialization."""
    parser = DeepSeekOcr2Parser(
        backend="http",
        api_url="http://localhost:8000/v1/ocr",
        api_key="test-key"
    )
    assert parser.backend == "http"
    assert parser.api_url == "http://localhost:8000/v1/ocr"


def test_local_backend_is_default():
    """Test local backend is default."""
    parser = DeepSeekOcr2Parser()
    assert parser.backend == "local"


def test_check_available_http_invalid_url():
    """Test HTTP endpoint availability check with invalid URL."""
    parser = DeepSeekOcr2Parser(
        backend="http",
        api_url="http://invalid-url:9999/v1/ocr"
    )
    available, reason = parser.check_available()
    assert available is False
    assert "not accessible" in reason.lower() or "not configured" in reason.lower()


def test_check_available_local_backend():
    """Test local backend availability check."""
    parser = DeepSeekOcr2Parser(backend="local", device="cpu")
    available, reason = parser.check_available()
    # Should check for torch availability - may pass or fail depending on env
    assert isinstance(available, bool)
    assert isinstance(reason, str)


def test_parser_prompts_defined():
    """Test that parsing prompts are correctly defined."""
    parser = DeepSeekOcr2Parser()
    assert hasattr(parser, 'PROMPT_DOCUMENT_PARSE')
    assert hasattr(parser, 'PROMPT_FREE_OCR')
    assert "<image>" in parser.PROMPT_DOCUMENT_PARSE
    assert "markdown" in parser.PROMPT_DOCUMENT_PARSE.lower()
    assert "<image>" in parser.PROMPT_FREE_OCR


def test_check_installation():
    """Test check_installation method returns expected format."""
    parser = DeepSeekOcr2Parser()
    result = parser.check_installation()
    assert isinstance(result, tuple)
    assert len(result) == 2
    assert isinstance(result[0], bool)
    assert isinstance(result[1], str)


def test_http_backend_missing_url():
    """Test HTTP backend with missing URL returns not available."""
    parser = DeepSeekOcr2Parser(backend="http", api_url=None)
    available, reason = parser.check_available()
    assert available is False
    assert "url" in reason.lower() or "configured" in reason.lower()
