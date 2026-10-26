---
sidebar_position: 8
title: Improve Agent Response Time
slug: /accelerate_agent_question_answering
---

# Improve Agent Response Time

Use a representative question and inspect component results and execution logs to find the slow step. Change one setting at a time, then compare both response time and answer quality.

| Slow step | What to check |
| --- | --- |
| Agent generation | Keep prompts focused, request a concise answer, and reduce unnecessary history. Use a faster model for simple formatting or classification tasks. |
| Planning and tool calls | Remove tools or sub-agents that the task does not need. Keep reflection and multi-step planning for tasks that benefit from them. |
| Retrieval | Compare with and without a Rerank model. Adjust Rerank candidate count and Top N while checking that the evidence needed for the answer is still returned. |
| External tools | Inspect individual tool durations, response sizes, timeouts, and provider errors before increasing retries. |
| Iteration | For independent items, try a bounded Maximum concurrency greater than `1`. Respect model and service rate limits. |

If the selected model provides reasoning settings, adjust them and compare response time and answer quality.

Retest a normal question, a question with no matching knowledge, and a tool failure after tuning. A faster answer is useful only if routing, evidence, and error handling still work.
