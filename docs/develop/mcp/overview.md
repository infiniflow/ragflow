---
sidebar_position: 1
title: MCP Overview
sidebar_label: Overview
slug: /mcp_overview
sidebar_custom_props:
  categoryIcon: LucideBookOpen
---

# MCP Overview

## What is MCP?

Model Context Protocol (MCP) is an open protocol for connecting AI applications with external tools, data sources, and services.

Through MCP, an MCP client can discover and invoke tools provided by an MCP server. RAGFlow can either act as an MCP server to expose its capabilities to external clients or connect to external MCP servers to extend the tools available to RAGFlow Agents.

## MCP usage scenarios in RAGFlow

RAGFlow currently supports the following two MCP usage scenarios.

### Use RAGFlow as an MCP server

RAGFlow can act as an MCP server and expose capabilities such as knowledge base retrieval to external MCP clients.

The RAGFlow MCP server runs as a standalone component alongside a properly functioning RAGFlow server.

The MCP server supports two launch modes:

* **Self-host mode**: Provide a RAGFlow API key when starting the MCP server. The MCP server uses this API key to access RAGFlow resources available to the corresponding user.
* **Host mode**: The MCP server is not bound to a fixed user. Each MCP client provides its own authentication information in requests and can access only the RAGFlow resources available to that user.

The RAGFlow MCP server supports the following transports:

* **Streamable HTTP**: Communicates through the `/mcp` endpoint.
* **SSE**: Communicates through the `/sse` and `/messages/` endpoints and is retained for compatibility with legacy MCP clients.

A typical request flow is:

```text
External MCP Client
        ↓
RAGFlow MCP Server
        ↓
RAGFlow Server
        ↓
RAGFlow capabilities
```

For information about starting and configuring the RAGFlow MCP server, see **Launch RAGFlow MCP Server**.

For details about the MCP tools exposed by RAGFlow and their parameters, see **RAGFlow MCP Tools**.

For examples of calling the RAGFlow MCP server with Python or curl, see **RAGFlow MCP Client Examples**.

### Connect an external MCP server to RAGFlow

RAGFlow can also connect to third-party or self-hosted MCP servers and discover the MCP tools they provide.

Once configured, these tools can be used by RAGFlow Agents, allowing agents to interact with external systems and services.

A typical request flow is:

```text
External MCP Server
        ↓
      RAGFlow
        ↓
       Agent
```

For information about configuring, testing, and using external MCP servers, see **Connect an External MCP Server to RAGFlow**.

## Choose the right MCP scenario

Choose an MCP usage scenario based on what y
