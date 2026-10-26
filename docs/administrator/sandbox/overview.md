---
sidebar_position: 0
title: Code Execution Sandbox
sidebar_label: Overview
slug: /code_execution_sandbox
---

# Code Execution Sandbox

RAGFlow Agents can use the **Code** component to run Python or JavaScript. Before users can run code, an administrator must select and configure a sandbox provider.

The provider determines where the code runs. Depending on your choice, RAGFlow can send code to containers managed in your own environment, an existing SSH server, or a managed cloud sandbox.

:::warning System-wide setting
The active sandbox provider is a system-wide setting. Changing it affects subsequent Code component runs across the RAGFlow deployment. Test the new configuration before saving it.
:::

For instructions on using the Code component after an administrator has configured a provider, see [Code component](../../guides/agent/agent_workflow/data_manipulation_components.md).

## Choose a sandbox provider

Choose the provider that matches where you want code to run and who should manage the execution environment.

| Provider | Where code runs | Choose it when |
| --- | --- | --- |
| [**Self-Managed**](./quickstart_with_self_managed.md) | Python or Node.js Runner containers managed by Sandbox Executor Manager in your environment | You want to keep code execution under your operational control. This is the default option and the focus of the Self-Managed guides. |
| [**SSH**](./configure_ssh_and_cloud_providers.md#configure-ssh) | An existing remote Linux host | You already operate a suitable execution server with Python or Node.js and can manage its SSH access and host keys. |
| [**Aliyun Code Interpreter**](./configure_ssh_and_cloud_providers.md#configure-aliyun-code-interpreter) | Alibaba Cloud Code Interpreter | Your organization uses Alibaba Cloud and wants a managed execution environment. |
| [**E2B**](./configure_ssh_and_cloud_providers.md#configure-e2b) | E2B cloud sandboxes | You want an externally managed cloud sandbox without operating local Runner containers. |
| [**Tenki**](./configure_ssh_and_cloud_providers.md#configure-tenki) | A disposable Tenki microVM created for each execution | You want each run to use an isolated, short-lived cloud microVM. |
| [**UCloud Agent Sandbox**](./configure_ssh_and_cloud_providers.md#configure-ucloud-agent-sandbox) | A disposable UCloud Agent Sandbox | You want a managed cloud sandbox in a supported UCloud region. |

:::note Before choosing a cloud provider
Confirm that your RAGFlow deployment can reach the provider's API, and that your account has valid credentials, sufficient quota, and access to the required region.
:::

## Recommended path for private deployments

For most private or on-premises deployments, start with **Self-Managed**. In the standard deployment:

1. RAGFlow sends the execution request to Sandbox Executor Manager.
2. Sandbox Executor Manager selects a Python or Node.js Runner container.
3. The Runner executes the code and returns its output and generated files to RAGFlow.

RAGFlow can run with Docker Compose or directly from source while Sandbox Executor Manager and its Runners use Docker. Follow [Quickstart with Self-Managed](./quickstart_with_self_managed.md) for both deployment patterns.

## Next steps

- Get started with [Quickstart with Self-Managed](./quickstart_with_self_managed.md).
- To use SSH or a cloud service, see [Configure SSH and Cloud Providers](./configure_ssh_and_cloud_providers.md).
- For maintenance and troubleshooting, see [Manage Self-Managed Sandbox](./manage_self_managed_sandbox.md) and [FAQ](./faq.md).
