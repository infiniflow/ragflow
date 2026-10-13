---
sidebar_position: 3
title: Configure SSH and Cloud Providers
sidebar_label: Configure SSH and Cloud Providers
slug: /configure_ssh_and_cloud_sandbox_providers
---

# Configure SSH and Cloud Providers

## Configuration workflow

- Prepare the required host, account, credentials, and network access.
- Select the provider in **Admin > Sandbox Settings**.
- Run **Test connection**, save the configuration, and run a minimal CodeExec test.

## Configure SSH

- List remote-host prerequisites.
- Explain password and private-key authentication.
- Cover the remote workspace, Python/Node.js binaries, output limits, artifact limits, and timeout.

### Prepare the SSH `known_hosts` file

RAGFlow verifies the SSH server's host key before sending credentials or running code. Confirm the server's host-key fingerprint with the server administrator, then add the verified key to the `known_hosts` file used by RAGFlow.

For a server that listens on a non-default port, include the port when collecting the key. For example:

```bash
ssh-keyscan -p 2222 192.168.1.5 >> ~/.ssh/known_hosts
```

The resulting entry identifies the server as `[192.168.1.5]:2222`. Do not trust the collected key until you have compared its fingerprint with the fingerprint supplied by the server administrator.

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

Make sure the host file exists, then recreate the RAGFlow container so that the environment variable and read-only mount take effect.

#### Source deployment

When RAGFlow runs from source, it checks the `~/.ssh/known_hosts` file of the operating-system user running RAGFlow. To use another file, set `SSH_KNOWN_HOSTS` to its path before starting RAGFlow. `RAGFLOW_SSH_KNOWN_HOSTS` is only used by Docker Compose and is not required for a source deployment.

After preparing the file, return to **Admin > Sandbox Settings**, select **SSH**, and run **Test connection**.

## Configure Aliyun Code Interpreter

- List the required account, access key, region, template, and endpoint settings.
- Explain connection testing and credential security.

## Configure E2B

- List the required API credential, template, lifetime, resource, network, and output settings.
- Explain per-execution sandbox creation and cleanup.

## Configure Tenki

- List the required API key, project, image, resource, lifetime, and outbound-network settings.
- Explain per-execution microVM creation and cleanup.

## Configure UCloud Agent Sandbox

- List the required API key, region, template, domain, API URL, lifetime, network, and output settings.
- Explain per-execution sandbox creation and cleanup.

## Verify code execution

- Provide a common minimal Python test.
- Describe what a successful test confirms.
- Link failures to the relevant FAQ sections.
