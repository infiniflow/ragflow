---
sidebar_position: 4
slug: /llm_api_key_setup
---

# Configure your API key

An API key is required for RAGFlow to interact with an online AI model. This guide provides information about setting your API key in RAGFlow.

## Get your API key

For now, RAGFlow supports the following online LLMs. Click the corresponding link to apply for your API key. Most LLM providers grant newly-created accounts trial credit, which will expire in a couple of months, or a promotional amount of free quota.

- [OpenAI](https://platform.openai.com/login?launch)
- [Azure-OpenAI](https://ai.azure.com/)
- [Gemini](https://aistudio.google.com/)
- [Groq](https://console.groq.com/)
- [Mistral](https://mistral.ai/)
- [Bedrock](https://aws.amazon.com/cn/bedrock/)
- [Tongyi-Qianwen](https://dashscope.console.aliyun.com/model)
- [ZHIPU-AI](https://open.bigmodel.cn/)
- [MiniMax](https://platform.minimaxi.com/)
- [Moonshot](https://platform.moonshot.cn/docs)
- [DeepSeek](https://platform.deepseek.com/api-docs/)
- [Baichuan](https://www.baichuan-ai.com/home)
- [VolcEngine](https://www.volcengine.com/docs/82379)
- [Jina](https://jina.ai/reader/)
- [OpenRouter](https://openrouter.ai/)
- [StepFun](https://platform.stepfun.com/)

:::note
If you find your online LLM is not on the list, don't feel disheartened. The list is expanding, and you can [file a feature request](https://github.com/infiniflow/ragflow/issues/new?assignees=&labels=feature+request&projects=&template=feature_request.yml&title=%5BFeature+Request%5D%3A+) with us! Alternatively, if you have customized or locally-deployed models, you can [bind them to RAGFlow using Ollama, Xinferenc, or LocalAI](./deploy_local_llm.md).
:::

## Configure your API key

You have two options for configuring your API key:

- Configure it in **service_conf.yaml** before starting RAGFlow.
- Configure it on the **Model Providers** page after logging into RAGFlow.

### Configure API key before starting up RAGFlow

1. Navigate to **./docker/ragflow**.
2. Find entry **user_default_llm**:
   - Update `factory` with your chosen LLM.
   - Update `api_key` with yours.
   - Update `base_url` if you use a proxy to connect to the remote service.
3. Reboot your system for your changes to take effect.
4. Log into RAGFlow.
   
   *After logging into RAGFlow, you will find your chosen model appears under **Added models** on the **Model Providers** page.*

### Configure API key after logging into RAGFlow

:::caution WARNING
After logging into RAGFlow, configuring API key through the **service_conf.yaml** file will no longer take effect.
:::

After logging into RAGFlow, you can *only* configure API Key on the **Model Providers** page:

1. Click on your logo on the top right of the page **>** **Model Providers**.
2. Find your model card under **Models to be added** and click **Add the model**:
   ![add model](https://github.com/infiniflow/ragflow/assets/93570324/07e43f63-367c-4c9c-8ed3-8a3a24703f4e)
3. Paste your API key.
4. Fill in your base URL if you use a proxy to connect to the remote service.
5. Click **OK** to confirm your changes.

:::note
If you wish to update an existing API key at a later point:
![update api key](https://github.com/infiniflow/ragflow/assets/93570324/0bfba679-33f7-4f6b-9ed6-f0e6e4b228ad)
:::