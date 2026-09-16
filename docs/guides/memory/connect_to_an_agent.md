---
sidebar_position: 4
slug: /connect_to_an_agent
title: Connect to an Agent
sidebar_label: Connect to an Agent
sidebar_custom_props: {
  categoryIcon: LucideBox
}
---

# Connect to an Agent

A memory must be connected to an Agent before historical messages can be written during Agent execution or historical context can be retrieved when answering. Usually, both directions, **write to memory** and **retrieve from memory**, need to be configured.

### Write Agent Messages to Memory

1. Go to the **Agent** page and open the Agent that needs to be configured.
2. Select or add the **Message** component on the Agent canvas.
3. In the **Message** component configuration, find **Save to Memory**.
4. Select the target memory. If historical records need to be distinguished by user, configure the user ID.
5. Save the Agent and run a test. After it runs, return to the memory's **Messages** page to check whether new messages have been generated.

![Write agent messages to memory](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/write_agent_messages_to_memory.jpg)


### Let an Agent Retrieve from Memory

1. Select or add the **Retrieval** component on the Agent canvas.
2. In **Query variable**, select the query source. `sys.query` is commonly used.
3. In **Retrieval source**, select **Memory**, and select one or more target memories.
4. Adjust **Similarity threshold**, **Vector similarity weight**, and **Top N** as needed.
5. Connect the retrieval result to a subsequent large language model or answer component, so that the Agent can use the retrieved historical context.
6. Save and run a test, and check whether the answer correctly references historical memories.


![Let an agent retrieve from memory](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/let_an_agent_retrieve_from_memory.jpg)
