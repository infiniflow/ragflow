---
sidebar_position: 5
title: Code Execution Sandbox FAQ
sidebar_label: FAQ
slug: /sandbox_faq
---

# Code Execution Sandbox FAQ

## Provider selection and configuration

- Which provider should I choose?
- Does **Test connection** save the configuration?
- When does a newly saved provider become active?

## Self-Managed startup and connection

- Why did `sandbox-executor-manager` not start?
- Which endpoint should Docker and source deployments use?
- Why does the connection return `Connection refused`, DNS errors, a timeout, or `401 Unauthorized`?
- Why does the health check succeed while CodeExec fails?

## Self-Managed execution

- Why did a Runner image fail to start?
- Why is the Docker API version incompatible?
- Why did execution exceed the memory or timeout limit?
- Why can executed code not access the internet?

## SSH

### Why does SSH return `knownhosts: key is unknown`?

The network connection reached the SSH server, but RAGFlow could not find a trusted host key for that host and port. RAGFlow rejects unknown keys before attempting password or private-key authentication.

For a non-default SSH port, the `known_hosts` entry must include the port—for example, `[192.168.1.5]:2222`. Verify the server fingerprint, add the key to the file used by RAGFlow, and ensure that the file is visible to the RAGFlow process or container.

For Docker and source deployment instructions, see [Prepare the SSH `known_hosts` file](./configure_ssh_and_cloud_providers.md#prepare-the-ssh-known_hosts-file).

If the message reports a host-key mismatch instead, verify whether the server key was intentionally changed before replacing the existing entry. An unexpected key change can indicate that the connection is being intercepted.

- Why does authentication fail?
- Why can RAGFlow not create the remote workspace?
- Why can the remote host not find Python or Node.js?

## Cloud providers

- Why is the API credential rejected?
- Why does sandbox creation time out?
- How do region, quota, network, and account limits affect execution?

## Artifacts

- Why was a generated file not returned?
- Where should code write generated files?
- What size, count, type, and retention limits apply?

## Security

- Is gVisor required?
- Can the Executor Manager endpoint be exposed publicly?
- Why should Runner network access remain disabled by default?
