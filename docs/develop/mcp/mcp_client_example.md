---
sidebar_position: 3
slug: /mcp_client
---

# RAGFlow MCP client example

## Python example for interacting with the RAGFlow MCP server

We provide a *prototype* MCP client example for testing [here](https://github.com/infiniflow/ragflow/blob/main/mcp/client/client.py).

:::danger IMPORTANT
If your MCP server is running in host mode, include your acquired API key in your client's `headers` as shown below:
```python
async with sse_client("http://localhost:9382/sse", headers={"api_key": "YOUR_KEY_HERE"}) as streams:
    # Rest of your code...
```
:::

## curl example for interacting with the RAGFlow MCP server

### MCP initialization process

When interacting with the MCP server over raw HTTP requests, make sure to follow the correct initialization sequence. The connection lifecycle consists of the following steps:

1. **Client sends `initialize` request** with protocol version and capabilities.
2. **Server replies with `initialize` response**, including its supported protocol and capabilities.
3. **Client sends `initialized` notification** to acknowledge readiness.
4. **The connection is ready** and further operations can proceed (e.g., tool listing or invocation).

For more information, see [here](https://modelcontextprotocol.io/docs/concepts/architecture#1-initialization).

### Server-Sent Events

First, to listen for incoming messages in self-host mode (for example) and get the required `session_id`, use:

```bash
$ curl -N -H "api_key: YOUR_API_KEY" http://127.0.0.1:9382/sse

event: endpoint
data: /messages/?session_id=5c6600ef61b845a788ddf30dceb25c54

event: message
data: {"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{"experimental":{"headers":{"host":"127.0.0.1:9382","user-agent":"curl/8.7.1","accept":"*/*","api_key":"ragflow-xxxxxxxxxxxx","accept-encoding":"gzip"}},"tools":{"listChanged":false}},"serverInfo":{"name":"ragflow-server","version":"1.9.4"}}}

: ping - 2025-06-14 08:15:18.217575+00:00

event: message
data: {"jsonrpc":"2.0","id":3,"result":{"tools":[{"name":"ragflow_retrieval","description":"Retrieve relevant chunks from the RAGFlow retrieve interface based on the question, using the specified dataset_ids and optionally document_ids. Below is the list of all available datasets, including their descriptions and IDs. If you're unsure which datasets are relevant to the question, simply pass all dataset IDs to the function.","inputSchema":{"type":"object","properties":{"dataset_ids":{"type":"array","items":{"type":"string"}},"document_ids":{"type":"array","items":{"type":"string"}},"question":{"type":"string"}},"required":["dataset_ids","question"]}}]}}

event: message
data: {"jsonrpc":"2.0","id":4,"result":{...}}

```

This will stream messages such as tool results, server responses, and keepalive pings.

### Example using `curl`

```bash
session_id="YOUR_SESSION_ID" && \

# Step 1: Initialize request
curl -X POST "http://127.0.0.1:9382/messages/?session_id=$session_id" \
  -H "api_key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "1.0",
      "capabilities": {},
      "clientInfo": {
        "name": "ragflow-mcp-client",
        "version": "0.1"
      }
    }
  }' && \

sleep 2 && \

# Step 2: Initialized notification
curl -X POST "http://127.0.0.1:9382/messages/?session_id=$session_id" \
  -H "api_key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "notifications/initialized",
    "params": {}
  }' && \

sleep 2 && \

# Step 3: Tool listing
curl -X POST "http://127.0.0.1:9382/messages/?session_id=$session_id" \
  -H "api_key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/list",
    "params": {}
  }' && \

sleep 2 && \

# Step 4: Tool call
curl -X POST "http://127.0.0.1:9382/messages/?session_id=$session_id" \
  -H "api_key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "ragflow_retrieval",
      "arguments": {
        "question": "How to install neovim?",
        "dataset_ids": ["DATASET_ID_HERE"],
        "document_ids": []
      }
    }
  }'
```
