---
sidebar_position: 1
slug: /llm_api_key_setup
---

# Configure model API key

An API key is required for RAGFlow to interact with an online AI model. This guide provides information about setting your model API key in RAGFlow.

## Get model API key

RAGFlow supports most mainstream LLMs. Please refer to [Supported Models](../../references/supported_models.mdx) for a complete list of supported models. You will need to apply for your model API key online. Note that most LLM providers grant newly-created accounts trial credit, which will expire in a couple of months, or a promotional amount of free quota.

:::note
If you find your online LLM is not on the list, don't feel disheartened. The list is expanding, and you can [file a feature request](https://github.com/infiniflow/ragflow/issues/new?assignees=&labels=feature+request&projects=&template=feature_request.yml&title=%5BFeature+Request%5D%3A+) with us! Alternatively, if you have customized or locally-deployed models, you can [bind them to RAGFlow using Ollama, Xinference, or LocalAI](./deploy_local_llm.mdx).
:::

## Configure model API key

You have two options for configuring your model API key:

- Configure it in **service_conf.yaml.template** before starting RAGFlow.
- Configure it on the **Model providers** page after logging into RAGFlow.

### Configure model API key before starting up RAGFlow

1. Navigate to **./docker/ragflow**.
2. Find entry **user_default_llm**:
   - Update `factory` with your chosen LLM.
   - Update `api_key` with yours.
   - Update `base_url` if you use a proxy to connect to the remote service.
3. Reboot your system for your changes to take effect.
4. Log into RAGFlow.  
   _After logging into RAGFlow, you will find your chosen model appears under **Added models** on the **Model providers** page._

### Configure model API key after logging into RAGFlow

:::caution WARNING
After logging into RAGFlow, configuring your model API key through the **service_conf.yaml.template** file will no longer take effect.
:::

After logging into RAGFlow, you can *only* configure API Key on the **Model providers** page:

1. Click on your logo on the top right of the page **>** **Model providers**.
2. Find your model card under **Models to be added** and click **Add the model**:
   ![add model](https://github.com/infiniflow/ragflow/assets/93570324/07e43f63-367c-4c9c-8ed3-8a3a24703f4e)
3. Paste your model API key.
4. Fill in your base URL if you use a proxy to connect to the remote service.
5. Click **OK** to confirm your changes.

:::note
To update an existing model API key at a later point:
![update api key](https://github.com/infiniflow/ragflow/assets/93570324/0bfba679-33f7-4f6b-9ed6-f0e6e4b228ad)
:::