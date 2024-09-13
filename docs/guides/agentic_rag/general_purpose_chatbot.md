---
sidebar_position: 2
slug: /general_purpose_chatbot
---

# Create and configure a chatbot agent

Chatbot is one of the most common AI scenarios. However, effectively understanding user queries and responding appropriately remains a challenge. RAGFlow's native chatbot agent is our attempt to tackle this longstanding issue.  

This chatbot closely resembles the chatbot introduced in [Start an AI chat](../start_chat.md), but with a key difference - it introduces a reflective mechanism that allows it to improve the retrieval from the target knowledge bases by rewriting the user's query.

This document provides guides on creating such a chatbot using our chatbot template.

## Prerequisites

1. Ensure you have properly set the LLM to use. See the guides on [Configure your API key](../llm_api_key_setup.md) or [Deploy a local LLM](../deploy_local_llm.mdx) for more information.
2. Ensure you have a knowledge base configured and the corresponding files properly parsed. See the guide on [Configure a knowledge base](../configure_knowledge_base.md) for more information.
3. Make sure you have read the [Introduction to Agentic RAG](./agentic_rag_introduction.md).

## Create a chatbot agent from template

To create a general-purpose chatbot agent using our template:

1. Click the **Agent** tab in the middle top of the page to show the **Agent** page.
2. Click **+ Create agent** on the top right of the page to show the **agent template** page.
3. On the **agent template** page, hover over the card on **General-purpose chatbot** and click **Use this template**.  
   *You are now directed to the **no-code workflow editor** page.*

   ![workflow_editor](https://github.com/user-attachments/assets/9fc6891c-7784-43b8-ab4a-3b08a9e551c4)

:::tip NOTE
RAGFlow's no-code editor spares you the trouble of coding, making agent development effortless.
:::

## Understand each component in the template

Here’s a breakdown of each component and its role and requirements in the chatbot template:

- **Begin**
  - Function: Sets the opening greeting for the user.
  - Purpose: Establishes a welcoming atmosphere and prepares the user for interaction.

- **Answer**:
  - Function: Serves as the interface between human and the bot.
  - Role: Acts as the downstream component of **Begin**.  
  - Note: Though named "Answer", it does not engage with the LLM.

- **Retrieval**:
  - Function: Retrieves information from specified knowledge base(s).
  - Requirement: Must have `knowledgebases` set up to function.

- **Relevant**:  
  - Function: Assesses the relevance of the retrieved information from the **Retrieval** component to the user query.
  - Process:  
    - If relevant, it directs the data to the **Generate** component for final response generation.
    - Otherwise, it triggers the **Rewrite** component to refine the user query and redo the retrival process.

- **Generate**: Prompts the LLM to generate responses.  
  - This is where you control the way in which the LLM responses.  
  - Ensure you review the prompts and make changes where you see necessary.

- **Rewrite**:  
  - Function: Refines a user query when no relevant information from the knowledge base is retrieved.  
  - Usage: Often used in conjunction with **Relevant** and **Retrieval** to create a reflective/feedback loop.  

## Configure your chatbot agent

1. Click **Begin** to set an opening greeting:  
   ![opener](https://github.com/user-attachments/assets/4416bc16-2a84-4f24-a19b-6dc8b1de0908)

2. Choose 

![loop_time](https://github.com/user-attachments/assets/09a4ce34-7aac-496f-aa59-d8aa33bf0b1f)
![choose_model](https://github.com/user-attachments/assets/2bac1d6c-c4f1-42ac-997b-102858c3f550)






10. Updates your workflow where you see necessary.

11. Click **Save** to save all your changes.

11. General speaking, now you can do the following:
   - Drag and drop a desired component to your workflow,
   - Select the knowledge base to use,
   - Update settings of specific components,
   - Update LLM settings
   - Sets the input and output for a specific component, and more.
12. Click **Save** to apply changes to your agent and **Run** to test it.

