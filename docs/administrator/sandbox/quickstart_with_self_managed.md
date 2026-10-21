---
sidebar_position: 1
title: Quickstart with Self-Managed
sidebar_label: Quickstart with Self-Managed
slug: /sandbox_quickstart_self_managed
---

# Quickstart with Self-Managed

This guide helps you connect RAGFlow to a Self-Managed Sandbox and run your first Code component task. The standard setup uses the published Sandbox Executor Manager and Python and Node.js Runner images with Docker.

## Before you begin

You need:

- A Linux host with Docker and Docker Compose.
- Permission to run Docker containers. The deployment must allow privileged containers and Docker socket access.
- A running RAGFlow deployment or a working RAGFlow source environment.
- Access to **Admin > Sandbox Settings**.
- Network access to pull images from Docker Hub.

You do not need to install Python or Node.js on the RAGFlow host. They run in the Sandbox Runner containers.

Confirm that the deployment account can access the Docker daemon:

```bash
docker info
```

Continue only if this command displays Docker server information. If it returns `permission denied` for `/var/run/docker.sock`, use an account already authorized to manage Docker or configure Docker access according to your organization's host-security policy. Membership in the Docker group grants root-equivalent control of the host; do not grant it broadly. The deployment must also allow the privileged Executor Manager container to mount the Docker socket.

## 1. Determine the Sandbox endpoint

First, identify where RAGFlow and Sandbox Executor Manager are running. Their deployment locations determine the endpoint that you must enter later in **Admin > Sandbox Settings**.

| Deployment scenario | Endpoint |
| --- | --- |
| RAGFlow and Sandbox Executor Manager run in the supplied Docker Compose deployment | `http://sandbox-executor-manager:9385` |
| RAGFlow runs from source, and Sandbox Executor Manager runs in Docker on the same host | `http://127.0.0.1:9385` |
| Sandbox Executor Manager runs on another host | An address reachable from RAGFlow, such as `https://sandbox.example.com` |

These endpoints use the default ports. If you change `SANDBOX_EXECUTOR_MANAGER_PORT`, update the source-deployment endpoint accordingly. Replace `https://sandbox.example.com` with the actual remote address, including its port when required.

:::note
The endpoint is resolved from where RAGFlow runs. If RAGFlow runs in a container, `127.0.0.1` refers to that container—not the Docker host or another container.

In the supplied Compose deployment, RAGFlow resolves `sandbox-executor-manager` through the shared Docker network. You do not need to add it to `/etc/hosts`.
:::

If an Executor Manager is already deployed, skip Steps 2–4 and use its endpoint and API token in [Configure Self-Managed in Admin](#5-configure-self-managed-in-admin). Before connecting to a Manager on another host, follow [Connect to a remote Executor Manager](./manage_self_managed_sandbox.md#connect-to-a-remote-executor-manager) to protect the endpoint.

## 2. Prepare the Sandbox service

In the RAGFlow repository, find the Sandbox section in `docker/.env` and uncomment the following settings:

```dotenv
COMPOSE_PROFILES=${COMPOSE_PROFILES},sandbox
SANDBOX_EXECUTOR_MANAGER_API_TOKEN=replace-with-a-generated-secret
```

Adding `sandbox` to `COMPOSE_PROFILES` ensures that future RAGFlow Compose startups also include Sandbox Executor Manager. Step 3 explicitly names the service so that you can start it immediately without restarting the other RAGFlow services.

Do not use the example token literally. Replace `replace-with-a-generated-secret` with a secret value, which you can generate with:

```bash
openssl rand -hex 32
```

Keep this token private. You will enter the same value in the RAGFlow Admin settings.

Pull the default runtime images:

```bash
docker pull infiniflow/sandbox-base-python:latest
docker pull infiniflow/sandbox-base-nodejs:latest
```

The Python image runs Python code, and the Node.js image runs JavaScript code. The supplied Compose configuration uses both images by default unless you override them in `docker/.env`.

The `latest` tag is convenient for initial setup. For production, use tested and compatible Manager and Runner image versions.

## 3. Start Sandbox Executor Manager

From the RAGFlow repository, enter the `docker` directory:

```bash
cd docker
```

Start Sandbox Executor Manager without restarting the other RAGFlow services:

```bash
docker compose up -d sandbox-executor-manager
```

This command works whether RAGFlow itself runs with Docker Compose or from source. By default, it publishes Executor Manager at `127.0.0.1:9385` on the Docker host. `SANDBOX_EXECUTOR_MANAGER_PORT` changes the published host port.

:::caution Use the existing Compose project
If RAGFlow was started with custom Compose options, use the same options in every command. For example:

```bash
docker compose -p YOUR_PROJECT_NAME up -d sandbox-executor-manager
```

Replace `YOUR_PROJECT_NAME` with the existing RAGFlow Compose project name. Otherwise, the Manager may be healthy but unreachable from RAGFlow.
:::

:::tip First startup can take longer
Docker may need to pull the Executor Manager image and initialize its Runners.
:::

## 4. Verify the service

Check its status:

```bash
docker compose ps --all sandbox-executor-manager
```

Wait until the service reports `healthy`, then check its health endpoint. The command below uses the default host port; if you changed `SANDBOX_EXECUTOR_MANAGER_PORT`, use that port instead:

```bash
curl -f http://127.0.0.1:9385/healthz
```

The health check succeeds when the command completes without an error and Docker Compose reports the service as `healthy`. The response body may vary by Executor Manager version.

Continue only when both checks succeed. If the status remains `starting`, allow more time for initialization. If it reports `unhealthy` or `exited`, inspect the logs:

```bash
docker compose logs --tail=200 sandbox-executor-manager
```

For a Compose deployment, also verify access from RAGFlow:

```bash
docker compose exec ragflow-cpu curl -f http://sandbox-executor-manager:9385/healthz
```

If you changed `SANDBOX_EXECUTOR_MANAGER_PORT`, use that port for the host health check. The Compose endpoint remains on port `9385`. See the [FAQ](./faq.md) if the service does not become healthy.

:::caution Verify the effective isolation runtime
Check the startup logs for the active runtime. `falling back to runc` means execution works without gVisor isolation. Confirm that this meets your security requirements.
:::

## 5. Configure Self-Managed in Admin

1. Open **Admin > Sandbox Settings**.
2. Select **Self-Managed**.
3. Complete the connection settings:

   | Setting | Value |
   | --- | --- |
   | **Executor Manager Endpoint** | Use the endpoint selected in [Step 1](#1-determine-the-sandbox-endpoint). |
   | **Executor Manager API Token** | Enter the same token configured for Executor Manager. |
   | **Request Timeout (seconds)** | Keep the default `30` for the first test. |

4. Select **Test connection**.
5. Confirm that the test result shows `PASSED`.
6. Select **Save**.

The connection test runs a small Python program. It verifies more than the `/healthz` endpoint: Executor Manager must also be able to start a Runner and execute code.

## 6. Run your first Code component

1. Create or open an Agent.
2. Add a **Code** component.
3. Set the language to Python.
4. Paste the following code:

```python
def main():
    return "Sandbox is ready"
```

5. Run the component.
6. Confirm that the result is:

```text
Sandbox is ready
```

This confirms the complete path from RAGFlow to Executor Manager and a Python Runner.

If **Test connection** succeeds but the Code component fails, see [Self-Managed execution](./faq.md#self-managed-execution) in the FAQ.

## Confirm the setup

The setup is complete when:

- `docker info` displays Docker server information.
- `docker compose ps --all sandbox-executor-manager` reports the service as `healthy`.
- The `/healthz` request completes without an error.
- **Test connection** displays `PASSED`.
- The Code component returns `Sandbox is ready`.

## Troubleshooting and next steps

- To adjust Runner capacity, images, resource limits, network access, artifacts, or authentication, see [Manage Self-Managed Sandbox](./manage_self_managed_sandbox.md).
- To connect RAGFlow to an Executor Manager on another host, see [Connect to a remote Executor Manager](./manage_self_managed_sandbox.md#connect-to-a-remote-executor-manager).
- If startup, connection, authentication, or execution fails, see [Code Execution Sandbox FAQ](./faq.md).
- To use SSH or a managed cloud service instead, see [Configure SSH and Cloud Providers](./configure_ssh_and_cloud_providers.md).
