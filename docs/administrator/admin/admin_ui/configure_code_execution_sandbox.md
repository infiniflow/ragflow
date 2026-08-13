---
sidebar_position: 4
sidebar_label: Configure Code Execution Sandbox
title: Configure Code Execution Sandbox
---

# Configure Code Execution Sandbox

## Select a Sandbox Provider

If your organization uses the **Code** component in Agent, administrators must configure the code execution sandbox in **Sandbox settings**.

The current page supports the following `Provider` options:

| Provider | Description |
| --- | --- |
| `Local` | Execute code directly in the current host process. |
| `Self-Managed` | Use Daytona or Docker for localized deployment. |
| `SSH` | Execute code on a remote machine through SSH. |
| `AliyunCodeInterpreter` | Use Alibaba Cloud Function Compute Code Interpreter. |
| `E2B` | Use E2B Cloud Code Execution Sandboxes. |
| `UCloud Agent Sandbox` | Run code in disposable UCloud cloud sandboxes. |

After selecting a `Provider`, the page displays the corresponding configuration area.

![Select Sandbox Provider](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/select_sandbox_provider.jpg)

**Caution:** Sandbox configuration affects not only connection availability, but also code isolation, network access, file access, and runtime resource limits. In production environments, prefer `Self-Managed`, cloud, or independent remote execution solutions with isolation capabilities. Direct use of `Local` is not recommended.

| Provider | Applicable scenario | Main characteristics | Recommendation |
| --- | --- | --- | --- |
| `Local` | Local development and functional debugging | Runs code directly on the current host and is easy to configure. | Recommended only for controlled development and test environments. |
| `Self-Managed` | Enterprise intranet and private deployment | Uses a self-managed sandbox service to execute code. Data and runtime environments can be controlled independently. | Suitable for production environments and scenarios with high data security requirements. |
| `SSH` | Existing independent execution servers | Executes code on a remote host through SSH. | Suitable for reusing existing servers or custom runtime environments. |
| `AliyunCodeInterpreter` | Alibaba Cloud-related services | Uses a cloud service to provide the code execution environment, making elastic scaling easier. | Suitable for organizations already using the corresponding cloud service. |
| `E2B` | Fast access to a cloud sandbox | Executes code in an isolated environment provided by E2B. | Suitable for scenarios that need quick use without deployment. |
| `UCloud Agent Sandbox` | UCloud-hosted agent workloads | Creates a disposable sandbox from a template containing Python and Node.js. | Suitable for teams using UCloud or needing managed sandboxes in China or North America. |

## Test and Save Configuration

When configuring a sandbox, first fill in the connection information and runtime parameters required by the selected `Provider`, then use **Test connection** to verify availability. After the test succeeds, click **Save**.

## Local Runtime Configuration

`Local configuration` configures the local code execution environment. The system uses the following configuration when executing code tasks such as Python and Node.js.

1. Go to **Local Configuration**.
2. Configure each parameter according to the actual runtime environment.
3. Click **Test connection** to test whether the configuration is correct.
4. After the test succeeds, click **Save**.

| Parameter | Description | Recommendation |
| --- | --- | --- |
| `Working Directory` | Working directory used during code execution, for temporary files generated during runtime. | Keep the default, or configure a local directory with read and write permissions. |
| `Python Binary` | Python executable name or path. | Usually `python3`. If using a virtual environment, enter the full Python path. |
| `Node.js Binary` | Node.js executable name or path. | Usually `node`. If installed elsewhere, enter the full path. |
| `Max Artifact Size (bytes)` | Maximum allowed size for a single generated file. | Configure according to business needs. The default usually satisfies common use. |
| `Max Artifacts` | Maximum number of files allowed in one run. | Keep the default to avoid generating too many temporary files. |
| `Max Memory (MB)` | Maximum memory available to a single code run. | Configure according to server resources. Increase it when resources are sufficient. |
| `Max Output (bytes)` | Maximum length of console output. | Keep the default to avoid oversized output. |
| `Timeout (seconds)` | Maximum runtime for a single code execution. | Set according to the business scenario. The task is terminated automatically after timeout. |

If Python or Node.js is not installed, or the executable path is wrong, the connection test fails.

## Self-Managed Configuration

`Self-Managed` configuration connects to a deployed `SandboxExecutorManager` service for remote sandbox execution. If the code execution service is deployed on an independent server or in a container, complete this configuration.

1. Go to **Self-Managed Configuration**.
2. Configure the code execution service connection information.
3. Click **Test connection** to test whether the connection is normal.
4. After the test succeeds, click **Save**.

| Parameter | Description | Recommendation |
| --- | --- | --- |
| `Executor Manager Endpoint` | Access address of the `SandboxExecutorManager` service. | Enter the HTTP address of the code execution service, such as `http://sandbox-executor-manager:9385` or `http://<SERVER_IP>:9385`. Make sure RAGFlow can access this address. |
| `Request Timeout (seconds)` | Timeout for requests to the code execution service. | Configure according to the network environment and code execution duration. The default 30 seconds is sufficient for most cases; increase it for large tasks. |

`Executor Manager Endpoint` must be the address of a deployed and normally running `SandboxExecutorManager` service.

`Deployment Defaults` displays the current default deployment parameters of `SandboxExecutorManager`.

## SSH Configuration

`SSH` configuration connects to a remote Linux host and executes Python, Node.js, and other code tasks on the remote server. After configuration, the system logs in to the specified server through SSH and executes code in the remote working directory.

1. Go to **System management > Model service > SSH Configuration**.
2. Fill in the remote server connection information.
3. Select the authentication method: password authentication or private-key authentication.
4. Configure the remote code runtime environment.
5. Click **Test connection** to test whether the connection is normal.
6. After the test succeeds, click **Save**.

### SSH Connection Configuration

| Parameter | Description | Recommendation |
| --- | --- | --- |
| `SSH Host` | Remote server IP address or host name. | For example, `192.168.1.10` or `server.example.com`. |
| `SSH Username` | User name used to log in to the remote server. | Enter a user with code execution permissions, such as `root`, `ubuntu`, or `ragflow`. |
| `SSH Port` | Port on which the SSH service listens. | The default is `22`. If the server uses another SSH port, enter the actual port. |

### Authentication

The system supports `Password` and `PrivateKey` authentication.

| Authentication method | Description | Recommendation |
| --- | --- | --- |
| `Password` | Uses the SSH user password for authentication. | Fill in `SSH Password`. |
| `PrivateKey` | Uses an SSH private key for authentication. | Fill in the SSH private key content. If the private key has a password, also fill in the corresponding `Passphrase`. Private-key authentication is recommended in production. |

### Execution

| Parameter | Description | Recommendation |
| --- | --- | --- |
| `Remote Workspace Root` | Working directory on the remote server for temporary files generated during code runs. | Configure a directory with read and write permissions, such as `/tmp` or `/home/ragflow/workspace`. |
| `Python Binary` | Python executable name or full path. | Usually `python3`. If using a virtual environment, enter the full Python path. |
| `Node.js Binary` | Node.js executable name or full path. | Usually `node`. If installed elsewhere, enter the full path. |
| `Max Artifact Bytes` | Maximum allowed size for a single generated file. | Keep the default or adjust according to business needs. |
| `Max Artifacts` | Maximum number of files allowed in one run. | Keep the default configuration. |
| `Max Output Bytes` | Maximum size of console output. | Keep the default. Output exceeding the limit is truncated. |
| `Timeout (seconds)` | Maximum runtime for a single code execution. | The default is 30 seconds. Increase it when execution takes longer. |

Make sure the remote server has SSH enabled and allows the current user to log in. The configured user must have read and write permissions on the remote working directory and be able to execute Python, Node.js, and other runtime environments. When using private-key authentication, make sure the private key matches the server configuration.

## Alibaba Cloud CodeInterpreter Configuration

Alibaba Cloud `CodeInterpreter` configuration connects to Alibaba Cloud CodeInterpreter. The system executes code tasks through this service.

1. Go to **AliyunCodeInterpreter Configuration**.
2. Fill in the connection information for Alibaba Cloud CodeInterpreter.
3. Click **Test connection** to test whether the connection is normal.
4. After the test succeeds, click **Save**.

| Parameter | Description | Recommendation |
| --- | --- | --- |
| `AccessKeyID` | Alibaba Cloud account `AccessKeyID` for authentication. | Enter an `AccessKeyID` with access to the CodeInterpreter service. |
| `AccessKeySecret` | Secret corresponding to `AccessKeyID`. | Enter the corresponding `AccessKeySecret` and keep it secure to avoid leakage. |
| `AccountID` | Alibaba Cloud account ID. | Enter the current Alibaba Cloud account ID. |
| `Region` | Region where the CodeInterpreter service is located. | Enter the actual deployment region, such as `cn-hangzhou`. |
| `TemplateName` | Template name used by CodeInterpreter. | Enter the created template name, such as `my-interpreter`. |
| `ExecutionTimeout (seconds)` | Maximum runtime allowed for a single code execution. | The default is 30 seconds. Adjust according to business needs. |

## E2B Configuration

`E2B` configuration connects to the E2B Cloud code execution service. After configuration, the system executes code tasks through the E2B cloud sandbox.

1. Go to **E2B**.
2. Fill in the E2B service connection information.
3. Click **Test connection** to test whether the connection is normal.
4. After the test succeeds, click **Save**.

| Parameter | Description | Recommendation |
| --- | --- | --- |
| `API Key` | API key provided by E2B Cloud for authentication. | Log in to the E2B platform, create the corresponding API key, and enter it. |
| `Region` | Region where the E2B service is located. | Enter the actual region, such as `us`. |
| `Request Timeout (seconds)` | Timeout for requests to the E2B service. | The default is 30 seconds. Increase it if the network is slow or execution takes longer. |

## UCloud Agent Sandbox Configuration

`UCloud Agent Sandbox` creates a fresh cloud sandbox for each code execution and destroys it when execution finishes. RAGFlow uses UCloud's native SDK and supports Python and JavaScript with the built-in `base` template.

1. Obtain an API key from [UCloud ModelVerse API Keys](https://astraflow.ucloud.cn/modelverse/api-keys).
2. Go to **UCloud Agent Sandbox Configuration**.
3. Enter the API key and select the desired region.
4. Click **Test connection**, then click **Save** after the test succeeds.

| Parameter | Description | Recommendation |
| --- | --- | --- |
| `API Key` | UCloud Agent Sandbox API key used for authentication. | Keep it secret and rotate it according to your organization's credential policy. |
| `Region` | Sandbox region. Supported values include `cn-wlcb` and `us-ca`. | Keep `cn-wlcb` unless workloads should run in North America. |
| `Domain` | Optional sandbox domain override. | Leave empty to derive `<region>.sandbox.ucloudai.com` from `Region`. |
| `API URL` | Optional control-plane endpoint override. | Leave empty for the public service endpoint. |
| `Template` | UCloud sandbox template. | Use `base`, which includes both Python and Node.js. |
| `Allow Internet Access` | Allows executed code to make outbound network requests. | Disabled by default. Enable only when the code needs external services or package downloads. |
| `Use Insecure HTTP` | Uses HTTP rather than HTTPS. | Keep disabled except in a trusted private test deployment. |
| `Execution Timeout` | Maximum duration of one code execution. | The default is 30 seconds. |
| `Sandbox Lifetime` | Maximum lifetime of the disposable sandbox. | The default is 300 seconds and is extended when needed for an allowed execution. |
| Output and artifact limits | Bound console output and files returned from `artifacts/`. | Keep the defaults unless the workload has a known need for larger results. |

Files written under the execution workspace's `artifacts/` directory are returned to RAGFlow. Supported artifact extensions are `.csv`, `.html`, `.jpeg`, `.jpg`, `.json`, `.pdf`, `.png`, and `.svg`.

See the [UCloud Agent Sandbox prerequisites](https://astraflow.ucloud.cn/docs/agent-sandbox/product/prerequisites) and [region documentation](https://astraflow.ucloud.cn/docs/agent-sandbox/product/region) for service-side details.
