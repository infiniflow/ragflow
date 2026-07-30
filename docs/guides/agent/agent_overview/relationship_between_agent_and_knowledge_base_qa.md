---
sidebar_position: 2
title: "Relationship Between Agent and Knowledge Base Q&A"
sidebar_label: "Relationship Between Agent and Knowledge Base Q&A"
slug: /relationship_between_agent_and_knowledge_base_qa
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Relationship Between Agent and Knowledge Base Q&A
- **Chat**: Suitable for applications mainly based on knowledge base Q&A and multi-turn dialogue.
- **Agent**: Suitable for business workflows requiring conditional branching, tool calling, data processing or multi-step orchestration.

The `Retrieval` component can be used inside an Agent to query knowledge bases. Retrieval can also be used as a tool under the Agent component, allowing the LLM to autonomously decide when to perform retrieval.

For example, an after-sales Agent can first use `Categorize` to judge the type of user questions, then use `Retrieval` to query product materials. If the user requests installation reservation, an HTTP request is used to call the external work order system. Finally, the `Message` component outputs the result.
