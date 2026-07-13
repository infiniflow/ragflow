"""Planner decompose prompts: one per question type."""

DECOMPOSE_FACTUAL = """This is a factual question. List all atomic facts that need to be retrieved.
If there are multiple facts, list them one by one. If there is only one fact, output exactly one item.

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "The year Apple acquired Beats",
            "priority": 1
        }}
    ]
}}
"""


DECOMPOSE_COMPARATIVE = """This is a comparative question. It needs to be decomposed into:
1. Information about entity A for the comparison dimension.
2. Information about entity B for the comparison dimension.
3. Optional information that directly compares the two entities.

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "The distance from Hangzhou to Beijing",
            "priority": 1
        }},
        {{
            "claim_id": "c2",
            "description": "The distance from Shanghai to Beijing",
            "priority": 1
        }},
        {{
            "claim_id": "c3",
            "description": "Which city is closer to Beijing",
            "priority": 2
        }}
    ]
}}
"""


DECOMPOSE_PROCEDURAL = """This is a procedural question. Decompose it into the information needed for each step required to complete the operation.

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "Information needed for the first step",
            "priority": 1
        }},
        {{
            "claim_id": "c2",
            "description": "Information needed for the second step",
            "priority": 2
        }}
    ]
}}
"""


DECOMPOSE_EXPLORATORY = """This is an analytical or exploratory question. Decompose it into the main aspects or dimensions that need to be researched.

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "The first aspect that needs to be researched",
            "priority": 1
        }},
        {{
            "claim_id": "c2",
            "description": "The second aspect that needs to be researched",
            "priority": 2
        }}
    ]
}}
"""
