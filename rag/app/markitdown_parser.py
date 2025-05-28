import re
import tempfile
import os
from markitdown import MarkItDown # Assuming this is the correct import
# We might need to import LLM utilities later
# from some_ragflow_llm_utility import RAGFlowLLMClient # Hypothetical placeholder for actual client

class MarkitdownParser:
    def __init__(self, llm_api_key=None, llm_base_url=None, llm_model_name=None): # Added model_name
        self.md_converter = MarkItDown(enable_plugins=False) # As per issue spec
        # self.llm_client = RAGFlowLLMClient(api_key=llm_api_key, base_url=llm_base_url, model=llm_model_name) # Hypothetical
        # For now, if no actual client, simulate:
        self.llm_client = self._get_mock_llm_client(api_key=llm_api_key, base_url=llm_base_url, model=llm_model_name)

    def _get_mock_llm_client(self, api_key, base_url, model): # Mock LLM client for development
        class MockLLMClient:
            def __init__(self, api_key, base_url, model):
                self.api_key = api_key
                self.base_url = base_url
                self.model = model
                print(f"MockLLMClient initialized (key: {'set' if api_key else 'not set'}, base_url: {base_url}, model: {model})")

            def generate(self, prompt):
                print(f"MockLLMClient received prompt:\n{prompt[:400]}...") # Print start of prompt
                # Simulate LLM response based on the prompt's content for testing
                # This mock response should align with the format expected by _classify_headings_with_llm
                if "项目背景" in prompt and "用户登录" in prompt and "数据导出" in prompt and "整体方案概述" in prompt:
                    return """
需求背景相关：
# 项目背景
# 业务场景

需求目标相关：
# 项目目标

需求方案相关：
# 整体方案概述

功能点1相关：
## 用户登录
### 功能说明
### 实现细节
#### 安全考虑

功能点2相关：
## 数据导出
### 导出格式说明

功能点3相关：
## 管理员面板
### 用户管理

测试要点相关：
# 测试要点
"""
                elif "唯一的标题" in prompt and "子标题" in prompt : # For simple test
                    return """
需求背景相关：
# 唯一的标题
## 子标题
"""
                else: # Generic fallback
                    return """
需求背景相关：
# Fallback Heading Title
"""
        return MockLLMClient(api_key=api_key, base_url=base_url, model=model)

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

    def _classify_headings_with_llm(self, headings): # llm_client removed from params, uses self.llm_client
        # This function will interact with an LLM.
        # Construct the prompt as specified in the issue.
        # Send the list of headings to the LLM and get back the classified structure.
        if not headings:
            return {}

        headings_list_str = "\n".join(headings)
        # The prompt from the issue:
        prompt = f"""
请对下列 Markdown 文档标题，按照如下结构归类输出：
- 需求背景
- 需求目标
- 需求方案
- 功能点1、功能点2（依次类推，功能点是指相对独立的最小功能单元）
- 测试要点

要求：
1. 标题顺序必须与原文一致，不能跳行或遗漏。
2. 标题内容全部包含在某一类下，且不能交叉分类。
3. 每一类下可包含多个标题，必须连续。
4. 输出格式如下示例：

需求背景相关：
# 项目背景
# 现状分析

需求目标相关：
# 项目目标

功能点1相关：
## 用户登录
### 功能描述

功能点2相关：
## 数据导出
### 设计说明

测试要点相关：
## 测试用例设计

---

下面是待分类的文档标题列表：
{headings_list_str}
"""
        try:
            # response_text = self.llm_client.generate(prompt) # Use actual client when available
            response_text = self.llm_client.generate(prompt) # Using the mock client for now
        except Exception as e:
            print(f"LLM API call failed: {e}") # Or use proper logging
            return {} # Return empty on error

        # Parse response_text
        classified_headings = {}
        current_category_key = None # This will be like "需求背景", "功能点1" etc.
        
        # print(f"LLM Raw Response:\n{response_text}") # For debugging LLM output
        for line in response_text.splitlines():
            line = line.strip()
            if not line: # Skip empty lines
                continue
            
            # Check if the line defines a category
            # Categories end with "相关：" or "相关:" or just ":"
            match_category = re.match(r"(.+?)(?:相关：|相关:|：|:)$", line)
            if match_category:
                category_name_from_llm = match_category.group(1).strip()
                # Normalize category name for use as key, e.g. "需求背景"
                current_category_key = category_name_from_llm
                if current_category_key not in classified_headings:
                    classified_headings[current_category_key] = []
            elif line.startswith("#") and current_category_key:
                # This line is a heading, add it to the current category
                if current_category_key in classified_headings: # Ensure category was properly initialized
                     classified_headings[current_category_key].append(line)
                else:
                    # This case should ideally not be reached if LLM output is well-formed
                    print(f"Warning: Line '{line}' looks like a heading, but current category key '{current_category_key}' is not initialized. Skipping.")
            # else:
                # Line is neither a category definition nor a heading under the current category.
                # Could be descriptive text from LLM, or malformed. For now, we ignore such lines.
                # print(f"Info: Skipping unclassified line from LLM response: '{line}'")

        # Filter out categories that might have been initialized but received no valid headings
        final_classified_headings = {}
        for category_key, hs in classified_headings.items():
            valid_headings = [h for h in hs if h.startswith("#")]
            if valid_headings:
                final_classified_headings[category_key] = valid_headings
        
        return final_classified_headings


    def _split_markdown_by_classified_headings(self, markdown_content, classified_headings):
        if not classified_headings:
            return [("全部内容", markdown_content)]

        chunks = []
        lines = markdown_content.splitlines()

        # Defined order of categories (keys should match those from _classify_headings_with_llm)
        category_order_main = ["需求背景", "需求目标", "需求方案"]
        functional_point_keys = sorted([k for k in classified_headings.keys() if k.startswith("功能点")])
        category_order_test = ["测试要点"]
        
        # Full ordered list of category keys as they should be processed
        ordered_category_keys_for_splitting = category_order_main + functional_point_keys + category_order_test

        # Create a list of (marker_heading_text, category_key_for_chunk)
        # These are the first headings of each category that will define the start of a chunk.
        category_start_markers = []
        
        # print(f"Debug classified_headings for splitting: {classified_headings}")
        # print(f"Debug ordered_category_keys_for_splitting: {ordered_category_keys_for_splitting}")

        for key_in_order in ordered_category_keys_for_splitting:
            if key_in_order in classified_headings and classified_headings[key_in_order]:
                # classified_headings[key_in_order] is a list of headings for this category
                # The first one is the primary marker for this category section
                first_heading_of_this_category = classified_headings[key_in_order][0].strip()
                category_start_markers.append((first_heading_of_this_category, key_in_order))
            # else:
                # print(f"Debug: Category key '{key_in_order}' not found in classified_headings or has no headings.")


        if not category_start_markers:
            # This can happen if LLM output was empty, malformed, or no standard categories found
            # print("Warning: No category start markers identified. Returning content as a single chunk.")
            return [("全部内容", markdown_content)]

        # Split content based on these category_start_markers
        for i, (marker_heading_text, category_key_for_chunk) in enumerate(category_start_markers):
            start_line_idx = -1
            # Find the line number of the current marker_heading_text in the original markdown lines
            for line_idx, line_content in enumerate(lines):
                if line_content.strip() == marker_heading_text:
                    start_line_idx = line_idx
                    break
            
            if start_line_idx == -1:
                # This means a heading classified by LLM was not found in the original document.
                # This could indicate an LLM hallucination or a bug.
                print(f"Warning: Category marker heading '{marker_heading_text}' for category '{category_key_for_chunk}' not found in document. Skipping this category.")
                continue

            # Determine the end_line_idx for this chunk.
            # It's the line *before* the marker_heading_text of the *next* category in category_start_markers.
            # Or, if this is the last category, it's the end of the document.
            end_line_idx = len(lines) # Default to end of document for the last chunk
            if i + 1 < len(category_start_markers):
                next_marker_heading_text, _ = category_start_markers[i+1]
                # Find the line of the next_marker_heading_text.
                # Search must start *after* the current chunk's start_line_idx to avoid issues
                # if headings are duplicated or if a heading is a substring of another.
                for next_marker_line_search_idx, line_content in enumerate(lines): # Search from beginning
                    if line_content.strip() == next_marker_heading_text:
                        # Ensure this found marker is truly after the current one,
                        # relevant if headings could be non-unique and appear earlier.
                        # However, our markers are unique and ordered by appearance due to `category_start_markers` construction.
                        if line_idx > start_line_idx : # Check if the found line_idx is after current start_line_idx
                             end_line_idx = line_idx # line_idx here is the line of next_marker_heading_text
                             break
                        # If we find the next marker but it's not after current, it means something is wrong or
                        # the same heading is used for different categories (which shouldn't happen with this logic)
                        # For now, we assume markers are distinct and appear in order.
                        # The critical part is that `next_marker_heading_text` is found in `lines`.
                
                # Refined search for end_line_idx:
                temp_end_idx = -1
                for line_num_for_next_marker in range(start_line_idx + 1, len(lines)):
                    if lines[line_num_for_next_marker].strip() == next_marker_heading_text:
                        temp_end_idx = line_num_for_next_marker
                        break
                if temp_end_idx != -1:
                    end_line_idx = temp_end_idx
            
            current_chunk_lines = lines[start_line_idx:end_line_idx]
            chunks.append((category_key_for_chunk, "\n".join(current_chunk_lines)))
            
        return chunks

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
            # Handle case with no headings - return markdown_content as a single chunk under a generic category
            formatted_chunk = f"文档名：{metadata['doc_name']}\n上线时间：{metadata['publish_date']}\n分类名：通用内容\n内容：{markdown_content}"
            return [formatted_chunk]

        # For LLM client: This might be passed in or accessed via a global/singleton
        classified_headings = self._classify_headings_with_llm(headings) 
        
        split_chunks_with_categories = self._split_markdown_by_classified_headings(markdown_content, classified_headings)
        
        final_formatted_chunks = []
        for category_name, chunk_content in split_chunks_with_categories:
            formatted_chunk = f"文档名：{metadata['doc_name']}\n上线时间：{metadata['publish_date']}\n分类名：{category_name}\n内容：{chunk_content}"
            final_formatted_chunks.append(formatted_chunk)
            
        return final_formatted_chunks

# Example Usage (for testing purposes, remove or comment out in final version):
if __name__ == '__main__':
    # Initialize parser with mock LLM parameters
    parser = MarkitdownParser(llm_api_key="test_key", llm_base_url="http://localhost:8000", llm_model_name="test_model")
    
    # 1. Test filename parsing (remains the same)
    fn_test = "20250105发布_JK005-54748_【需求确认】XQSQ2024122500167 关于新增放开满期退保阻断的需求V1.0.docx"
    meta = parser._parse_filename(fn_test)
    print(f"Parsed Filename Metadata: {meta}\n")

    fn_test_no_match = "mydocument.docx"
    meta_no_match = parser._parse_filename(fn_test_no_match)
    print(f"Parsed Filename (no match): {meta_no_match}\n")

    # More complex mock Markdown content
    mock_md_content_complex = """
# 项目背景
这是项目的起源和原因。
包含一些历史数据。

# 业务场景
描述当前业务如何运作。
以及面临的痛点。

# 项目目标
我们希望达成以下目标：
1. 提高效率
2. 降低成本

# 整体方案概述
我们将采用微服务架构。
结合事件驱动模式。

## 用户登录
这是用户登录功能的详细描述。
支持多种登录方式。
### 功能说明
- 用户名密码登录
- OAuth 2.0
### 实现细节
后端采用Python Django。
前端Vue.js。
#### 安全考虑
密码加密存储。

## 数据导出
用户可以将数据导出为多种格式。
### 导出格式说明
- CSV
- Excel
- PDF (未来支持)

## 管理员面板
管理员有专属面板。
### 用户管理
- 查看用户
- 禁用用户

# 测试要点
对以下方面进行重点测试：
- 登录安全性
- 数据导出准确性
- 管理员操作权限
"""
    
    # 2. Test DOCX to Markdown conversion (mocked)
    def mock_complex_converter(file_path_or_bytes):
        return mock_md_content_complex
    
    parser._convert_docx_to_markdown = mock_complex_converter # Monkey patch

    # 3. Test heading extraction
    headings_extracted = parser._extract_markdown_headings(mock_md_content_complex)
    print(f"Extracted Headings: {headings_extracted}\n")
    # Expected: All lines starting with #, ##, ###, #### from mock_md_content_complex

    # 4. Test LLM classification (uses MockLLMClient)
    # MockLLMClient's response is triggered by keywords in `headings_extracted`.
    classified = parser._classify_headings_with_llm(headings_extracted)
    print(f"Classified Headings (from Mock LLM): {classified}\n")
    # Expected output depends on MockLLMClient's logic. 
    # Keys in `classified` should NOT have "相关" suffix.
    # e.g. {'需求背景': ['# 项目背景', '# 业务场景'], '功能点1': ['## 用户登录', ...], ...}

    # 5. Test Markdown splitting with refined logic
    split_content = parser._split_markdown_by_classified_headings(mock_md_content_complex, classified)
    print("Split Content (refined logic):")
    for cat, content in split_content:
        print(f"--- Category: {cat} ---") # Category name should be like "需求背景", "功能点1"
        print(content)
        print("--- End Category ---\n")
    
    # 6. Test full parse_file method
    print("Testing full parse_file method (COMPLEX CONTENT):")
    # Uses patched _convert_docx_to_markdown and mock LLM.
    final_chunks_complex = parser.parse_file("dummy_complex.docx", fn_test) 
    
    print("\nFinal Formatted Chunks (COMPLEX CONTENT from parse_file):")
    if final_chunks_complex:
        for chunk_idx, chunk_data in enumerate(final_chunks_complex):
            print(f"--- Chunk {chunk_idx+1} ---")
            print(chunk_data)
            print("--- End Chunk ---\n")
    else:
        print("No chunks returned from parse_file for complex content.")

    # Test with simpler markdown to check LLM fallback and splitting
    print("\nTesting with simpler content (forcing different LLM path):")
    mock_md_simple = """
# 唯一的标题
这是唯一的内容。
## 子标题
更多内容。
"""
    def mock_simple_converter(file_path_or_bytes):
        return mock_md_simple
    parser._convert_docx_to_markdown = mock_simple_converter # Patch again for simple content

    headings_simple = parser._extract_markdown_headings(mock_md_simple)
    print(f"Simple Extracted Headings: {headings_simple}")
    
    # MockLLMClient will provide a specific response for this type of input
    classified_simple = parser._classify_headings_with_llm(headings_simple)
    print(f"Classified Simple Headings (Mock LLM): {classified_simple}\n")

    split_simple = parser._split_markdown_by_classified_headings(mock_md_simple, classified_simple)
    print("\nSplit Simple Content:")
    for cat, content in split_simple:
        print(f"--- Category: {cat} ---")
        print(content)
        print("--- End Category ---\n")
        
    final_chunks_simple = parser.parse_file("dummy_simple.docx", "simple_doc_name_20230101.docx")
    print("\nFinal Formatted Chunks (SIMPLE CONTENT from parse_file):")
    if final_chunks_simple:
        for chunk_idx, chunk_data in enumerate(final_chunks_simple):
            print(f"--- Chunk {chunk_idx+1} ---")
            print(chunk_data)
            print("--- End Chunk ---\n")

    else:
        print("No chunks returned from simple parse_file.")

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
