---
sidebar_position: 4
slug: /llm_api_key_setup
---

# Set your LLM API key

You have two ways to input your LLM API key. 

## Before Starting The System

In **user_default_llm** of [service_conf.yaml](https://github.com/infiniflow/ragflow/blob/main/docker/service_conf.yaml), you need to specify LLM factory and your own _API_KEY_. 
RagFlow supports the flowing LLM factory, and with more coming in the pipeline:

> [OpenAI](https://platform.openai.com/login?launch), [Tongyi-Qianwen](https://dashscope.console.aliyun.com/model), 
> [ZHIPU-AI](https://open.bigmodel.cn/), [Moonshot](https://platform.moonshot.cn/docs)

After sign in these LLM suppliers, create your own API-Key, they all have a certain amount of free quota.

## After Starting The System

You can also set API-Key in **User Setting** as following:

![](https://github.com/infiniflow/ragflow/assets/12318111/e4e4066c-e964-45ff-bd56-c3fc7fb18bd3)

