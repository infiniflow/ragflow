---
sidebar_position: 8
sidebar_label: "Permission effective rules"
---

## Permission effective rules

Resource permissions take effect according to the following general rules:

- The resource owner always has all permissions.
- If the sharing scope is **Only me**, only the owner can access the resource.
- If the sharing scope is **Team**, only team members who meet the permission conditions can access the resource.
- In RAGFlow Enterprise, the system checks collaborator configuration for **Member**, **Department**, and **Group** collaborators.
- If finer-grained permissions exist, such as document-level permissions in a knowledge base, those permissions continue to apply on top of resource access permission.
