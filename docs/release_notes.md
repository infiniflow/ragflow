---
sidebar_position: 2
slug: /release_notes
---

# Release notes

Key features and improvements in the latest releases.

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
- Adds support for OpenTTS and SparkTTS models.
- Implements an **Excel to HTML** toggle in the **General** chunk method, allowing users to parse an spreadsheet into either an HTML table or key-value pairs by row.
- Adds agent tools **YahooFance** and **Jin10**.
- Adds a template for an investment advisor agent.

### Compatibility changes

As of this release, RAGFlow offers slim editions of its Docker images to improve the experience for users with limited Internet access. A slim edition of RAGFlow's Docker image does not include built-in BGE/BCE embedding models and has a size of about 1GB; a full edition of RAGFlow is approximately 9GB and includes both built-in embedding models and embedding models that will be downloaded once you select them in the RAGFlow UI.

The default Docker image edition is `dev-slim`. The following list clarifies the differences:

- `dev-slim`: The slim edition of the most recent tested Docker image.
- `v0.14.0-slim`: The slim edition of the most recent **officially** released Docker image.
- `dev`: The full edition of the most recent tested Docker image.
- `v0.14.0`: The full edition of the most recent **officially** released Docker image.

See [Upgrade RAGFlow](https://ragflow.io/docs/dev/upgrade_ragflow) for instructions on upgrading.

### Documentation

#### Added documents

- [Upgrade RAGFlow](https://ragflow.io/docs/dev/upgrade_ragflow)