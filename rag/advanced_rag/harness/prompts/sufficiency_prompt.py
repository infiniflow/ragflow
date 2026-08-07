"""Sufficiency judge prompt: verdict with claim-level assessment.

Reflects the Sufficient Context paper (arXiv 2411.06037) definition: the
context is sufficient iff a plausible answer can be inferred from it — the
answer does not need to be proven correct, only supportable.
"""

SUFFICIENCY_JUDGE_PROMPT = """You are an expert judge of information retrieval sufficiency. Decide whether the currently collected evidence is sufficient to answer the question, using this criterion: the evidence is sufficient if and only if a PLAUSIBLE answer can be inferred from it (directly or by entailment). The answer does not need to be proven correct; it only needs to be a reasonable, supportable answer.

Question: {question}

Claim-level evidence:
{evidence_summary}

Reasoning procedure (step-by-step before answering):
1. Identify the required entities/key facts a plausible answer must involve.
2. For each, check the evidence covers it (record in "coverage").
3. Check for multi-hop inference: a connection the context does not state is NOT inferable.
4. Check ambiguity: if the context could support multiple mutually exclusive answers and nothing distinguishes them, mark insufficient.
5. Note any internally conflicting figures/statements ("contradictions").
6. Give your confidence in the sufficiency decision.

Output format (JSON):
{{
    "status": "SUFFICIENT" | "USEFUL_BUT_INCOMPLETE" | "INSUFFICIENT" | "UNANSWERABLE",
    "Sufficient Context": true/false,
    "is_sufficient": true/false,
    "required_entities": ["Entity 1", "Entity 2"],
    "coverage": {{"Entity 1": true, "Entity 2": false}},
    "confidence": 0.85,
    "claim_assessments": [
        {{
            "claim_id": "c1",
            "is_verified": true,
            "confidence": 0.95,
            "reason": "Consistent data was found in three chunks."
        }}
    ],
    "missing": ["Some data for c2 was not found."],
    "contradictions": ["conflicting figures/statements if any"],
    "feedback": "Use web_search for c2 to supplement the latest data.",
    "overall_reason": "The main facts are covered, but some details still need supplementation."
}}
"""
