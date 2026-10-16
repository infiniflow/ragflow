---
sidebar_position: 3
title: Configure SSH and Cloud Providers
sidebar_label: Configure SSH and Cloud Providers
slug: /configure_ssh_and_cloud_sandbox_providers
---

# Configure SSH and Cloud Providers

Use this guide to connect RAGFlow to an existing SSH host or a managed cloud sandbox. These providers do not require the Self-Managed Sandbox Executor Manager.

:::caution Provider changes are system-wide
Saving a provider configuration makes it the active code execution provider for this RAGFlow deployment. Test the new configuration before replacing a provider that is already in use.
:::

## Configuration workflow

Use the same workflow for every provider:

1. Prepare the remote host or cloud account, credentials, and network access.
2. Open **Admin > Sandbox Settings**.
3. Select the provider and complete its settings.
4. Select **Test connection**.
5. Confirm that the result shows `PASSED`, then select **Save**.
6. Run the test in [Verify code execution](#verify-code-execution).

:::note
**Test connection** uses the values currently entered in the form; you do not need to save them first. The test creates a temporary Python execution environment, runs a small diagnostic, and then attempts to clean up the environment. Cloud providers may consume account quota or incur short-lived charges during this test.

A cleanup failure is written to the RAGFlow logs and may not change a successful test result. Check the provider console for leftover resources if the logs report a cleanup failure.
:::

## Configure SSH

The SSH provider runs code on a Linux host that you operate. RAGFlow connects to the host for each execution, creates a temporary workspace, uploads and runs the script, collects generated artifacts, and removes the workspace.

:::caution SSH does not provide strong isolation
Code runs with the permissions of the configured SSH user. Use a dedicated execution host and a low-privilege account, and restrict its filesystem and network access. Do not run untrusted code as `root` or on a server that hosts production workloads.
:::

### Before you begin

Make sure that:

- RAGFlow can reach the SSH host and port.
- The SSH service accepts either password or private-key authentication for the selected user.
- The user can create directories and files under the configured workspace root.
- Python 3 is installed and available through the configured **Python Binary**. The connection test requires Python.
- Node.js is installed and available through the configured **Node.js Binary** if users will run JavaScript.
- The SSH server's host key has been verified and added to a trusted `known_hosts` file.

### Prepare the SSH `known_hosts` file

RAGFlow verifies the SSH server's host key before sending credentials or running code. Confirm the server's host-key fingerprint with the server administrator, then add the verified key to the `known_hosts` file used by RAGFlow.

For a server that listens on a non-default port, include the port when collecting the key. Replace `SSH_HOST` and `SSH_PORT` with the values configured in **Admin > Sandbox Settings**. For example, if the actual host is `sandbox.example.com` and the port is `2222`, use `[sandbox.example.com]:2222`.

The `[SSH_HOST]:SSH_PORT` format is for non-default ports. For port `22`, use `SSH_HOST` without brackets or a port in the `ssh-keygen -F` and `ssh-keygen -R` commands.

First, create a unique temporary file and save the key to it:

```bash
ssh_key_file=$(mktemp /tmp/ragflow-ssh-host-key.XXXXXX)
trap 'rm -f "$ssh_key_file"' EXIT
ssh-keyscan -p SSH_PORT SSH_HOST > "$ssh_key_file"
```

Continue the following commands in the same terminal session so that `ssh_key_file` remains available. `mktemp` avoids overwriting another administrator's scan and prevents the command from following a predictable file name prepared by another local user.

Display the scanned fingerprints:

```bash
ssh-keygen -lf "$ssh_key_file"
```

`ssh-keyscan` may return more than one host-key type. Compare every fingerprint shown with the fingerprints supplied by the server administrator. Continue only if every key in the temporary file is verified.

If the same host and port already have an entry, inspect it first:

```bash
ssh-keygen -F '[SSH_HOST]:SSH_PORT' -f ~/.ssh/known_hosts
```

After confirming that the verified new keys are legitimate replacements, remove the old entry before appending them:

```bash
ssh-keygen -R '[SSH_HOST]:SSH_PORT' -f ~/.ssh/known_hosts
```

Add the verified keys:

```bash
mkdir -p ~/.ssh
chmod 700 ~/.ssh
cat "$ssh_key_file" >> ~/.ssh/known_hosts
chmod 600 ~/.ssh/known_hosts
rm -f "$ssh_key_file"
trap - EXIT
```

For port `22`, the entry identifies the server as `SSH_HOST`. For a non-default port, it uses `[SSH_HOST]:SSH_PORT`. Never add a host key that has not been independently verified.

#### Docker deployment

Set the following values in `docker/.env`:

```dotenv
RAGFLOW_SSH_KNOWN_HOSTS=${HOME}/.ssh/known_hosts
SSH_KNOWN_HOSTS=/etc/ragflow/ssh_known_hosts
```

The variables refer to different locations:

- `RAGFLOW_SSH_KNOWN_HOSTS` is the `known_hosts` file on the Docker host.
- `SSH_KNOWN_HOSTS` is the path at which RAGFlow reads that file inside its container.

In `docker/docker-compose.yml`, uncomment the following volume mount for the RAGFlow service:

```yaml
- ${RAGFLOW_SSH_KNOWN_HOSTS:-${HOME}/.ssh/known_hosts}:${SSH_KNOWN_HOSTS:-/etc/ragflow/ssh_known_hosts}:ro
```

Make sure the host file exists, then apply the changes to RAGFlow:

```bash
cd docker
docker compose up -d ragflow-cpu
```

In **SSH known_hosts File**, enter the container path: `/etc/ragflow/ssh_known_hosts`.

#### Source deployment

When RAGFlow runs from source, it checks the `~/.ssh/known_hosts` file of the operating-system user running RAGFlow. To use another file, set `SSH_KNOWN_HOSTS` to its path before starting RAGFlow. `RAGFLOW_SSH_KNOWN_HOSTS` is only used by Docker Compose and is not required for a source deployment.

In **SSH known_hosts File**, enter the path that is visible to the RAGFlow process.

### Configure the connection

In **Admin > Sandbox Settings**, select **SSH** and configure:

| Setting | Required | Value |
| --- | --- | --- |
| **SSH Host** | Yes | Hostname or IP address of the remote execution server. Do not include a URL scheme or port. |
| **SSH Port** | Yes | SSH port. The default is `22`. |
| **SSH Username** | Yes | Remote user that will run the code. |
| **SSH known_hosts File** | No | Path to an OpenSSH-format `known_hosts` file visible to the RAGFlow process or container. When empty, RAGFlow uses the running user's `~/.ssh/known_hosts`. Unknown host keys are rejected. |

### Choose an authentication method

Configure one of the following methods:

| Method | Settings |
| --- | --- |
| Password | Enter **SSH Password**. |
| Private key | Paste the PEM-formatted key into **SSH Private Key**, or enter a private-key file path that is readable by the RAGFlow process. If the key is encrypted, also enter **Private Key Passphrase**. |

Private-key authentication is recommended for production. Do not configure both methods; selecting one method in the Admin UI clears fields belonging to the other.

For a Docker deployment, pasting the PEM content is the simplest option. A private-key path must refer to a file inside the RAGFlow container, not an unmounted path on the Docker host. If you use a path, mount the key into the RAGFlow service as a read-only file and enter its container path in **SSH Private Key**.

### Configure the execution environment

The defaults are appropriate for an initial test:

| Setting | Default | Description |
| --- | --- | --- |
| **Python Binary** | `python3` | Python executable name or absolute path on the SSH host. |
| **Node.js Binary** | `node` | Node.js executable name or absolute path on the SSH host. |
| **Remote Workspace Root** | `/tmp` | Writable directory under which RAGFlow creates a temporary workspace for each execution. |
| **Timeout (seconds)** | `30` | Maximum SSH execution time for one run. Range: 1–600 seconds. |
| **Max Output Bytes** | `1048576` | Maximum combined stdout and stderr size. |
| **Max Artifacts** | `20` | Maximum number of files collected from the execution's `artifacts/` directory. |
| **Max Artifact Bytes** | `10485760` | Maximum size of one artifact file. |

Select **Test connection**. Confirm that the result shows `PASSED`, then select **Save**. If the test returns `knownhosts: key is unknown`, see [Why does SSH return `knownhosts: key is unknown`?](./faq.md#why-does-ssh-return-knownhosts-key-is-unknown).

## Configure Aliyun Code Interpreter

Prepare a RAM user with Code Interpreter permissions and [create an Alibaba Cloud AccessKey pair](https://help.aliyun.com/en/ram/user-guide/create-an-accesskey-pair). Then select **Aliyun Code Interpreter** in **Admin > Sandbox Settings** and configure:

| Setting | Required | Default | Description |
| --- | --- | --- | --- |
| **Access Key ID** | Yes | — | Alibaba Cloud AccessKey ID. |
| **Access Key Secret** | Yes | — | Secret associated with the AccessKey ID. |
| **Account ID** | Yes | — | Alibaba Cloud primary account ID used for API requests. |
| **Region** | No | `cn-hangzhou` | Code Interpreter region. The UI also supports `cn-beijing`, `cn-shanghai`, `cn-shenzhen`, and `cn-guangzhou`. |
| **Template Name** | No | — | Existing Code Interpreter template. When empty, RAGFlow uses a language-specific default template and creates it if it does not exist. |
| **Execution Timeout (seconds)** | No | `30` | Maximum execution time. Alibaba Cloud limits this value to 30 seconds. |

## Configure E2B

Create an API key in the [E2B dashboard](https://e2b.dev/dashboard). Then select **E2B** in **Admin > Sandbox Settings** and configure:

| Setting | Required | Default | Description |
| --- | --- | --- | --- |
| **API Key** | Yes | — | E2B API key, normally beginning with `e2b_sk_`. |
| **Region** | No | `us` | E2B service region. `us` works with the Admin settings alone. Using `eu` also requires `E2B_DOMAIN` in the RAGFlow runtime environment. |
| **Request Timeout (seconds)** | No | `30` | Maximum time RAGFlow waits for an E2B request. Range: 5–300 seconds. |

For the `eu` region, obtain the domain from E2B. For Docker deployments, set `E2B_DOMAIN` in `docker/.env`, then apply the change from the RAGFlow repository root:

```bash
cd docker
docker compose up -d ragflow-cpu
```

For source deployments, set `E2B_DOMAIN` in the RAGFlow process environment and restart RAGFlow. This value is not configured in Admin.

## Configure Tenki

Create an API key in the [Tenki console](https://app.tenki.cloud). For provider setup, see the [Tenki Sandbox Quickstart](https://tenki.cloud/docs/sandbox/quickstart). Then select **Tenki** in **Admin > Sandbox Settings** and configure:

| Setting | Required | Default | Description |
| --- | --- | --- | --- |
| **API Key** | Yes | — | Normally begins with `tk_`. |
| **API Endpoint** | No | Tenki default | Override the Tenki API endpoint. Leave empty for the hosted service. |
| **Sandbox Image** | No | Tenki default | Custom images must include the required runtime. |
| **Allow Outbound Network** | No | Disabled | Enable only when required. |
| **Timeout (seconds)** | No | `30` | Range: 1–600. |
| **Max Sandbox Lifetime (seconds)** | No | `3600` | Range: 60–86400. |
| **vCPU Cores** | No | `0` | `0` uses the provider default; maximum: 16. |
| **Memory (MB)** | No | `0` | `0` uses the provider default. |
| **Disk Size (GB)** | No | `0` | `0` uses the provider default; maximum: 100. |
| **Max Output Bytes** | No | `1048576` | Combined stdout and stderr. |
| **Max Artifacts** | No | `20` | Number of returned files. |
| **Max Artifact Bytes** | No | `10485760` | Per returned file. |

Keep outbound networking disabled unless executed code requires it.

## Configure UCloud Agent Sandbox

Obtain an API key from [UCloud ModelVerse API Keys](https://astraflow.ucloud.cn/modelverse/api-keys). Then select **UCloud Agent Sandbox** in **Admin > Sandbox Settings** and configure:

| Setting | Required | Default | Description |
| --- | --- | --- | --- |
| **API Key** | Yes | — | UCloud Agent Sandbox API key. |
| **Region** | No | `cn-wlcb` | For example, `cn-wlcb` or `us-ca`. |
| **Domain** | No | Derived from Region | Override only when required. |
| **API URL** | No | Public service endpoint | Override only when required. |
| **Template** | No | `base` | The default includes Python and Node.js. |
| **Allow Internet Access** | No | Disabled | Enable only when required. |
| **Use Insecure HTTP** | No | Disabled | Use only in a trusted private environment. |
| **Execution Timeout (seconds)** | No | `30` | Range: 1–600. |
| **Sandbox Lifetime (seconds)** | No | `300` | Range: 60–86400. |
| **Max Output Bytes** | No | `1048576` | Combined stdout and stderr. |
| **Max Artifacts** | No | `20` | Number of returned files. |
| **Max Artifact Bytes** | No | `10485760` | Per returned file. |

Keep internet access and insecure HTTP disabled unless required.

## Verify code execution

After saving the provider:

1. Create or open an Agent.
2. Add a **Code** component.
3. Set the language to Python.
4. Run the following code:

```python
def main():
    return "Sandbox is ready"
```

5. Confirm that the result is:

```text
Sandbox is ready
```

This verifies the complete path from RAGFlow to the selected provider. **Test connection** uses Python, so run a separate JavaScript Code component if JavaScript support is required.

When replacing a provider that is already in use, also test representative existing Agents. Confirm that their required packages, outbound network access, execution time, and artifact handling work in the new environment.

If the test fails, see the [Code Execution Sandbox FAQ](./faq.md). For SSH, also verify the host key, authentication method, remote workspace permissions, and runtime paths. For cloud providers, verify credentials, region availability, account quota, and outbound network access from the RAGFlow server.
