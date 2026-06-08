## Current system prompt

```
{{ current_prompt }}
```

## Quality report

```json
{{ quality_report }}
```

**Quality report key**
- `avg_score` — mean `avg_answer_relevancy` from automated evaluations (0–1, higher is better)
- `thumbs_down_rate` — fraction of conversations where the user gave no upvote (0–1, lower is better)
- `citation_hit_rate` — mean citation precision from automated evaluations (0–1, higher is better)

## Task

Generate exactly **{{ n }}** improved variants of the system prompt above.

Focus on the weakest signals in the quality report. For example:
- Low `avg_score` → improve answer focus, specificity, and relevance instructions.
- High `thumbs_down_rate` → improve tone, clarity, and completeness instructions.
- Low `citation_hit_rate` → add or strengthen citation format and source-attribution instructions.

Remember: preserve all `{placeholder}` tokens exactly as they appear in the original.

Return only the JSON array of {{ n }} strings.
