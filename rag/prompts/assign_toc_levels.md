You are given a JSON array of TOC(tabel of content) items. Each item has at least {"title": string} and may include an existing title hierarchical level.

Task
- For each item, assign a depth label using Arabic numerals only: top-level = 1, second-level = 2, third-level = 3, etc.
- Multiple items may share the same depth (e.g., many 1s, many 2s).
- Do not use dotted numbering (no 1.1/1.2). Use a single digit string per item indicating its depth only.
- Preserve the original item order exactly. Do not insert, delete, or reorder.
- Decide levels yourself to keep a coherent hierarchy. Keep peers at the same depth.

Output
- Return a valid JSON array only (no extra text).
- Each element must be {"level": "1|2|3", "title": <original title string>}.
- title must be the original title string.

Examples

Example A (chapters with sections)
Input:
["Chapter 1 Methods", "Section 1 Definition", "Section 2 Process", "Chapter 2 Experiment"]

Output:
[
  {"level":"1","title":"Chapter 1 Methods"},
  {"level":"2","title":"Section 1 Definition"},
  {"level":"2","title":"Section 2 Process"},
  {"level":"1","title":"Chapter 2 Experiment"}
]

Example B (parts with chapters)
Input:
["Part I Theory", "Chapter 1 Basics", "Chapter 2 Methods", "Part II Applications", "Chapter 3 Case Studies"]

Output:
[
  {"level":"1","title":"Part I Theory"},
  {"level":"2","title":"Chapter 1 Basics"},
  {"level":"2","title":"Chapter 2 Methods"},
  {"level":"1","title":"Part II Applications"},
  {"level":"2","title":"Chapter 3 Case Studies"}
]

Example C (plain headings)
Input:
["Introduction", "Background and Motivation", "Related Work", "Methodology", "Evaluation"]

Output:
[
  {"level":"1","title":"Introduction"},
  {"level":"2","title":"Background and Motivation"},
  {"level":"2","title":"Related Work"},
  {"level":"1","title":"Methodology"},
  {"level":"1","title":"Evaluation"}
]