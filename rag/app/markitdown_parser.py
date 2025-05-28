import re
import tempfile
import os
from unittest.mock import MagicMock # Added for __main__ block
from markitdown import MarkItDown # Assuming this is the correct import
# We might need to import LLM utilities later
# from some_ragflow_llm_utility import RAGFlowLLMClient # Hypothetical placeholder for actual client

class MarkitdownParser:
    def __init__(self, llm_api_key=None, llm_base_url=None, llm_model_name=None):
        self.md_converter = MarkItDown(enable_plugins=False)
        self.llm_api_key = llm_api_key
        self.llm_base_url = llm_base_url
        self.llm_model_name = llm_model_name
        # Ensure llm_client is initialized, preferably here.
        self.llm_client = self._get_mock_llm_client(api_key=self.llm_api_key, base_url=self.llm_base_url, model=self.llm_model_name)

    def _get_mock_llm_client(self, api_key=None, base_url=None, model=None): # Added defaults for fallback
        class MockLLMClient:
            def __init__(self, inner_api_key, inner_base_url, inner_model): # Renamed params to avoid clash
                # This print can be removed once real client is integrated
                print(f"MockLLMClient initialized for new prompt logic (key: {'set' if inner_api_key else 'not set'}, base_url: {inner_base_url}, model: {inner_model})")
                self.api_key = inner_api_key
                self.base_url = inner_base_url
                self.model = inner_model

            def generate(self, prompt):
                # This print can be removed once real client is integrated
                print(f"MockLLMClient received prompt (first 400 chars):\n{prompt[:400]}...")
                # Simulate LLM response based on the new prompt's expectation from the subtask description
                if "背景" in prompt and "功能点1" in prompt and "功能点2" in prompt: # Based on example in subtask
                     return "# 背景|# 调研|# 名词解释|# 方案|## 概述|## 功能点1|## 功能点2"
                elif "My Doc Title" in prompt and "Section 1" in prompt and "Section 2" in prompt: # For a different test case
                     return "# My Doc Title|## Section 1|### Detail 1.1|## Section 2"
                elif "# Fallback Title 1" in prompt and "# Fallback Title 2" in prompt: # Specific test for this
                     return "# Fallback Title 1|# Fallback Title 2" # Ensure mock returns what's expected in a test
                else: # Generic fallback for other cases
                     # Extract some headings from the prompt to make mock more dynamic for basic tests (from subtask description)
                     headings_list_str = prompt.split("下面是文档标题列表：\n")[-1] if "下面是文档标题列表：\n" in prompt else ""
                     headings_in_prompt = [line for line in headings_list_str.splitlines() if line.strip().startswith("#")]
                     if len(headings_in_prompt) >= 2:
                         return f"{headings_in_prompt[0].strip()}|{headings_in_prompt[-1].strip()}"
                     elif headings_in_prompt:
                         return headings_in_prompt[0].strip()
                     return "# Default Mocked Title" # Absolute fallback
        
        # This method is responsible for returning an instance of MockLLMClient
        # The check for self.llm_client's existence is done in _classify_headings_with_llm
        return MockLLMClient(api_key, base_url, model) # Pass the arguments received by _get_mock_llm_client

    def _parse_filename(self, filename_with_ext):
        # Filename format: {发布日期}发布_{jira-key}_{文档名}.docx
        # Example: 20250105发布_JK005-54748_【需求确认】XQSQ2024122500167 关于新增放开满期退保阻断的需求V1.0.docx
        # This function should extract and return a dictionary:
        # {
        #     "publish_date": "YYYY-MM-DD" or raw string,
        #     "jira_key": "JK005-54748",
        #     "doc_name": "【需求确认】XQSQ2024122500167 关于新增放开满期退保阻断的需求V1.0"
        # }
        # Handle cases where the filename might not perfectly match the pattern.
        # Remove the .docx extension from doc_name.
        match = re.match(r"(\d{8})发布_([A-Z0-9-]+)_(.+)\.docx", filename_with_ext, re.IGNORECASE)
        if match:
            publish_date_str = match.group(1)
            # Convert YYYYMMDD to YYYY-MM-DD if desired, or keep as is
            # For now, let's keep it simple, can refine later
            return {
                "publish_date": publish_date_str,
                "jira_key": match.group(2),
                "doc_name": match.group(3)
            }
        else:
            # Fallback if pattern doesn't match: use the whole name as doc_name and leave others blank
            doc_name = filename_with_ext
            if doc_name.lower().endswith(".docx"):
                doc_name = doc_name[:-5]
            return {
                "publish_date": "",
                "jira_key": "",
                "doc_name": doc_name
            }

    def _convert_docx_to_markdown(self, file_path_or_bytes):
        # Use self.md_converter.convert(file_path_or_bytes)
        # This should return a string containing the Markdown content.
        # For now, this can be a placeholder if direct docx processing is complex in this environment.
        # We can refine this based on how `markitdown` expects input (file path vs. bytes).
        # The issue states: result = md.convert("test.docx")
        # Let's assume it takes a file path for now.
        # If it's bytes, the caller of this parser will need to handle reading the file.
        try:
            markdown_content = self.md_converter.convert(file_path_or_bytes)
            return markdown_content
        except Exception as e:
            print(f"Error converting DOCX to Markdown: {e}") # Or use proper logging
            return "" # Return empty string on error

    def _extract_markdown_headings(self, markdown_content):
        # Parse markdown_content and extract all headings (lines starting with #, ##, ###, etc.)
        # Return a list of strings, where each string is a heading.
        headings = []
        for line in markdown_content.splitlines():
            if line.strip().startswith("#"):
                headings.append(line.strip())
        return headings

    def _extract_headings_with_lineno(self, markdown_content: str) -> list[tuple[str, int]]:
        headings_with_lineno = []
        lines = markdown_content.splitlines()
        for i, line in enumerate(lines):
            stripped_line = line.strip()
            if stripped_line.startswith("#"):
                headings_with_lineno.append((stripped_line, i)) # i is the 0-indexed line number
                
        return headings_with_lineno

    def _classify_headings_with_llm(self, headings: list[str]) -> list[str]:
        if not headings:
            return []

        # Ensure llm_client is available. This is a fallback if __init__ somehow failed or was bypassed.
        if not hasattr(self, 'llm_client') or self.llm_client is None:
             print("Warning: llm_client not initialized by __init__. Initializing mock client in _classify_headings_with_llm.")
             # Use instance attributes stored during __init__ for fallback
             self.llm_client = self._get_mock_llm_client(
                 api_key=self.llm_api_key, 
                 base_url=self.llm_base_url, 
                 model=self.llm_model_name
            )

        headings_list_str = "\n".join(headings) # Original headings sent to LLM

        # New prompt template from the user
        prompt = f"""
请对下列 Markdown 文档标题，按照其在文档中的原始顺序，提取所有“相对独立、完整的内容块”。
- 每个内容块以其标题开头，包含该标题下的全部内容，直到下一个同级或更高级标题为止。
- 标题可以是需求背景、需求目标、需求方案、功能点、测试建议，也可以是其他文档实际存在的独立部分。
- 如果文档结构不规范（如层级混乱、标题名非标准），也请按你理解将每个具有独立意义的内容拆分出来。
- 返回结果格式如下：所有标题顺序排列，标题之间用“|”分隔。例如：
# 背景|# 调研|# 名词解释|# 方案|## 概述|## 功能点1|## 功能点2

下面是文档标题列表：
{headings_list_str}
"""
        try:
            response_text = self.llm_client.generate(prompt)
            
            # Parse the | separated string into a flat list of heading strings
            # Strip whitespace from each heading and filter out empty strings
            classified_block_start_headings = [h.strip() for h in response_text.strip().split('|') if h.strip()]
            
            return classified_block_start_headings
        except Exception as e:
            print(f"LLM API call failed in _classify_headings_with_llm: {e}") 
            return [] 

    # Renamed method and updated signature parameter name for clarity
    def _split_markdown_by_llm_selected_headings(self, markdown_content: str, llm_selected_start_headings: list[str]) -> list[tuple[str, str]]:
        if not llm_selected_start_headings or not markdown_content.strip():
            # If llm_selected_start_headings is empty, or markdown_content is empty/whitespace
            if not markdown_content.strip(): # Specifically if markdown is empty
                 return []
            # If markdown has content but no headings were selected by LLM, treat all as one chunk
            return [("全部内容", markdown_content)]


        original_headings_with_lineno = self._extract_headings_with_lineno(markdown_content)
        if not original_headings_with_lineno:
            # No headings in the original document.
            # If llm_selected_start_headings is somehow not empty, they won't be found.
            # Treat all content as one chunk if LLM didn't provide specific splits,
            # or if it did but they can't be mapped to original headings.
            return [("全部内容", markdown_content)]


        lines = markdown_content.splitlines()
        result_chunks = []

        # Create a quick lookup for original heading line numbers
        original_heading_to_lineno_map = {text: lineno for text, lineno in original_headings_with_lineno}

        for i, current_block_start_heading in enumerate(llm_selected_start_headings):
            start_line_num = original_heading_to_lineno_map.get(current_block_start_heading, -1)

            if start_line_num == -1:
                print(f"Warning: LLM selected heading '{current_block_start_heading}' not found in original document. Skipping.")
                continue

            end_line_num = len(lines) # Default to end of document

            if i + 1 < len(llm_selected_start_headings):
                next_block_start_heading = llm_selected_start_headings[i+1]
                next_heading_original_lineno = original_heading_to_lineno_map.get(next_block_start_heading, -1)
                
                if next_heading_original_lineno != -1:
                    # Ensure the next heading is actually after the current one,
                    # in case LLM returns headings out of original document order or duplicates.
                    if next_heading_original_lineno > start_line_num:
                        end_line_num = next_heading_original_lineno
                    else:
                        print(f"Warning: LLM selected 'next' heading '{next_block_start_heading}' appears before or at the same line as current block '{current_block_start_heading}'. Current block will extend to EOF or next valid heading.")
                        # This block will extend to the next valid LLM-selected heading that is correctly ordered, or EOF.
                        # We need to find the *actual* next heading in document order from the remaining llm_selected_start_headings
                        # that appears after start_line_num.
                        found_next_valid_heading = False
                        for j in range(i + 1, len(llm_selected_start_headings)):
                            potential_next_heading = llm_selected_start_headings[j]
                            potential_next_lineno = original_heading_to_lineno_map.get(potential_next_heading, -1)
                            if potential_next_lineno > start_line_num:
                                end_line_num = potential_next_lineno
                                found_next_valid_heading = True
                                break
                        if not found_next_valid_heading:
                            end_line_num = len(lines) # Extends to EOF if no subsequent valid heading
                else:
                    # This case implies LLM returned a "next" heading that isn't in the document.
                    # The current block will extend to the end of the document.
                    print(f"Warning: LLM selected 'next' heading '{next_block_start_heading}' not found in original document. Current block '{current_block_start_heading}' will extend to EOF.")

            chunk_text = "\n".join(lines[start_line_num:end_line_num])
            result_chunks.append((current_block_start_heading, chunk_text)) # Category name is the heading itself
            
        return result_chunks

    def parse_file(self, file_path_or_bytes, filename):
        # Main public method for the parser.
        # 1. Parse filename to get metadata
        # 2. Convert DOCX to Markdown
        # 3. Extract headings from Markdown
        # 4. Classify headings using LLM
        # 5. Split Markdown into chunks based on classification
        # 6. Format each chunk with metadata
        # Return a list of formatted chunk strings.

        metadata = self._parse_filename(filename)
        
        # Assuming file_path_or_bytes is a file path for markitdown, as per issue
        # If it's bytes, we might need to save to a temp file or see if markitdown handles bytes
        markdown_content = self._convert_docx_to_markdown(file_path_or_bytes)
        if not markdown_content:
            # Handle case where conversion failed - perhaps return a single chunk with error or basic info
            return [f"文档名：{metadata['doc_name']}\n上线时间：{metadata['publish_date']}\n分类名：错误\n内容：无法转换文档 {filename} 为Markdown。"]

        headings = self._extract_markdown_headings(markdown_content)
        if not headings:
            # Fallback for when no headings are extracted from the document.
            # Use the new formatting style.
            doc_name = metadata.get('doc_name', 'N/A')
            publish_date = metadata.get('publish_date', 'N/A')
            # Use document name as title, or a generic placeholder if doc_name is also missing.
            chunk_title = metadata.get('doc_name', '原始文档') 
            formatted_chunk = f"文档名：{doc_name}\n上线时间：{publish_date}\n标题：{chunk_title}\n内容：{markdown_content}"
            return [formatted_chunk]

        # LLM call returns a flat list of selected start headings for blocks
        llm_selected_headings = self._classify_headings_with_llm(headings) 
        
        # Pass this flat list to the splitting method
        split_content_tuples = self._split_markdown_by_llm_selected_headings(markdown_content, llm_selected_headings)
        
        final_formatted_chunks = []
        # Handle case where no chunks were generated by _split_markdown_by_llm_selected_headings
        # (e.g., if LLM returns empty list, or if markdown_content was empty and handled inside split method)
        if not split_content_tuples:
            # If headings were present but splitting resulted in no chunks (e.g. LLM issues, or all selected headings not found)
            # Return a single chunk with the full content, similar to the "no headings" case, or decide on error handling.
            # For now, let's be consistent with the "no headings" fallback.
            print("Warning: Headings were extracted, but no chunks were generated after LLM classification and splitting. Returning full content as one chunk.")
            doc_name = metadata.get('doc_name', 'N/A')
            publish_date = metadata.get('publish_date', 'N/A')
            chunk_title = metadata.get('doc_name', '原始内容') # Title indicating it's un-split content
            formatted_chunk = f"文档名：{doc_name}\n上线时间：{publish_date}\n标题：{chunk_title}\n内容：{markdown_content}"
            return [formatted_chunk]

        for block_start_heading, chunk_text in split_content_tuples:
            doc_name = metadata.get('doc_name', 'N/A') # Ensure these are fresh for each chunk, though they are doc-level
            publish_date = metadata.get('publish_date', 'N/A')
            
            chunk_title = block_start_heading # The block_start_heading is the title for this chunk
            
            formatted_chunk = f"文档名：{doc_name}\n上线时间：{publish_date}\n标题：{chunk_title}\n内容：{chunk_text}"
            final_formatted_chunks.append(formatted_chunk)
            
        return final_formatted_chunks

# Example Usage (for testing purposes, remove or comment out in final version):
if __name__ == '__main__':
    parser = MarkitdownParser(llm_api_key="test_key_main", llm_base_url="http://localhost:8001", llm_model_name="test_model_main") 

    # 1. Test filename parsing (likely unchanged, but verify)
    fn_test = "20250105发布_JK005-54748_【需求确认】文档A_V1.0.docx"
    meta = parser._parse_filename(fn_test)
    print(f"Parsed Filename Metadata: {meta}")
    # Expected: {'publish_date': '20250105', 'jira_key': 'JK005-54748', 'doc_name': '【需求确认】文档A_V1.0'}
    assert meta['doc_name'] == '【需求确认】文档A_V1.0'


    # 2. Mock Markdown Content
    mock_md_content_complex = """# 文档总标题
这是文档的简介部分。
也可能有一些初步的描述。

# 背景故事
这里是背景故事的详细内容。
应该有很多行。

## 次级背景点
更细致的背景。

# 需求方案
整体方案描述。

## 功能点1：用户登录
描述用户登录功能。
### 细节A
登录细节A。
### 细节B
登录细节B。

## 功能点2：数据导出
描述数据导出功能。

# 测试要点
需要测试的关键点。
"""

    # 3. Test _extract_headings_with_lineno
    headings_with_lineno = parser._extract_headings_with_lineno(mock_md_content_complex)
    print(f"\nExtracted Headings with Line Numbers: {headings_with_lineno}")
    expected_headings_with_lineno = [
        ('# 文档总标题', 0), 
        ('# 背景故事', 4), 
        ('## 次级背景点', 7), 
        ('# 需求方案', 10), 
        ('## 功能点1：用户登录', 13), 
        ('### 细节A', 15), 
        ('### 细节B', 17), 
        ('## 功能点2：数据导出', 20), 
        ('# 测试要点', 23)
    ]
    assert headings_with_lineno == expected_headings_with_lineno


    # 4. Test _classify_headings_with_llm (mocked LLM response)
    # Define the expected LLM output for this specific complex content
    expected_llm_output_for_complex_content = "# 文档总标题|# 背景故事|# 需求方案|## 功能点1：用户登录|## 功能点2：数据导出|# 测试要点"
    
    # Patch the generate method of the llm_client instance for this test run
    # The parser's __init__ already creates self.llm_client (which is a MockLLMClient instance)
    original_generate_method = parser.llm_client.generate 
    parser.llm_client.generate = MagicMock(return_value=expected_llm_output_for_complex_content)

    original_headings_for_llm = [h_text for h_text, lineno in headings_with_lineno]
    llm_selected_block_start_headings = parser._classify_headings_with_llm(original_headings_for_llm)
    print(f"\nLLM Selected Block Start Headings (mocked): {llm_selected_block_start_headings}")
    
    assert llm_selected_block_start_headings == expected_llm_output_for_complex_content.split('|')
    parser.llm_client.generate.assert_called_once() # Check that our mock was called
    
    # Restore original generate method if other tests for _classify_headings_with_llm depend on the default mock behavior
    parser.llm_client.generate = original_generate_method


    # 5. Test _split_markdown_by_llm_selected_headings
    split_chunks_tuples = parser._split_markdown_by_llm_selected_headings(mock_md_content_complex, llm_selected_block_start_headings)
    print("\nSplit Content Tuples (Title, Text):")
    for i, (title, content) in enumerate(split_chunks_tuples):
        print(f"--- Chunk Tuple {i+1} ---")
        print(f"Original Title: {title}")
        print(f"Content Preview: {content[:100].replace(chr(10), ' ')}...") # Replace newline for print
    
    assert len(split_chunks_tuples) == 6
    assert split_chunks_tuples[0][0] == "# 文档总标题"
    assert "这是文档的简介部分" in split_chunks_tuples[0][1]
    assert split_chunks_tuples[1][0] == "# 背景故事"
    assert "## 次级背景点" in split_chunks_tuples[1][1] 
    assert split_chunks_tuples[3][0] == "## 功能点1：用户登录"
    assert "### 细节B" in split_chunks_tuples[3][1]


    # 6. Test full parse_file method (end-to-end test for the module)
    # Mock the _convert_docx_to_markdown method
    def mock_converter_func(file_path_or_bytes):
        return mock_md_content_complex
    parser._convert_docx_to_markdown = mock_converter_func
    
    # The llm_client.generate needs to be patched again for the call within parse_file
    parser.llm_client.generate = MagicMock(return_value=expected_llm_output_for_complex_content)

    print("\nTesting full parse_file method:")
    final_formatted_chunks = parser.parse_file("dummy_path.docx", fn_test) # fn_test provides metadata
    
    print("\nFinal Formatted Chunks (New Format):")
    for chunk_idx, chunk_data in enumerate(final_formatted_chunks):
        print(f"--- Chunk {chunk_idx+1} ---")
        print(chunk_data)
    
    if final_formatted_chunks:
        assert f"文档名：{meta['doc_name']}" in final_formatted_chunks[0]
        assert f"上线时间：{meta['publish_date']}" in final_formatted_chunks[0]
        if llm_selected_block_start_headings: 
             assert f"标题：{llm_selected_block_start_headings[0]}" in final_formatted_chunks[0]
        assert "这是文档的简介部分" in final_formatted_chunks[0] 
        
        # Check title of a later chunk
        if len(final_formatted_chunks) > 3 and len(llm_selected_block_start_headings) > 3:
            assert f"标题：{llm_selected_block_start_headings[3]}" in final_formatted_chunks[3] #功能点1
            assert "描述用户登录功能" in final_formatted_chunks[3]
    
    parser.llm_client.generate = original_generate_method # Restore for any other potential tests not shown

    print("\n__main__ tests completed.")

# Module-level chunk function to align with RAGFlow's parser interface
def chunk(name, binary, from_page, to_page, lang, callback, kb_id, parser_config, tenant_id, **kwargs):
    """
    Chunks a .docx file using MarkitdownParser.

    :param name: Filename (e.g., "mydoc.docx")
    :param binary: File content as bytes.
    :param from_page: Start page (not used by this parser).
    :param to_page: End page (not used by this parser).
    :param lang: Language code (e.g., "en", "zh").
    :param callback: Optional callback function for progress/status.
    :param kb_id: Knowledge base ID.
    :param parser_config: Dictionary with parser-specific configurations.
                          Expected keys: 'llm_api_key', 'llm_base_url', 'llm_model_name'.
    :param tenant_id: Tenant ID.
    :param kwargs: Additional keyword arguments.
    :return: List of dictionaries, each representing a chunk.
    """

    # Retrieve LLM configurations:
    # Priority: parser_config -> kwargs -> environment variables -> defaults
    llm_api_key = parser_config.get('llm_api_key', kwargs.get('llm_api_key', os.environ.get('LLM_API_KEY_MARKITDOWN')))
    llm_base_url = parser_config.get('llm_base_url', kwargs.get('llm_base_url', os.environ.get('LLM_BASE_URL_MARKITDOWN')))
    llm_model_name = parser_config.get('llm_model_name', kwargs.get('llm_model_name', os.environ.get('LLM_MODEL_NAME_MARKITDOWN', 'default_model')))

    # print(f"MarkitdownParser 'chunk' invoked for: {name}, lang: {lang}, kb_id: {kb_id}, tenant_id: {tenant_id}")
    # print(f"LLM Config - API Key Set: {'Yes' if llm_api_key else 'No'}, Base URL: {llm_base_url}, Model: {llm_model_name}")

    parser_instance = MarkitdownParser(
        llm_api_key=llm_api_key,
        llm_base_url=llm_base_url,
        llm_model_name=llm_model_name
    )

    file_ext = ".docx" # Default
    if name and '.' in name:
        extracted_ext = "." + name.split('.')[-1].lower()
        if extracted_ext in [".docx"]: # Ensure it's a supported extension
             file_ext = extracted_ext
        else:
            # If not a docx, this parser might not be suitable.
            # For now, we'll proceed, but MarkItDown library will likely fail.
            print(f"Warning: MarkitdownParser received file '{name}' with non-docx extension '{extracted_ext}'. Conversion might fail.")


    temp_file_path = None
    try:
        # Create a temporary file with the correct suffix
        with tempfile.NamedTemporaryFile(delete=False, suffix=file_ext) as tmpfile:
            tmpfile.write(binary)
            temp_file_path = tmpfile.name
        
        if callback:
            # Example: 0% progress before starting
            callback(progress=0, msg=f"Starting Markitdown parsing for {name}")

        # Call the parser's main method using the temporary file's path
        list_of_formatted_chunk_strings = parser_instance.parse_file(temp_file_path, name)
        
        processed_chunks = []
        for i, chunk_str in enumerate(list_of_formatted_chunk_strings):
            processed_chunks.append({
                'content_with_weight': chunk_str, # This is the rich string from parse_file
                'source': name,                   # Original filename
                'page_num': i + 1                 # Using chunk index + 1 as a pseudo page number
            })
        
        if callback:
            # Example: 100% progress upon successful completion
            callback(progress=100, msg=f"Finished Markitdown parsing for {name}, {len(processed_chunks)} chunks created.")
            
        return processed_chunks

    except Exception as e:
        error_msg = f"Error in MarkitdownParser chunking for file '{name}': {str(e)}"
        import traceback
        print(f"{error_msg}\n{traceback.format_exc()}") # Log detailed error
        if callback:
            callback(progress=-1, msg=error_msg) # Signal error with progress=-1
        
        # Return a list containing a single chunk with the error message
        return [{'content_with_weight': error_msg, 'source': name, 'page_num': 0}]
    finally:
        if temp_file_path and os.path.exists(temp_file_path):
            try:
                os.remove(temp_file_path)
            except Exception as e:
                print(f"Error deleting temporary file {temp_file_path}: {e}")
