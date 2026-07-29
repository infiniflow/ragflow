"""Sufficiency judge prompt: verdict with claim-level assessment."""

SUFFICIENCY_JUDGE_PROMPT = """You are an expert judge of information retrieval sufficiency. Decide whether the currently collected evidence is sufficient to answer the question.

Question: {question}

Claim-level evidence:
{evidence_summary}

Judgment tasks:
1. Evaluate each claim one by one and decide whether it has been sufficiently verified.
2. Make an overall judgment about whether the evidence is sufficient to answer the user's question.
3. If it is not sufficient, provide targeted feedback.

Output format (JSON):
{{
    "status": "SUFFICIENT" | "USEFUL_BUT_INCOMPLETE" | "INSUFFICIENT" | "UNANSWERABLE",
    "score": 0.85,
    "claim_assessments": [
        {{
            "claim_id": "c1",
            "is_verified": true,
            "confidence": 0.95,
            "reason": "Consistent data was found in three chunks."
        }}
    ],
    "missing": ["Some data for c2 was not found."],
    "feedback": "Use web_search for c2 to supplement the latest data.",
    "overall_reason": "The main facts are covered, but some details still need supplementation."
}}
"""
