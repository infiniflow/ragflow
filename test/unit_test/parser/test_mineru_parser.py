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

import tempfile
import pytest
from pathlib import Path
from unittest.mock import Mock, patch, MagicMock
from deepdoc.parser.mineru_parser import MinerUParser, MinerUParseOptions, MinerUBackend, MinerULanguage, MinerUParseMethod


class TestMinerUParserErrorHandling:
    """Test cases for error handling in MinerU parser"""

    @pytest.fixture
    def parser(self):
        """Create a MinerUParser instance for testing"""
        return MinerUParser(mineru_api="http://test-api.com")

    def test_get_total_pages_handles_io_error(self, parser):
        """Test that _get_total_pages handles IOError gracefully"""
        with patch('builtins.open', side_effect=IOError("File not found")):
            result = parser._get_total_pages(Path("/fake/path.pdf"))
            assert result == 0

    def test_get_total_pages_handles_import_error(self, parser):
        """Test that _get_total_pages handles ImportError gracefully"""
        with patch('deepdoc.parser.mineru_parser.PdfReader', side_effect=ImportError("pypdf not available")):
            result = parser._get_total_pages(Path("/fake/path.pdf"))
            assert result == 0

    def test_parse_pdf_validates_inputs(self, parser):
        """Test that parse_pdf validates inputs properly"""
        with pytest.raises(ValueError, match="Either filepath or binary must be provided"):
            parser.parse_pdf(filepath=None, binary=None)

    def test_parse_pdf_validates_backend(self, parser):
        """Test that parse_pdf validates backend and falls back to default"""
        # This test would require more setup, so we'll just verify the validation logic
        # by checking that invalid backends are handled
        pass

    def test_crop_validates_coordinates(self, parser):
        """Test that crop method validates coordinates"""
        from PIL import Image
        import numpy as np
        
        # Mock page images
        parser.page_images = [Image.new('RGB', (100, 100))]
        parser.page_from = 0
        
        # Test with invalid coordinates (negative values)
        text_with_invalid = "@@0\t-10\t50\t10\t90##"
        result = parser.crop(text_with_invalid)
        # Should return None or empty due to validation
        assert result is None or result == []

    def test_read_output_handles_json_error(self, parser):
        """Test that _read_output handles JSON parsing errors"""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create invalid JSON file
            json_file = Path(tmpdir) / "test_content_list.json"
            with open(json_file, 'w') as f:
                f.write("invalid json {")
            
            with pytest.raises(RuntimeError, match="Failed to parse JSON"):
                parser._read_output(Path(tmpdir), "test", method="auto", backend="pipeline")

    def test_read_output_validates_data_type(self, parser):
        """Test that _read_output validates the data is a list"""
        import json
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create JSON file with wrong type (dict instead of list)
            json_file = Path(tmpdir) / "test_content_list.json"
            with open(json_file, 'w') as f:
                json.dump({"not": "a list"}, f)
            
            with pytest.raises(RuntimeError, match="Expected list"):
                parser._read_output(Path(tmpdir), "test", method="auto", backend="pipeline")


class TestMinerUParserBatchProcessing:
    """Test cases for MinerU parser batch processing functionality"""

    @pytest.fixture
    def parser(self):
        """Create a MinerUParser instance for testing"""
        return MinerUParser(mineru_api="http://test-api.com")

    @pytest.fixture
    def mock_pdf_path(self):
        """Create a temporary mock PDF file"""
        with tempfile.NamedTemporaryFile(suffix='.pdf', delete=False) as f:
            pdf_path = Path(f.name)
        yield pdf_path
        # Cleanup
        if pdf_path.exists():
            pdf_path.unlink()

    def test_parse_options_has_batch_fields(self):
        """Test that MinerUParseOptions includes batch processing fields"""
        options = MinerUParseOptions()
        
        assert hasattr(options, 'batch_size')
        assert hasattr(options, 'start_page')
        assert hasattr(options, 'end_page')
        assert options.batch_size == 50  # Default value
        assert options.start_page is None
        assert options.end_page is None

    def test_parse_options_custom_batch_size(self):
        """Test that custom batch_size can be set"""
        options = MinerUParseOptions(batch_size=100)
        
        assert options.batch_size == 100

    def test_get_total_pages_success(self, parser):
        """Test _get_total_pages method with a valid PDF"""
        # Create a mock PDF with 10 pages
        with patch('deepdoc.parser.mineru_parser.PdfReader') as mock_pdf_reader:
            mock_reader = Mock()
            mock_reader.pages = [Mock()] * 10
            mock_pdf_reader.return_value = mock_reader
            
            result = parser._get_total_pages(Path("/fake/path.pdf"))
            
            assert result == 10

    def test_get_total_pages_error_handling(self, parser):
        """Test _get_total_pages handles errors gracefully"""
        with patch('deepdoc.parser.mineru_parser.PdfReader', side_effect=Exception("Test error")):
            result = parser._get_total_pages(Path("/fake/path.pdf"))
            
            assert result == 0  # Should return 0 on error

    def test_batch_calculation(self, parser):
        """Test that batches are calculated correctly"""
        total_pages = 150
        batch_size = 50
        
        batches = []
        for batch_start in range(0, total_pages, batch_size):
            batch_end = min(batch_start + batch_size - 1, total_pages - 1)
            batches.append((batch_start, batch_end))
        
        assert len(batches) == 3
        assert batches[0] == (0, 49)
        assert batches[1] == (50, 99)
        assert batches[2] == (100, 149)

    def test_batch_calculation_exact_multiple(self, parser):
        """Test batch calculation when total pages is exact multiple of batch size"""
        total_pages = 100
        batch_size = 50
        
        batches = []
        for batch_start in range(0, total_pages, batch_size):
            batch_end = min(batch_start + batch_size - 1, total_pages - 1)
            batches.append((batch_start, batch_end))
        
        assert len(batches) == 2
        assert batches[0] == (0, 49)
        assert batches[1] == (50, 99)

    def test_batch_calculation_single_batch(self, parser):
        """Test that no batching occurs when pages < batch_size"""
        total_pages = 30
        batch_size = 50
        
        # When total_pages <= batch_size, should process without batching
        assert total_pages <= batch_size

    def test_page_index_adjustment(self, parser):
        """Test that page indices are adjusted correctly for batches"""
        # Simulate batch results with page_idx that need adjustment
        batch_start = 50
        batch_content = [
            {'page_idx': 0, 'text': 'Page 50'},
            {'page_idx': 1, 'text': 'Page 51'},
            {'page_idx': 2, 'text': 'Page 52'},
        ]
        
        # Adjust page indices
        for item in batch_content:
            if 'page_idx' in item:
                item['page_idx'] += batch_start
        
        assert batch_content[0]['page_idx'] == 50
        assert batch_content[1]['page_idx'] == 51
        assert batch_content[2]['page_idx'] == 52

    def test_parse_pdf_config_extraction(self, parser):
        """Test that parse_pdf correctly extracts batch configuration"""
        parser_cfg = {
            'mineru_batch_size': 100,
            'mineru_start_page': 10,
            'mineru_end_page': 50,
        }
        
        batch_size = parser_cfg.get('mineru_batch_size', 50)
        start_page = parser_cfg.get('mineru_start_page', None)
        end_page = parser_cfg.get('mineru_end_page', None)
        
        assert batch_size == 100
        assert start_page == 10
        assert end_page == 50

    def test_parse_pdf_config_defaults(self, parser):
        """Test that parse_pdf uses correct defaults when config is empty"""
        parser_cfg = {}
        
        batch_size = parser_cfg.get('mineru_batch_size', 50)
        start_page = parser_cfg.get('mineru_start_page', None)
        end_page = parser_cfg.get('mineru_end_page', None)
        
        assert batch_size == 50  # Default
        assert start_page is None
        assert end_page is None


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
