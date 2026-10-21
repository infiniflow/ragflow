---
sidebar_position: 4
sidebar_label: Configure Code Execution Sandbox
title: Configure Code Execution Sandbox
---

# Configure Code Execution Sandbox

Administrators configure the active code execution provider in **Admin > Sandbox Settings**. For provider selection, deployment, security, and troubleshooting guidance, start with [Code Execution Sandbox](../../sandbox/overview.md).

:::caution System-wide setting
Saving a provider makes it active for subsequent Code component runs across this RAGFlow deployment. Test the configuration before replacing a provider that is already in use.
:::

## Select a provider

The page supports:

| Provider | Detailed instructions |
| --- | --- |
| **Self-Managed** | [Quickstart with Self-Managed](../../sandbox/quickstart_with_self_managed.md) |
| **SSH** | [Configure SSH](./configure_ssh_and_cloud_providers.md#configure-ssh) |
| **Aliyun Code Interpreter** | [Configure Aliyun Code Interpreter](./configure_ssh_and_cloud_providers.md#configure-aliyun-code-interpreter) |
| **E2B** | [Configure E2B](./configure_ssh_and_cloud_providers.md#configure-e2b) |
| **Tenki** | [Configure Tenki](./configure_ssh_and_cloud_providers.md#configure-tenki) |
| **UCloud Agent Sandbox** | [Configure UCloud Agent Sandbox](./configure_ssh_and_cloud_providers.md#configure-ucloud-agent-sandbox) |

## Test and save the configuration

1. Select a provider and complete its fields.
2. Select **Test connection**. The test uses the values currently shown in the form and does not save them.
3. Confirm that the result shows `PASSED`.
4. Select **Save** to make the provider active.

The connection test runs a small Python program. For Self-Managed, this verifies that Executor Manager can start a Python Runner; `/healthz` alone is not sufficient. Cloud tests may consume account quota or incur short-lived charges.

## Self-Managed fields

The Admin page configures only the connection from RAGFlow to Executor Manager:

| Field | Purpose |
| --- | --- |
| **Executor Manager Endpoint** | Manager address reachable from the RAGFlow process or container. |
| **Executor Manager API Token** | Shared token that must match `SANDBOX_EXECUTOR_MANAGER_API_TOKEN` on the Manager. |
| **Request Timeout (seconds)** | Maximum time RAGFlow waits for one Manager request. |

Runner images, capacity, memory, service timeout, and network access are deployment settings. Manage them through [Manage Self-Managed Sandbox](../../sandbox/manage_self_managed_sandbox.md).

If a connection or execution test fails, see the [Code Execution Sandbox FAQ](../../sandbox/faq.md).
