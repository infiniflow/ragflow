---
sidebar_position: 11
title: Enable and Connect to the MCP Endpoint
sidebar_label: Enable and Connect to MCP
slug: /launch_mcp_server
sidebar_custom_props:
  categoryIcon: LucideTvMinimalPlay
---

# Enable and Connect to the MCP Endpoint

RAGFlow's Go API server implements MCP in-process. The API route `POST /api/v1/mcp` is available when the Go API runs. You can also enable an MCP listener in the same process for Streamable HTTP and legacy SSE clients.

## Use the Go API endpoint

Start the Go API as usual. For a local binary, use `bin/ragflow_server --api --config conf/service_conf.yaml`. With the checked-in configuration, the Go API listens on port `9384`, so its direct MCP URL is `http://127.0.0.1:9384/api/v1/mcp`. Use your configured Go API address if you changed the port or access it through a reverse proxy. This route requires each client's RAGFlow credential in the `Authorization` header.

The API route accepts POST requests and returns JSON. It does not provide `/sse` or `/messages/`.

## Enable the optional MCP listener

Uncomment or add the following settings in `conf/service_conf.yaml` for a local binary:

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

Then start the Go API with `bin/ragflow_server --api --config conf/service_conf.yaml`. In host mode, each client sends `Authorization: Bearer <RAGFLOW_API_KEY>` on every request. The listener's Streamable HTTP URL is `http://127.0.0.1:9382/mcp`; legacy SSE clients use `http://127.0.0.1:9382/sse` and post messages to `/messages/`.

To use self-host mode, set `launch_mode: self-host` and supply `RAGFLOW_MCP_HOST_API_KEY` in the process environment. Every client then uses that configured user's identity, even if it sends a different credential. Limit access to trusted clients. The listener is disabled by default; its default mode when enabled is self-host and requires a valid configured key.

The same settings can be supplied as `RAGFLOW_MCP_*` environment variables, which override YAML. See [Native Go MCP Runtime](./native_go_mcp.md) for the complete settings and defaults.

## Docker

For the Go API container, uncomment the `mcp` example in `docker/service_conf.yaml.template` or set the equivalent `RAGFLOW_MCP_*` values in `docker/.env-go`. The Docker entrypoint generates the runtime configuration from the template. Use `docker/docker-compose-go.yml` with `docker/.env-go` for the Go deployment.

To reach the optional listener from outside the container, set `mcp.host: 0.0.0.0` (or `RAGFLOW_MCP_HOST=0.0.0.0`) and add a port mapping under the relevant `ragflow-cpu` or `ragflow-gpu` service in `docker/docker-compose-go.yml`:

```yaml
ports:
  - ${SVR_MCP_PORT}:9382
```

The checked-in Go Compose file does not publish `9382` by default. `SVR_MCP_PORT` is defined in `docker/.env-go`; change the mapping if you configure a different container-side MCP port. Use `launch_mode: host` when clients must authenticate individually.

## Verify the endpoint

The following initialization request checks the optional listener in host mode. Use the API URL instead to check `/api/v1/mcp`.

```bash
curl -sS http://127.0.0.1:9382/mcp \
  -H 'Authorization: Bearer <RAGFLOW_API_KEY>' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
```

A successful response contains an MCP `result` with `serverInfo`. An authentication error means the credential or launch-mode configuration needs attention. See [RAGFlow MCP Client Examples](./mcp_client_example.md) for a tool-listing request.
