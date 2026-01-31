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
import importlib.util

import pytest

# Add project root to path for direct module import
project_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
sys.path.insert(0, project_root)

# Mock DeepSeekOcr2Parser and DeepSeekOcr2Backend
class MockDeepSeekOcr2Parser:
    def __init__(self, **kwargs):
        self.model_path = kwargs.get('model_path')
        self.device = kwargs.get('device', 'cuda')
        self.use_flash_attn = kwargs.get('use_flash_attn', True)
        self.backend = kwargs.get('backend', 'local')
        self.api_url = kwargs.get('api_url')
        self.api_key = kwargs.get('api_key')

    def check_available(self):
        return True, ""

    def parse_pdf(self, **kwargs):
        return [], []


class MockDeepSeekOcr2Backend:
    LOCAL = "local"
    HTTP = "http"


class MockMinerUParser:
    pass


# Create minimal mock modules for deepdoc dependencies
deepdoc_module = types.ModuleType('deepdoc')
deepdoc_parser_module = types.ModuleType('deepdoc.parser')
deepdoc_parser_module.__path__ = []

deepdoc_parser_deepseek_ocr2_module = types.ModuleType('deepdoc.parser.deepseek_ocr2_parser')
deepdoc_parser_deepseek_ocr2_module.DeepSeekOcr2Parser = MockDeepSeekOcr2Parser
deepdoc_parser_deepseek_ocr2_module.DeepSeekOcr2Backend = MockDeepSeekOcr2Backend

deepdoc_parser_mineru_parser_module = types.ModuleType('deepdoc.parser.mineru_parser')
deepdoc_parser_mineru_parser_module.MinerUParser = MockMinerUParser

sys.modules['deepdoc'] = deepdoc_module
sys.modules['deepdoc.parser'] = deepdoc_parser_module
sys.modules['deepdoc.parser.deepseek_ocr2_parser'] = deepdoc_parser_deepseek_ocr2_module
sys.modules['deepdoc.parser.mineru_parser'] = deepdoc_parser_mineru_parser_module


def _load_ocr_model():
    """Load ocr_model.py directly to avoid full rag.llm import."""
    spec = importlib.util.spec_from_file_location(
        "ocr_model",
        os.path.join(project_root, "rag/llm/ocr_model.py")
    )
    ocr_model = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(ocr_model)
    return ocr_model


def test_deepseek_ocr2_class_exists():
    """Test DeepSeekOcr2Model class can be imported."""
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = getattr(ocr_model, 'DeepSeekOcr2Model', None)
    assert DeepSeekOcr2Model is not None, "DeepSeekOcr2Model class not found in ocr_model.py"
    assert hasattr(DeepSeekOcr2Model, "_FACTORY_NAME")
    assert DeepSeekOcr2Model._FACTORY_NAME == "DeepSeek-OCR2"


def test_deepseek_ocr2_instantiation():
    """Test DeepSeek-OCR2 model can be instantiated."""
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
    model = DeepSeekOcr2Model(
        key='{"backend": "local"}',
        model_name="test-model"
    )
    assert model is not None
    assert model.backend == "local"


def test_deepseek_ocr2_backend_config():
    """Test backend configuration is correctly parsed."""
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
    
    # Test HTTP backend
    model = DeepSeekOcr2Model(
        key='{"backend": "http", "api_url": "http://localhost:8000/v1/ocr"}',
        model_name="test-model"
    )
    assert model.backend == "http"
    assert model.api_url == "http://localhost:8000/v1/ocr"


def test_deepseek_ocr2_handles_invalid_flash_attn_config():
    """Test model handles various use_flash_attn config values without crashing."""
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
    
    # Empty string should not crash
    model1 = DeepSeekOcr2Model(key='{"use_flash_attn": ""}', model_name="test")
    assert model1 is not None
    
    # String "true" should not crash
    model2 = DeepSeekOcr2Model(key='{"use_flash_attn": "true"}', model_name="test")
    assert model2.use_flash_attn is True
    
    # String "false" should not crash
    model3 = DeepSeekOcr2Model(key='{"use_flash_attn": "false"}', model_name="test")
    assert model3.use_flash_attn is False
    
    # Numeric "1" should work
    model4 = DeepSeekOcr2Model(key='{"use_flash_attn": "1"}', model_name="test")
    assert model4.use_flash_attn is True
    
    # Numeric "0" should work
    model5 = DeepSeekOcr2Model(key='{"use_flash_attn": "0"}', model_name="test")
    assert model5.use_flash_attn is False


def test_deepseek_ocr2_env_config():
    """Test configuration from environment variables."""
    import os
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
    
    # Set env vars
    os.environ['DEEPSEEK_OCR2_BACKEND'] = 'http'
    os.environ['DEEPSEEK_OCR2_API_URL'] = 'http://env-test:8000/v1/ocr'
    
    try:
        # Empty key should fall back to env vars
        model = DeepSeekOcr2Model(key='{}', model_name="test")
        assert model.backend == "http"
        assert model.api_url == "http://env-test:8000/v1/ocr"
    finally:
        # Clean up
        del os.environ['DEEPSEEK_OCR2_BACKEND']
        del os.environ['DEEPSEEK_OCR2_API_URL']


def test_deepseek_ocr2_check_available():
    """Test check_available method."""
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
    
    model = DeepSeekOcr2Model(key='{"backend": "local"}', model_name="test")
    result = model.check_available()
    assert isinstance(result, tuple)
    assert len(result) == 2
    assert isinstance(result[0], bool)


def test_deepseek_ocr2_parse_pdf_method_exists():
    """Test parse_pdf method exists and is callable."""
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
    
    model = DeepSeekOcr2Model(key='{"backend": "local"}', model_name="test")
    assert hasattr(model, 'parse_pdf')
    assert callable(model.parse_pdf)


def test_deepseek_ocr2_invalid_json_key():
    """Test model handles invalid JSON key gracefully."""
    ocr_model = _load_ocr_model()
    DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
    
    # Should not crash with invalid JSON
    model = DeepSeekOcr2Model(key='not-valid-json', model_name="test")
    assert model is not None
    # Should use defaults
    assert model.backend == "local"
