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

Derived / computed assertions — APPLY ONLY IF the report actually states a
computed/derived result that the QUESTION explicitly asks for:
- These rules are STRICTLY CONDITIONAL. They apply ONLY when the question asks
  for a DERIVED result (a DIFFERENCE, PERCENTAGE, RATIO, TIMESPAN, or
  LETTER-COUNT comparison) AND the report states that computed result.
- If the question is a PLAIN FACTUAL question (e.g. "what year", "what is the
  name", "when did X happen") and the report simply states facts, do NOT apply
  the recompute rules. Judge groundedness normally: each assertion just needs to
  be supported by (or semantically paraphrased from) the evidence. Do NOT flag a
  factual assertion UNGROUNDED merely because an adjacent number in the evidence
  was not combined into a calculation — the question never asked for one.
- HOW TO TELL: check whether the QUESTION requests a computed result AND the
  report states that computed result. Either missing → skip this section.
- WHEN APPLICABLE and the report states a computed result (e.g. "there were
  12,000 more votes", "a 5% increase", "2 years apart", "the name has 1 more
  letter"), VERIFY THE CALCULATION YOURSELF from the values in the evidence,
  step by step.
- EVERY OPERAND of the computation must have its explicit value in the evidence,
  AND the operands must be the RIGHT entities the question references (e.g. the
  correct city pair for a letter-count difference, not a near-miss pair). Then
  recompute and check the report's stated result matches.
- If any operand's figure is absent, OR the operands are the wrong entities, OR
  the recomputed result disagrees with the report, the computed assertion is
  UNGROUNDED — even if the report's arithmetic is internally consistent. Flag it
  with a reason naming the missing/wrong operand or the recomputed vs stated value.
- This catches both failures: only-one-side retrieved, and a difference computed
  from the wrong intermediate entities.

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
