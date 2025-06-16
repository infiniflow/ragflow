---
sidebar_position: 3
slug: /mcp_client
---

# RAGFlow MCP client example

We provide a *prototype* MCP client example for testing [here](https://github.com/infiniflow/ragflow/blob/main/mcp/client/client.py).

:::danger IMPORTANT
If your MCP server is running in host mode, include your acquired API key in your client's `headers` as shown below:
```python
async with sse_client("http://localhost:9382/sse", headers={"api_key": "YOUR_KEY_HERE"}) as streams:
    # Rest of your code...
```

Or follow the requirements of [OAuth 2.1 Section 5](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-12#section-5) by providing an Authorization request headers field:
```python
async with sse_client("http://localhost:9382/sse", headers={"Authorization": "YOUR_KEY_HERE"}) as streams:
    # Rest of your code...
```
:::
