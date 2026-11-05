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

| Field               | Description                                                                       | Example                   |
|---------------------|-----------------------------------------------------------------------------------|---------------------------|
| Name                | A name containing letters, numbers, underscores, or hyphens (up to 64 characters) | `web_search_tools`        |
| URL                 | The complete MCP endpoint                                                         | `https://example.com/mcp` |
| Server type         | The transport provided by the MCP server                                          | `streamable-http`         |
| Authorization Token | Optional token sent as `Authorization: Bearer <token>`                            | The server's token        |

The **Server type** must match the transport exposed by the MCP server:

```text
streamable-http
```

or:

```text
sse
```

If the MCP server does not require authentication, leave **Authorization Token** empty. The manual form does not expose arbitrary headers, variables, or a description field. For another authentication scheme or custom headers, use a JSON import with a string-to-string `headers` object, as shown in the example below.

When the server supports bearer authentication, keep the URL free of credentials and put the key in **Authorization Token**. RAGFlow's own MCP endpoint uses this header; query-string keys do not authenticate it. Existing third-party query-authentication URLs remain supported because their authentication requirements depend on the server. Ordinary MCP configuration responses mask the authorization token, and unchanged edits and connection tests reuse the saved value server-side. Explicit configuration downloads include credentials so they can be imported again.

Make sure that the path in **URL** matches the endpoint actually exposed by the MCP server. For example, a Streamable HTTP server commonly uses `/mcp`, while an SSE server commonly uses `/sse`.

RAGFlow keeps **Save** disabled until the current connection settings have been tested successfully and at least one tool has been discovered. If you change the URL, server type, or authorization token, test the connection again before saving.

## Connection requirements

RAGFlow validates the endpoint before connecting:

- The URL must use `http://` or `https://` and contain a host. `stdio://` is not supported.
- By default, every address returned by DNS must be publicly routable. Loopback, private, link-local, reserved, Docker-network, and other non-public addresses are rejected.
- The Go MCP client connects to the validated address directly and does not use HTTP proxy environment variables.

To connect to self-hosted MCP endpoints on private or Docker networks, set `ALLOW_ANY_HOST=true` in the RAGFlow backend environment and restart the backend. This shared setting also permits private database and connector hosts. Accepted true values are `1`, `true`, `yes`, and `on` (case-insensitive, with surrounding whitespace ignored). HTTP/HTTPS scheme validation, DNS resolution, and connection pinning remain enabled.

## Troubleshoot a failed connection

| Symptom                                        | What to check                                                                                                                                                                |
|------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Invalid MCP url` or a disallowed-scheme error | Use a complete `http://` or `https://` endpoint with the correct `/mcp` or `/sse` path.                                                                                      |
| `URL resolves to a non-public address`         | Check DNS from the RAGFlow backend environment. Public hostnames are also rejected if any result is private or synthetic. Proxy tools in Fake-IP mode can cause this result. |
| Connection or discovery timeout                | Check outbound network access, the selected transport, endpoint path, and server availability. The default discovery timeout is 10 seconds.                                  |
| Authentication failure                         | Verify the token, or import string-valued custom headers when the server does not use a Bearer token.                                                                        |
| Save remains disabled                          | Test the current settings again and confirm that the server advertises at least one tool.                                                                                    |

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

## Example: connect Baizhi Cloud Browser

[Baizhi Cloud Browser](https://baizhibrowser.cloud/) (also known as CapyBrowser) hosts browser runs and persistent browser profiles. Its MCP endpoint checks credentials and prices, and manages runs, profiles, and control handover. It does not provide page navigation or clicking through MCP alone; page operations also require a compatible CDP client. Use this connection when a RAGFlow workflow needs to manage cloud-browser runs or profiles. The first check below does not create a run.

Use a key authorized for Baizhi Cloud Browser from the [service console](https://capybrowser.app.baizhi.cloud/). A RAGFlow API key is not a substitute. In the manual form, enter:

| Field               | Value                                          |
| ------------------- | ---------------------------------------------- |
| Name                | `baizhi-cloud-browser`                         |
| URL                 | `https://capybrowser.app.baizhi.cloud/mcp`     |
| Server type         | `streamable-http`                              |
| Authorization Token | Your raw API key, without the `Bearer ` prefix |

Refresh **Tools available**, check the advertised tools, then save. Alternatively, save the repository's [example configuration](https://github.com/infiniflow/ragflow/blob/main/example/mcp/baizhi_cloud_browser.json), or copy the JSON below to a local file. Replace `REPLACE_WITH_YOUR_CAPY_API_KEY` with your key and import it under **User settings → MCP**. Here the header value includes exactly one `Bearer ` prefix:

```json
{
  "mcpServers": {
    "baizhi-cloud-browser": {
      "type": "streamable-http",
      "url": "https://capybrowser.app.baizhi.cloud/mcp",
      "headers": {
        "Authorization": "Bearer REPLACE_WITH_YOUR_CAPY_API_KEY"
      }
    }
  }
}
```

The placeholder is literal text, not an environment-variable lookup. RAGFlow stores the connection on its backend. Ordinary responses mask credential fields, but configuration downloads include them. Keep filled files private, and grant access to the saved MCP resource only to people permitted to use the connected Baizhi account and its resources.

For a first check, add only the discovered `browser_auth_check` tool to an **Agent** component and ask:

> Call `browser_auth_check` once. Report whether authentication succeeded. Do not create a browser run or profile, request control, or print credentials or account identifiers.

Inspect the actual tool call and result. Successful discovery alone does not prove a tool was called; inspect the returned authentication result and any error. Authentication checks do not create a browser run or incur browser runtime charges. The key's account/team metadata may be absent on some deployments, so missing fields do not prove account ownership. If authentication fails, verify the endpoint and token; do not put the key in the URL or paste it into chat.

Before using other tools, review the service's [current connection and lifecycle guide](https://capybrowser.app.baizhi.cloud/docs/installation.md) and pricing. Browser runs can incur charges. Disconnecting MCP, removing the tool from an agent, or deleting this connection does not stop an existing run: explicitly stop the original run and inspect its terminal and billing state. A released run and settled billing are separate results.

To remove the integration, first finish any existing runs, remove its tools from agents, and delete its MCP entry in **User settings → MCP**. Revoke the service key separately if it is no longer needed. Calls and supplied task context are sent to Baizhi Cloud Browser. Public integration references are in the [Chaitin integration repository](https://github.com/chaitin/baizhi-cloud-browser). `ctcbyzx` maintains this connection example and has contributed to that repository; the [personal integration repository](https://github.com/ctcbyzx/baizhi-cloud-browser) and its [Issues](https://github.com/ctcbyzx/baizhi-cloud-browser/issues) provide maintenance and support for this connection.
