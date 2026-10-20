---
sidebar_position: 4
title: Connect external MCP tools
sidebar_label: Connect external MCP tools
slug: /connect_external_mcp_servers
sidebar_custom_props: {
  categoryIcon: LucideBookMarked
}
---
# Connect external MCP tools

RAGFlow agents can use an external MCP server alongside their existing tools. This is different from [running RAGFlow's MCP server](./launch_mcp_server.md), which exposes your RAGFlow resources to other clients.

## Add web research with Parallel

[Parallel Search MCP](https://docs.parallel.ai/integrations/mcp/search-mcp) provides `web_search` for finding web sources and `web_fetch` for reading known public URLs. Its anonymous endpoint does not require a Parallel account or API key. Free access is rate limited.

1. Save the [example configuration](https://github.com/infiniflow/ragflow/blob/main/example/mcp/parallel_search.json) as a JSON file:

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

2. Open **Settings → MCP** and import the file. RAGFlow connects to the server and discovers its tools. Use **Streamable HTTP**, not SSE, if entering the connection manually. Leave the Parallel authorization token empty.
3. In an **Agent** component, add the MCP tools from `parallel-search` and enable `web_search` and `web_fetch` as needed. Importing the server does not replace an existing search tool or make Parallel the default.
4. Try a public-web question such as: “Find RAGFlow's current release notes and summarize the latest changes with source links.” The agent can search, then fetch a relevant result. No private dataset is needed for this example.

The `User-Agent` identifies RAGFlow so Parallel can measure aggregate usage by project. Keep it project-wide; do not add a user, installation or device identifier. Imported headers are used during discovery and subsequent tool calls and are retained when exporting the server configuration.

When enabled, the agent may call these tools during its normal workflow. Supplied search queries, requested URLs, objectives/context and request metadata go to Parallel. Review its [privacy policy](https://parallel.ai/privacy-policy) before sending sensitive content. Anonymous Parallel access does not remove authentication required by your RAGFlow installation.

## Check a connection

Confirm that tool discovery returns `web_search` and `web_fetch`. A successful connection alone does not prove that the agent selected a tool; inspect the agent run's tool calls and returned source links.

For connection errors, check the transport, outbound access to `search.parallel.ai`, and the configured headers. The free example deliberately omits authentication headers. If you configured an authenticated connection, investigate its credentials instead of silently removing them. Respect rate-limit responses and avoid repeated immediate retries.

For other MCP servers, use their documented URL, transport and authentication. Custom `headers` must be an object with string names and string values. Never commit real credentials in an exported configuration.
