---
sidebar_position: 4
title: Manage Self-Managed Sandbox
sidebar_label: Manage Self-Managed Sandbox
slug: /manage_self_managed_sandbox
---

# Manage Self-Managed Sandbox

## Check service health

- Check the Executor Manager service state, health endpoint, and logs.
- Distinguish service health from successful CodeExec execution.

## Connect to a remote Executor Manager

- Describe the supported remote deployment topology and prerequisites.
- Configure a protected endpoint that is reachable from RAGFlow.
- Configure the matching API token in Executor Manager and RAGFlow Admin.
- Cover port exposure, TLS, firewall or private-network controls, health checks, and connection testing.

## Adjust execution capacity

- Configure the Runner pool size.
- Configure memory and execution limits.
- Explain when changes require a service restart.

## Manage Runner images

- Select Python and Node.js Runner images.
- Describe the update and verification workflow.
- Explain when to use a custom image for additional dependencies.

## Manage Runner network access

- Explain the default network-disabled behavior.
- Describe when outbound access may be required and its security implications.

## Manage generated artifacts

- Explain the `artifacts/` directory, size and count limits, storage bucket, and expiration.

## Rotate the API token

- Update the Executor Manager token.
- Update the matching Admin setting.
- Restart, test the connection, save, and verify CodeExec.

## Production checklist

- Protect the Executor Manager endpoint.
- Configure authentication, resource limits, network restrictions, trusted images, and artifact retention.

## Admin UI settings

- Link to the Admin UI page that describes the Self-Managed configuration fields.
