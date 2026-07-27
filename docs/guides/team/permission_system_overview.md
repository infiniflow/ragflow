---
sidebar_position: 1
slug: /permission_system_overview
sidebar_custom_props: {
  categoryIcon: LucideUserCog
}
---
# Permission system overview

RAGFlow manages team collaboration and resource permissions through three layers: team membership, resource sharing scope, and resource operation permissions.

The **Team and Permissions** section describes how users enter the team page, view members in the current workspace, invite or remove members, accept or decline invitations, leave joined teams, and configure visible scopes for shared resources such as knowledge bases, Agents, chats, Search applications, and memories.

## Team member management

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

## Team and resource permissions

Team membership and resource permissions are separate concepts.

A user may be a member of a team, but that does not mean the user can access every resource in the team. A resource must also be shared with the team or explicitly authorized to the user through collaborator permissions.

For common resource sharing scopes:

| Sharing scope | Description |
| --- | --- |
| Only me / me | Only the owner can access the resource. |
| Team / team | Users who have joined the team may access the resource if the resource-level permission rules allow it. |

When a team owner invites a user, the relationship is first created with the `invite` role. After the invited user accepts the invitation, the role changes to `normal`.

## Resource sharing scope

Sharing scope controls the visible scope of a resource, that is, which users can access the current resource.

In the open-source edition, supported resources can usually be set to **Only me** or **Team**. In RAGFlow Enterprise, resources can be authorized more finely by adding collaborators such as members, departments, or groups.

A resource set to **Team** is not necessarily visible to every user in every case. The system still checks whether the user belongs to the resource owner's team, whether the resource supports team sharing, and whether the user's operation is allowed by the corresponding permission rules.

## Resource operation permissions

Resource operation permissions define what a user can do after the resource becomes visible to the user.

Operation permissions can apply to the following resource types:

- Knowledge bases and knowledge base documents.
- Chat applications.
- Agents.
- MCP servers.
- Memories and memory messages.
- Search applications.
- Model providers, model instances, and default model configuration.

Different resources use different permission levels. Common permission levels include **Read permission**, **Write permission**, and **Manage permission**. The resource owner always has the highest permission on the resource.

## Permission effective rules

When a user performs an operation on a resource, RAGFlow evaluates permissions in the following order:

1. Whether the user has formally joined the team that owns the resource.
2. Whether the resource sharing scope allows team access.
3. Whether the user has knowledge base-level, resource-level, or collaborator-level **Read**, **Write**, or **Manage** permission.
4. Whether the target document has additional document-level **Read**, **Write**, or **Manage** permission requirements.
5. Whether the current operation is allowed by the permission table for that resource type.

The resource owner always has all permissions. If a resource is set to **Only me**, only the owner can access it. If a resource is set to **Team**, only team members who meet the permission conditions can access it. In RAGFlow Enterprise, the system also checks collaborator configuration for members, departments, and groups.

If finer-grained permissions exist, such as document-level permissions in a knowledge base, those permissions continue to apply on top of resource access permission.