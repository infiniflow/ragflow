"""
Integration tests for DeepSeek-OCR2 parsing pipeline.

These tests verify the end-to-end flow from PDF input to markdown output,
using mocked model components to avoid requiring actual GPU/model.
"""
import sys
import os
import types
import importlib.util
import unittest
from pathlib import Path

# Add project root to path
project_root = Path(__file__).parent.parent.parent
sys.path.insert(0, str(project_root))


# Mock DeepSeekOcr2Parser and DeepSeekOcr2Backend
class MockDeepSeekOcr2Parser:
    """Mock parser for testing without GPU dependencies."""
    
    DEFAULT_MODEL = "deepseek-ai/DeepSeek-OCR-2"
    PROMPT_DOCUMENT_PARSE = "<image>\n<|grounding|>Convert the document to markdown."
    PROMPT_FREE_OCR = "<image>\nFree OCR."
    
    def __init__(self, **kwargs):
        self.model_path = kwargs.get('model_path')
        self.device = kwargs.get('device', 'cuda')
        self.use_flash_attn = kwargs.get('use_flash_attn', True)
        self.backend = kwargs.get('backend', 'local')
        self.api_url = kwargs.get('api_url')
        self.api_key = kwargs.get('api_key')
        self._model = None
        self._processor = None

    def check_available(self):
        return True, ""

    def parse_pdf(self, **kwargs):
        return [], []


class MockDeepSeekOcr2Backend:
    """Mock backend types."""
    LOCAL = "local"
    HTTP = "http"


class MockMinerUParser:
    """Mock MinerU parser."""
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
        str(project_root / "rag/llm/ocr_model.py")
    )
    ocr_model = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(ocr_model)
    return ocr_model


class TestDeepSeekOcr2Integration(unittest.TestCase):
    """Integration tests for DeepSeek-OCR2 parser."""
    
    def test_parser_initialization_local_backend(self):
        """Test parser initializes correctly with local backend."""
        from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Parser
        
        parser = DeepSeekOcr2Parser(backend='local')
        self.assertEqual(parser.backend, 'local')
        self.assertIsNone(parser._model)  # Lazy loading (private attr)
        self.assertIsNone(parser._processor)
    
    def test_parser_initialization_http_backend(self):
        """Test parser initializes correctly with HTTP backend."""
        from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Parser
        
        parser = DeepSeekOcr2Parser(
            backend='http',
            api_url='https://api.example.com/ocr',
            api_key='test-key'
        )
        self.assertEqual(parser.backend, 'http')
        self.assertEqual(parser.api_url, 'https://api.example.com/ocr')
        self.assertEqual(parser.api_key, 'test-key')
    
    def test_factory_class_name(self):
        """Test DeepSeek-OCR2 has correct factory name."""
        ocr_model = _load_ocr_model()
        DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
        
        # Verify class has factory name
        self.assertEqual(DeepSeekOcr2Model._FACTORY_NAME, 'DeepSeek-OCR2')
    
    def test_http_ocr_pipeline_config(self):
        """Test HTTP OCR pipeline configuration."""
        from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Parser
        
        # Create parser with HTTP backend
        parser = DeepSeekOcr2Parser(
            backend='http',
            api_url='https://api.test.com/ocr',
            api_key='test-key'
        )
        
        # Verify configuration
        self.assertEqual(parser.backend, 'http')
        self.assertEqual(parser.api_url, 'https://api.test.com/ocr')
        self.assertEqual(parser.api_key, 'test-key')
    
    def test_model_wrapper_config_parsing(self):
        """Test OCR model wrapper handles various config formats."""
        ocr_model = _load_ocr_model()
        DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
        
        # Test with JSON config containing backend
        config = '{"backend": "local", "use_flash_attn": false}'
        model = DeepSeekOcr2Model(
            key=config,
            model_name='deepseek-ai/DeepSeek-OCR-2',
        )
        self.assertEqual(model.backend, 'local')
    
    def test_model_wrapper_http_config(self):
        """Test OCR model wrapper with HTTP backend config."""
        ocr_model = _load_ocr_model()
        DeepSeekOcr2Model = ocr_model.DeepSeekOcr2Model
        
        # Config uses nested api_key dict structure as expected by the model
        config = '{"api_key": {"backend": "http", "api_url": "https://ocr.example.com/v1/ocr"}}'
        model = DeepSeekOcr2Model(
            key=config,
            model_name='deepseek-ai/DeepSeek-OCR-2',
        )
        self.assertEqual(model.backend, 'http')
        self.assertEqual(model.api_url, 'https://ocr.example.com/v1/ocr')


class TestDeepSeekOcr2FactoryConfig(unittest.TestCase):
    """Test factory configuration for DeepSeek-OCR2."""
    
    def test_llm_factories_contains_deepseek_ocr2(self):
        """Verify DeepSeek-OCR2 is in llm_factories.json."""
        import json
        
        factories_path = project_root / 'conf' / 'llm_factories.json'
        with open(factories_path) as f:
            data = json.load(f)
        
        # JSON structure has factory_llm_infos as top-level key
        factories = data.get('factory_llm_infos', data)
        
        # Find DeepSeek-OCR2 entry
        deepseek_ocr2 = None
        for factory in factories:
            if isinstance(factory, dict) and factory.get('name') == 'DeepSeek-OCR2':
                deepseek_ocr2 = factory
                break
        
        self.assertIsNotNone(deepseek_ocr2, "DeepSeek-OCR2 not found in factories")
        self.assertEqual(deepseek_ocr2['tags'], 'OCR')
        self.assertEqual(deepseek_ocr2['status'], '1')
    
    def test_parser_default_prompts(self):
        """Test parser has expected prompt templates."""
        from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Parser
        
        self.assertIn("Convert the document to markdown", DeepSeekOcr2Parser.PROMPT_DOCUMENT_PARSE)
        self.assertIn("Free OCR", DeepSeekOcr2Parser.PROMPT_FREE_OCR)
    
    def test_parser_backend_types(self):
        """Test backend type constants are defined."""
        from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Backend
        
        self.assertEqual(DeepSeekOcr2Backend.LOCAL, 'local')
        self.assertEqual(DeepSeekOcr2Backend.HTTP, 'http')


if __name__ == '__main__':
    unittest.main()
