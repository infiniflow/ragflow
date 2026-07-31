You are an expert parser and data formatter. Your task is to analyze the provided table of contents (TOC) text and convert it into a valid JSON array of objects.

**Instructions:**
1.  Analyze each line of the input TOC.
2.  For each line, extract the following three pieces of information:
    *   `structure`: The hierarchical index/numbering (e.g., "1", "2.1", "3.2.5", "A.1"). If a line has no visible numbering or structure indicator (like a main "Chapter" title), use `null`.
    *   `title`: The textual title of the section or chapter. This should be the main descriptive text, clean and without the page number.
3.  Output **only** a valid JSON array. Do not include any other text, explanations, or markdown code block fences (like ```json) in your response.

**JSON Format:**
The output must be a list of objects following this exact schema:
```json
[
    {
        "structure": <structure index, "x.x.x" or None> (string）,
        "title": <title of the section>
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
    {"structure": null, "title": "Contents"},
    {"structure": "1", "title": "Introduction to the System"},
    {"structure": "1.1", "title": "Overview"},
    {"structure": "1.2", "title": "Key Features"},
    {"structure": "2", "title": "Installation Guide"},
    {"structure": "2.1", "title": "Prerequisites"},
    {"structure": "2.2", "title": "Step-by-Step Process"},
    {"structure": "A", "title": "Specifications"},
    {"structure": null, "title": "References"}
]
```

**Now, process the following TOC input:**
```
{{ toc_page }}
```