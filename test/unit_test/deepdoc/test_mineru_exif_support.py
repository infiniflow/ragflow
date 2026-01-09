"""
Unit tests for MinerU EXIF orientation correction support.
"""

import pytest
from pathlib import Path
from unittest.mock import Mock, patch, mock_open

from deepdoc.parser.mineru_parser import (
    MinerUParser,
    MinerUBackend,
    MinerULanguage,
    MinerUParseMethod
)

class TestMinerUExifSupport:
    """Test EXIF orientation correction features in MinerU parser"""

    def setup_method(self):
        """Setup test fixtures"""
        self.parser = MinerUParser(mineru_api="http://test-api")

    @patch('requests.post')
    def test_exif_correction_enabled_in_request(self, mock_post):
        """Test that exif_correction is enabled in API requests"""
        # Mock successful response
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.headers = {"Content-Type": "application/json"}
        mock_response.content = b"fake zip content"
        mock_post.return_value = mock_response

        with patch('tempfile.mkdtemp') as mock_mkdtemp, \
             patch('tempfile.mkstemp') as mock_mkstemp, \
             patch('os.path.exists'), \
             patch('os.makedirs'), \
             patch('builtins.open', mock_open()), \
             patch.object(self.parser, '_extract_zip_no_root'):
            
            # Mock the temp directories
            mock_mkdtemp.side_effect = ["/tmp/test_output", "/tmp/test_batch"]
            mock_mkstemp.return_value = (None, "/tmp/test_file.zip")

            # Create test options with exif correction enabled
            from deepdoc.parser.mineru_parser import MinerUParseOptions
            
            options = MinerUParseOptions(
                backend=MinerUBackend.PIPELINE,
                lang=MinerULanguage.EN,
                method=MinerUParseMethod.AUTO,
                server_url=None,
                delete_output=True,
                parse_method="raw",
                formula_enable=True,
                table_enable=True,
                exif_correction=True
            )

            # Mock file operations 
            with patch('builtins.open', Mock()), \
                 patch('os.unlink'), \
                 patch('shutil.rmtree'):
                
                try:
                    # Call the API (return value is not used in this test)
                    self.parser._run_mineru_api(
                        Path("/tmp/test.pdf"), 
                        Path("/tmp/output"),
                        options,
                        None  # callback
                    )
                    
                    # Verify the POST call was made with exif_correction parameter
                    assert mock_post.called
                    call_args = mock_post.call_args
                    assert "data" in call_args[1]
                    assert "exif_correction" in call_args[1]["data"]
                    assert call_args[1]["data"]["exif_correction"] is True
                    call_args = mock_post.call_args
                    assert "data" in call_args[1]
                    assert "exif_correction" in call_args[1]["data"]
                    assert call_args[1]["data"]["exif_correction"] is True
                    
                except Exception as e:
                    print(f"Test failed with exception: {e}")
                    raise

    def test_mineru_backend_enum_values(self):
        """Test that all MinerU backend enum values are correctly defined"""
        # Test the backends we know should exist based on our implementation
        assert hasattr(MinerUBackend, 'PIPELINE') 
        assert hasattr(MinerUBackend, 'HYBRID_AUTO_ENGINE')
        assert hasattr(MinerUBackend, 'HYBRID')
        assert hasattr(MinerUBackend, 'VLM_HTTP_CLIENT')
        assert hasattr(MinerUBackend, 'VLM_VLLM_ASYNC_ENGINE')
        
    def test_mineru_language_enum_values(self):
        """Test that MinerU language enum values are correctly defined"""
        # Test some key languages
        assert hasattr(MinerULanguage, 'EN')
        assert hasattr(MinerULanguage, 'CH')
        assert hasattr(MinerULanguage, 'JAPAN')

    def test_mineru_parse_method_enum_values(self):
        """Test that MinerU parse method enum values are correctly defined"""
        # Test parse methods
        assert hasattr(MinerUParseMethod, 'AUTO')
        assert hasattr(MinerUParseMethod, 'TXT') 
        assert hasattr(MinerUParseMethod, 'OCR')

if __name__ == "__main__":
    pytest.main([__file__, "-v"])
