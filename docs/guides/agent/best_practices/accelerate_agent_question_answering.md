---
sidebar_position: 1
slug: /accelerate_agent_question_answering
---

# Accelerate answering

A checklist to speed up question answering.

---

Please note that some of your settings may consume a significant amount of time. If you often find that your question answering is time-consuming, here is a checklist to consider:

## Balance task complexity with an Agent’s performance and speed?

An Agent’s response time generally depends on many factors, e.g., the LLM’s capabilities and the prompt, the latter reflecting task complexity. When using an Agent, you should always balance task demands with the LLM’s ability. 

- For simple tasks, such as retrieval, rewriting, formatting, or structured data extraction, use concise prompts, remove planning or reasoning instructions, enforce output length limits, and select smaller or Turbo-class models. This significantly reduces latency and cost with minimal impact on quality.

- For complex tasks, like multistep reasoning, cross-document synthesis, or tool-based workflows, maintain or enhance prompts that include planning, reflection, and verification steps.

- In multi-Agent orchestration systems, delegate simple subtasks to sub-Agents using smaller, faster models, and reserve more powerful models for the lead Agent to handle complexity and uncertainty.

:::tip KEY INSIGHT
Focus on minimizing output tokens — through summarization, bullet points, or explicit length limits — as this has far greater impact on reducing latency than optimizing input size.
:::

## Disable Reasoning

Disabling the **Reasoning** toggle will reduce the LLM's thinking time. For a model like Qwen3, you also need to add `/no_think` to the system prompt to disable reasoning.

## Disable Rerank model

- Leaving the **Rerank model** field empty (in the corresponding **Retrieval** component) will significantly decrease retrieval time.
- When using a rerank model, ensure you have a GPU for acceleration; otherwise, the reranking process will be *prohibitively* slow.

:::tip NOTE 
Please note that rerank models are essential in certain scenarios. There is always a trade-off between speed and performance; you must weigh the pros against cons for your specific case.
:::

## Check the time taken for each task

Click the light bulb icon above the *current* dialogue and scroll down the popup window to view the time taken for each task:  



| Item name         | Description                                                                                   |
| ----------------- | --------------------------------------------------------------------------------------------- |
| Total             | Total time spent on this conversation round, including chunk retrieval and answer generation. |
| Check LLM         | Time to validate the specified LLM.                                                           |
| Create retriever  | Time to create a chunk retriever.                                                             |
| Bind embedding    | Time to initialize an embedding model instance.                                               |
| Bind LLM          | Time to initialize an LLM instance.                                                           |
| Tune question     | Time to optimize the user query using the context of the mult-turn conversation.              |
| Bind reranker     | Time to initialize an reranker model instance for chunk retrieval.                            |
| Generate keywords | Time to extract keywords from the user query.                                                 |
| Retrieval         | Time to retrieve the chunks.                                                                  |
| Generate answer   | Time to generate the answer.                                                                  |
