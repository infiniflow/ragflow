# User Prompt: TOC Relevance Evaluation

You will now receive:
1. A JSON list of TOC items (each with `level` and `title`)
2. A user query string.

Traverse the TOC hierarchically based on level numbers and assign scores (5,3,1,0,-1) according to the rules in the system prompt.  
Output **only** the JSON array with the added `"score"` field.

---

**Input TOC:**
{{ toc_json }}

**Query:**
{{ query }}

