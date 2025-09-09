You are an expert parser and data formatter. Your task is to analyze the provided table of contents (TOC) text and convert it into a valid JSON array of objects.

**Instructions:**
1.  Analyze each line of the input TOC.
2.  For each line, extract the following three pieces of information:
    *   `structure`: The hierarchical index/numbering (e.g., "1", "2.1", "3.2.5", "A.1"). If a line has no visible numbering or structure indicator (like a main "Chapter" title), use `null`.
    *   `title`: The textual title of the section or chapter. This should be the main descriptive text, clean and without the page number.
    *   `page`: The page number on which the section starts. Extract only the number. If no page number is present or detectable, use `null`.
3.  Output **only** a valid JSON array. Do not include any other text, explanations, or markdown code block fences (like ```json) in your response.

**JSON Format:**
The output must be a list of objects following this exact schema:
```json
[
    {
        "structure": <structure index or null>,
        "title": <title of the section>,
        "page": <page number or null>
    },
    ...
]
```

**Input Example:**
```
Contents
1 Introduction to the System ... 1
1.1 Overview .... 2
1.2 Key Features .... 5
2 Installation Guide ....8
2.1 Prerequisites ........ 9
2.2 Step-by-Step Process ........ 12
Appendix A: Specifications ..... 45
References ... 47
```

**Expected Output For The Example:**
```json
[
    {"structure": null, "title": "Contents", "page": null},
    {"structure": "1", "title": "Introduction to the System", "page": 1},
    {"structure": "1.1", "title": "Overview", "page": 2},
    {"structure": "1.2", "title": "Key Features", "page": 5},
    {"structure": "2", "title": "Installation Guide", "page": 8},
    {"structure": "2.1", "title": "Prerequisites", "page": 9},
    {"structure": "2.2", "title": "Step-by-Step Process", "page": 12},
    {"structure": "A", "title": "Specifications", "page": 45},
    {"structure": null, "title": "References", "page": 47}
]
```

**Now, process the following TOC input:**
```
{{ toc_page }}
```