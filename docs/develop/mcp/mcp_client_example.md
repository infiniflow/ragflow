---
sidebar_position: 13
title: RAGFlow MCP Client Examples
sidebar_label: RAGFlow MCP Client Examples
slug: /mcp_client
sidebar_custom_props: {
  categoryIcon: LucideBookMarked
}
---

# RAGFlow MCP Client Examples

These clients call the MCP implementation in RAGFlow's Go API server. The Python code below is a client example; it does not start a Python MCP server. To use external MCP tools inside a RAGFlow Agent, see [Connect external MCP tools](./connect_external_mcp_servers.md).

## Python client: Streamable HTTP

Install the Python MCP client SDK in your client environment, then connect to either the Go API route or the optional listener. This example uses the optional listener in host mode:

```python
import asyncio

from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client


async def main():
    async with streamablehttp_client(
        "http://127.0.0.1:9382/mcp",
        headers={"Authorization": "Bearer <RAGFLOW_API_KEY>"},
    ) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            tools = await session.list_tools()
            print([tool.name for tool in tools.tools])


asyncio.run(main())
```

To use the API route, change the URL to the Go API address ending in `/api/v1/mcp` and keep the `Authorization` header. The optional listener must be enabled before connecting to port `9382`. A legacy SSE client can use that listener's `/sse` URL when SSE is enabled.

## curl: Streamable HTTP

Send each request to the same endpoint. The optional listener is stateless for Streamable HTTP, so no SSE session ID is needed. For the API route, replace the URL with your Go API address ending in `/api/v1/mcp`.

Initialize:

```bash
curl -sS http://127.0.0.1:9382/mcp \
  -H 'Authorization: Bearer <RAGFLOW_API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
```

List tools:

```bash
curl -sS http://127.0.0.1:9382/mcp \
  -H 'Authorization: Bearer <RAGFLOW_API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

The list contains `ragflow_retrieval`, `ragflow_list_datasets`, and `ragflow_list_chats`. A full MCP client sends `notifications/initialized` after initialization. The Go Streamable HTTP listener is stateless, so these independent curl requests do not reuse a session.

Call a tool:

```bash
curl -sS http://127.0.0.1:9382/mcp \
  -H 'Authorization: Bearer <RAGFLOW_API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ragflow_retrieval","arguments":{"question":"How do I install Neovim?"}}}'
```

To limit retrieval to particular datasets, add `dataset_ids` to `arguments`. See [RAGFlow MCP Tools](./mcp_tools.md) for all supported arguments.
