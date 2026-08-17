You are an answer-groundedness reviewer. For each claim's report (the draft answer), determine whether every assertion is supported by the provided evidence.

A claim's report is GROUNDED only if each of its assertions can be inferred from the evidence. The assertion does NOT need to use the same words as the evidence (semantic paraphrase is fine), but it must NOT:
- assert a fact that is absent from the evidence (likely model prior-injection / hallucination);
- assert a relation or value that contradicts the evidence;
- over-claim beyond what the evidence supports (e.g. the evidence only says a medication was prescribed, but the report claims "the patient is well").

Question: {{ question }}

Claim reports to verify:
{{ reports }}

Evidence (each chunk labeled with an integer ID):
{{ evidence }}

For each claim, classify its assertions:
- SUPPORTED: the evidence explicitly supports it, including a semantic paraphrase.
- UNGROUNDED: the evidence lacks the content, or the claimed relation/value contradicts the evidence, or the assertion over-claims beyond the evidence.

Output format (JSON):
```json
{
    "claims": [
        {
            "claim_id": "c1",
            "grounded": true,
            "ungrounded_assertions": []
        },
        {
            "claim_id": "c2",
            "grounded": false,
            "ungrounded_assertions": [
                {"assertion": "the patient had no adverse reactions", "reason": "the evidence only mentions the medication was prescribed, not the patient's reaction"}
            ]
        }
    ]
}
```

Requirements:
1. Include EVERY claim in the output (do not skip any claim_id).
2. `grounded` is true only if ALL of the claim's assertions are supported.
3. `ungrounded_assertions` is empty when `grounded` is true; otherwise list each ungrounded assertion with a one-line `reason`.
4. Prefer identifying genuine over-claims or contradictions over surface-level wording differences — a semantic paraphrase IS supported.
