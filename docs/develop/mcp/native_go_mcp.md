---
sidebar_position: 4
title: Native Go MCP Runtime
sidebar_label: Native Go MCP Runtime
slug: /native_go_mcp
sidebar_custom_props: {
  categoryIcon: SiModelcontextprotocol
}
---
# Native Go MCP Runtime

The Go API server can host RAGFlow's MCP runtime in-process. It does not launch `mcp/server/server.py` and does not need a RAGFlow base URL because MCP tool calls use Go services directly.

## Docker

Use the Go entrypoint with MCP enabled:

```bash
./entrypoint-go.sh --enable-mcpserver \
  --mcp-host=0.0.0.0 \
  --mcp-port=9382 \
  --mcp-mode=self-host \
  --mcp-host-api-key=ragflow-xxxxx
```

Host mode requires each client request to provide a credential:

```bash
./entrypoint-go.sh --enable-mcpserver --mcp-mode=host
```

Host-mode requests accept `Authorization: Bearer <key>`, `api_key`, `X-API-Key`, or `Api-Key`.

## Transport flags

Both legacy SSE (`/sse` and `/messages/`) and streamable HTTP (`/mcp`) are enabled by default. JSON responses for streamable HTTP are also enabled by default.

```bash
--no-transport-sse-enabled              # disable legacy SSE
--no-transport-streamable-http-enabled  # disable streamable HTTP
--no-json-response                      # use SSE-style streamable HTTP responses
```

If both transports are disabled, streamable HTTP is re-enabled and JSON responses stay disabled, matching the Python MCP server's flag resolution.

The same settings can be supplied through environment variables. Environment values override CLI flags:

```bash
RAGFLOW_MCP_ENABLED=true
RAGFLOW_MCP_HOST=0.0.0.0
RAGFLOW_MCP_PORT=9382
RAGFLOW_MCP_LAUNCH_MODE=self-host
RAGFLOW_MCP_HOST_API_KEY=ragflow-xxxxx
RAGFLOW_MCP_TRANSPORT_SSE_ENABLED=true
RAGFLOW_MCP_TRANSPORT_STREAMABLE_ENABLED=true
RAGFLOW_MCP_JSON_RESPONSE=true
```

## Streamable HTTP smoke example

```bash
curl -sS http://127.0.0.1:9382/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Authorization: Bearer ragflow-xxxxx' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
```

Then send the `notifications/initialized` notification before `tools/list` or `tools/call` when using a real MCP client.
