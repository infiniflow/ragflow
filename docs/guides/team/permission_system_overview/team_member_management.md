---
sidebar_position: 1
sidebar_label: Team Member Management
title: Team Member Management
---

# Team Member Management

Each user has a personal workspace. The team page of that workspace is displayed with the current user's nickname and **workspace** as its title.

The team page contains two main areas:

- **Team members**: Members in the current workspace.
- **Joined teams**: Other workspaces that the current user has joined or has been invited to join.

RAGFlow uses the `user_tenant` relationship to record team membership. Common role values include:

| Role | Description |
| --- | --- |
| owner | The owner of the workspace. The owner can invite members and remove members. |
| normal | A user who has accepted an invitation and joined the workspace. |
| invite | A user who has been invited but has not accepted the invitation. |
| admin | A backend role value used by the system. |

The team role is different from the Enterprise role management feature. Team roles define whether a user belongs to a workspace and what team-level actions the user can perform. Enterprise roles define action permissions for administrative resources.
