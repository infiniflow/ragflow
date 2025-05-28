import pytest
import os
import tempfile
from unittest.mock import patch, MagicMock

from rag.app.markitdown_parser import MarkitdownParser
from rag.app import markitdown_parser as markitdown_module

def test_parse_filename():
    parser = MarkitdownParser()
    fn_test = "20250105发布_JK005-54748_【需求确认】XQSQ2024122500167 关于新增放开满期退保阻断的需求V1.0.docx"
    expected = {'publish_date': '20250105', 'jira_key': 'JK005-54748', 'doc_name': '【需求确认】XQSQ2024122500167 关于新增放开满期退保阻断的需求V1.0'}
    assert parser._parse_filename(fn_test) == expected

    fn_test_no_match = "mydocument.docx"
    expected_no_match = {'publish_date': '', 'jira_key': '', 'doc_name': 'mydocument'}
    assert parser._parse_filename(fn_test_no_match) == expected_no_match
    
    fn_test_case_insensitive = "20250105发布_JK005-54748_Test_Doc.DOCX"
    expected_case_insensitive = {'publish_date': '20250105', 'jira_key': 'JK005-54748', 'doc_name': 'Test_Doc'}
    assert parser._parse_filename(fn_test_case_insensitive) == expected_case_insensitive

def test_extract_markdown_headings():
    parser = MarkitdownParser()
    mock_md_content = """
# Project Background
Some text
## Sub heading
### Sub sub heading

# Next Top Level
"""
    expected_headings = ['# Project Background', '## Sub heading', '### Sub sub heading', '# Next Top Level']
    assert parser._extract_markdown_headings(mock_md_content) == expected_headings
    assert parser._extract_markdown_headings("") == []

def test_classify_headings_with_llm_mocked():
    # Note: The MarkitdownParser constructor initializes a mock LLM client if no real one is provided.
    # We are testing the parser's ability to use its llm_client and parse the response.
    parser = MarkitdownParser(llm_api_key="dummy_key") # Ensures llm_client is the MockLLMClient
    headings = ['# H1', '## H2', '### H3']
    
    # This mock response should be what our MockLLMClient in markitdown_parser.py would produce,
    # or what a real LLM would produce, for the given headings.
    # The MockLLMClient in the parser might have its own fixed responses based on input,
    # so we patch its `generate` method to control the output for this specific test.
    mock_llm_response_str = """
需求背景相关：
# H1
功能点1相关：
## H2
### H3
"""
    # Patch the `generate` method of the llm_client instance within the parser instance
    with patch.object(parser.llm_client, 'generate', return_value=mock_llm_response_str) as mock_generate_method:
        classified = parser._classify_headings_with_llm(headings)
        mock_generate_method.assert_called_once() 
        # The prompt is constructed inside _classify_headings_with_llm, so we check if generate was called.
        # We don't need to check the prompt argument precisely here unless it's critical and varies.

        expected_classified = {
            "需求背景": ["# H1"], # Keys are without "相关" due to parsing logic in _classify_headings_with_llm
            "功能点1": ["## H2", "### H3"]
        }
        assert classified == expected_classified

    # Test with empty headings list
    with patch.object(parser.llm_client, 'generate') as mock_generate_method_empty:
        assert parser._classify_headings_with_llm([]) == {}
        mock_generate_method_empty.assert_not_called() # generate shouldn't be called if headings is empty


def test_split_markdown_by_classified_headings():
    parser = MarkitdownParser()
    mock_md_content = """
# Project Background
Background details.
Some more background.
# Business Scene
Scene details.

## User Login
Login form.
### Functional Specification
Must be secure.
# Testing Points
Test everything.
"""
    # Keys in classified_headings should match what _classify_headings_with_llm produces (i.e., without "相关")
    classified_headings_for_split = {
        "需求背景": ["# Project Background"],
        "需求目标": ["# Business Scene"], 
        "功能点1": ["## User Login", "### Functional Specification"], # Assuming "### Functional Specification" is part of "功能点1"
        "测试要点": ["# Testing Points"]
    }

    split_chunks = parser._split_markdown_by_classified_headings(mock_md_content, classified_headings_for_split)
    
    assert len(split_chunks) == 4
    
    assert split_chunks[0][0] == "需求背景" # Category name from dict key
    assert split_chunks[0][1].strip() == """# Project Background
Background details.
Some more background."""

    assert split_chunks[1][0] == "需求目标"
    assert split_chunks[1][1].strip() == """# Business Scene
Scene details.""" 
    
    assert split_chunks[2][0] == "功能点1"
    # Content includes all sub-headings until the next main classified heading
    assert split_chunks[2][1].strip() == """## User Login
Login form.
### Functional Specification
Must be secure."""

    assert split_chunks[3][0] == "测试要点"
    assert split_chunks[3][1].strip() == """# Testing Points
Test everything."""

    assert parser._split_markdown_by_classified_headings(mock_md_content, {}) == [("全部内容", mock_md_content)]
    # If markdown content is empty, but classification is provided, it might return empty list or list of empty content depending on logic
    # Current logic of _split_markdown_by_classified_headings will try to find heading markers. If not found, returns empty list.
    assert parser._split_markdown_by_classified_headings("", classified_headings_for_split) == []


@patch('rag.app.markitdown_parser.MarkItDown') 
def test_parse_file_and_chunk_integration(MockMarkItDownClass):
    # Setup mock for MarkItDown converter
    mock_md_converter_instance = MockMarkItDownClass.return_value
    mock_md_converter_instance.convert.return_value = """
# Doc Title
Details about the doc.
## Section A
Content for A.
### Sub A1
More A1.
## Section B
Content for B.
# Final Section
The end.
"""
    
    # Expected classification result from the (mocked) LLM
    # Keys should be as produced by _classify_headings_with_llm (i.e. without "相关")
    expected_llm_classification = {
        "需求背景": ["# Doc Title"],
        "需求方案": ["## Section A", "### Sub A1"], 
        "功能点1": ["## Section B"],
        "测试要点": ["# Final Section"]
    }

    # Patch the _classify_headings_with_llm method within the MarkitdownParser class
    # This is called by the parse_file method, which is in turn called by the module-level chunk function
    with patch('rag.app.markitdown_parser.MarkitdownParser._classify_headings_with_llm', 
               return_value=expected_llm_classification) as mock_classify_llm_method:
        
        dummy_docx_binary = b"dummy docx content for testing"
        fn_test = "20240101发布_JIRA-123_My Test Document.docx"
        
        # Call the module-level chunk function
        result_chunks = markitdown_module.chunk(
            name=fn_test,
            binary=dummy_docx_binary,
            from_page=0, to_page=0, lang='en', callback=MagicMock(), kb_id='test_kb',
            parser_config={'llm_api_key': 'dummy'}, # Provide dummy LLM config for parser init
            tenant_id='test_tenant'
        )

        # Assertions
        mock_md_converter_instance.convert.assert_called_once()
        # Check that the temp file path was passed to convert
        args, _ = mock_md_converter_instance.convert.call_args
        assert isinstance(args[0], str) # Temp file path
        assert args[0].endswith(".docx")

        mock_classify_llm_method.assert_called_once()

        assert len(result_chunks) == 4 
        
        chunk1 = result_chunks[0]
        assert chunk1['source'] == fn_test
        assert chunk1['page_num'] == 1
        assert "文档名：My Test Document" in chunk1['content_with_weight']
        assert "上线时间：20240101" in chunk1['content_with_weight']
        assert "分类名：需求背景" in chunk1['content_with_weight'] # Check category name used
        assert "# Doc Title\nDetails about the doc." in chunk1['content_with_weight'] 
        assert "## Section A" not in chunk1['content_with_weight'] # Should be in next chunk

        chunk2 = result_chunks[1]
        assert "分类名：需求方案" in chunk2['content_with_weight']
        assert "## Section A\nContent for A.\n### Sub A1\nMore A1." in chunk2['content_with_weight']
        assert "## Section B" not in chunk2['content_with_weight'] # Should be in next chunk
        
        chunk3 = result_chunks[2]
        assert "分类名：功能点1" in chunk3['content_with_weight']
        assert "## Section B\nContent for B." in chunk3['content_with_weight']
        assert "# Final Section" not in chunk3['content_with_weight'] # Should be in next chunk

        chunk4 = result_chunks[3]
        assert "分类名：测试要点" in chunk4['content_with_weight']
        assert "# Final Section\nThe end." in chunk4['content_with_weight']

        # Test case for when conversion fails
        mock_md_converter_instance.convert.reset_mock()
        mock_md_converter_instance.convert.return_value = "" # Simulate conversion failure
        mock_classify_llm_method.reset_mock()

        result_chunks_fail_convert = markitdown_module.chunk(
            name=fn_test, binary=dummy_docx_binary, from_page=0, to_page=0, lang='en', 
            callback=None, kb_id='test_kb', parser_config={'llm_api_key': 'dummy'}, tenant_id='test_tenant'
        )
        assert len(result_chunks_fail_convert) == 1
        assert "无法转换文档" in result_chunks_fail_convert[0]['content_with_weight']
        mock_classify_llm_method.assert_not_called() # LLM classification should not be called if conversion fails

        # Test case for when no headings are extracted
        mock_md_converter_instance.convert.reset_mock()
        mock_md_converter_instance.convert.return_value = "No headings here, just plain text."
        mock_classify_llm_method.reset_mock() # Reset mock for _classify_headings_with_llm
        # Patch _extract_markdown_headings to return empty list for this specific sub-test
        with patch('rag.app.markitdown_parser.MarkitdownParser._extract_markdown_headings', return_value=[]) as mock_extract_empty:
            result_chunks_no_headings = markitdown_module.chunk(
                name=fn_test, binary=dummy_docx_binary, from_page=0, to_page=0, lang='en', 
                callback=None, kb_id='test_kb', parser_config={'llm_api_key': 'dummy'}, tenant_id='test_tenant'
            )
            assert len(result_chunks_no_headings) == 1
            assert "分类名：通用内容" in result_chunks_no_headings[0]['content_with_weight']
            assert "No headings here, just plain text." in result_chunks_no_headings[0]['content_with_weight']
            mock_extract_empty.assert_called_once()
            mock_classify_llm_method.assert_not_called() # LLM classification should not be called if no headings
