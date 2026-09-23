---
sidebar_position: 5
title: RAGFlow MCP Client Examples
sidebar_label: RAGFlow MCP Client Examples
slug: /mcp_client
sidebar_custom_props: {
  categoryIcon: LucideBookMarked
}
---

# RAGFlow MCP Client Examples

These clients call the MCP implementation in RAGFlow's Go API server. The Python code is a client; it does not start a Python MCP server. To use tools from an external MCP server inside a RAGFlow Agent, see [Connect an External MCP Server to RAGFlow](./connect_an_external_mcp_to_ragflow.md).

The examples use the built-in Go API route at `http://127.0.0.1:9384/api/v1/mcp`. Replace the host or port if your API is exposed through a reverse proxy. [Acquire a RAGFlow API key](../acquire_ragflow_api_key.md) and send it as `Authorization: Bearer <RAGFLOW_API_KEY>`.

## Python client

This example targets version 2 of the MCP Python SDK, which requires Python 3.10 or later:

```bash
python -m pip install 'mcp>=2,<3'
```

The current SDK uses `streamable_http_client`. HTTP headers belong on an `httpx2.AsyncClient` passed to that transport:

```python
import asyncio

import httpx2
from mcp import Client
from mcp.client.streamable_http import streamable_http_client


async def main() -> None:
    async with httpx2.AsyncClient(
        headers={"Authorization": "Bearer <RAGFLOW_API_KEY>"},
        timeout=httpx2.Timeout(30.0, read=300.0),
    ) as http_client:
        transport = streamable_http_client(
            "http://127.0.0.1:9384/api/v1/mcp",
            http_client=http_client,
        )
        async with Client(transport) as client:
            tools = await client.list_tools()
            print([tool.name for tool in tools.tools])

            result = await client.call_tool(
                "ragflow_retrieval",
                {"question": "How do I install Neovim?"},
            )
            print(result.content)


asyncio.run(main())
```

Entering the `Client` context negotiates the supported MCP protocol automatically. The Go server currently supports `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05`.

## curl smoke checks

The Go Streamable HTTP handler is stateless: each POST uses a temporary MCP session with default initialization parameters. The following commands are therefore **independent smoke checks**. The initialization request demonstrates protocol negotiation; it does not create a reusable session for the later commands.

### Initialize

```bash
curl -sS http://127.0.0.1:9384/api/v1/mcp \
  -H 'Authorization: Bearer <RAGFLOW_API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
```

A successful response contains `result.serverInfo.name` set to `ragflow-mcp-server` and a negotiated `protocolVersion`.

### List tools

```bash
curl -sS http://127.0.0.1:9384/api/v1/mcp \
  -H 'Authorization: Bearer <RAGFLOW_API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

The response lists `ragflow_retrieval`, `ragflow_list_datasets`, and `ragflow_list_chats`. A normal MCP SDK client performs initialization and sends the initialized notification automatically; raw stateless smoke checks do not need to reuse that state.

### Call a tool

```bash
curl -sS http://127.0.0.1:9384/api/v1/mcp \
  -H 'Authorization: Bearer <RAGFLOW_API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ragflow_retrieval","arguments":{"question":"How do I install Neovim?"}}}'
```

The MCP result contains a text content item whose text is the JSON retrieval result. Add `dataset_ids` or other supported arguments under `arguments` when needed. See [RAGFlow MCP Tools](./mcp_tools.md) for the complete schemas.

## Legacy SSE clients

The built-in `/api/v1/mcp` route does not provide SSE. When SSE is enabled on the optional listener, legacy clients connect to `http://127.0.0.1:9382/sse`; the server provides the corresponding `/messages/` URL. Use an MCP SDK to manage the SSE session and include the same authentication header on both requests.
