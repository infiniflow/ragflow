"""Route node prompt: classify query type."""

ROUTE_PROMPT = """Analyze the following question and output a structured query analysis.

Question: {question}

Analyze it across these dimensions:
1. Question type: factual / comparative / analytical / procedural / exploratory / verification / summarization.
2. Whether it needs decomposition into atomic facts, meaning whether multiple independent pieces of information must be retrieved separately before answering: true/false.
   - REQUIRES decomposition (true): open-ended list/enumeration questions (who, what kinds, which people, list all, 谁, 哪些), multi-entity comparisons, multi-step procedures, analytical questions spanning domains, questions whose answer has no single concise fact but requires gathering many independent facts.
   - Does NOT require decomposition (false): simple fact lookups (single person's birth year, definition of one term), yes/no verifications against a known fact, summarization of a single document or event.
3. Suggested knowledge compilation tool: null (none) / toc (document table of contents) / graph (knowledge graph) / wiki (compiled domain knowledge).

Examples:

Q: "曹操是谁？" (Who is Cao Cao?) → factual, single entity → NO decomposition
Q: "曹操认识谁？" (Who did Cao Cao know?) → exploratory open-ended list → NEEDS decomposition (many independent relationship facts)
Q: "秦始皇和汉武帝谁更伟大？" (Who was greater, Qin Shi Huang or Han Wudi?) → comparative → NEEDS decomposition
Q: "光合作用的过程是什么？" (What is the process of photosynthesis?) → procedural, but single process → NO decomposition (OR: NEEDS decomposition if the KB spans many domains)
Q: "2024年诺贝尔物理学奖得主是谁？" (Who won the 2024 Nobel Prize in Physics?) → factual, single fact → NO decomposition
Q: "有哪些方法可以降低胆固醇？" (What are the methods to lower cholesterol?) → exploratory list → NEEDS decomposition

Output format (JSON):
{{
    "question_type": "exploratory",
    "requires_decomposition": true,
    "suggests_compilation": "graph",
    "reasoning": "Open-ended question about a person's relationships. Many independent facts must be gathered, so decomposition is required. A knowledge graph compilation tool may help."
}}
"""
