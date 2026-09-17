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

| Field       | Description                                              | Example                     |
| ----------- | -------------------------------------------------------- | --------------------------- |
| Name        | A custom name used to identify the MCP server in RAGFlow | `Local file tools`          |
| URL         | The complete MCP endpoint                                | `https://example.com/mcp`   |
| Server type | The transport provided by the MCP server                 | `streamable-http`           |
| Description | Optional description of the MCP server                   | `Read and manage files`     |
| Headers     | HTTP headers required by the MCP server                  | `Authorization: Bearer xxx` |
| Variables   | Variables required by the MCP server configuration       | Depends on the MCP server   |

The **Server type** must match the transport exposed by the MCP server:

```text
streamable-http
```

or:

```text
sse
```

If the MCP server does not require authentication, leave **Headers** empty.

Make sure that the path in **URL** matches the endpoint actually exposed by the MCP server. For example, a Streamable HTTP server commonly uses `/mcp`, while an SSE server commonly uses `/sse`.

## MCP server URL access restrictions

RAGFlow validates MCP server URLs before establishing a connection to protect the RAGFlow backend against server-side request forgery (SSRF) and internal network probing.

### Supported URL schemes

MCP server URLs must use one of the following schemes:

```text
http://
https://
```

The URL must contain a valid hostname or IP address.

Other URL schemes, including `stdio://`, are not supported for MCP server connections.

### Public address validation

By default, RAGFlow allows an MCP server URL only when **all IP addresses resolved from its hostname are publicly routable addresses**.

With the default configuration:

```text
ALLOW_ANY_HOST=0
```

RAGFlow rejects URLs that resolve to loopback, private, link-local, reserved, or other non-public addresses.

Examples of addresses that are rejected by default include:

```text
127.0.0.1
::1
10.x.x.x
172.16.x.x - 172.31.x.x
192.168.x.x
```

This also affects hostnames. Even if the URL uses a public domain name, RAGFlow rejects the connection if that domain resolves to a non-public IP address in the environment where the RAGFlow backend is running.

For example:

```text
https://mcp.example.com/mcp
```

can still be rejected if `mcp.example.com` resolves to a private, loopback, or synthetic address.

### Fake-IP DNS results

Proxy applications such as Mihomo or Clash may use Fake-IP mode and return synthetic addresses for public domains.

For example, a public domain may resolve to an address in a range such as:

```text
fdfe:dcba:9876::/48
```

RAGFlow identifies such addresses as non-public and may display an error similar to:

```text
URL resolves to a non-public address (...), which is not allowed.
```

If you see this error, check DNS resolution from the environment where the RAGFlow backend is running.

The error does not necessarily indicate that the MCP server URL, transport type, or authentication configuration is incorrect.

## Connect to a local or private MCP server

For trusted local development or testing environments, you can allow RAGFlow to connect to MCP servers running on localhost, a private network, or a Docker network.

In `docker/.env`, set:

```text
ALLOW_ANY_HOST=1
```

Then fully restart the RAGFlow backend services so that the new environment setting takes effect.

For example:

```bash
docker compose down
docker compose up -d
```

When `ALLOW_ANY_HOST=1`, RAGFlow skips the public-address validation described above.
