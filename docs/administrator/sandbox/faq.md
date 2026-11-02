---
sidebar_position: 3
title: Code Execution Sandbox FAQ
sidebar_label: FAQ
slug: /sandbox_faq
---

# Code Execution Sandbox FAQ

## Provider selection and configuration

### Which provider should I choose?

- Choose **Self-Managed** to run Docker-managed Runners in infrastructure you control.
- Choose **SSH** to reuse a dedicated Linux execution host with a custom runtime.
- Choose **Aliyun Code Interpreter**, **E2B**, **Tenki**, or **UCloud Agent Sandbox** when a managed cloud execution environment fits your region, security, and operational requirements.

See [Choose a sandbox provider](./overview.md#choose-a-sandbox-provider) for a comparison and links to each setup guide.

### Does Test connection save the configuration?

No. **Test connection** uses the values currently entered in the form without changing the active provider. After the result shows `PASSED`, select **Save** to activate the configuration.

Cloud connection tests create a temporary execution environment and may consume quota or incur short-lived charges. RAGFlow attempts to remove the environment afterward; cleanup failures are written to the RAGFlow logs.

### When does a newly saved provider become active?

The saved provider is used for subsequent Code component executions across the RAGFlow deployment. A run that is already in progress continues with the provider configuration it started with. Test representative existing Agents after changing the provider.

## Self-Managed startup and connection

### Why did `sandbox-executor-manager` not start?

From the repository's `docker` directory, check its state and logs:

If RAGFlow uses custom Compose options, use the same options in every command below.

```bash
docker compose ps --all sandbox-executor-manager
docker compose logs --tail=200 sandbox-executor-manager
```

Common causes include:

- The service was not explicitly started and `sandbox` is missing from `COMPOSE_PROFILES`.
- The deployment account cannot access the Docker daemon.
- Docker cannot pull the Manager or Runner images.
- The host cannot run privileged containers or expose `/var/run/docker.sock` to the Manager.
- Port `9385`, or the configured `SANDBOX_EXECUTOR_MANAGER_PORT`, is already in use.
- A deployment value in `docker/.env` is invalid.

After correcting the problem, apply the changes:

```bash
docker compose up -d sandbox-executor-manager
```

If Docker reports `permission denied` for `/var/run/docker.sock`, verify access with `docker info`. Run the deployment with an account authorized to manage Docker, or configure access according to the host-security policy. Docker daemon access is root-equivalent and should be restricted to trusted administrators.

### Which endpoint should Docker and source deployments use?

| RAGFlow location | Executor Manager location | Endpoint |
| --- | --- | --- |
| RAGFlow container in the supplied Compose project | Same Compose project | `http://sandbox-executor-manager:9385` |
| RAGFlow source process on the Docker host | Manager container on the same host | `http://127.0.0.1:9385` |
| RAGFlow and Manager on different hosts | Protected remote endpoint | A private or protected HTTPS address reachable from RAGFlow |

An endpoint is resolved from the RAGFlow process or container. Inside a container, `127.0.0.1` refers to that container and cannot reach another container.

### Why does the connection return an error?

| Error | Likely cause | What to check |
| --- | --- | --- |
| `Connection refused` | Nothing is listening at the address and port. | Manager state, published port, and endpoint. |
| DNS or `no such host` | The service name cannot be resolved from the RAGFlow network. | Compose project and network membership; do not use the Compose service name from an unrelated host. |
| Timeout | A firewall, route, proxy, or overloaded service prevented a timely response. | Connectivity from the RAGFlow container or process, Manager logs, and request timeout. |
| `401 Unauthorized` | The Admin token does not match the Manager token. | `SANDBOX_EXECUTOR_MANAGER_API_TOKEN` on the Manager and **Executor Manager API Token** in Admin. |
| `503 Service Unavailable` | Executor Manager may not have an API token configured. | Set `SANDBOX_EXECUTOR_MANAGER_API_TOKEN`, restart the Manager, and enter the same token in Admin. |

When testing connectivity, run the check from the same host or container network where RAGFlow runs.

### Why does the health check succeed while Code execution fails?

`/healthz` only shows that Executor Manager is responding. It does not prove that the Manager can authenticate a `/run` request, access Docker, start the selected Runner image, execute code, or return artifacts.

Run **Test connection** in Admin. It starts a Python Runner and executes a diagnostic. If it fails, inspect Manager logs and verify the token, Docker socket, Runner images, resource limits, and execution timeout.

## Self-Managed execution

### Why did a Runner image fail to start?

Check that the configured image exists, supports the host architecture, and is accessible to the Docker daemon used by Executor Manager:

```bash
docker pull infiniflow/sandbox-base-python:latest
docker pull infiniflow/sandbox-base-nodejs:latest
```

If custom image variables are set, pull those exact tags instead. Then apply the changes to Executor Manager. Also verify available disk space, memory, Docker socket access, and Manager–Runner version compatibility.

### Why is the Docker API version incompatible?

Executor Manager communicates with the host Docker daemon through `/var/run/docker.sock`. An error stating that a client API version is too old or unsupported indicates that the Manager image's Docker client and the host daemon do not support a common API version.

Pull a current, tested Executor Manager image or use a Docker daemon version supported by that image. Do not work around the error by setting an arbitrary API version without confirming compatibility. Record the Manager image tag and Docker server version when reporting the issue.

### Why did execution exceed the memory or timeout limit?

Self-Managed execution has separate limits:

- The Code request can specify an execution timeout.
- `SANDBOX_TIMEOUT` limits execution on Executor Manager.
- **Request Timeout (seconds)** limits how long RAGFlow waits for the HTTP response.
- `SANDBOX_MAX_MEMORY` limits Runner memory.

The shortest applicable timeout ends the request first. Increase limits only after confirming that the workload is expected and that the host has sufficient resources. Apply the changes after updating Compose deployment values.

### Why can executed code not access the internet?

The supplied deployment uses `SANDBOX_CONTAINER_NETWORK=none` by default. This is an intentional security control.

Prefer adding required dependencies to a trusted Runner image. If outbound access is necessary, use a controlled Docker network and apply egress restrictions before changing `SANDBOX_CONTAINER_NETWORK`. See [Manage Runner network access](./manage_self_managed_sandbox.md#manage-runner-network-access).

## SSH

### Why does SSH return `knownhosts: key is unknown`?

The network connection reached the SSH server, but RAGFlow could not find a trusted host key for that host and port. RAGFlow rejects unknown keys before attempting password or private-key authentication.

For a non-default SSH port, the `known_hosts` entry must include the port—for example, `[192.168.1.5]:2222`. Verify the server fingerprint, add the key to the file used by RAGFlow, and ensure that the file is visible to the RAGFlow process or container.

For Docker and source deployment instructions, see [Prepare the SSH `known_hosts` file](./configure_ssh_and_cloud_providers.md#prepare-the-ssh-known_hosts-file).

If the message reports a host-key mismatch instead, verify whether the server key was intentionally changed before replacing the existing entry. An unexpected key change can indicate that the connection is being intercepted.

### Why does SSH authentication fail?

Verify the username and the selected authentication method. For password authentication, confirm that the server permits password login. For private-key authentication, confirm that:

- The public key is authorized for the selected remote user.
- The pasted PEM is complete, or the configured path is readable by the RAGFlow process.
- A Docker path refers to a file mounted inside the RAGFlow container, not only to a host path.
- The passphrase matches the encrypted key.

Do not configure both password and private-key authentication at the same time.

### Why can RAGFlow not create the remote workspace?

The configured SSH user must be able to create and remove directories under **Remote Workspace Root**. Check the path, ownership, permissions, available disk space, filesystem mount options, and user quota. `/tmp` is the default; use a dedicated writable path when server policy restricts it.

### Why can the remote host not find Python or Node.js?

The configured binary must be available in the non-interactive SSH session. Shell profile files may not modify `PATH` for this type of session. Enter an absolute executable path, such as `/usr/bin/python3` or the interpreter inside a virtual environment, and verify that the SSH user can execute it.

**Test connection** always uses Python. Node.js is only required for JavaScript Code components.

## Cloud providers

Return to the detailed configuration for [Aliyun Code Interpreter](./configure_ssh_and_cloud_providers.md#configure-aliyun-code-interpreter), [E2B](./configure_ssh_and_cloud_providers.md#configure-e2b), [Tenki](./configure_ssh_and_cloud_providers.md#configure-tenki), or [UCloud Agent Sandbox](./configure_ssh_and_cloud_providers.md#configure-ucloud-agent-sandbox) when checking credentials, regions, endpoints, resource settings, or network access.

### Why is the API credential rejected?

Check that the credential belongs to the selected provider and account, has not expired or been revoked, and has permission to list or create the required resources. Confirm the account ID and region where applicable. For Aliyun, the credentials need Code Interpreter template and sandbox permissions. For E2B `eu`, the RAGFlow runtime must also have `E2B_DOMAIN` configured.

After changing a deployment-level environment variable, apply the change to RAGFlow before testing again.

### Why does sandbox creation time out?

Confirm that RAGFlow can reach the provider API, the selected region is available, and the account has quota. Check provider status, firewall and proxy rules, DNS, and RAGFlow logs. Increasing a request timeout does not fix invalid credentials, unavailable quota, or a blocked network path.

### How do region, quota, network, and account limits affect execution?

Cloud providers create resources in the configured account and region. An unsupported region, exhausted concurrency quota, billing restriction, or organization policy can prevent creation even when credentials are valid. The provider console and audit logs usually contain the most specific service-side error.

Test connection and Code execution may consume quota or incur charges. Review provider limits before enabling the service for multiple users.

## Artifacts

### Why was a generated file not returned?

The file must be written under the execution workspace's `artifacts/` directory and use a supported extension. It can also be omitted when it exceeds provider size or count limits, cannot be decoded, or cannot be uploaded to object storage. Check Code execution output, RAGFlow logs, provider limits, and object-storage availability.

### Where should code write generated files?

Use a path under `artifacts/` relative to the execution workspace. Files written elsewhere are treated as temporary execution files and are not returned.

### What size, count, type, and retention limits apply?

SSH, Tenki, and UCloud expose **Max Artifacts** and **Max Artifact Bytes** settings. Other providers or Executor Manager versions may enforce their own limits. Supported returned extensions are `.csv`, `.html`, `.jpeg`, `.jpg`, `.json`, `.pdf`, `.png`, and `.svg`.

RAGFlow stores returned files in the bucket selected by `SANDBOX_ARTIFACT_BUCKET`, which defaults to `sandbox-artifacts`. Configure retention and deletion through the object-storage service; RAGFlow does not define a separate Sandbox artifact retention setting here.

## Security

### Can the Executor Manager endpoint be exposed publicly?

Do not expose it directly to the public internet. The endpoint accepts code-execution requests and the Manager has privileged Docker access in the supplied deployment. Keep it on a private network or behind a protected TLS reverse proxy, require a strong API token, restrict allowed source addresses, and monitor authentication failures.

### Why should Runner network access remain disabled by default?

Network-disabled Runners cannot directly reach internal services, cloud metadata endpoints, other containers, or the internet. This reduces the effect of malicious or accidental code. Prefer trusted images with dependencies pre-installed. Enable outbound access only for a defined workload and protect it with egress controls.
