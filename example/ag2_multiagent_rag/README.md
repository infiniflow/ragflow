# AG2 Multi-Agent RAG with RAGFlow

This example demonstrates how to use [AG2](https://ag2.ai/) (formerly AutoGen)
multi-agent conversations with RAGFlow as the knowledge retrieval backend.

AG2 is a multi-agent conversation framework with 500K+ monthly PyPI downloads,
4,300+ GitHub stars, and 400+ contributors.

## Architecture

```
User Query --> AG2 UserProxy --> GroupChat [Research Assistant + Analyst]
                                       |
                                 RAGFlow SDK (dataset retrieval)
                                       |
                                 Grounded, citation-backed response
```

Two AG2 agents collaborate:
- **Research Assistant**: Analyzes queries and retrieves relevant documents via RAGFlow
- **Analyst**: Synthesizes retrieved information into comprehensive answers

## Prerequisites

1. A running RAGFlow instance (local or cloud: https://cloud.ragflow.io)
2. A RAGFlow API key (in RAGFlow UI: **Settings > API > API KEY > Create new key**)
3. At least one dataset with uploaded and parsed documents in RAGFlow
4. An OpenAI API key (for AG2 agents)

## Installation

```bash
pip install "ag2[openai]>=0.11.4,<1.0" ragflow-sdk
```

## Configuration

Set the following environment variables:

```bash
export OPENAI_API_KEY="your-openai-api-key"
export RAGFLOW_API_KEY="your-ragflow-api-key"
export RAGFLOW_BASE_URL="http://localhost:9380"  # or https://cloud.ragflow.io
```

## Usage

```bash
python run.py
```

## How It Works

1. The user sends a question to the AG2 UserProxy agent
2. The Research Assistant agent calls the `search_ragflow` tool to query RAGFlow datasets
3. RAGFlow returns relevant document chunks with similarity scores and citations
4. The Analyst agent synthesizes the retrieved information into a final answer
5. The response includes source citations from RAGFlow
