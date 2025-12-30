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
import json
from unittest.mock import patch, Mock
from rag.llm.ocr_model import MinerUOcrModel


class TestMinerUOcrModelConfigHandling:
    """Test cases for MinerU OCR model configuration handling"""

    def test_config_handles_dict_key(self):
        """Test that config handles dict key properly"""
        config_dict = {
            "api_key": {
                "mineru_apiserver": "http://test-api.com",
                "MINERU_BACKEND": "hybrid-auto-engine"
            }
        }
        
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            model = MinerUOcrModel(key=config_dict, model_name="test")
            assert model.mineru_api == "http://test-api.com"
            assert model.mineru_backend == "hybrid-auto-engine"

    def test_config_handles_json_string_key(self):
        """Test that config handles JSON string key properly"""
        config_dict = {
            "api_key": {
                "mineru_apiserver": "http://test-api.com",
                "MINERU_BACKEND": "hybrid"
            }
        }
        config_str = json.dumps(config_dict)
        
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            model = MinerUOcrModel(key=config_str, model_name="test")
            assert model.mineru_api == "http://test-api.com"
            assert model.mineru_backend == "hybrid"

    def test_config_handles_invalid_json(self):
        """Test that config handles invalid JSON gracefully"""
        invalid_json = "{invalid json"
        
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            model = MinerUOcrModel(key=invalid_json, model_name="test")
            # Should use empty config and defaults
            assert model.mineru_backend == "hybrid-auto-engine"

    def test_config_handles_empty_key(self):
        """Test that config handles empty key gracefully"""
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            model = MinerUOcrModel(key=None, model_name="test")
            # Should use defaults
            assert model.mineru_backend == "hybrid-auto-engine"
            assert model.mineru_delete_output == True

    def test_config_handles_flat_config(self):
        """Test that config handles flat (non-nested) config"""
        config_dict = {
            "MINERU_APISERVER": "http://flat-api.com",
            "MINERU_BACKEND": "pipeline"
        }
        
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            model = MinerUOcrModel(key=config_dict, model_name="test")
            assert model.mineru_api == "http://flat-api.com"
            assert model.mineru_backend == "pipeline"

    def test_config_normalizes_api_urls(self):
        """Test that API URLs are normalized (trailing slash removed)"""
        config_dict = {
            "mineru_apiserver": "http://test-api.com/",
            "mineru_server_url": "http://server.com/"
        }
        
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            model = MinerUOcrModel(key=config_dict, model_name="test")
            assert model.mineru_api == "http://test-api.com"
            assert model.mineru_server_url == "http://server.com"

    def test_config_handles_invalid_delete_output(self):
        """Test that invalid delete_output values are handled"""
        config_dict = {
            "mineru_delete_output": "invalid"
        }
        
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            model = MinerUOcrModel(key=config_dict, model_name="test")
            # Should default to True on error
            assert model.mineru_delete_output == True

    def test_config_resolves_env_vars(self):
        """Test that config resolves environment variables"""
        import os
        os.environ['MINERU_APISERVER'] = 'http://env-api.com'
        
        try:
            with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
                model = MinerUOcrModel(key={}, model_name="test")
                assert model.mineru_api == "http://env-api.com"
        finally:
            del os.environ['MINERU_APISERVER']

    def test_config_redacts_sensitive_data(self):
        """Test that sensitive config data is redacted in logs"""
        config_dict = {
            "api_key": "secret123",
            "password": "pass123",
            "normal_field": "visible"
        }
        
        with patch('rag.llm.ocr_model.MinerUParser.__init__', return_value=None):
            with patch('logging.info') as mock_log:
                model = MinerUOcrModel(key=config_dict, model_name="test")
                # Check that logging.info was called
                assert mock_log.called
                # The sensitive fields should be redacted in the log message
                log_call_args = str(mock_log.call_args)
                assert "[REDACTED]" in log_call_args or "REDACTED" in log_call_args


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
