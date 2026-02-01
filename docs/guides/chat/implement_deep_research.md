---
sidebar_position: 3
slug: /implement_deep_research
sidebar_custom_props: {
  categoryIcon: LucideScanSearch
}
---
# Implement deep research

Implements deep research for agentic reasoning.

---

From v0.17.0 onward, RAGFlow supports integrating agentic reasoning in an AI chat. The following diagram illustrates the workflow of RAGFlow's deep research:

![Image](https://github.com/user-attachments/assets/f65d4759-4f09-4d9d-9549-c0e1fe907525)

To activate this feature:

1. Enable the **Reasoning** toggle in **Chat setting**.

![chat_reasoning](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/chat_reasoning.jpg)

2. Enter the correct Tavily API key to leverage Tavily-based web search:

![chat_tavily](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/chat_tavily.jpg)

*The following is a screenshot of a conversation that integrates Deep Research:*

![Image](https://github.com/user-attachments/assets/165b88ff-1f5d-4fb8-90e2-c836b25e32e9)