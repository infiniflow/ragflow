---
sidebar_position: 1
title: MCP Overview
sidebar_label: Overview
slug: /mcp_overview
sidebar_custom_props:
  categoryIcon: LucideBookOpen
---

# MCP Overview

Model Context Protocol (MCP) lets AI applications discover and call tools exposed by an MCP server. RAGFlow supports both sides of this integration:

| Goal | RAGFlow's role | Start here |
| --- | --- | --- |
| Call RAGFlow retrieval, dataset, and chat-listing tools from another application | MCP server | [Use RAGFlow as an MCP Server](./use_ragflow_as_mcp_server.md) |
| Use tools from another system in a RAGFlow Agent | MCP client | [Connect an External MCP Server to RAGFlow](./connect_an_external_mcp_to_ragflow.md) |

## Use RAGFlow as an MCP server

The Go API server exposes RAGFlow tools directly through its services. No separate Python MCP process or RAGFlow base URL is required.

```text
External MCP client
        ↓ MCP request
Go API server: /api/v1/mcp or optional MCP listener
        ↓
RAGFlow services (datasets, chats, retrieval)
```

Use the built-in `POST /api/v1/mcp` route, or enable the optional listener when you need a separate `/mcp` endpoint or legacy SSE. Both are owned by the Go API process and call RAGFlow services directly.

See [Use RAGFlow as an MCP Server](./use_ragflow_as_mcp_server.md) for endpoints and authentication, [RAGFlow MCP Tools](./mcp_tools.md) for the registered tools, and [RAGFlow MCP Client Examples](./mcp_client_example.md) for requests.

## Connect an external MCP server to RAGFlow

RAGFlow can also connect to third-party or self-hosted MCP servers. Their tools become available to RAGFlow Agents.

```text
External MCP server → RAGFlow Agent → tool invocation
```

See [Connect an External MCP Server to RAGFlow](./connect_an_external_mcp_to_ragflow.md) for supported transports, connection requirements, and an example.
