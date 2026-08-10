"""Planner decompose prompts: one per question type.

Each prompt is grounded in a preliminary hybrid search: ``{retrieved}`` carries
the (keyword-narrowed) chunks retrieved for the user's question, so the
decomposition reflects what the corpus actually contains rather than guessing.
"""

DECOMPOSE_FACTUAL = """This is a factual question. List all atomic facts that need to be retrieved.
If there are multiple facts, list them one by one. If there is only one fact, output exactly one item.
Base the decomposition on BOTH the question and the preliminary retrieved context below.

Relevance rules (strict):
- Every claim MUST be directly necessary to answer the question. Do NOT add
  background, biographical, or tangential facts about entities merely mentioned
  in the corpus (e.g. an unrelated person's birthplace) unless the question
  explicitly asks for them.
- Prefer fewer, atomic claims over many overlapping ones: merge facts about the
  same entity/measure into one claim when they can be researched together.
- Each claim must be verifiable from the corpus; do not invent facts.
- OPEN QUERY (CRITICAL): Each claim's description must be an OPEN research target
  (what/which/when/how many/what is X), NOT a pre-answered assertion. Never bake a
  guessed value into the description. WRONG: "Dustin Brown's hometown is Ithaca".
  RIGHT: "What is Dustin Brown's hometown?". The researcher verifies the OPEN target;
  a pre-answered assertion biases it to merely confirm the guess and skips the real
  answer. When a fact is genuinely unknown (e.g. a person's hometown), phrase the
  claim as a question, never as "X is <guessed value>".

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Preliminary retrieved context for this question (use it to ground each claim in what the corpus actually contains; do not invent facts it cannot support):
{retrieved}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "What year did Apple acquire Beats?",
            "priority": 1
        }}
    ]
}}
"""


DECOMPOSE_COMPARATIVE = """This is a comparative question. It needs to be decomposed into:
1. Information about entity A for the comparison dimension.
2. Information about entity B for the comparison dimension.
3. Optional information that directly compares the two entities.
Base the decomposition on BOTH the question and the preliminary retrieved context below.

Relevance rules (strict):
- Every claim MUST be directly needed to answer the comparison. Do NOT add
  tangential background about the entities (e.g. a third party's unrelated
  details) unless the question explicitly asks for them.
- Merge overlapping facts about the same entity into one claim.
- OPEN QUERY (CRITICAL): Each claim's description must be an OPEN research target
  (what/which/when/how many/what is X), NOT a pre-answered assertion. Never bake a
  guessed value into the description. RIGHT: "What is the distance from Hangzhou to
  Beijing?". WRONG: "The distance from Hangzhou to Beijing is 1,200 km". The
  researcher verifies the OPEN target; an assertion only biases it to confirm the guess.

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Preliminary retrieved context for this question (use it to ground each claim in what the corpus actually contains; do not invent facts it cannot support):
{retrieved}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "What is the distance from Hangzhou to Beijing?",
            "priority": 1
        }},
        {{
            "claim_id": "c2",
            "description": "What is the distance from Shanghai to Beijing?",
            "priority": 1
        }},
        {{
            "claim_id": "c3",
            "description": "Which city is closer to Beijing?",
            "priority": 2
        }}
    ]
}}
"""


DECOMPOSE_PROCEDURAL = """This is a procedural question. Decompose it into the information needed for each step required to complete the operation.
Base the decomposition on BOTH the question and the preliminary retrieved context below.

Relevance rules (strict):
- Every claim MUST be information directly needed by one of the procedure's
  steps. Skip any fact that is not required to carry out the operation.
- OPEN QUERY (CRITICAL): Each claim's description must be an OPEN research target
  (what/which/when/how/what is X), NOT a pre-answered assertion. Never bake a
  guessed value into the description. RIGHT: "What is the input format for step 2?".
  WRONG: "The input format for step 2 is CSV".

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Preliminary retrieved context for this question (use it to ground each claim in what the corpus actually contains; do not invent facts it cannot support):
{retrieved}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "What information is needed for the first step?",
            "priority": 1
        }},
        {{
            "claim_id": "c2",
            "description": "What information is needed for the second step?",
            "priority": 2
        }}
    ]
}}
"""


DECOMPOSE_EXPLORATORY = """This is an analytical or exploratory question. Decompose it into the main aspects or dimensions that need to be researched.
Base the decomposition on BOTH the question and the preliminary retrieved context below.

Relevance rules (strict):
- Every aspect/claim MUST be directly relevant to the question's core. Do NOT
  list tangential or background aspects just because the corpus mentions them.
- Prefer fewer, well-scoped aspects over many overlapping ones.
- OPEN QUERY (CRITICAL): Each claim's description must be an OPEN research target
  (what/which/when/how/what is X), NOT a pre-answered assertion. Never bake a
  guessed value into the description. RIGHT: "What were the main factors behind X?".
  WRONG: "The main factor behind X was Y".

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Preliminary retrieved context for this question (use it to ground each aspect in what the corpus actually contains; do not invent aspects it cannot support):
{retrieved}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "What is the first aspect that needs to be researched?",
            "priority": 1
        }},
        {{
            "claim_id": "c2",
            "description": "What is the second aspect that needs to be researched?",
            "priority": 2
        }}
    ]
}}
"""
