---
sidebar_position: 1
slug: /agent_introduction
---

# Introduction to agents

Key concepts, basic operations, a quick view of the agent editor.

---

## Key concepts

:::danger DEPRECATED!
A new version is coming soon.
:::

Agents and RAG are complementary techniques, each enhancing the other’s capabilities in business applications. RAGFlow v0.8.0 introduces an agent mechanism, featuring a no-code workflow editor on the front end and a comprehensive graph-based task orchestration framework on the back end. This mechanism is built on top of RAGFlow's existing RAG solutions and aims to orchestrate search technologies such as query intent classification, conversation leading, and query rewriting to:

- Provide higher retrievals and,
- Accommodate more complex scenarios.

## Create an agent

:::tip NOTE

Before proceeding, ensure that:  

1. You have properly set the LLM to use. See the guides on [Configure your API key](../models/llm_api_key_setup.md) or [Deploy a local LLM](../models/deploy_local_llm.mdx) for more information.
2. You have a knowledge base configured and the corresponding files properly parsed. See the guide on [Configure a knowledge base](../dataset/configure_knowledge_base.md) for more information.

:::

Click the **Agent** tab in the middle top of the page to show the **Agent** page. As shown in the screenshot below, the cards on this page represent the created agents, which you can continue to edit.

![agent_mainpage](https://github.com/user-attachments/assets/5c0bb123-8f4e-42ea-b250-43f640dc6814)

We also provide templates catered to different business scenarios. You can either generate your agent from one of our agent templates or create one from scratch:

1. Click **+ Create agent** to show the **agent template** page:

   ![agent_templates](https://github.com/user-attachments/assets/73bd476c-4bab-4c8c-82f8-6b00fb2cd044)

2. To create an agent from scratch, click the **Blank** card. Alternatively, to create an agent from one of our templates, hover over the desired card, such as **General-purpose chatbot**, click **Use this template**, name your agent in the pop-up dialogue, and click **OK** to confirm.  

   *You are now taken to the **no-code workflow editor** page. The left panel lists the components (operators): Above the dividing line are the RAG-specific components; below the line are tools. We are still working to expand the component list.*

   ![workflow_editor](https://github.com/user-attachments/assets/47b4d5ce-b35a-4d6b-b483-ba495a75a65d)

3. General speaking, now you can do the following:
   - Drag and drop a desired component to your workflow,
   - Select the knowledge base to use,
   - Update settings of specific components,
   - Update LLM settings
   - Sets the input and output for a specific component, and more.
4. Click **Save** to apply changes to your agent and **Run** to test it.

## Components

Please review the flowing description of the RAG-specific components before you proceed:

| Component      | Description                                                                                                                                                                                              |
|----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Retrieval**  | A component that retrieves information from specified knowledge bases and returns 'Empty response' if no information is found. Ensure the correct knowledge bases are selected.                          |
| **Generate**   | A component that prompts the LLM to generate responses. You must ensure the prompt is set correctly.                                                                                                     |
| **Interact**   | A component that serves as the interface between human and the bot, receiving user inputs and displaying the agent's responses.                                                                          |
| **Categorize** | A component that uses the LLM to classify user inputs into predefined categories. Ensure you specify the name, description, and examples for each category, along with the corresponding next component. |
| **Message**    | A component that sends out a static message. If multiple messages are supplied, it randomly selects one to send. Ensure its downstream is **Interact**, the interface component.                         |
| **Rewrite**    | A component that rewrites a user query from the **Interact** component, based on the context of previous dialogues.                                                                                      |
| **Keyword**    | A component that extracts keywords from a user query, with TopN specifying the number of keywords to extract.                                                                                            |

:::caution NOTE

- Ensure **Rewrite**'s upstream component is **Relevant** and downstream component is **Retrieval**.
- Ensure the downstream component of **Message** is **Interact**.
- The downstream component of **Begin** is always **Interact**.

:::

## Basic operations

| Operation                 | Description                                                                                                                              |
|---------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| Add a component           | Drag and drop the desired component from the left panel onto the canvas.                                                                 |
| Delete a component        | On the canvas, hover over the three dots (...) of the component to display the delete option, then select it to remove the component.    |
| Copy a component          | On the canvas, hover over the three dots (...) of the component to display the copy option, then select it to make a copy the component. |
| Update component settings | On the canvas, click the desired component to display the component settings.                                                            |
