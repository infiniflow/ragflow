---
sidebar_position: 2
slug: /release_notes
---

# Release notes

Key features and improvements in the latest releases.

## v0.14.0

Released on November 26, 2014.

### New features

- Supports [Infinity](https://github.com/infiniflow/infinity) or Elasticsearch (default) as document engine for vector storage and full-text indexing. 
- Enhances user experience by adding more variables to the Agent and implementing auto-saving.
- Adds a three-step translation agent template, inspired by [Andrew Ng's translation agent](https://github.com/andrewyng/translation-agent).
- Adds an SEO-optimized blog writing agent template.
- Provides HTTP and Python APIs for conversing with an agent.
- Supports the use of English synonyms during retrieval processes.
- Optimizes term weight calculations, reducing the retrieval time by 50%.
- Improves task executor monitoring with additional performance indicators.
- Replaces Redis with Valkey.
- Adds three new UI languages (contributed by community): Indonesian, Spanish, and Vietnamese.

### Related APIs

#### HTTP APIs

- [Create session with agent](https://ragflow.io/docs/dev/http_api_reference#create-session-with-an-agent)
- [Converse with agent](https://ragflow.io/docs/dev/http_api_reference#converse-with-agent)

#### Python APIs

- [Create session with agent](https://ragflow.io/docs/dev/python_api_reference#create-session-with-agent)
- [Converse with agent](https://ragflow.io/docs/dev/python_api_reference#create-session-with-agent)

### Statistics

| Number of merged pull requests | Number of new contributors |
| ------------------------------ | -------------------------- |
| 197                            | 18                         |

