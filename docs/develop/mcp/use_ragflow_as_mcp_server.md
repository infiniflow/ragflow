---
sidebar_position: 2
title: Use RAGFlow as an MCP Server
sidebar_label: Use RAGFlow as an MCP Server
slug: /use_ragflow_as_mcp_server
sidebar_custom_props:
  categoryIcon: LucideBookOpen
---

# Use RAGFlow as an MCP Server

RAGFlow can act as an MCP server and expose its capabilities to external MCP clients.

The Python-based RAGFlow MCP server runs as a standalone component alongside RAGFlow. It communicates with the RAGFlow server through the RAGFlow API and exposes RAGFlow capabilities through the Model Context Protocol (MCP).

To start and configure the server, see [Launch RAGFlow MCP Server](./launch_mcp_server.md).

For details about the MCP tools exposed by RAGFlow, see [RAGFlow MCP Tools](./mcp_tools.md).

For examples of calling the RAGFlow MCP server from an MCP client, see [RAGFlow MCP Client Examples](./mcp_client_example.md).

## How it works

The Python MCP server runs independently from the RAGFlow server.

A typical request flow is:

```text
External MCP Client
        ↓
RAGFlow MCP Server
        ↓
RAGFlow API
        ↓
RAGFlow capabilities
```

The MCP server receives requests from external MCP clients and forwards the corresponding operations to RAGFlow through its API.

By default, the MCP server listens on port `9382`.

## Prerequisites

Before starting the RAGFlow MCP server, ensure that:

* RAGFlow is deployed and running normally.
* The RAGFlow API is accessible from the MCP server.
* A valid RAGFlow user has been created.
* A RAGFlow API key is available if you use Self-host mode.
* The MCP client can access the MCP server.
* The required Python dependencies are installed if you start the MCP server from source.

## Launch modes

The RAGFlow MCP server supports two launch modes:

* **Self-host mode**
* **Host mode**

The main difference is how RAGFlow user authentication is handled.

### Self-host mode

In Self-host mode, the RAGFlow API key is specified when the MCP server starts.

The MCP server uses this API key for requests sent to RAGFlow. All MCP clients connected to this server therefore access RAGFlow using the same RAGFlow user identity.

The request flow is:

```text
External MCP Client
        ↓
RAGFlow MCP Server
        ↓
Configured RAGFlow API Key
        ↓
RAGFlow Server
```

Self-host mode is suitable when the MCP server is dedicated to a specific RAGFlow user or trusted environment.

### Host mode

In Host mode, the MCP server is not bound to a fixed RAGFlow user when it starts.

Instead, each MCP client provides its own authentication information. RAGFlow determines the user and resource permissions based on the credentials provided by that client.

The request flow is:

```text
External MCP Client
        ↓
Client authentication
        ↓
RAGFlow MCP Server
        ↓
RAGFlow Server
```

Host mode is more suitable when a shared MCP server needs to serve multiple RAGFlow users.

## Transport protocols

The Python MCP server supports the following transports:

* **Streamable HTTP**
* **SSE**

### Streamable HTTP

Streamable HTTP uses the following endpoint:

```text
/mcp
```

For example:

```text
http://127.0.0.1:9382/mcp
```

Streamable HTTP should be preferred for MCP clients that support it.

### SSE

The MCP server also supports the legacy SSE transport.

The SSE endpoint is:

```text
/sse
```

Messages are sent through:

```text
/messages/
```

For example:

```text
http://127.0.0.1:9382/sse
```

SSE is retained for compatibility with MCP clients that still use the legacy MCP transport.

> When using Host mode, check the transport requirements of
