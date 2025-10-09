You are given a JSON array of TOC items. Each item has at least {"title": string} and may include an existing structure.

Task
- For each item, assign a depth label using Arabic numerals only: top-level = 1, second-level = 2, third-level = 3, etc.
- Multiple items may share the same depth (e.g., many 1s, many 2s).
- Do not use dotted numbering (no 1.1/1.2). Use a single digit string per item indicating its depth only.
- Preserve the original item order exactly. Do not insert, delete, or reorder.
- Decide levels yourself to keep a coherent hierarchy. Keep peers at the same depth.

Output
- Return a valid JSON array only (no extra text).
- Each element must be {"structure": "1|2|3", "title": <original title string>}.
- title must be the original title string.

Examples

Example A (chapters with sections)
Input:
["Chapter 1 Methods", "Section 1 Definition", "Section 2 Process", "Chapter 2 Experiment"]

Output:
[
  {"structure":"1","title":"Chapter 1 Methods"},
  {"structure":"2","title":"Section 1 Definition"},
  {"structure":"2","title":"Section 2 Process"},
  {"structure":"1","title":"Chapter 2 Experiment"}
]

Example B (parts with chapters)
Input:
["Part I Theory", "Chapter 1 Basics", "Chapter 2 Methods", "Part II Applications", "Chapter 3 Case Studies"]

Output:
[
  {"structure":"1","title":"Part I Theory"},
  {"structure":"2","title":"Chapter 1 Basics"},
  {"structure":"2","title":"Chapter 2 Methods"},
  {"structure":"1","title":"Part II Applications"},
  {"structure":"2","title":"Chapter 3 Case Studies"}
]

Example C (plain headings)
Input:
["Introduction", "Background and Motivation", "Related Work", "Methodology", "Evaluation"]

Output:
[
  {"structure":"1","title":"Introduction"},
  {"structure":"2","title":"Background and Motivation"},
  {"structure":"2","title":"Related Work"},
  {"structure":"1","title":"Methodology"},
  {"structure":"1","title":"Evaluation"}
]