import pytest
import os
import tempfile
from unittest.mock import patch, MagicMock, call # Ensure call is imported if used for sequence

from rag.app.markitdown_parser import MarkitdownParser
from rag.app import markitdown_parser as markitdown_module

# Helper function to create a parser instance for tests, allowing easy LLM mocking
def create_test_parser(llm_generate_mock_return_value=None):
    parser = MarkitdownParser(llm_api_key="dummy_key", llm_base_url="dummy_url", llm_model_name="dummy_model")
    if llm_generate_mock_return_value is not None:
        # The llm_client is an instance of MockLLMClient (or a real one if configured).
        # We need to mock its `generate` method.
        parser.llm_client.generate = MagicMock(return_value=llm_generate_mock_return_value)
    return parser

def test_parse_filename():
    parser = create_test_parser()
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
    assert parser._classify_headings_with_llm([]) == [] # Expect empty list for empty input
    # Test with LLM returning empty or whitespace string
    parser.llm_client.generate = MagicMock(return_value="  ")
    assert parser._classify_headings_with_llm(["# Some heading"]) == []
    parser.llm_client.generate = MagicMock(return_value="") # Restore or set new mock for subsequent tests if needed
    assert parser._classify_headings_with_llm(["# Some heading"]) == []


def test_extract_headings_with_lineno():
    parser = create_test_parser()
    md_content = """# Title 1
Content 1
## Subtitle 1.1
Content 1.1

# Title 2
Content 2
"""
    expected = [
        ("# Title 1", 0),
        ("## Subtitle 1.1", 2),
        ("# Title 2", 5)
    ]
    assert parser._extract_headings_with_lineno(md_content) == expected
    assert parser._extract_headings_with_lineno("") == []
    assert parser._extract_headings_with_lineno("No headings here") == []


def test_split_markdown_by_llm_selected_headings():
    parser = create_test_parser()
    mock_md_content = """# Head1
Content for Head1.
## SubHead1A
More content for SubHead1A.
# Head2
Content for Head2.
### SubHead2A
Detail for SubHead2A.
# Head3
Content for Head3.
"""
    # Scenario 1: Standard case
    llm_selected_headings1 = ["# Head1", "## SubHead1A", "# Head2", "# Head3"]
    split_chunks1 = parser._split_markdown_by_llm_selected_headings(mock_md_content, llm_selected_headings1)
    assert len(split_chunks1) == 4
    assert split_chunks1[0][0] == "# Head1"
    assert split_chunks1[0][1] == "# Head1\nContent for Head1." # Up to next LLM selected heading ## SubHead1A
    assert split_chunks1[1][0] == "## SubHead1A"
    assert split_chunks1[1][1] == "## SubHead1A\nMore content for SubHead1A." # Up to next LLM selected heading # Head2
    assert split_chunks1[2][0] == "# Head2"
    assert split_chunks1[2][1] == "# Head2\nContent for Head2.\n### SubHead2A\nDetail for SubHead2A." # Up to next LLM selected heading # Head3
    assert split_chunks1[3][0] == "# Head3"
    assert split_chunks1[3][1] == "# Head3\nContent for Head3." # To EOF

    # Scenario 2: Empty llm_selected_start_headings
    assert parser._split_markdown_by_llm_selected_headings(mock_md_content, []) == [("全部内容", mock_md_content)]
    assert parser._split_markdown_by_llm_selected_headings("", []) == [] # Empty content, empty selection

    # Scenario 3: LLM selected heading not in original document
    llm_selected_headings2 = ["# Head1", "# NonExistentHead", "# Head3"]
    split_chunks2 = parser._split_markdown_by_llm_selected_headings(mock_md_content, llm_selected_headings2)
    assert len(split_chunks2) == 2 # NonExistentHead is skipped
    assert split_chunks2[0][0] == "# Head1" 
    # Extends to # Head3 because # NonExistentHead is skipped
    assert split_chunks2[0][1] == "# Head1\nContent for Head1.\n## SubHead1A\nMore content for SubHead1A.\n# Head2\nContent for Head2.\n### SubHead2A\nDetail for SubHead2A."
    assert split_chunks2[1][0] == "# Head3"

    # Scenario 4: Markdown content is empty
    assert parser._split_markdown_by_llm_selected_headings("", ["# Head1"]) == []

    # Scenario 5: No headings in original markdown, but LLM returns some (should be handled by original_headings_with_lineno check)
    md_no_headings = "Just plain text.\nNo markers here."
    assert parser._split_markdown_by_llm_selected_headings(md_no_headings, ["# Bogus LLM Heading"]) == [("全部内容", md_no_headings)]
    
    # Scenario 6: LLM returns headings out of document order (robustness check)
    llm_selected_headings_outoforder = ["# Head2", "# Head1"]
    split_chunks_outoforder = parser._split_markdown_by_llm_selected_headings(mock_md_content, llm_selected_headings_outoforder)
    assert len(split_chunks_outoforder) == 2
    assert split_chunks_outoforder[0][0] == "# Head2" 
    # This chunk for #Head2 will extend to end, because #Head1 is before it or not used as next marker
    assert "Content for Head2." in split_chunks_outoforder[0][1]
    assert split_chunks_outoforder[1][0] == "# Head1"
    assert "Content for Head1." in split_chunks_outoforder[1][1]
    # The exact content needs careful verification based on the implementation's handling of out-of-order.
    # Current logic: if next LLM heading is before current, current extends to next *valid* LLM heading or EOF.

@patch('rag.app.markitdown_parser.MarkItDown') 
def test_parse_file_and_chunk_integration(MockMarkItDownClass):
    mock_md_converter_instance = MockMarkItDownClass.return_value
    
    mock_md_content = """# Doc Title
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
    mock_md_converter_instance.convert.return_value = mock_md_content
    
    # Define the pipe-separated string that the LLM's `generate` method will be mocked to return.
    llm_response_pipe_separated = "# Doc Title|## Section A|## Section B|# Final Section"
    # This list is what _classify_headings_with_llm is expected to produce from the pipe-separated string.
    expected_llm_selected_headings = ["# Doc Title", "## Section A", "## Section B", "# Final Section"]

    dummy_docx_binary = b"dummy docx content"
    fn_test = "20240101发布_JIRA-123_My Test Document.docx"
    
    # We need to mock the llm_client.generate method that will be called by 
    # the MarkitdownParser instance created *inside* the chunk function.
    # The easiest way to do this is to patch MarkitdownParser._get_mock_llm_client
    # to return a MagicMock instance where `generate` is also a MagicMock.
    
    mock_llm_client_instance = MagicMock()
    mock_llm_client_instance.generate = MagicMock(return_value=llm_response_pipe_separated)

    with patch('rag.app.markitdown_parser.MarkitdownParser._get_mock_llm_client', return_value=mock_llm_client_instance) as mock_get_llm_client:
        result_chunks = markitdown_module.chunk(
            name=fn_test,
            binary=dummy_docx_binary,
            from_page=0, to_page=0, lang='en', callback=MagicMock(), kb_id='test_kb',
            parser_config={'llm_api_key': 'dummy_config_key'}, # Passed to parser's __init__
            tenant_id='test_tenant'
        )

        mock_md_converter_instance.convert.assert_called_once()
        args, _ = mock_md_converter_instance.convert.call_args
        assert isinstance(args[0], str) 
        assert args[0].endswith(".docx")

        # Check that _get_mock_llm_client was called (by parser's __init__)
        mock_get_llm_client.assert_called_once_with(api_key='dummy_config_key', base_url=None, model='default_model')
        
        # Check that the llm_client.generate (which we mocked) was called by _classify_headings_with_llm
        mock_llm_client_instance.generate.assert_called_once()
        # (Add more specific prompt assertion if necessary)

        assert len(result_chunks) == len(expected_llm_selected_headings)
        
        # Verify content of each chunk based on the new format "标题：{block_start_heading}"
        # Chunk 1: # Doc Title
        chunk1_content = result_chunks[0]['content_with_weight']
        assert "文档名：My Test Document" in chunk1_content
        assert "上线时间：20240101" in chunk1_content
        assert "标题：# Doc Title" in chunk1_content
        assert "# Doc Title\nDetails about the doc." in chunk1_content
        assert "## Section A" not in chunk1_content # Boundary check

        # Chunk 2: ## Section A
        chunk2_content = result_chunks[1]['content_with_weight']
        assert "标题：## Section A" in chunk2_content
        assert "## Section A\nContent for A.\n### Sub A1\nMore A1." in chunk2_content
        assert "## Section B" not in chunk2_content # Boundary check

        # Chunk 3: ## Section B
        chunk3_content = result_chunks[2]['content_with_weight']
        assert "标题：## Section B" in chunk3_content
        assert "## Section B\nContent for B." in chunk3_content
        assert "# Final Section" not in chunk3_content # Boundary check
        
        # Chunk 4: # Final Section
        chunk4_content = result_chunks[3]['content_with_weight']
        assert "标题：# Final Section" in chunk4_content
        assert "# Final Section\nThe end." in chunk4_content

    # Test case for when conversion fails
    mock_md_converter_instance.reset_mock()
    mock_md_converter_instance.convert.return_value = "" 
    mock_llm_client_instance.generate.reset_mock() # Reset for the next call
    
    with patch('rag.app.markitdown_parser.MarkitdownParser._get_mock_llm_client', return_value=mock_llm_client_instance):
        result_chunks_fail_convert = markitdown_module.chunk(
            name=fn_test, binary=dummy_docx_binary, from_page=0, to_page=0, lang='en', 
            callback=None, kb_id='test_kb', parser_config={}, tenant_id='test_tenant'
        )
        assert len(result_chunks_fail_convert) == 1
        assert "无法转换文档" in result_chunks_fail_convert[0]['content_with_weight']
        mock_llm_client_instance.generate.assert_not_called()

    # Test case for when no headings are extracted from markdown
    mock_md_converter_instance.reset_mock()
    mock_md_converter_instance.convert.return_value = "No headings here, just plain text."
    mock_llm_client_instance.generate.reset_mock()

    # No need to patch _extract_markdown_headings if parse_file handles its empty output correctly
    with patch('rag.app.markitdown_parser.MarkitdownParser._get_mock_llm_client', return_value=mock_llm_client_instance):
        result_chunks_no_headings = markitdown_module.chunk(
            name=fn_test, binary=dummy_docx_binary, from_page=0, to_page=0, lang='en', 
            callback=None, kb_id='test_kb', parser_config={}, tenant_id='test_tenant'
        )
        assert len(result_chunks_no_headings) == 1
        content = result_chunks_no_headings[0]['content_with_weight']
        assert "文档名：My Test Document" in content
        assert "标题：My Test Document" in content # Fallback title is doc name
        assert "No headings here, just plain text." in content
        mock_llm_client_instance.generate.assert_not_called() # _classify_headings should not be called if no headings

    # Test case: LLM returns empty list of headings
    mock_md_converter_instance.reset_mock()
    mock_md_converter_instance.convert.return_value = mock_md_content # Has headings
    
    # Mock LLM to return empty string, resulting in empty list from _classify_headings_with_llm
    empty_llm_response_mock_client = MagicMock()
    empty_llm_response_mock_client.generate = MagicMock(return_value="") # LLM gives empty response
    
    with patch('rag.app.markitdown_parser.MarkitdownParser._get_mock_llm_client', return_value=empty_llm_response_mock_client):
        result_chunks_empty_llm = markitdown_module.chunk(
            name=fn_test, binary=dummy_docx_binary, from_page=0, to_page=0, lang='en', 
            callback=None, kb_id='test_kb', parser_config={}, tenant_id='test_tenant'
        )
        assert len(result_chunks_empty_llm) == 1
        content = result_chunks_empty_llm[0]['content_with_weight']
        assert "文档名：My Test Document" in content
        assert "标题：原始内容" in content # Fallback title for this case in parse_file
        assert mock_md_content in content # Full original content
        empty_llm_response_mock_client.generate.assert_called_once()
