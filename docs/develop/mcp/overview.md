---
sidebar_position: 1
title: MCP Overview
sidebar_label: Overview
slug: /mcp_overview
sidebar_custom_props:
  categoryIcon: LucideBookOpen
---

# MCP Overview

Model Context Protocol (MCP) lets AI applications discover and call tools exposed by an MCP server. RAGFlow can provide tools to external MCP clients and can connect its Agents to external MCP servers.

## Use RAGFlow as an MCP server

The Go API server exposes RAGFlow tools directly through its services. No separate Python MCP process or RAGFlow base URL is required.

```text
External MCP client
        ↓ MCP request
Go API server: /api/v1/mcp or optional MCP listener
        ↓
RAGFlow services (datasets, chats, retrieval)
```

There are two ways to connect:

| Endpoint | Availability | Transport | Authentication |
| --- | --- | --- | --- |
| `POST /api/v1/mcp` on the Go API port | Available when the Go API runs | Streamable HTTP requests with JSON responses | Each client sends its RAGFlow credential in `Authorization`. |
| `/mcp` on the optional MCP listener (default `127.0.0.1:9382`) | Enable `mcp.enabled` or `RAGFLOW_MCP_ENABLED` | Streamable HTTP; legacy SSE at `/sse` and `/messages/` when enabled | Host mode authenticates each request; self-host mode uses one configured API key for all clients. |

The listener is owned by the Go API process. Its host, port, launch mode, and transports are configurable. See [Use RAGFlow as an MCP Server](./use_ragflow_as_mcp_server.md), [Enable and Connect to the MCP Endpoint](./launch_mcp_server.md), [RAGFlow MCP Tools](./mcp_tools.md), and [RAGFlow MCP Client Examples](./mcp_client_example.md).

## Connect an external MCP server to RAGFlow

RAGFlow can also connect to third-party or self-hosted MCP servers. Their tools become available to RAGFlow Agents.

```text
External MCP server → RAGFlow Agent → tool invocation
```

See [Connect an External MCP Server to RAGFlow](./connect_an_external_mcp_to_ragflow.md).

## Choose a scenario

Use RAGFlow as the server when an external application needs RAGFlow's retrieval, dataset, or chat-listing tools. Connect an external server when a RAGFlow Agent needs tools supplied by another system.
