---
sidebar_position: 3
title: Connect an External MCP Server to RAGFlow
sidebar_label: Connect an External MCP Server to RAGFlow
slug: /connect_external_mcp_server
sidebar_custom_props:
  categoryIcon: LucideBookOpen
---

# Connect an External MCP Server to RAGFlow

Connect an external MCP server to RAGFlow to make its tools available to RAGFlow Agents.

RAGFlow supports MCP servers using **Streamable HTTP** or **SSE**. After an MCP server is added successfully, RAGFlow discovers the tools exposed by the server and makes them available for Agent workflows.

## Prepare the MCP server

Before adding an MCP server to RAGFlow, make sure that the server is running and identify the transport it provides.

RAGFlow supports the following MCP server types:

* **Streamable HTTP**, for example:

  ```text
  https://example.com/mcp
  ```

* **SSE**, for example:

  ```text
  https://example.com/sse
  ```

Streamable HTTP is recommended when supported by the MCP server. SSE is retained for compatibility with MCP servers that use the older transport.

RAGFlow does not directly connect to MCP servers that use `stdio`. To use a `stdio`-based MCP server, expose it through a gateway or another service that provides an SSE or Streamable HTTP endpoint.

## Add an MCP server to RAGFlow

1. Log in to RAGFlow.
2. Click your profile picture in the upper-right corner and open **User settings**.
3. Open **MCP**.
4. Click **Add MCP**.
5. Configure the MCP server.
6. Click the refresh button in **Tools available** to test the connection and discover tools.
7. After the test succeeds and the discovered tools appear, click **Save**.

| Field | Description | Example |
| --- | --- | --- |
| Name | A name containing letters, numbers, underscores, or hyphens (up to 64 characters) | `web_search_tools` |
| URL | The complete MCP endpoint | `https://example.com/mcp` |
| Server type | The transport provided by the MCP server | `streamable-http` |
| Authorization Token | Optional token sent as `Authorization: Bearer <token>` | The server's token |

The **Server type** must match the transport exposed by the MCP server:

```text
streamable-http
```

or:

```text
sse
```

If the MCP server does not require authentication, leave **Authorization Token** empty. The manual form does not expose arbitrary headers, variables, or a description field. For another authentication scheme or custom headers, use a JSON import with a string-to-string `headers` object, as shown in the example below.

Make sure that the path in **URL** matches the endpoint actually exposed by the MCP server. For example, a Streamable HTTP server commonly uses `/mcp`, while an SSE server commonly uses `/sse`.

RAGFlow keeps **Save** disabled until the current connection settings have been tested successfully and at least one tool has been discovered. If you change the URL, server type, or authorization token, test the connection again before saving.

## Connection requirements

RAGFlow validates the endpoint before connecting:

- The URL must use `http://` or `https://` and contain a host. `stdio://` is not supported.
- Every address returned by DNS must be publicly routable. Loopback, private, link-local, reserved, Docker-network, and other non-public addresses are rejected.
- The Go MCP client connects to the validated address directly and does not use HTTP proxy environment variables.

Consequently, endpoints such as `localhost`, `127.0.0.1`, `::1`, `10.x.x.x`, `172.16.x.x` through `172.31.x.x`, and `192.168.x.x` cannot be added through this path. `ALLOW_ANY_HOST` does not disable this validation. Publish the MCP endpoint on a public address whose DNS records all resolve to public addresses.

## Troubleshoot a failed connection

| Symptom | What to check |
| --- | --- |
| `Invalid MCP url` or a disallowed-scheme error | Use a complete `http://` or `https://` endpoint with the correct `/mcp` or `/sse` path. |
| `URL resolves to a non-public address` | Check DNS from the RAGFlow backend environment. Public hostnames are also rejected if any result is private or synthetic. Proxy tools in Fake-IP mode can cause this result. |
| Connection or discovery timeout | Check outbound network access, the selected transport, endpoint path, and server availability. The default discovery timeout is 10 seconds. |
| Authentication failure | Verify the token, or import string-valued custom headers when the server does not use a Bearer token. |
| Save remains disabled | Test the current settings again and confirm that the server advertises at least one tool. |

## Example: add Parallel Search MCP tools

[Parallel Search MCP](https://docs.parallel.ai/integrations/mcp/search-mcp) provides `web_search` and `web_fetch` tools. Save the repository's [example configuration](https://github.com/infiniflow/ragflow/blob/main/example/mcp/parallel_search.json) as a JSON file:

```json
{
  "mcpServers": {
    "parallel-search": {
      "type": "streamable-http",
      "url": "https://search.parallel.ai/mcp",
      "headers": {
        "User-Agent": "ragflow"
      }
    }
  }
}
```

Open **User settings → MCP** and import the file. RAGFlow discovers the server's tools during import. The example has no authorization token. If you enter the connection manually, select **Streamable HTTP** and leave **Authorization Token** empty.

In an **Agent** component, add the imported tools from `parallel-search` and enable `web_search` or `web_fetch` as needed. Importing a server does not automatically make its tools the agent's default search tool.

Check that discovery returns both tool names. Then inspect an agent run's tool calls to confirm it selected one. If connection fails, check the transport, outbound access to `search.parallel.ai`, and configured headers. Queries, requested URLs, and request context sent through these tools reach Parallel; review its [privacy policy](https://parallel.ai/privacy-policy) before sending sensitive content.

For other MCP servers, use their documented URL, transport, and authentication. Imported `headers` must be an object with string names and string values. Keep real credentials out of committed configuration files.
