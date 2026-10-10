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

## Configuration

The Go API process owns the optional standalone MCP listener. Configure it in the
existing `service_conf.yaml` (selected with `--config`), or use environment
variables. MCP settings are no longer command-line flags in either
`ragflow_server` or `entrypoint-go.sh`; replace those flags with the settings below.
Mode selection, help, version and configuration-file selection remain CLI options.

This example enables the listener for clients that each supply their own credential:

```yaml
mcp:
  enabled: true
  host: 127.0.0.1
  port: 9382
  launch_mode: host
  transport_sse_enabled: true
  transport_streamable_enabled: true
  json_response: true
```

Start the binary with `bin/ragflow_server --api --config conf/service_conf.yaml`.
Uncomment the MCP example in `conf/service_conf.yaml` for a local binary, or in
`docker/service_conf.yaml.template` for Docker. The Docker entrypoint generates
`service_conf.yaml` from that template on startup, so edit the template rather
than the generated file. Alternatively, put the environment settings in
`docker/.env-go`. Start `entrypoint-go.sh` without MCP flags.
For access through a published Docker port, set the container bind address to
`0.0.0.0` and publish `9382`; use host mode for clients outside a trusted network.
The existing MCP endpoint on the main API remains available independently.

Environment values override YAML, which overrides built-in defaults:

| Environment variable | YAML field under `mcp` | Default |
| --- | --- | --- |
| `RAGFLOW_MCP_ENABLED` | `enabled` | `false` |
| `RAGFLOW_MCP_HOST` | `host` | `127.0.0.1` |
| `RAGFLOW_MCP_PORT` | `port` | `9382` |
| `RAGFLOW_MCP_LAUNCH_MODE` | `launch_mode` | `self-host` |
| `RAGFLOW_MCP_HOST_API_KEY` | `host_api_key` | empty |
| `RAGFLOW_MCP_TRANSPORT_SSE_ENABLED` | `transport_sse_enabled` | `true` |
| `RAGFLOW_MCP_TRANSPORT_STREAMABLE_ENABLED` | `transport_streamable_enabled` | `true` |
| `RAGFLOW_MCP_JSON_RESPONSE` | `json_response` | `true` |

Boolean values `1`, `true`, `yes` and `on` are true (case-insensitive, surrounding
whitespace ignored); other values are false. Explicitly empty environment values
override YAML too. An empty host binds all interfaces; use `127.0.0.1` to restrict
access to the local machine. Invalid ports or launch modes reject startup.

## Authentication

Host mode resolves each request independently. Clients may supply
`Authorization: Bearer <key>`, `api_key`, `X-API-Key`, or `Api-Key`.

Self-host mode intentionally gives **every connected client the configured user's
identity**, ignoring client credentials. It is for a trusted local deployment or
an access-controlled network. Enabling it requires `RAGFLOW_MCP_HOST_API_KEY` or
`mcp.host_api_key`. The key is checked at startup and on every request, so revocation
still takes effect. Keep secrets in the deployment environment, out of command
arguments and committed files. The key is omitted from configuration logs.

## Transports

Both SSE (`/sse` and `/messages/`) and streamable HTTP (`/mcp`) are enabled by
default. Set `transport_sse_enabled` or `transport_streamable_enabled` to `false`
to disable one. Set `json_response: false` to stream streamable HTTP responses.

If streamable HTTP is disabled, JSON responses are disabled first. If both
transports are disabled, streamable HTTP is then re-enabled with JSON responses
still disabled, preserving the Python server's resolution order.

## Streamable HTTP smoke example

```bash
curl -sS http://127.0.0.1:9382/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Authorization: Bearer ragflow-xxxxx' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
```

Then send the `notifications/initialized` notification before `tools/list` or `tools/call` when using a real MCP client.
