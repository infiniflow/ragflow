---
sidebar_position: 4
slug: /llm_api_key_setup
---

# Set your LLM API key

An API key is required for RAGFlow to interact with a selected online Large Language Model. This document provides information about where to update your API key in RAGFlow.

## Get your API key

For now, RAGFlow supports the following online LLMs. Clik the corresponding link to apply for your API key. Most LLM providers grant newly-created accounts trial credit, which will expire in a couple of months, or a promotional amount of free quota.

- [OpenAI](https://platform.openai.com/login?launch),
- [Tongyi-Qianwen](https://dashscope.console.aliyun.com/model), 
- [ZHIPU-AI](https://open.bigmodel.cn/),
- [Moonshot](https://platform.moonshot.cn/docs),
- [DeepSeek](https://platform.deepseek.com/api-docs/),
- [Baichuan](https://www.baichuan-ai.com/home),
- [VolcEngine](https://www.volcengine.com/docs/82379).

:::note
If you find your online LLM is not on the list, don't feel disheartened. The list is expanding, and you can [file a feature request](https://github.com/infiniflow/ragflow/issues/new?assignees=&labels=feature+request&projects=&template=feature_request.yml&title=%5BFeature+Request%5D%3A+) with us! Alternatively, if you have customized your own models or have locally deployed models, you can [bind them to RAGFlow using Ollama or Xinference](./deploy_local_llm.md).
:::

## Before Starting The System

In **user_default_llm** of [service_conf.yaml](https://github.com/infiniflow/ragflow/blob/main/docker/service_conf.yaml), you need to specify LLM factory and your own _API_KEY_. 

:::caution IMPORTANT
Updates to all system configuration files require a system reboot to take effect.
:::



## After Starting The System

You can also set API-Key in **User Setting** as following:

![](https://github.com/infiniflow/ragflow/assets/12318111/e4e4066c-e964-45ff-bd56-c3fc7fb18bd3)

