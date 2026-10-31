---
sidebar_position: 1
title: Configure Model Providers
sidebar_label: Configure Model Providers
slug: /llm_api_key_setup
sidebar_custom_props: {
  categoryIcon: LucideKey
}
---
# Configure Model Providers

RAGFlow model provider management allows you to connect online models, local models, and OpenAI-compatible models to RAGFlow for use in knowledge bases, chats, search, and agents.

## Get a Model Provider API Key

RAGFlow supports most mainstream model providers. If an online provider uses API key authentication, obtain an API key from that provider before configuring it in RAGFlow. Local providers and providers that use other authentication methods may not require an API key.

:::note
If the model provider you need is not listed, don't feel disheartened. The list is expanding, and you can [file a feature request](https://github.com/infiniflow/ragflow/issues/new?assignees=&labels=feature+request&projects=&template=feature_request.yml&title=%5BFeature+Request%5D%3A+) with us! Alternatively, you can [connect locally deployed models to RAGFlow using Ollama or vLLM](./deploy_local_llm.mdx).
:::

## Add a Model Provider Instance

### Select a Model Provider

Go to **User settings** **>** **Model providers**. In **Available models**, select the provider you want to configure.

![Select model provider](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/select_model_provider.jpeg)

### Create a Model Provider Instance and Configure Connection Information

An instance stores a set of connection settings under a provider. You can create separate instances for test environments, production environments, local models, or proxy gateways to avoid mixing configurations for different purposes. Configure the instance name, connection information, and models in the instance card, verify the selected models, and then save the instance.

When present, **API Key** is used for authentication and **Base URL** specifies the model service endpoint. The required fields depend on the provider. For example, local providers such as Ollama and VLLM allow an empty API key, some official providers use a fixed endpoint, and Amazon Bedrock supports multiple authentication methods.

For official providers, keep the default **Base URL** in most cases. For proxies, gateways, local models, or compatible APIs, enter the actual service address.

To configure a model provider:

1. Select the provider you want to configure.
2. Enter an instance name.
3. Complete the required fields displayed for that provider.
4. Select models from the available model list or add custom models.
5. Verify models individually, or click **Verify all models** to verify every model currently shown by the active search and type filters.
6. Save the instance.

![Create instance](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_instance.jpg)

:::caution
Do not expose your API Key. An incorrect Base URL causes model verification or model calls to fail. When using a compatible API, confirm whether the path must include `/v1`.
:::

#### Amazon Bedrock API keys

For **Bedrock**, select **API Key**, enter the Bedrock API key and AWS Region, then list and select the models available to that key. RAGFlow keeps the key scoped to that provider instance and sends it as a Bearer token only for that instance's requests.

Bedrock API key authentication does not support rerank models.

Use short-term Bedrock API keys for production whenever possible. Long-term keys remain valid until they expire or are deleted, so store them as secrets, restrict access to the RAGFlow instance, and rotate them regularly.

## Add Models to an Instance

In the provider instance card, add and configure the models required by your business, such as large language models (LLMs), embedding models, vision-language models (VLMs), automatic speech recognition models (ASR), rerank models, and text-to-speech models (TTS).

After adding models, you can set them as the default models for the corresponding model types.

### Add Models from the List

RAGFlow displays models discovered from the provider together with any models defined in its built-in catalog. You can search for the models you need and add them one by one, or add the models in the current list in batch.

![Add models from list](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/add_models_from_list.png)

### Add a Custom Model

If the required model is not shown in the list but is actually supported by the model provider, you can add it manually as a custom model. When adding a custom model, fill in the model name and select the model type.

The model name must match the model ID exposed by the model provider API. Otherwise, RAGFlow may not be able to identify the model correctly during calls.

To add a custom model:

1. Go to the configured provider instance and click **Add custom model**.
2. Enter the model name. The name must match the actual model identifier provided by the provider.
3. Select the corresponding model type.
4. Optionally fill in **Max context length** to override the detected value or provide one when the provider does not return it. This value sets the maximum number of tokens the model can process in its context, including the input and generated output. If you do not need to override it, keep the default value.
5. If the model supports tool calling, enable **Tool calling**. After it is enabled, the model can call external tools or functions during chats or agent runs, such as knowledge retrieval or API requests. Do not enable it for models that do not support this capability.
6. Click **Confirm** to add the model.


![Add custom model1](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/add_custom_model_1.jpg)

![Add custom model2](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/add_custom_model_2.jpg)

### Verify Models

Verify the models you plan to use with the instance's current credentials and endpoint.

1. Open **User settings > Model providers**, select a provider, and open the instance you want to check.
2. Complete the required connection fields, such as **API Key** and **Base URL**, and make sure the required models are in the instance's model list.
3. To check one model, click the circular-arrow verification icon on its row. The button is identified as **Verify** followed by the model name.
4. To check multiple models, use the model search and type tabs to narrow the list, then click **Verify all models**. This checks the models displayed by the active filters. Clear the search and select **All models** if you want to check the full list.
5. Wait for the row indicators: a spinner means verification is in progress, a green check means success, and a warning symbol means failure. For a failed model, check the reported error and correct the credentials, endpoint, model name, service availability, or account quota, then verify it again.
6. Click **Save** to save the instance configuration. Verification does not replace saving.

## Set Default Models

Default models are used when RAGFlow needs to select a model automatically and no model has been specified separately. Set default models after adding and verifying models to avoid selecting unavailable models on business pages.

At minimum, set the following defaults:

1. Default LLM.
2. Default embedding model.

If you have configured a rerank model, it is also recommended to set a default rerank model. Configure VLM, ASR, and TTS defaults as required by your business.

![Set default models](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_default_models.jpg)

### Decision Model

**Decision model** selects the default model used to judge whether retrieved passages are relevant enough to pass to the LLM. It is separate from the LLM that generates the answer and from the rerank model that scores and orders candidates. Leave it unset if you do not need this filtering.

1. Add and verify a model that supports the **Decision** type under **User settings > Model providers**.
2. Open **Set default models** and select the model in **Decision model**.
3. Check that the selected model and provider instance are displayed in the field.

For a particular Chat, you can select its decision model under **Chat setting > Advanced settings > Agentic > Decision model** and save the Chat settings. See [Chat Configuration](../chat/chat_configuration.md#agentic-settings).

## Model Types and Usage

| Model type | Full name | Main function | Input | Output | Typical scenarios |
| --- | --- | --- | --- | --- | --- |
| LLM | Large Language Model | Understands, reasons over, and generates text | Text prompts | Text | Intelligent question answering, content generation, summarization, and information extraction |
| Embedding | Embedding model | Converts text into vector representations | Text | Vectors | Semantic retrieval, similarity calculation, and knowledge base indexing |
| Rerank | Rerank model | Scores and reorders initially retrieved candidate results by relevance | Query text and candidate text | Relevance scores and ranking results | Optimizing retrieval results and improving knowledge base Q&A accuracy |
| VLM | Vision-Language Model | Understands images and the text, objects, and scene information in them | Images, text, or images with text | Text | Image Q&A, chart understanding, and visual content analysis |
| ASR | Automatic Speech Recognition model | Converts speech into text | Audio | Text | Speech transcription, meeting records, and real-time captions |
| TTS | Text-to-Speech model | Converts text into speech | Text | Audio | Voice playback, audio content, and voice interaction |
| Decision | Decision model | Judges the relevance of retrieved passages before they are passed to the LLM | Query and retrieved content | Relevance decision | Filtering retrieval evidence |
| OCR | Optical Character Recognition model | Recognizes text in images or scanned documents | Images or scanned documents | Recognized text | Scanned document recognition and receipt recognition |

The following model types serve different parts of retrieval, generation, and multimodal workflows:

1. **Embedding**, **Rerank**, and **LLM**: The embedding model converts queries and knowledge chunks into vectors, and the system recalls candidate chunks based on vector similarity. The rerank model scores and reorders the candidate chunks by relevance. The LLM understands the question based on the selected knowledge content and generates the answer.

2. **VLM**, **ASR**, **TTS**, and **OCR**: Configure these capabilities individually or combine them according to the multimodal scenario. VLM understands images and image-text information, OCR recognizes text in images or scanned documents, ASR converts speech into text, and TTS converts text into speech.
