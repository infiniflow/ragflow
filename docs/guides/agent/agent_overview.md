---
sidebar_position: 2
title: Agent Overview
sidebar_label: Agent Overview
slug: /agent_overview
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Agent Overview

## Purpose of Agent
Use Agent to build automated workflows by connecting components on a visual canvas. Connections define the execution order; conditions, categories, and loops control which steps run.

Agents are commonly used in the following scenarios:
- Answering user questions based on knowledge bases.
- Identifying user intents and routing to different processing flows.
- Calling HTTP interfaces, databases, MCP tools or custom code.
- Splitting and batch processing long texts.
- Saving session memory or exporting processing results.

## Relationship Between Agent and Knowledge Base Q&A
- **Chat**: Suitable for applications mainly based on knowledge base Q&A and multi-turn dialogue.
- **Agent**: Suitable for business workflows requiring conditional branching, tool calling, data processing or multi-step orchestration.

The `Retrieval` component can be used inside an Agent to query knowledge bases. Retrieval can also be used as a tool under the Agent component, allowing the LLM to autonomously decide when to perform retrieval.

For example, an after-sales Agent can first use `Categorize` to judge the type of user questions, then use `Retrieval` to query product materials. If the user requests installation reservation, an HTTP request is used to call the external work order system. Finally, the `Message` component outputs the result.

## Try a complete workflow

Follow [Build a Customer Support Agent](./quickstarts/build_customer_support_agent.md) for a complete example with sample content, prompts, two branches, and expected test results.
