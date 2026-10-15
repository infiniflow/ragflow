---
sidebar_position: 2
title: Sandbox Quickstart
sidebar_label: Sandbox Quickstart
slug: /sandbox_quickstart
sidebar_custom_props: {
  categoryIcon: LucideCog
}
---
# Sandbox quickstart

RAGFlow's `CodeExec` agent component needs a sandbox provider to run Python and JavaScript code.

The simplest setup flow is:

1. Start the required sandbox services.
2. Open the RAGFlow admin page.
3. Go to **Admin > Sandbox Settings**.
4. Choose a provider and save the configuration.
5. Test the connection in the same page.

## Admin page

Configure sandbox providers from the admin page:

- `self_managed`: Uses the executor manager service.
- `ssh`: Runs code on a remote machine over SSH.
- `aliyun_codeinterpreter`, `e2b`, and `tenki`: Cloud providers.

## Provider options

RAGFlow supports multiple sandbox providers. Configure the active provider in Admin > Sandbox Settings after the services are up.

- `self_managed`: Runs code inside Docker-managed sandbox containers. This is the default provider.
- `ssh`: Runs code on a remote machine over SSH.
- `aliyun_codeinterpreter` and `e2b`: Cloud-hosted providers that remain available in the admin provider list.
- `tenki`: Cloud-hosted provider that runs each execution in a disposable [Tenki](https://tenki.cloud) microVM. See [Tenki](#tenki) below.

### Tenki

`tenki` runs each code execution in a fresh Tenki microVM and destroys it afterward. It is cloud-hosted, so it needs no local sandbox services, gVisor, or Docker base images — only outbound network access and an API key.

Configure it in **Admin > Sandbox Settings**:

- `api_key` (required): Tenki API key. Create one at [app.tenki.cloud](https://app.tenki.cloud) under **API Keys**.
- `project_id` (required): the Tenki project that sandboxes are created under.
- `base_url` (optional): override the Tenki API endpoint.
- `image` (optional): sandbox base image. Leave empty to use the Tenki default image, which includes `python3` and `node`.
- `allow_outbound` (optional, security-relevant): whether the sandbox may make outbound network connections. Defaults to `false` so sandboxed code has no network access; set it to `true` when code needs the network (for example, to install packages).
- `timeout`, `max_lifetime`, `cpu_cores`, `memory_mb`, `disk_size_gb`, and the output/artifact limits have sensible defaults and can be tuned in the same page.

Notes:

- Supported languages are Python and JavaScript.
- Files written to the `artifacts/` directory of the working directory are returned as run artifacts.
- The provider uses only Tenki's create/exec/destroy operations; it does not use volumes or snapshots.

## Self-managed architecture

RAGFlow is a client of the external executor-manager service. The [Standalone Sandbox repository](https://github.com/infiniflow/sandbox) owns its implementation, Python and Node.js runtime images, Dockerfiles, image builds, container lifecycle, dependencies, and security configuration.

Follow that repository's installation and build instructions on the **sandbox service host**. RAGFlow does not build sandbox images. A separately hosted sandbox does not require Docker or gVisor on the RAGFlow host; RAGFlow needs network access to its endpoint.

## Connect RAGFlow to an external sandbox

In **Admin > Sandbox Settings**, select `self_managed` and configure:

| Setting | Purpose |
| --- | --- |
| `endpoint` | Executor-manager HTTP address reachable from RAGFlow, such as `http://sandbox-host:9385`. |
| `api_token` | Shared authentication token; use the same value on the service. A blank value falls back to `SANDBOX_EXECUTOR_MANAGER_API_TOKEN`. |
| `timeout` | HTTP request timeout in seconds, default `30`. This does not configure the service's execution timeout. |

Save the configuration and click **Test Connection**. This executes a small Python program; a successful `/healthz` response alone does not establish that code execution is available.

Without saved settings, the client uses `SANDBOX_PROVIDER_TYPE` (default `self_managed`), `SANDBOX_EXECUTOR_MANAGER_URL` (default `http://sandbox-executor-manager:9385`), and `SANDBOX_EXECUTOR_MANAGER_TIMEOUT` (a duration such as `30s`). Saved connection settings take precedence.

Current [Sandbox authentication source](https://github.com/infiniflow/sandbox/blob/main/executor_manager/services/auth.py) rejects `/run` with HTTP 503 when no token is configured, unless the service operator explicitly enables unauthenticated access. Configure a strong shared token. Published image tags may differ from current source; verify the authentication behavior of the version you deploy.

The existing `base_python_image` and `base_nodejs_image` settings send an optional `base_image` request field for custom managers. The current reference service does not apply that field: configure its runtime images on the service host instead.

## Optional published-image deployment with RAGFlow

The `sandbox` Compose profile can start a published executor-manager image alongside RAGFlow. It pulls/runs the image and does not build it. Its runtime containers and isolation still belong to the external service.

In `docker/.env`:

- Include `sandbox` in `COMPOSE_PROFILES`.
- Set `SANDBOX_EXECUTOR_MANAGER_API_TOKEN` to the shared secret passed to both services.
- Pull the Python and Node.js runtime images named by the deployment before starting it.

Start RAGFlow using its usual Compose instructions, then configure `http://sandbox-executor-manager:9385` as the admin endpoint. Services on the shared Compose network resolve that name automatically; no host `/etc/hosts` entry is needed.

For RAGFlow running directly on the same host, use `http://127.0.0.1:<published-port>`. The supplied Compose service publishes its port on loopback only. For a remote sandbox, use a reachable address secured by the sandbox operator rather than the local Compose service name.

Compose deployment variables remain available in `docker/.env`: `SANDBOX_EXECUTOR_MANAGER_IMAGE`, `SANDBOX_EXECUTOR_MANAGER_PORT`, `SANDBOX_EXECUTOR_MANAGER_POOL_SIZE`, `SANDBOX_BASE_PYTHON_IMAGE`, `SANDBOX_BASE_NODEJS_IMAGE`, `SANDBOX_ENABLE_SECCOMP`, `SANDBOX_MAX_MEMORY`, `SANDBOX_CONTAINER_NETWORK`, and `SANDBOX_TIMEOUT`. These configure the optional service, not RAGFlow's HTTP connection. Apply service deployment changes through Compose and follow the Sandbox repository's security guidance. Preserve the default runner network isolation (`none`) unless outbound access is deliberately required.

Admin schema values are not a readback of the external service's deployed configuration. Saving connection settings does not restart or reconfigure that service.

## RAGFlow artifact storage

`SANDBOX_ARTIFACT_BUCKET` selects RAGFlow's object-storage bucket for files returned by sandbox providers. It is independent of executor-manager deployment and defaults to `sandbox-artifacts`. RAGFlow publishes returned artifact bytes and serves them through its authorized artifact endpoint.

## Other providers

SSH and cloud providers do not require the optional executor-manager service. For SSH, retain the host known-hosts mount configured by `RAGFLOW_SSH_KNOWN_HOSTS` and the container path `SSH_KNOWN_HOSTS`.
