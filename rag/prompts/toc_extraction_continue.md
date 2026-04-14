You are an expert parser and data formatter, currently in the process of building a JSON array from a multi-page table of contents (TOC). Your task is to analyze the new page of content and **append** the new entries to the existing JSON array.

**Instructions:**
1.  You will be given two inputs:
    *   `current_page_text`: The text content from the new page of the TOC.
    *   `existing_json`: The valid JSON array you have generated from the previous pages.
2.  Analyze each line of the `current_page_text` input.
3.  For each new line, extract the following three pieces of information:
    *   `structure`: The hierarchical index/numbering (e.g., "1", "2.1", "3.2.5"). Use `null` if none exists.
    *   `title`: The clean textual title of the section or chapter.
    *   `page`: The page number on which the section starts. Extract only the number. Use `null` if not present.
4.  **Append these new entries** to the `existing_json` array. Do not modify, reorder, or delete any of the existing entries.
5.  Output **only** the complete, updated JSON array. Do not include any other text, explanations, or markdown code block fences (like ```json).

**JSON Format:**
The output must be a valid JSON array following this schema:
```json
[
    {
        "structure": <string or null>,
        "title": <string>,
        "page": <number or null>
    },
    ...
]
```

**Input Example:**
`current_page_text`:
```
3.2 Advanced Configuration ........... 25
3.3 Troubleshooting .................. 28
4 User Management .................... 30
```

`existing_json`:
```json
[
    {"structure": "1", "title": "Introduction", "page": 1},
    {"structure": "2", "title": "Installation", "page": 5},
    {"structure": "3", "title": "Configuration", "page": 12},
    {"structure": "3.1", "title": "Basic Setup", "page": 15}
]
```

**Expected Output For The Example:**
```json
[
    {"structure": "3.2", "title": "Advanced Configuration", "page": 25},
    {"structure": "3.3", "title": "Troubleshooting", "page": 28},
    {"structure": "4", "title": "User Management", "page": 30}
]
```

**Now, process the following inputs:**
`current_page_text`:
{{ toc_page }}

`existing_json`:
{{ toc_json }}