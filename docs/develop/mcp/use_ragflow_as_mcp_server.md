---
sidebar_position: 2
title: Use RAGFlow as an MCP Server
sidebar_label: Use RAGFlow as an MCP Server
slug: /use_ragflow_as_mcp_server
sidebar_custom_props:
  categoryIcon: LucideBookOpen
---

# Use RAGFlow as an MCP Server

RAGFlow's Go API server exposes MCP tools through Go services. It does not need a separate Python MCP server or a RAGFlow base URL.

```text
External MCP client → Go MCP endpoint → RAGFlow services
```

## Choose an endpoint

| Endpoint | Availability | Transport | Authentication |
| --- | --- | --- | --- |
| `POST /api/v1/mcp` on the Go API port | Available while the Go API runs | Streamable HTTP with JSON responses | Each client sends its RAGFlow credential in `Authorization`. |
| `/mcp` on the optional MCP listener | Enable `mcp.enabled` or `RAGFLOW_MCP_ENABLED` | Streamable HTTP; legacy SSE at `/sse` and `/messages/` when enabled | `host` mode authenticates each request; `self-host` mode uses one configured key for all clients. |

For a local binary, start the API with `bin/ragflow_server --api --config conf/service_conf.yaml`. With the checked-in configuration, the Go API listens on port `9384`, so its direct endpoint is `http://127.0.0.1:9384/api/v1/mcp`. Use the configured address if your port or reverse proxy differs. This API route accepts POST requests; it does not provide the legacy SSE paths.

## Enable the optional MCP listener

The Go API process also owns an optional listener. It is disabled by default and binds to `127.0.0.1:9382` unless configured otherwise. Uncomment or add this example in `conf/service_conf.yaml` for local use:

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

Start the Go API with `bin/ragflow_server --api --config conf/service_conf.yaml`. The listener's Streamable HTTP URL is `http://127.0.0.1:9382/mcp`. If SSE is enabled, legacy clients use `http://127.0.0.1:9382/sse` and send messages to `/messages/`.

Environment variables override YAML settings:

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

Boolean values `1`, `true`, `yes`, and `on` are true, regardless of case. An explicitly empty environment value also overrides YAML. An empty host binds all interfaces. If both transports are disabled, Streamable HTTP is re-enabled with streamed responses. Invalid ports or launch modes prevent startup.

## Authentication

The optional listener supports two modes:

| Mode | Credential handling |
| --- | --- |
| `host` | Every request is authenticated as its client. Prefer `Authorization: Bearer <key>`; `api_key`, `X-API-Key`, and `Api-Key` headers are also accepted. Both Streamable HTTP and SSE work in this mode. |
| `self-host` (default) | Set `mcp.host_api_key` or `RAGFLOW_MCP_HOST_API_KEY` before startup. Every connected client uses the configured user's identity, regardless of its own headers. Restrict access to trusted clients. |

Enabling the listener with its default `self-host` mode requires `mcp.host_api_key` or `RAGFLOW_MCP_HOST_API_KEY`; startup fails when that key is empty. The configured key is checked at startup and for each request, so revoking it takes effect. Keep keys out of command arguments and committed files. The `/api/v1/mcp` API route always authenticates each client through its own `Authorization` header; listener launch modes do not change that route.

## Docker

For the Go deployment, use `docker/docker-compose-go.yml` with `docker/.env-go`. Uncomment the `mcp` example in `docker/service_conf.yaml.template` or set the equivalent `RAGFLOW_MCP_*` values in `docker/.env-go`. The Go entrypoint generates the runtime configuration from that template.

To connect to the optional listener from outside the container, set its host to `0.0.0.0` and add this mapping under the relevant `ragflow-cpu` or `ragflow-gpu` service's `ports` in `docker/docker-compose-go.yml`:

```yaml
ports:
  - ${SVR_MCP_PORT}:9382
```

The checked-in Go Compose file does not publish port `9382` by default. `SVR_MCP_PORT` is defined in `docker/.env-go`. Adjust the container-side port if you change `mcp.port`. Use `host` mode when external clients must authenticate individually.

## Verify the endpoint

Use [RAGFlow MCP Client Examples](./mcp_client_example.md) to initialize the endpoint, list its tools, and call a tool. A successful initialization response contains an MCP `result` with `serverInfo`. For the API route, use your Go API address ending in `/api/v1/mcp`; for the optional listener, use its `/mcp` URL. See [RAGFlow MCP Tools](./mcp_tools.md) for the available tools.
