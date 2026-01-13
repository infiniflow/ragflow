---
sidebar_position: 1
slug: /start_chat
sidebar_custom_props: {
  categoryIcon: LucideBot
}
---
# Start AI chat

Initiate an AI-powered chat with a configured chat assistant.

---

Chats in RAGFlow are based on a particular dataset or multiple datasets. Once you have created your dataset, finished file parsing, and [run a retrieval test](../dataset/run_retrieval_test.md), you can go ahead and start an AI conversation.

## Start an AI chat

You start an AI conversation by creating an assistant.

1. Click the **Chat** tab in the middle top of the page **>** **Create an assistant** to show the **Chat Configuration** dialogue *of your next dialogue*.

   > RAGFlow offers you the flexibility of choosing a different chat model for each dialogue, while allowing you to set the default models in **System Model Settings**.

2. Update Assistant-specific settings:

   - **Assistant name** is the name of your chat assistant. Each assistant corresponds to a dialogue with a unique combination of datasets, prompts, hybrid search configurations, and large model settings.
   - **Empty response**:
     - If you wish to *confine* RAGFlow's answers to your datasets, leave a response here. Then, when it doesn't retrieve an answer, it *uniformly* responds with what you set here.
     - If you wish RAGFlow to *improvise* when it doesn't retrieve an answer from your datasets, leave it blank, which may give rise to hallucinations.
   - **Show quote**: This is a key feature of RAGFlow and enabled by default. RAGFlow does not work like a black box. Instead, it clearly shows the sources of information that its responses are based on.
   - Select the corresponding datasets. You can select one or multiple datasets, but ensure that they use the same embedding model, otherwise an error would occur.

3. Update Prompt-specific settings:

   - In **System**, you fill in the prompts for your LLM, you can also leave the default prompt as-is for the beginning.
   - **Similarity threshold** sets the similarity "bar" for each chunk of text. The default is 0.2. Text chunks with lower similarity scores are filtered out of the final response.
   - **Vector similarity weight** is set to 0.3 by default. RAGFlow uses a hybrid score system to evaluate the relevance of different text chunks. This value sets the weight assigned to the vector similarity component in the hybrid score.
     - If **Rerank model** is left empty, the hybrid score system uses keyword similarity and vector similarity, and the default weight assigned to the keyword similarity component is 1-0.3=0.7.
     - If **Rerank model** is selected, the hybrid score system uses keyword similarity and reranker score, and the default weight assigned to the reranker score is 1-0.7=0.3.
   - **Top N** determines the *maximum* number of chunks to feed to the LLM. In other words, even if more chunks are retrieved, only the top N chunks are provided as input.
   - **Multi-turn optimization** enhances user queries using existing context in a multi-round conversation. It is enabled by default. When enabled, it will consume additional LLM tokens and significantly increase the time to generate answers.
   - **Use knowledge graph** indicates whether to use knowledge graph(s) in the specified dataset(s) during retrieval for multi-hop question answering. When enabled, this would involve iterative searches across entity, relationship, and community report chunks, greatly increasing retrieval time.
   - **Reasoning** indicates whether to generate answers through reasoning processes like Deepseek-R1/OpenAI o1. Once enabled, the chat model autonomously integrates Deep Research during question answering when encountering an unknown topic. This involves the chat model dynamically searching external knowledge and generating final answers through reasoning.
   - **Rerank model** sets the reranker model to use. It is left empty by default.
     - If **Rerank model** is left empty, the hybrid score system uses keyword similarity and vector similarity, and the default weight assigned to the vector similarity component is 1-0.7=0.3.
     - If **Rerank model** is selected, the hybrid score system uses keyword similarity and reranker score, and the default weight assigned to the reranker score is 1-0.7=0.3.
   - [Cross-language search](../../references/glossary.mdx#cross-language-search): Optional  
     Select one or more target languages from the dropdown menu. The system’s default chat model will then translate your query into the selected target language(s). This translation ensures accurate semantic matching across languages, allowing you to retrieve relevant results regardless of language differences.  
     - When selecting target languages, please ensure that these languages are present in the dataset to guarantee an effective search.
     - If no target language is selected, the system will search only in the language of your query, which may cause relevant information in other languages to be missed.
   - **Variable** refers to the variables (keys) to be used in the system prompt. `{knowledge}` is a reserved variable. Click **Add** to add more variables for the system prompt.
      - If you are uncertain about the logic behind **Variable**, leave it *as-is*.
      - As of v0.17.2, if you add custom variables here, the only way you can pass in their values is to call:
         - HTTP method [Converse with chat assistant](../../references/http_api_reference.md#converse-with-chat-assistant), or
         - Python method [Converse with chat assistant](../../references/python_api_reference.md#converse-with-chat-assistant).

4. Update Model-specific Settings:

   - In **Model**: you select the chat model. Though you have selected the default chat model in **System Model Settings**, RAGFlow allows you to choose an alternative chat model for your dialogue.
   - **Creativity**: A shortcut to **Temperature**, **Top P**, **Presence penalty**, and **Frequency penalty** settings, indicating the freedom level of the model. From **Improvise**, **Precise**, to **Balance**, each preset configuration corresponds to a unique combination of **Temperature**, **Top P**, **Presence penalty**, and **Frequency penalty**.   
   This parameter has three options:
      - **Improvise**: Produces more creative responses.
      - **Precise**: (Default) Produces more conservative responses.
      - **Balance**: A middle ground between **Improvise** and **Precise**.
   - **Temperature**: The randomness level of the model's output.  
   Defaults to 0.1.
      - Lower values lead to more deterministic and predictable outputs.
      - Higher values lead to more creative and varied outputs.
      - A temperature of zero results in the same output for the same prompt.
   - **Top P**: Nucleus sampling.  
      - Reduces the likelihood of generating repetitive or unnatural text by setting a threshold *P* and restricting the sampling to tokens with a cumulative probability exceeding *P*.
      - Defaults to 0.3.
   - **Presence penalty**: Encourages the model to include a more diverse range of tokens in the response.  
      - A higher **presence penalty** value results in the model being more likely to generate tokens not yet been included in the generated text.
      - Defaults to 0.4.
   - **Frequency penalty**: Discourages the model from repeating the same words or phrases too frequently in the generated text.  
      - A higher **frequency penalty** value results in the model being more conservative in its use of repeated tokens.
      - Defaults to 0.7.

5. Now, let's start the show:

   ![chat_thermal_solution](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/chat_thermal_solution.jpg)

:::tip NOTE

1. Click the light bulb icon above the answer to view the expanded system prompt:

   ![prompt_display](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/prompt_display.jpg)

   *The light bulb icon is available only for the current dialogue.*

2. Scroll down the expanded prompt to view the time consumed for each task:

   ![time_elapsed](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/time_elapsed.jpg)
:::

## Update settings of an existing chat assistant

![chat_setting](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/chat_setting.jpg)

## Integrate chat capabilities into your application or webpage

RAGFlow offers HTTP and Python APIs for you to integrate RAGFlow's capabilities into your applications. Read the following documents for more information:

- [Acquire a RAGFlow API key](../../develop/acquire_ragflow_api_key.md)
- [HTTP API reference](../../references/http_api_reference.md)
- [Python API reference](../../references/python_api_reference.md)

You can use iframe to embed the created chat assistant into a third-party webpage:

1. Before proceeding, you must [acquire an API key](../../develop/acquire_ragflow_api_key.md); otherwise, an error message would appear.
2. Hover over an intended chat assistant **>** **Edit** to show the **iframe** window:

   ![chat-embed](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/embed_chat_into_webpage.jpg)

3. Copy the iframe and embed it into your webpage.

![chat-embed](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/embedded_chat_app.jpg)
