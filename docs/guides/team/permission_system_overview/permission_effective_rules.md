---
sidebar_position: 5
sidebar_label: Permission Effective Rules
title: Permission Effective Rules
---

# Permission Effective Rules

When a user performs an operation on a resource, RAGFlow evaluates permissions in the following order:

1. Whether the user has formally joined the team that owns the resource.
2. Whether the resource sharing scope allows team access.
3. Whether the user has knowledge base-level, resource-level, or collaborator-level **Read**, **Write**, or **Manage** permission.
4. Whether the target document has additional document-level **Read**, **Write**, or **Manage** permission requirements.
5. Whether the current operation is allowed by the permission table for that resource type.

The resource owner always has all permissions. If a resource is set to **Only me**, only the owner can access it. If a resource is set to **Team**, only team members who meet the permission conditions can access it.

If finer-grained permissions exist, such as document-level permissions in a knowledge base, those permissions continue to apply on top of resource access permission.
