---
sidebar_position: 2
slug: /start_chat
---

# Start an AI-powered chat

Knowledge base, hallucination-free chat, and file management are the three pillars of RAGFlow. Chats in RAGFlow are based on a particular knowledge base or multiple knowledge bases. Once you have created your knowledge base and finished file parsing, you can go ahead and start an AI conversation.

## Start an AI chat

You start an AI conversation by creating an assistant.

1. Click the **Chat** tab in the middle top of the page **>** **Create an assistant** to show the **Chat Configuration** dialogue *of your next dialogue*.

   > RAGFlow offers you the flexibility of choosing a different chat model for each dialogue, while allowing you to set the default models in **System Model Settings**.

2. Update **Assistant Setting**:

   - **Assistant name** is the name of your chat assistant. Each assistant corresponds to a dialogue with a unique combination of knowledge bases, prompts, hybrid search configurations, and large model settings.
   - **Empty response**:
     - If you wish to *confine* RAGFlow's answers to your knowledge bases, leave a response here. Then, when it doesn't retrieve an answer, it *uniformly* responds with what you set here.
     - If you wish RAGFlow to *improvise* when it doesn't retrieve an answer from your knowledge bases, leave it blank, which may give rise to hallucinations.
   - **Show quote**: This is a key feature of RAGFlow and enabled by default. RAGFlow does not work like a black box. instead, it clearly shows the sources of information that its responses are based on.
   - Select the corresponding knowledge bases. You can select one or multiple knowledge bases, but ensure that they use the same embedding model, otherwise an error would occur.

3. Update **Prompt Engine**:

   - In **System**, you fill in the prompts for your LLM, you can also leave the default prompt as-is for the beginning.
   - **Similarity threshold** sets the similarity "bar" for each chunk of text. The default is 0.2. Text chunks with lower similarity scores are filtered out of the final response.
   - **Vector similarity weight** is set to 0.3 by default. RAGFlow uses a hybrid score system, combining keyword similarity and vector similarity, for evaluating the relevance of different text chunks. This value sets the weight assigned to the vector similarity component in the hybrid score.
   - **Top N** determines the *maximum* number of chunks to feed to the LLM. In other words, even if more chunks are retrieved, only the top N chunks are provided as input.
   - **Variable**:

4. Update **Model Setting**:

   - In **Model**: you select the chat model. Though you have selected the default chat model in **System Model Settings**, RAGFlow allows you to choose an alternative chat model for your dialogue.
   - **Freedom** refers to the level that the LLM improvises. From **Improvise**, **Precise**, to **Balance**, each freedom level corresponds to a unique combination of **Temperature**, **Top P**, **Presence penalty**, and **Frequency penalty**.
   - **Temperature**: Level of the prediction randomness of the LLM. The higher the value, the more creative the LLM is.
   - **Top P** is also known as "nucleus sampling". See [here](https://en.wikipedia.org/wiki/Top-p_sampling) for more information.
   - **Max Tokens**: The maximum length of the LLM's responses. Note that the responses may be curtailed if this value is set too low.

5. Now, let's start the show:

   ![question1](https://github.com/infiniflow/ragflow/assets/93570324/bb72dd67-b35e-4b2a-87e9-4e4edbd6e677)

   ![question2](https://github.com/infiniflow/ragflow/assets/93570324/7cc585ae-88d0-4aa2-817d-0370b2ad7230)

## Update settings of an existing chat assistant

Hover over an intended chat assistant **>** **Edit** to show the chat configuration dialogue:

![edit_chat](https://github.com/user-attachments/assets/5c514cf0-a959-4cfe-abad-5e42a0e23974)

![chat_config](https://github.com/user-attachments/assets/1a4eaed2-5430-4585-8ab6-930549838c5b)

## Integrate chat capabilities into your application

RAGFlow also offers HTTP and Python APIs for you to integrate RAGFlow's capabilities into your applications. Read the following documents for more information:

- [Acquire a RAGFlow API key](./develop/acquire_ragflow_api_key.md)
- [HTTP API reference](../references/http_api_reference.md)
- [Python API reference](../references/python_api_reference.md)
