---
sidebar_position: 2
slug: /release_notes
sidebar_custom_props: {
  sidebarIcon: LucideClipboardPenLine
}
---
# Releases

Key features, improvements and bug fixes in the latest releases.


## v0.23.1

Released on December 31, 2025.

### Improvements

- Memory: Enhances the stability of memory extraction when all memory types are selected.
- RAG: Refines the context window extraction strategy for images and tables.


### Fixed issues

- Memory: 
  - The RAGFlow server failed to start if an empty memory object existed.
  - Unable to delete a newly created empty Memory.
- RAG: MDX file parsing was not supported.

### Data sources

- GitHub
- Gitlab
- Asana
- IMAP

## v0.23.0

Released on December 27, 2025.

### New features

- Memory
   - Implements a **Memory** interface for managing memory.
   - Supports configuring context via the **Retrieval** or **Message** component.
- Agent
   - Improves the **Agent** component's performance by refactoring the underlying architecture.
   - The **Agent** component can now output structured data for use in downstream components.
   - Supports using webhook to trigger agent execution.
   - Supports voice input/output.
   - Supports configuring multiple **Retrieval** components per **Agent** component.
- Ingestion pipeline
  - Supports extracting table of contents in the **Transformer** component to improve long-context RAG performance.
- Dataset
   - Supports configuring context window for images and tables.
   - Introduces parent-child chunking strategy.
   - Supports auto-generation of metadata during file parsing.
- Chat: Supports voice input.

### Improvements

- RAG: Accelerates GraphRAG generation significantly.
- Bumps RAGFlow's document engine, [Infinity](https://github.com/infiniflow/infinity) to v0.6.15 (backward compatible).

### Data sources

- Google Cloud Storage
- Gmail
- Dropbox
- WebDAV
- Airtable

### Model support

- GPT-5.2
- GPT-5.2 Pro
- GPT-5.1
- GPT-5.1 Instant
- Claude Opus 4.5
- MiniMax M2
- GLM-4.7.
- A MinerU configuration interface.
- AI Badgr (model provider).

### API changes

#### HTTP API

- [Converse with Agent](./references/http_api_reference.md#converse-with-agent) returns complete execution trace logs.
- [Create chat completion](./references/http_api_reference.md#create-chat-completion) supports metadata-based filtering.
- [Converse with chat assistant](./references/http_api_reference.md#converse-with-chat-assistant) supports metadata-based filtering.

## v0.22.1

Released on November 19, 2025.

### Improvements

- Agent:
  - Supports exporting Agent outputs in Word or Markdown formats.
  - Adds a **List operations** component.
  - Adds a **Variable aggregator** component.
- Data sources:
  - Supports S3-compatible data sources, e.g., MinIO.
  - Adds data synchronization with JIRA.
- Continues the redesign of the **Profile** page layouts.
- Upgrades the Flask web framework from synchronous to asynchronous, increasing concurrency and preventing blocking issues caused when requesting upstream LLM services.

### Fixed issues

- A v0.22.0 issue: Users failed to parse uploaded files or switch embedding model in a dataset containing parsed files using a built-in model from a `-full` RAGFlow edition.
- Image concatenated in Word documents. [#11310](https://github.com/infiniflow/ragflow/pull/11310)
- Mixed images and text were not correctly displayed in the chat history.

### Newly supported models

- Gemini 3 Pro Preview

## v0.22.0

Released on November 12, 2025.

### Breaking Changes

:::danger IMPORTANT
From this release onwards, we ship only the slim edition (without embedding models) Docker image and no longer append the `-slim` suffix to the image tag.
:::

### New Features

- Dataset:
  - Supports data synchronization from five online sources (AWS S3, Google Drive, Notion, Confluence, and Discord).
  - RAPTOR can be built across an entire dataset or on individual documents.
- Ingestion pipeline: Supports [Docling document parsing](https://github.com/docling-project/docling) in the **Parser** component.
- Launches a new administrative Web UI dashboard for graphical user management and service status monitoring.
- Agent:
  - Supports structured output.
  - Supports metadata filtering in the **Retrieval** component.
  - Introduces a **Variable aggregator** component with data operation and session variable definition capabilities.

### Improvements

- Agent: Supports visualizing previous components' outputs in the **Await Response** component.
- Revamps the model provider page.
- Upgrades RAGFlow's document engine Infinity to v0.6.5.

### Added Models

- Kimi-K2-Thinking

### New agent templates

- Interactive Agent, incorporates real-time user feedback to dynamically optimize Agent output.

## v0.21.1

Released on October 23, 2025.

### New features

- Experimental: Adds support for PDF document parsing using MinerU. See [here](./faq.mdx#how-to-use-mineru-to-parse-pdf-documents).

### Improvements

- Enhances UI/UX for the dataset and personal center pages.
- Upgrades RAGFlow's document engine, [Infinity](https://github.com/infiniflow/infinity), to v0.6.1.

### Fixed issues

- An issue with video parsing.

## v0.21.0

Released on October 15, 2025.

### New features

- Orchestratable ingestion pipeline: Supports customized data ingestion and cleansing workflows, enabling users to flexibly design their data flows or directly apply the official data flow templates on the canvas.
- GraphRAG & RAPTOR write process optimized: Replaces the automatic incremental build process with manual batch building, significantly reducing construction overhead.
- Long-context RAG: Automatically generates document-level table of contents (TOC) structures to mitigate context loss caused by inaccurate or excessive chunking, substantially improving retrieval quality. This feature is now available via a TOC extraction template. See [here](./guides/dataset/extract_table_of_contents.md).
- Video file parsing: Expands the system's multimodal data processing capabilities by supporting video file parsing.
- Admin CLI: Introduces a new command-line tool for system administration, allowing users to manage and monitor RAGFlow's service status via command line.

### Improvements

- Redesigns RAGFlow's Login and Registration pages.
- Upgrades RAGFlow's document engine Infinity to v0.6.0.

### Newly supported models

- Tongyi Qwen 3 series
- Claude Sonnet 4.5
- Meituan LongCat-Flash-Thinking

### New agent templates

- Company Research Report Deep Dive Agent: Designed for financial institutions to help analysts quickly organize information, generate research reports, and make investment decisions.
- Orchestratable Ingestion Pipeline Template: Allows users to apply this template on the canvas to rapidly establish standardized data ingestion and cleansing processes.

## v0.20.5

Released on September 10, 2025.

### Improvements

- Agent:
  - Agent Performance Optimized: Improves planning and reflection speed for simple tasks; optimizes concurrent tool calls for parallelizable scenarios, significantly reducing overall response time.
  - Four framework-level prompt blocks are available in the **System prompt** section, enabling customization and overriding of prompts at the framework level, thereby enhancing flexibility and control. See [here](./guides/agent/agent_component_reference/agent.mdx#system-prompt).
  - **Execute SQL** component enhanced: Replaces the original variable reference component with a text input field, allowing users to write free-form SQL queries and reference variables. See [here](./guides/agent/agent_component_reference/execute_sql.md).
- Chat: Re-enables **Reasoning** and **Cross-language search**.

### Newly supported models

- Meituan LongCat
- Kimi: kimi-k2-turbo-preview and kimi-k2-0905-preview
- Qwen: qwen3-max-preview
- SiliconFlow: DeepSeek V3.1

### Fixed issues

- Dataset: Deleted files remained searchable.
- Chat: Unable to chat with an Ollama model.
- Agent:
  - A **Cite** toggle failure.
  - An Agent in task mode still required a dialogue to trigger.
  - Repeated answers in multi-turn dialogues.
  - Duplicate summarization of parallel execution results.

### API changes

#### HTTP APIs

- Adds a body parameter `"metadata_condition"` to the [Retrieve chunks](./references/http_api_reference.md#retrieve-chunks) method, enabling metadata-based chunk filtering during retrieval. [#9877](https://github.com/infiniflow/ragflow/pull/9877)

#### Python APIs

- Adds a parameter `metadata_condition` to the [Retrieve chunks](./references/python_api_reference.md#retrieve-chunks) method, enabling metadata-based chunk filtering during retrieval. [#9877](https://github.com/infiniflow/ragflow/pull/9877)

## v0.20.4

Released on August 27, 2025.

### Improvements

- Agent component: Completes Chinese localization for the Agent component.
- Introduces the `ENABLE_TIMEOUT_ASSERTION` environment variable to enable or disable timeout assertions for file parsing tasks.
- Dataset:
  - Improves Markdown file parsing, with AST support to avoid unintended chunking.
  - Enhances HTML parsing, supporting bs4-based HTML tag traversal.

### Newly supported models

ZHIPU GLM-4.5

### New Agent templates

Ecommerce Customer Service Workflow: A template designed to handle enquiries about product features and multi-product comparisons using the internal dataset, as well as to manage installation appointment bookings.

### Fixed issues

- Dataset:  
  - Unable to share resources with the team.
  - Inappropriate restrictions on the number and size of uploaded files.
- Chat:
  - Unable to preview referenced files in responses.
  - Unable to send out messages after file uploads.
- An OAuth2 authentication failure.
- A logical error in multi-conditioned metadata searches within a dataset.
- Citations infinitely increased in multi-turn conversations.

## v0.20.3

Released on August 20, 2025.

### Improvements

- Revamps the user interface for the **Datasets**, **Chat**, and **Search** pages.  
- Search and Chat: Introduces document-level metadata filtering, allowing automatic or manual filtering during chats or searches.
- Search: Supports creating search apps tailored to various business scenarios
- Chat: Supports comparing answer performance of up to three chat model settings on a single **Chat** page.
- Agent:  
  - Implements a toggle in the **Agent** component to enable or disable citation.  
  - Introduces a drag-and-drop method for creating components.  
- Documentation: Corrects inaccuracies in the API reference.

### New Agent templates

- Report Agent: A template for generating summary reports in internal question-answering scenarios, supporting the display of tables and formulae.  [#9427](https://github.com/infiniflow/ragflow/pull/9427)

### Fixed issues

- The timeout mechanism introduced in v0.20.0 caused tasks like GraphRAG to halt.
- Predefined opening greeting in the **Agent** component was missing during conversations.  
- An automatic line break issue in the prompt editor.  
- A memory leak issue caused by PyPDF. [#9469](https://github.com/infiniflow/ragflow/pull/9469)

### API changes

#### Deprecated

[Create session with agent](./references/http_api_reference.md#create-session-with-agent)

## v0.20.1

Released on August 8, 2025.

### New Features

- The **Retrieval** component now supports the dynamic specification of dataset names using variables.
- The user interface now includes a French language option.

### Newly supported models

- GPT-5
- Claude 4.1

### New agent templates (both workflow and agentic)

- SQL Assistant Workflow: Empowers non-technical teams (e.g., operations, product) to independently query business data.
- Choose Your Knowledge Base Workflow: Lets users select a dataset to query during conversations. [#9325](https://github.com/infiniflow/ragflow/pull/9325)
- Choose Your Knowledge Base Agent: Delivers higher-quality responses with extended reasoning time, suited for complex queries. [#9325](https://github.com/infiniflow/ragflow/pull/9325)

### Fixed Issues

- The **Agent** component was unable to invoke models installed via vLLM.
- Agents could not be shared with the team.
- Embedding an Agent into a webpage was not functioning properly.

## v0.20.0

Released on August 4, 2025.

### Compatibility changes

From v0.20.0 onwards, Agents are no longer compatible with earlier versions, and all existing Agents from previous versions must be rebuilt following the upgrade.

### New features

- Unified orchestration of both Agents and Workflows.
- A comprehensive refactor of the Agent, greatly enhancing its capabilities and usability, with support for Multi-Agent configurations, planning and reflection, and visual functionalities.
- Fully implemented MCP functionality, allowing for MCP Server import, Agents functioning as MCP Clients, and RAGFlow itself operating as an MCP Server.
- Access to runtime logs for Agents.
- Chat histories with Agents available through the management panel.
- Integration of a new, more robust version of Infinity, enabling the auto-tagging functionality with Infinity as the underlying document engine.
- An OpenAI-compatible API that supports file reference information.
- Support for new models, including Kimi K2, Grok 4, and Voyage embedding.
- RAGFlow’s codebase is now mirrored on Gitee.
- Introduction of a new model provider, Gitee AI.

### New agent templates introduced

- Multi-Agent based Deep Research: Collaborative Agent teamwork led by a Lead Agent with multiple Subagents, distinct from traditional workflow orchestration.
- An intelligent Q&A chatbot leveraging internal datasets, designed for customer service and training scenarios.
- A resume analysis template used by the RAGFlow team to screen, analyze, and record candidate information.
- A blog generation workflow that transforms raw ideas into SEO-friendly blog content.
- An intelligent customer service workflow.
- A user feedback analysis template that directs user feedback to appropriate teams through semantic analysis.
- Trip Planner: Uses web search and map MCP servers to assist with travel planning.
- Image Lingo: Translates content from uploaded photos.
- An information search assistant that retrieves answers from both internal datasets and the web.

## v0.19.1

Released on June 23, 2025.

### Fixed issues

- A memory leak issue during high-concurrency requests.
- Large file parsing freezes when GraphRAG entity resolution is enabled. [#8223](https://github.com/infiniflow/ragflow/pull/8223)
- A context error occurring when using Sandbox in standalone mode. [#8340](https://github.com/infiniflow/ragflow/pull/8340)
- An excessive CPU usage issue caused by Ollama. [#8216](https://github.com/infiniflow/ragflow/pull/8216)
- A bug in the Code Component. [#7949](https://github.com/infiniflow/ragflow/pull/7949)
- Added support for models installed via Ollama or VLLM when creating a dataset through the API. [#8069](https://github.com/infiniflow/ragflow/pull/8069)
- Enabled role-based authentication for S3 bucket access. [#8149](https://github.com/infiniflow/ragflow/pull/8149)

### Newly supported models

- Qwen 3 Embedding. [#8184](https://github.com/infiniflow/ragflow/pull/8184) 
- Voyage Multimodal 3. [#7987](https://github.com/infiniflow/ragflow/pull/7987)

## v0.19.0

Released on May 26, 2025.

### New features

- [Cross-language search](./references/glossary.mdx#cross-language-search) is supported in the Knowledge and Chat modules, enhancing search accuracy and user experience in multilingual environments, such as in Chinese-English datasets.
- Agent component: A new Code component supports Python and JavaScript scripts, enabling developers to handle more complex tasks like dynamic data processing.
- Enhanced image display: Images in Chat and Search now render directly within responses, rather than as external references. Knowledge retrieval testing can retrieve images directly, instead of texts extracted from images.
- Claude 4 and ChatGPT o3: Developers can now use the newly released, most advanced Claude model and OpenAI’s latest ChatGPT o3 inference model.

> The following features have been contributed by our community:

- Agent component: Enables tool calling within the Generate Component. Thanks to [notsyncing](https://github.com/notsyncing).
- Markdown rendering: Image references in a markdown file can be displayed after chunking. Thanks to [Woody-Hu](https://github.com/Woody-Hu).
- Document engine support: OpenSearch can now be used as RAGFlow's document engine. Thanks to [pyyuhao](https://github.com/pyyuhao).

### Documentation

#### Added documents

- [Select PDF parser](./guides/dataset/select_pdf_parser.md)
- [Enable Excel2HTML](./guides/dataset/enable_excel2html.md)
- [Code component](./guides/agent/agent_component_reference/code.mdx)

## v0.18.0

Released on April 23, 2025.

### Compatibility changes

From this release onwards, built-in rerank models have been removed because they have minimal impact on retrieval rates but significantly increase retrieval time.

### New features

- MCP server: enables access to RAGFlow's datasets via MCP.
- DeepDoc supports adopting VLM model as a processing pipeline during document layout recognition, enabling in-depth analysis of images in PDF and DOCX files.
- OpenAI-compatible APIs: Agents can be called via OpenAI-compatible APIs.
- User registration control: administrators can enable or disable user registration through an environment variable.
- Team collaboration: Agents can be shared with team members.
- Agent version control: all updates are continuously logged and can be rolled back to a previous version via export.

![export_agent](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/export_agent_as_json.jpg)

### Improvements

- Enhanced answer referencing: Citation accuracy in generated responses is improved.
- Enhanced question-answering experience: users can now manually stop streaming output during a conversation.

### Documentation

#### Added documents

- [Set page rank](./guides/dataset/set_page_rank.md)
- [Enable RAPTOR](./guides/dataset/enable_raptor.md)
- [Set variables for your chat assistant](./guides/chat/set_chat_variables.md)
- [Launch RAGFlow MCP server](./develop/mcp/launch_mcp_server.md)

## v0.17.2

Released on March 13, 2025.

### Compatibility changes

- Removes the **Max_tokens** setting from **Chat configuration**.
- Removes the **Max_tokens** setting from **Generate**, **Rewrite**, **Categorize**, **Keyword** agent components.

From this release onwards, if you still see RAGFlow's responses being cut short or truncated, check the **Max_tokens** setting of your model provider.

### Improvements

- Adds OpenAI-compatible APIs.
- Introduces a German user interface.
- Accelerates knowledge graph extraction.
- Enables Tavily-based web search in the **Retrieval** agent component.
- Adds Tongyi-Qianwen QwQ models (OpenAI-compatible).
- Supports CSV files in the **General** chunking method.

### Fixed issues

- Unable to add models via Ollama/Xinference, an issue introduced in v0.17.1.

### API changes

#### HTTP APIs

- [Create chat completion](./references/http_api_reference.md#openai-compatible-api)

#### Python APIs

- [Create chat completion](./references/python_api_reference.md#openai-compatible-api)

## v0.17.1

Released on March 11, 2025.

### Improvements

- Improves English tokenization quality.
- Improves the table extraction logic in Markdown document parsing.
- Updates SiliconFlow's model list.
- Supports parsing XLS files (Excel 97-2003) with improved corresponding error handling.
- Supports Huggingface rerank models.
- Enables relative time expressions ("now", "yesterday", "last week", "next year", and more) in chat assistant and the **Rewrite** agent component.

### Fixed issues

- A repetitive knowledge graph extraction issue.
- Issues with API calling.
- Options in the **PDF parser**, aka **Document parser**, dropdown are missing.
- A Tavily web search issue.
- Unable to preview diagrams or images in an AI chat.

### Documentation

#### Added documents

- [Use tag set](./guides/dataset/use_tag_sets.md)

## v0.17.0

Released on March 3, 2025.

### New features

- AI chat: Implements Deep Research for agentic reasoning. To activate this, enable the **Reasoning** toggle under the **Prompt engine** tab of your chat assistant dialogue.
- AI chat: Leverages Tavily-based web search to enhance contexts in agentic reasoning. To activate this, enter the correct Tavily API key under the **Assistant settings** tab of your chat assistant dialogue.
- AI chat: Supports starting a chat without specifying datasets.
- AI chat: HTML files can also be previewed and referenced, in addition to PDF files.
- Dataset: Adds a **PDF parser**, aka **Document parser**, dropdown menu to dataset configurations. This includes a DeepDoc model option, which is time-consuming, a much faster **naive** option (plain text), which skips DLA (Document Layout Analysis), OCR (Optical Character Recognition), and TSR (Table Structure Recognition) tasks, and several currently *experimental* large model options. See [here](./guides/dataset/select_pdf_parser.md).
- Agent component: **(x)** or a forward slash `/` can be used to insert available keys (variables) in the system prompt field of the **Generate** or **Template** component.
- Object storage: Supports using Aliyun OSS (Object Storage Service) as a file storage option.
- Models: Updates the supported model list for Tongyi-Qianwen (Qwen), adding DeepSeek-specific models; adds ModelScope as a model provider.
- APIs: Document metadata can be updated through an API.

The following diagram illustrates the workflow of RAGFlow's Deep Research:

![Image](https://github.com/user-attachments/assets/f65d4759-4f09-4d9d-9549-c0e1fe907525)

The following is a screenshot of a conversation that integrates Deep Research:

![Image](https://github.com/user-attachments/assets/165b88ff-1f5d-4fb8-90e2-c836b25e32e9)

### API changes

#### HTTP APIs

Adds a body parameter `"meta_fields"` to the [Update document](./references/http_api_reference.md#update-document) method.

#### Python APIs

Adds a key option `"meta_fields"` to the [Update document](./references/python_api_reference.md#update-document) method.

### Documentation

#### Added documents

- [Run retrieval test](./guides/dataset/run_retrieval_test.md)

## v0.16.0

Released on February 6, 2025.

### New features

- Supports DeepSeek R1 and DeepSeek V3.
- GraphRAG refactor: Knowledge graph is dynamically built on an entire dataset rather than on an individual file, and automatically updated when a newly uploaded file starts parsing. See [here](https://ragflow.io/docs/dev/construct_knowledge_graph).
- Adds an **Iteration** agent component and a **Research report generator** agent template. See [here](./guides/agent/agent_component_reference/iteration.mdx).
- New UI language: Portuguese.
- Allows setting metadata for a specific file in a dataset to enhance AI-powered chats. See [here](./guides/dataset/set_metadata.md).
- Upgrades RAGFlow's document engine [Infinity](https://github.com/infiniflow/infinity) to v0.6.0.dev3.
- Supports GPU acceleration for DeepDoc (see [docker-compose-gpu.yml](https://github.com/infiniflow/ragflow/blob/main/docker/docker-compose-gpu.yml)).
- Supports creating and referencing a **Tag** dataset as a key milestone towards bridging the semantic gap between query and response.

:::danger IMPORTANT
The **Tag dataset** feature is *unavailable* on the [Infinity](https://github.com/infiniflow/infinity) document engine.
:::

### Documentation

#### Added documents

- [Construct knowledge graph](./guides/dataset/construct_knowledge_graph.md)
- [Set metadata](./guides/dataset/set_metadata.md)
- [Begin component](./guides/agent/agent_component_reference/begin.mdx)
- [Generate component](./guides/agent/agent_component_reference/generate.mdx)
- [Interact component](./guides/agent/agent_component_reference/interact.mdx)
- [Retrieval component](./guides/agent/agent_component_reference/retrieval.mdx)
- [Categorize component](./guides/agent/agent_component_reference/categorize.mdx)
- [Keyword component](./guides/agent/agent_component_reference/keyword.mdx)
- [Message component](./guides/agent/agent_component_reference/message.mdx)
- [Rewrite component](./guides/agent/agent_component_reference/rewrite.mdx)
- [Switch component](./guides/agent/agent_component_reference/switch.mdx)
- [Concentrator component](./guides/agent/agent_component_reference/concentrator.mdx)
- [Template component](./guides/agent/agent_component_reference/template.mdx)
- [Iteration component](./guides/agent/agent_component_reference/iteration.mdx)
- [Note component](./guides/agent/agent_component_reference/note.mdx)

## v0.15.1

Released on December 25, 2024.

### Upgrades

- Upgrades RAGFlow's document engine [Infinity](https://github.com/infiniflow/infinity) to v0.5.2.
- Enhances the log display of document parsing status.

### Fixed issues

This release fixes the following issues:

- The `SCORE not found` and `position_int` errors returned by [Infinity](https://github.com/infiniflow/infinity).
- Once an embedding model in a specific dataset is changed, embedding models in other datasets can no longer be changed.
- Slow response in question-answering and AI search due to repetitive loading of the embedding model.
- Fails to parse documents with RAPTOR.
- Using the **Table** parsing method results in information loss.
- Miscellaneous API issues.

### API changes

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
- Supports using page rank score to improve retrieval performance when searching across multiple datasets.
- Offers an iframe in Chat and Agent to facilitate the integration of RAGFlow into your webpage.
- Adds a Helm chart for deploying RAGFlow on Kubernetes.
- Supports importing or exporting an agent in JSON format.
- Supports step run for Agent components/tools.
- Adds a new UI language: Japanese.
- Supports resuming GraphRAG and RAPTOR from a failure, enhancing task management resilience.
- Adds more Mistral models.
- Adds a dark mode to the UI, allowing users to toggle between light and dark themes.

### Improvements

- Upgrades the Document Layout Analysis model in DeepDoc.
- Significantly enhances the retrieval performance when using [Infinity](https://github.com/infiniflow/infinity) as document engine.

### API changes

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

From this release onwards, **service_config.yaml.template** replaces **service_config.yaml** for configuring backend services. Upon Docker container startup, the environment variables defined in this template file are automatically populated and a **service_config.yaml** is auto-generated from it. [#3341](https://github.com/infiniflow/ragflow/pull/3341)

This approach eliminates the need to manually update **service_config.yaml** after making changes to **.env**, facilitating dynamic environment configurations.

:::danger IMPORTANT
Ensure that you [upgrade **both** your code **and** Docker image to this release](https://ragflow.io/docs/dev/upgrade_ragflow#upgrade-ragflow-to-the-most-recent-officially-published-release) before trying this new approach.
:::

### API changes

#### HTTP APIs

- [Create session with agent](https://ragflow.io/docs/dev/http_api_reference#create-session-with-agent)
- [Converse with agent](https://ragflow.io/docs/dev/http_api_reference#converse-with-agent)

#### Python APIs

- [Create session with agent](https://ragflow.io/docs/dev/python_api_reference#create-session-with-agent)
- [Converse with agent](https://ragflow.io/docs/dev/python_api_reference#create-session-with-agent)

### Documentation

#### Added documents

- [Configurations](https://ragflow.io/docs/dev/configurations)
- [Manage team members](./guides/team/manage_team_members.md)
- [Run health check on RAGFlow's dependencies](https://ragflow.io/docs/dev/run_health_check)

## v0.13.0

Released on October 31, 2024.

### New features

- Adds the team management functionality for all users.
- Updates the Agent UI to improve usability.
- Adds support for Markdown chunking in the **General** chunking method.
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

- [Acquire a RAGFlow API key](./develop/acquire_ragflow_api_key.md)
- [HTTP API Reference](./references/http_api_reference.md)
- [Python API Reference](./references/python_api_reference.md)

## v0.12.0

Released on September 30, 2024.

### New features

- Offers slim editions of RAGFlow's Docker images, which do not include built-in BGE/BCE embedding or reranking models.
- Improves the results of multi-round dialogues.
- Enables users to remove added LLM vendors.
- Adds support for **OpenTTS** and **SparkTTS** models.
- Implements an **Excel to HTML** toggle in the **General** chunking method, allowing users to parse a spreadsheet into either HTML tables or key-value pairs by row.
- Adds agent tools **YahooFinance** and **Jin10**.
- Adds an investment advisor agent template.

### Compatibility changes

From this release onwards, RAGFlow offers slim editions of its Docker images to improve the experience for users with limited Internet access. A slim edition of RAGFlow's Docker image does not include built-in BGE/BCE embedding models and has a size of about 1GB; a full edition of RAGFlow is approximately 9GB and includes two built-in embedding models.

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
- Supports more LLMs or model services: **GPT-4o-mini**, **PerfXCloud**, **TogetherAI**, **Upstage**, **Novita AI**, **01.AI**, **SiliconFlow**, **PPIO**, **XunFei Spark**, **Jiekou.AI**, **Baidu Yiyan**, and **Tencent Hunyuan**.

## v0.9.0

Released on August 6, 2024.

### New features

- Supports GraphRAG as a chunking method.
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
- Supports DOCX files in the MANUAL chunking method.
- Supports DOCX, MD, and PDF files in the Q&A chunking method.

## v0.7.0

Released on May 31, 2024.

### New features

- Supports the use of reranker models.
- Integrates reranker and embedding models: [BCE](https://github.com/netease-youdao/BCEmbedding), [BGE](https://github.com/FlagOpen/FlagEmbedding), and [Jina](https://jina.ai/embeddings/).
- Supports LLMs Baichuan and VolcanoArk.
- Implements [RAPTOR](https://arxiv.org/html/2401.18059v1) for improved text retrieval.
- Supports HTML files in the GENERAL chunking method.
- Provides HTTP and Python APIs for deleting documents by ID.
- Supports ARM64 platforms.

:::danger IMPORTANT
While we also test RAGFlow on ARM64 platforms, we do not maintain RAGFlow Docker images for ARM.

If you are on an ARM platform, follow [this guide](./develop/build_docker_image.mdx) to build a RAGFlow Docker image.
:::

### API changes

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
- Supports disabling **Layout Recognition** in the GENERAL chunking method to reduce file chunking time.

### API changes

#### HTTP API

- [Retrieve chunks](https://ragflow.io/docs/dev/http_api_reference#retrieve-chunks)

#### Python API

- [Retrieve chunks](https://ragflow.io/docs/dev/python_api_reference#retrieve-chunks)

## v0.5.0

Released on May 8, 2024.

### New features

- Supports LLM DeepSeek.
