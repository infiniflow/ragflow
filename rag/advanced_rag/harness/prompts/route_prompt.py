"""Route node prompt: classify query type."""

ROUTE_PROMPT = """Analyze the following question and output a structured query analysis.

Question: {question}

Analyze it across these dimensions:
1. Question type: factual / comparative / analytical / procedural / exploratory / verification / summarization.
2. Whether it needs decomposition into atomic facts, meaning whether multiple independent pieces of information must be retrieved separately before answering: true/false.
3. Suggested knowledge compilation tool: null (none) / toc (document table of contents) / graph (knowledge graph) / wiki (compiled domain knowledge).

Output format (JSON):
{{
    "question_type": "comparative",
    "requires_decomposition": true,
    "suggests_compilation": null,
    "reasoning": "This is a comparative question, so it needs to be decomposed into two independent facts and one comparison relation."
}}
"""
