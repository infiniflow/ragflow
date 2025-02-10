---
sidebar_position: 2
slug: /release_notes
---

# Release notes

Key features, improvements and bug fixes in the latest releases.

## v0.16.0

Released on February 6, 2025.

### New features

- Supports DeepSeek R1 and DeepSeek V3.
- GraphRAG refactor: Knowledge graph is dynamically built on an entire knowledge base (dataset) rather than on an individual file, and automatically updated when files are added or removed.
- Adds an **Iteration** agent component and a **Research report generator** agent template.
- New UI language: Portuguese.
- Allows setting metadata for a specific file in a knowledge base to support AI-powered chats.
- Upgrades RAGFlow's document engine [Infinity](https://github.com/infiniflow/infinity) to v0.6.0.dev3.
- Supports creating and referencing a **Tag** knowledge base as a key milestone towards bridging the semantic gap between query and response.

:::danger IMPORTANT
The **Tag knowledge base** feature is *unavailable* on the [Infinity](https://github.com/infiniflow/infinity) document engine.
:::

### Documentation

#### Added documents

- [Begin component](https://ragflow.io/docs/dev/begin_component)
- [Generate component](https://ragflow.io/docs/dev/generate_component)
- [Interact component](https://ragflow.io/docs/dev/interact_component)
- [Retrieval component](https://ragflow.io/docs/dev/retrieval_component)
- [Categorize component](https://ragflow.io/docs/dev/categorize_component)
- [Keyword component](https://ragflow.io/docs/dev/keyword_component)
- [Message component](https://ragflow.io/docs/dev/message_component)
- [Rewrite component](https://ragflow.io/docs/dev/rewrite_component)
- [Switch component](https://ragflow.io/docs/dev/switch_component)
- [Concentrator component](https://ragflow.io/docs/dev/concentrator_component)
- [Template component](https://ragflow.io/docs/dev/template_component)
- [Iteration component](https://ragflow.io/docs/dev/iteration_component)
- [Note component](https://ragflow.io/docs/dev/note_component)

## v0.15.1

Released on December 25, 2024.

### Upgrades

- Upgrades RAGFlow's document engine [Infinity](https://github.com/infiniflow/infinity) to v0.5.2.
- Enhances the log display of document parsing status.

### Fixed issues

This release fixes the following issues:

- The `SCORE not found` and `position_int` errors returned by [Infinity](https://github.com/infiniflow/infinity).
- Once an embedding model in a specific knowledge base is changed, embedding models in other knowledge bases can no longer be changed.
- Slow response in question-answering and AI search due to repetitive loading of the embedding model.
- Fails to parse documents with RAPTOR.
- Using the **Table** parsing method results in information loss.
- Miscellaneous API issues.

### Related APIs

#### HTTP APIs

Adds an optional parameter `"user_id"` to the following APIs:

- [Create session with chat assistant](https://ragflow.io/docs/dev/http_api_reference#create-session-with-chat-assistant)
- [Update chat assistant's session](https://ragflow.io/docs/dev/http_api_reference#update-chat-assistants-session)
- [List chat assistant's sessions](https://ragflow.io/docs/dev/http_api_reference#list-chat-assistants-sessions)
- [Create session with agent](https://ragflow.io/docs/dev/http_api_reference#create-session-with-agent)
- [Converse with chat assistant](https://ragflow.io/docs/dev/http_api_reference#converse-with-chat-assistant)
- [Converse with agent](https://ragflow.io/docs/dev/http_api_reference#converse-with-agent)
- [List agent sessions](https://ragflow.io/docs/dev/http_api_reference#list-agent-sessions)

## v0.15.0

Released on December 18, 2024.

### New features

- Introduces additional Agent-specific APIs.
- Supports using page rank score to improve retrieval performance when searching across multiple knowledge bases.
- Offers an iframe in Chat and Agent to facilitate the integration of RAGFlow into your webpage.
- Adds a Helm chart for deploying RAGFlow on Kubernetes.
- Supports importing or exporting an agent in JSON format.
- Supports step run for Agent components/tools.
- Adds a new UI language: Japanese.
- Supports resuming GraphRAG and RAPTOR from a failure, enhancing task management resilience.
- Adds more Mistral models.
- Adds a dark mode to the UI, allowing users to toggle between light and dark themes.

### Improvements

- Upgrades the Document Layout Analysis model in Deepdoc.
- Significantly enhances the retrieval performance when using [Infinity](https://github.com/infiniflow/infinity) as document engine.

### Related APIs

#### HTTP APIs

- [List agent sessions](https://ragflow.io/docs/dev/http_api_reference#list-agent-sessions)
- [List agents](https://ragflow.io/docs/dev/http_api_reference#list-agents)

#### Python APIs

- [List agent sessions](https://ragflow.io/docs/dev/python_api_reference#list-agent-sessions)
- [List agents](https://ragflow.io/docs/dev/python_api_reference#list-agents)

## v0.14.1

Released on November 29, 2024.

### Improvements

Adds [Infinity's configuration file](https://github.com/infiniflow/ragflow/blob/main/docker/infinity_conf.toml) to facilitate integration and customization of [Infinity](https://github.com/infiniflow/infinity) as a document engine. From this release onwards, updates to Infinity's configuration can be made directly within RAGFlow and will take effect immediately after restarting RAGFlow using `docker compose`. [#3715](https://github.com/infiniflow/ragflow/pull/3715)

### Fixed issues

This release fixes the following issues:

- Unable to display or edit content of a chunk after clicking it.
- A `'Not found'` error in Elasticsearch.
- Chinese text becoming garbled during parsing.
- A compatibility issue with Polars.
- A compatibility issue between Infinity and GraphRAG.

## v0.14.0

Released on November 26, 2024.

### New features

- Supports [Infinity](https://github.com/infiniflow/infinity) or Elasticsearch (default) as document engine for vector storage and full-text indexing. [#2894](https://github.com/infiniflow/ragflow/pull/2894)
- Enhances user experience by adding more variables to the Agent and implementing auto-saving.
- Adds a three-step translation agent template, inspired by [Andrew Ng's translation agent](https://github.com/andrewyng/translation-agent).
- Adds an SEO-optimized blog writing agent template.
- Provides HTTP and Python APIs for conversing with an agent.
- Supports the use of English synonyms during retrieval processes.
- Optimizes term weight calculations, reducing the retrieval time by 50%.
- Improves task executor monitoring with additional performance indicators.
- Replaces Redis with Valkey.
- Adds three new UI languages (*contributed by the community*): Indonesian, Spanish, and Vietnamese.

### Compatibility changes

As of this release, **service_config.yaml.template** replaces **service_config.yaml** for configuring backend services. Upon Docker container startup, the environment variables defined in this template file are automatically populated and a **service_config.yaml** is auto-generated from it. [#3341](https://github.com/infiniflow/ragflow/pull/3341)

This approach eliminates the need to manually update **service_config.yaml** after making changes to **.env**, facilitating dynamic environment configurations.

:::danger IMPORTANT
Ensure that you [upgrade **both** your code **and** Docker image to this release](https://ragflow.io/docs/dev/upgrade_ragflow#upgrade-ragflow-to-the-most-recent-officially-published-release) before trying this new approach.
:::

### Related APIs

#### HTTP APIs

- [Create session with agent](https://ragflow.io/docs/dev/http_api_reference#create-session-with-agent)
- [Converse with agent](https://ragflow.io/docs/dev/http_api_reference#converse-with-agent)

#### Python APIs

- [Create session with agent](https://ragflow.io/docs/dev/python_api_reference#create-session-with-agent)
- [Converse with agent](https://ragflow.io/docs/dev/python_api_reference#create-session-with-agent)

### Documentation

#### Added documents

- [Configurations](https://ragflow.io/docs/dev/configurations)
- [Manage team members](https://ragflow.io/docs/dev/manage_team_members)
- [Run health check on RAGFlow's dependencies](https://ragflow.io/docs/dev/run_health_check)

## v0.13.0

Released on October 31, 2024.

### New features

- Adds the team management functionality for all users.
- Updates the Agent UI to improve usability.
- Adds support for Markdown chunking in the **General** chunk method.
- Introduces an **invoke** tool within the Agent UI.
- Integrates support for Dify's knowledge base API.
- Adds support for GLM4-9B and Yi-Lightning models.
- Introduces HTTP and Python APIs for dataset management, file management within dataset, and chat assistant management.

:::tip NOTE
To download RAGFlow's Python SDK:

```bash
pip install ragflow-sdk==0.13.0
```
:::

### Documentation

#### Added documents

- [Acquire a RAGFlow API key](https://ragflow.io/docs/dev/acquire_ragflow_api_key)
- [HTTP API Reference](https://ragflow.io/docs/dev/http_api_reference)
- [Python API Reference](https://ragflow.io/docs/dev/python_api_reference)

## v0.12.0

Released on September 30, 2024.

### New features

- Offers slim editions of RAGFlow's Docker images, which do not include built-in BGE/BCE embedding or reranking models.
- Improves the results of multi-round dialogues.
- Enables users to remove added LLM vendors.
- Adds support for **OpenTTS** and **SparkTTS** models.
- Implements an **Excel to HTML** toggle in the **General** chunk method, allowing users to parse a spreadsheet into either HTML tables or key-value pairs by row.
- Adds agent tools **YahooFinance** and **Jin10**.
- Adds an investment advisor agent template.

### Compatibility changes

As of this release, RAGFlow offers slim editions of its Docker images to improve the experience for users with limited Internet access. A slim edition of RAGFlow's Docker image does not include built-in BGE/BCE embedding models and has a size of about 1GB; a full edition of RAGFlow is approximately 9GB and includes both built-in embedding models and embedding models that will be downloaded once you select them in the RAGFlow UI.

The default Docker image edition is `nightly-slim`. The following list clarifies the differences between various editions:

- `nightly-slim`: The slim edition of the most recent tested Docker image.
- `v0.12.0-slim`: The slim edition of the most recent **officially released** Docker image.
- `nightly`: The full edition of the most recent tested Docker image.
- `v0.12.0`: The full edition of the most recent **officially released** Docker image.

See [Upgrade RAGFlow](https://ragflow.io/docs/dev/upgrade_ragflow) for instructions on upgrading.

### Documentation

#### Added documents

- [Upgrade RAGFlow](https://ragflow.io/docs/dev/upgrade_ragflow)

## v0.11.0

Released on September 14, 2024.

### New features

-  Introduces an AI search interface within the RAGFlow UI.
-  Supports audio output via **FishAudio** or **Tongyi Qwen TTS**.
-  Allows the use of Postgres for metadata storage, in addition to MySQL.
-  Supports object storage options with S3 or Azure Blob.
-  Supports model vendors: **Anthropic**, **Voyage AI**, and **Google Cloud**.
-  Supports the use of **Tencent Cloud ASR** for audio content recognition.
-  Adds finance-specific agent components: **WenCai**, **AkShare**, **YahooFinance**, and **TuShare**.
-  Adds a medical consultant agent template.
-  Supports running retrieval benchmarking on the following datasets:
    - [ms_marco_v1.1](https://huggingface.co/datasets/microsoft/ms_marco)
    - [trivia_qa](https://huggingface.co/datasets/mandarjoshi/trivia_qa)
    - [miracl](https://huggingface.co/datasets/miracl/miracl)

## v0.10.0

Released on August 26, 2024.

### New features

- Introduces a text-to-SQL template in the Agent UI.
- Implements Agent APIs.
- Incorporates monitoring for the task executor.
- Introduces Agent tools **GitHub**, **DeepL**, **BaiduFanyi**, **QWeather**, and **GoogleScholar**.
- Supports chunking of EML files.
- Supports more LLMs or model services: **GPT-4o-mini**, **PerfXCloud**, **TogetherAI**, **Upstage**, **Novita.AI**, **01.AI**, **SiliconFlow**, **XunFei Spark**, **Baidu Yiyan**, and **Tencent Hunyuan**.

## v0.9.0

Released on August 6, 2024.

### New features

- Supports GraphRAG as a chunk method.
- Introduces Agent component **Keyword** and search tools, including **Baidu**, **DuckDuckGo**, **PubMed**, **Wikipedia**, **Bing**, and **Google**.
- Supports speech-to-text recognition for audio files.
- Supports model vendors **Gemini** and **Groq**.
- Supports inference frameworks, engines, and services including **LM studio**, **OpenRouter**, **LocalAI**, and **Nvidia API**.
- Supports using reranker models in Xinference.

## v0.8.0

Released on July 8, 2024.

### New features

- Supports Agentic RAG, enabling graph-based workflow construction for RAG and agents.
- Supports model vendors **Mistral**, **MiniMax**, **Bedrock**, and **Azure OpenAI**.
- Supports DOCX files in the MANUAL chunk method.
- Supports DOCX, MD, and PDF files in the Q&A chunk method.

## v0.7.0

Released on May 31, 2024.

### New features

- Supports the use of reranker models.
- Integrates reranker and embedding models: [BCE](https://github.com/netease-youdao/BCEmbedding), [BGE](https://github.com/FlagOpen/FlagEmbedding), and [Jina](https://jina.ai/embeddings/).
- Supports LLMs Baichuan and VolcanoArk.
- Implements [RAPTOR](https://arxiv.org/html/2401.18059v1) for improved text retrieval.
- Supports HTML files in the GENERAL chunk method.
- Provides HTTP and Python APIs for deleting documents by ID.
- Supports ARM64 platforms.

:::danger IMPORTANT
While we also test RAGFlow on ARM64 platforms, we do not maintain RAGFlow Docker images for ARM.

If you are on an ARM platform, follow [this guide](https://ragflow.io/docs/dev/build_docker_image) to build a RAGFlow Docker image.
:::

### Related APIs

#### HTTP API

- [Delete documents](https://ragflow.io/docs/dev/http_api_reference#delete-documents)

#### Python API

- [Delete documents](https://ragflow.io/docs/dev/python_api_reference#delete-documents)

## v0.6.0

Released on May 21, 2024.

### New features

- Supports streaming output.
- Provides HTTP and Python APIs for retrieving document chunks.
- Supports monitoring of system components, including Elasticsearch, MySQL, Redis, and MinIO.
- Supports disabling **Layout Recognition** in the GENERAL chunk method to reduce file chunking time.

### Related APIs

#### HTTP API

- [Retrieve chunks](https://ragflow.io/docs/dev/http_api_reference#retrieve-chunks)

#### Python API

- [Retrieve chunks](https://ragflow.io/docs/dev/python_api_reference#retrieve-chunks)

## v0.5.0

Released on May 8, 2024.

### New features

- Supports LLM DeepSeek.
