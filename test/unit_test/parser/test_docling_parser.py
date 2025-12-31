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
from unittest.mock import Mock, patch, MagicMock, PropertyMock
from deepdoc.parser.docling_parser import DoclingParser


class TestDoclingParser:
    """Test cases for DoclingParser functionality"""

    @pytest.fixture
    def parser(self):
        """Create a DoclingParser instance for testing"""
        return DoclingParser()

    @pytest.fixture
    def mock_pdf_path(self):
        """Create a temporary mock PDF file"""
        with tempfile.NamedTemporaryFile(suffix='.pdf', delete=False) as f:
            pdf_path = Path(f.name)
            # Write some minimal PDF content
            f.write(b'%PDF-1.4\n')
        try:
            yield pdf_path
        finally:
            # Ensure cleanup happens even if test fails
            if pdf_path.exists():
                pdf_path.unlink()

    def test_parser_initialization(self, parser):
        """Test that DoclingParser initializes correctly"""
        assert parser.logger is not None
        assert isinstance(parser.page_images, list)
        assert parser.page_from == 0
        assert parser.page_to == 10_000
        assert isinstance(parser.outlines, list)

    def test_check_installation_docling_not_available(self, parser):
        """Test check_installation when docling is not available"""
        with patch('deepdoc.parser.docling_parser.DocumentConverter', None):
            # Force reimport to apply the patch
            from deepdoc.parser.docling_parser import DoclingParser as MockParser
            mock_parser = MockParser()
            result = mock_parser.check_installation()
            assert result is False

    def test_check_installation_docling_available(self, parser):
        """Test check_installation when docling is available"""
        with patch.object(parser, '_create_converter', return_value=Mock()):
            result = parser.check_installation()
            assert result is True

    def test_check_installation_converter_creation_fails(self, parser):
        """Test check_installation when converter creation fails"""
        with patch.object(parser, '_create_converter', side_effect=RuntimeError("Test error")):
            result = parser.check_installation()
            assert result is False

    def test_create_converter_not_available(self, parser):
        """Test _create_converter when DocumentConverter is not available"""
        with patch('deepdoc.parser.docling_parser.DocumentConverter', None):
            with pytest.raises(RuntimeError, match="DocumentConverter is not available"):
                parser._create_converter()

    def test_create_converter_with_configuration(self, parser):
        """Test _create_converter with configuration (docling 2.0+)"""
        mock_converter = Mock()
        
        with patch('deepdoc.parser.docling_parser.DocumentConverter', return_value=mock_converter):
            with patch('deepdoc.parser.docling_parser.PdfPipelineOptions', Mock()):
                with patch('deepdoc.parser.docling_parser.PdfFormatOption', Mock()):
                    with patch('deepdoc.parser.docling_parser.InputFormat', Mock(PDF=Mock())):
                        converter = parser._create_converter()
                        assert converter is not None

    def test_create_converter_fallback(self, parser):
        """Test _create_converter falls back to default when configuration fails"""
        mock_converter = Mock()
        
        # First call raises TypeError (config not supported), second call succeeds
        with patch('deepdoc.parser.docling_parser.DocumentConverter') as mock_dc:
            mock_dc.side_effect = [TypeError("Invalid argument"), mock_converter]
            with patch('deepdoc.parser.docling_parser.PdfPipelineOptions', Mock()):
                with patch('deepdoc.parser.docling_parser.PdfFormatOption', Mock()):
                    with patch('deepdoc.parser.docling_parser.InputFormat', Mock(PDF=Mock())):
                        converter = parser._create_converter()
                        assert converter is mock_converter
                        assert mock_dc.call_count == 2

    def test_parse_pdf_docling_not_available(self, parser, mock_pdf_path):
        """Test parse_pdf raises error when docling is not available"""
        with patch.object(parser, 'check_installation', return_value=False):
            with pytest.raises(RuntimeError, match="Docling not available"):
                parser.parse_pdf(filepath=str(mock_pdf_path))

    def test_parse_pdf_file_not_found(self, parser):
        """Test parse_pdf raises error when file doesn't exist"""
        with patch.object(parser, 'check_installation', return_value=True):
            with pytest.raises(FileNotFoundError):
                parser.parse_pdf(filepath="/nonexistent/path.pdf")

    def test_parse_pdf_with_binary_input(self, parser):
        """Test parse_pdf with binary input creates temporary file"""
        binary_content = b'%PDF-1.4\ntest content'
        mock_doc = Mock()
        mock_doc.num_pages = 1
        mock_doc.texts = []
        mock_doc.tables = []
        mock_doc.pictures = []
        
        mock_result = Mock()
        mock_result.document = mock_doc
        
        mock_converter = Mock()
        mock_converter.convert.return_value = mock_result
        
        with patch.object(parser, 'check_installation', return_value=True):
            with patch.object(parser, '_create_converter', return_value=mock_converter):
                with patch.object(parser, '__images__'):
                    with tempfile.TemporaryDirectory() as tmpdir:
                        sections, tables = parser.parse_pdf(
                            filepath="test.pdf",
                            binary=binary_content,
                            output_dir=tmpdir,
                            delete_output=True
                        )
                        
                        assert isinstance(sections, list)
                        assert isinstance(tables, list)
                        # Verify temporary file was cleaned up
                        temp_file = Path(tmpdir) / "test.pdf"
                        assert not temp_file.exists()

    def test_parse_pdf_callback_progress(self, parser, mock_pdf_path):
        """Test parse_pdf calls callback with progress updates"""
        callback = Mock()
        mock_doc = Mock()
        mock_doc.num_pages = 5
        mock_doc.texts = []
        mock_doc.tables = []
        mock_doc.pictures = []
        
        mock_result = Mock()
        mock_result.document = mock_doc
        
        mock_converter = Mock()
        mock_converter.convert.return_value = mock_result
        
        with patch.object(parser, 'check_installation', return_value=True):
            with patch.object(parser, '_create_converter', return_value=mock_converter):
                with patch.object(parser, '__images__'):
                    parser.parse_pdf(
                        filepath=str(mock_pdf_path),
                        callback=callback
                    )
                    
                    # Verify callback was called with progress updates
                    assert callback.call_count >= 5  # Multiple progress stages
                    # Check that final progress is 1.0
                    final_call = callback.call_args_list[-1]
                    assert final_call[0][0] == 1.0

    def test_parse_pdf_conversion_error_cleanup(self, parser):
        """Test parse_pdf cleans up temp file on conversion error"""
        binary_content = b'%PDF-1.4\ntest content'
        
        with patch.object(parser, 'check_installation', return_value=True):
            with patch.object(parser, '_create_converter', side_effect=RuntimeError("Conversion failed")):
                with patch.object(parser, '__images__'):
                    with tempfile.TemporaryDirectory() as tmpdir:
                        with pytest.raises(RuntimeError, match="Conversion failed"):
                            parser.parse_pdf(
                                filepath="test.pdf",
                                binary=binary_content,
                                output_dir=tmpdir,
                                delete_output=True
                            )
                        
                        # Verify temporary file was cleaned up even on error
                        temp_file = Path(tmpdir) / "test.pdf"
                        assert not temp_file.exists()

    def test_parse_pdf_extract_content_error(self, parser, mock_pdf_path):
        """Test parse_pdf handles content extraction errors properly"""
        mock_doc = Mock()
        mock_doc.num_pages = 1
        
        mock_result = Mock()
        mock_result.document = mock_doc
        
        mock_converter = Mock()
        mock_converter.convert.return_value = mock_result
        
        with patch.object(parser, 'check_installation', return_value=True):
            with patch.object(parser, '_create_converter', return_value=mock_converter):
                with patch.object(parser, '__images__'):
                    with patch.object(parser, '_transfer_to_sections', side_effect=RuntimeError("Extraction failed")):
                        with pytest.raises(RuntimeError, match="Failed to extract content"):
                            parser.parse_pdf(filepath=str(mock_pdf_path))

    def test_parse_pdf_different_parse_methods(self, parser, mock_pdf_path):
        """Test parse_pdf with different parse_method values"""
        mock_doc = Mock()
        mock_doc.num_pages = 1
        mock_doc.texts = []
        mock_doc.tables = []
        mock_doc.pictures = []
        
        mock_result = Mock()
        mock_result.document = mock_doc
        
        mock_converter = Mock()
        mock_converter.convert.return_value = mock_result
        
        with patch.object(parser, 'check_installation', return_value=True):
            with patch.object(parser, '_create_converter', return_value=mock_converter):
                with patch.object(parser, '__images__'):
                    for parse_method in ["raw", "manual", "paper"]:
                        sections, tables = parser.parse_pdf(
                            filepath=str(mock_pdf_path),
                            parse_method=parse_method
                        )
                        assert isinstance(sections, list)
                        assert isinstance(tables, list)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
