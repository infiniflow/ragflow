---
sidebar_position: 4
title: Manage Self-Managed Sandbox
sidebar_label: Manage Self-Managed Sandbox
slug: /manage_self_managed_sandbox
---

# Manage Self-Managed Sandbox

Use this guide after completing [Quickstart with Self-Managed](./quickstart_with_self_managed.md). The settings in this document control the optional `sandbox-executor-manager` Compose service and its Runner containers.

Run the Docker Compose commands below from the RAGFlow repository's `docker` directory. If the existing deployment uses a custom Compose project name, Compose file, or environment file, use the same options with every command in this guide.

## Check service health

Check the service state:

```bash
docker compose ps --all sandbox-executor-manager
```

Check the Manager health endpoint. The command below uses the default host port; if you changed `SANDBOX_EXECUTOR_MANAGER_PORT`, use that port instead:

```bash
curl -f http://127.0.0.1:9385/healthz
```

If the service is not `healthy`, inspect its logs:

```bash
docker compose logs --tail=200 sandbox-executor-manager
```

The `/healthz` endpoint only confirms that Executor Manager is responding. To verify that it can create a Runner and execute code, open **Admin > Sandbox Settings**, select **Self-Managed**, and run **Test connection**. A successful test displays `PASSED`.

After deployment changes, also run a Code component to verify the complete path from RAGFlow through Executor Manager to the Runner.

## Stop or restart the service

Stop Executor Manager without removing its Compose configuration:

```bash
docker compose stop sandbox-executor-manager
```

While the service is stopped, Code components that use the active Self-Managed provider cannot execute.

Start it again:

```bash
docker compose up -d sandbox-executor-manager
```

Wait until the service reports `healthy`, then run **Test connection** before allowing users to execute code again.

## Connect to a remote Executor Manager

Executor Manager can run on another host while RAGFlow runs in Docker or from source. RAGFlow only needs an HTTP or HTTPS endpoint that it can reach.

:::caution Protect the execution endpoint
The `/run` endpoint executes submitted code and must not be exposed directly to the public internet. Use a private network or a protected reverse proxy, require a strong API token, restrict source addresses, and use TLS when traffic crosses an untrusted network.
:::

The supplied RAGFlow Compose service publishes port `9385` on `127.0.0.1` only, so it is not remotely reachable by default. For a separately hosted Manager, follow the deployment and security guidance in the [Sandbox repository](https://github.com/infiniflow/sandbox), then:

1. Confirm that the remote Manager has the Python and Node.js Runner images available.
2. Set a strong `SANDBOX_EXECUTOR_MANAGER_API_TOKEN` on the Manager.
3. Make the protected endpoint reachable from the RAGFlow host or container.
4. Verify the remote health endpoint from the environment where RAGFlow runs. If RAGFlow runs in a container, test from that container or its network—not only from the Docker host:

   ```bash
   curl -f https://sandbox.example.com/healthz
   ```

5. Open **Admin > Sandbox Settings** and select **Self-Managed**.
6. Enter the remote URL in **Executor Manager Endpoint** and the matching token in **Executor Manager API Token**.
7. Run **Test connection**, confirm `PASSED`, and save the configuration.

Do not use `127.0.0.1` unless RAGFlow and the published Manager port are on the same host and RAGFlow runs directly on that host. Inside a RAGFlow container, `127.0.0.1` refers to the RAGFlow container itself.

## Configure deployment settings

Configure these values in `docker/.env`:

| Variable | Default | Purpose |
| --- | --- | --- |
| `SANDBOX_EXECUTOR_MANAGER_POOL_SIZE` | `3` | Number of Runner environments maintained by Executor Manager. Increase it only after measuring concurrent demand and host capacity. |
| `SANDBOX_EXECUTOR_MANAGER_PORT` | `9385` | Port published on `127.0.0.1` of the Docker host. Set it in `docker/.env`. This does not change the container port `9385` used inside the Compose network. |
| `SANDBOX_MAX_MEMORY` | `256m` | Memory limit applied to a Runner container. Accepts Docker-style sizes such as `512m` or `1g`. |
| `SANDBOX_TIMEOUT` | `10s` | Service-side execution limit, such as `30s` or `1m`. |
| `SANDBOX_ENABLE_SECCOMP` | `false` | Enables the seccomp behavior supported by the deployed Executor Manager version. Validate workload compatibility before enabling it. |

`SANDBOX_TIMEOUT` is different from **Request Timeout (seconds)** in Admin. The service-side limit controls the Runner execution; the Admin setting controls how long RAGFlow waits for the Manager response. Keep the Admin request timeout longer than the expected service execution time.

After changing deployment values, apply the changes:

```bash
docker compose up -d sandbox-executor-manager
```

Then check its health and run **Test connection** again.

## Manage Runner images

The default Runner images are:

```dotenv
SANDBOX_BASE_PYTHON_IMAGE=infiniflow/sandbox-base-python:latest
SANDBOX_BASE_NODEJS_IMAGE=infiniflow/sandbox-base-nodejs:latest
```

Use a custom image when every execution needs additional packages or system libraries. Prefer pre-installing dependencies in a trusted image instead of enabling Runner internet access and installing packages during each run.

To change an image:

1. Build or select an image compatible with the deployed Executor Manager.
2. Pin a tested version tag or digest instead of relying on a moving tag in production.
3. Pull the image on the Docker host:

   ```bash
   RUNNER_IMAGE=infiniflow/sandbox-base-python:latest
   docker pull "$RUNNER_IMAGE"
   ```

   Replace the `RUNNER_IMAGE` value with the full name and tag of the selected image.

4. Set `SANDBOX_BASE_PYTHON_IMAGE` or `SANDBOX_BASE_NODEJS_IMAGE` in `docker/.env`.
5. Apply the changes to Executor Manager and run **Test connection**.
6. Run a Code component in each language whose image changed.

To update Executor Manager itself, set `SANDBOX_EXECUTOR_MANAGER_IMAGE` to a tested image tag, pull it, and apply the change. Keep Manager and Runner image versions compatible.

## Manage Runner network access

Runner containers use no network by default:

```dotenv
SANDBOX_CONTAINER_NETWORK=none
```

This prevents executed code from reaching the internet, internal services, cloud metadata endpoints, and other containers. Keep this default for untrusted code.

If a controlled workload requires outbound access, set an explicitly managed Docker network. The commonly used Docker value `bridge` gives broader outbound access:

```dotenv
SANDBOX_CONTAINER_NETWORK=bridge
```

Before enabling it, apply egress filtering and confirm that Runners cannot reach sensitive internal services or host-management endpoints. Apply the change and verify the workload.

## Manage generated artifacts

Code must write files intended for return under the execution workspace's `artifacts/` directory. RAGFlow publishes supported files to object storage and returns authorized links to the Agent.

Supported extensions are `.csv`, `.html`, `.jpeg`, `.jpg`, `.json`, `.pdf`, `.png`, and `.svg`. Provider or Executor Manager limits may also restrict the number and size of files.

The object-storage bucket defaults to `sandbox-artifacts`. To use another bucket, set the following value in the RAGFlow runtime environment:

```dotenv
SANDBOX_ARTIFACT_BUCKET=ragflow-sandbox-artifacts
```

Replace the example value with the name of the bucket used by the deployment.

For Docker deployments, set it in `docker/.env` and apply the change to RAGFlow:

```bash
docker compose up -d ragflow-cpu
```

This variable controls RAGFlow storage, not the Runner or Executor Manager.

RAGFlow does not define a separate Sandbox artifact retention value in this configuration. Apply retention and lifecycle rules through the configured object-storage service according to your organization's policy.

## Rotate the API token

The token configured on Executor Manager must match **Executor Manager API Token** in Admin.

:::note Maintenance window recommended
The Compose service accepts one configured token. Requests can fail between restarting Executor Manager with the new token and saving the matching Admin configuration.
:::

1. Generate a new token:

   ```bash
   openssl rand -hex 32
   ```

2. Replace `SANDBOX_EXECUTOR_MANAGER_API_TOKEN` in `docker/.env`.
3. Apply the token change:

   ```bash
   docker compose up -d sandbox-executor-manager
   ```

4. In **Admin > Sandbox Settings**, enter the new token under **Self-Managed**.
5. Run **Test connection**, confirm `PASSED`, and select **Save**.
6. Run a Code component and confirm that execution succeeds.

## Production checklist

- Keep Executor Manager on a private network or behind a protected TLS reverse proxy.
- Configure a strong API token and rotate it according to your credential policy.
- Restrict access to the Docker socket and the privileged Manager container.
- Set Runner pool, memory, and timeout values that fit the host capacity.
- Keep Runner networking disabled unless outbound access is required and controlled.
- Use trusted, pinned Manager and Runner images and scan custom images before deployment.
- Apply object-storage access controls and lifecycle rules to generated artifacts.
- Monitor Manager logs, Runner creation failures, resource exhaustion, and repeated authentication failures.
- Test Python and JavaScript after image, network, token, or resource-limit changes.

## Admin UI settings

Admin controls the RAGFlow-to-Manager connection: endpoint, API token, and request timeout. Compose variables control the Manager and Runner deployment. Changing one does not automatically update the other.

For the connection fields, see [Configure Code Execution Sandbox](../admin/admin_ui/configure_code_execution_sandbox.md#self-managed-fields).
