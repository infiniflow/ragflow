---
sidebar_position: 2
sidebar_label: Team and Resource Permissions
title: Team and Resource Permissions
---

# Team and Resource Permissions

Team membership and resource permissions are separate concepts.

A user may be a member of a team, but that does not mean the user can access every resource in the team. A resource must also be shared with the team or explicitly authorized to the user through collaborator permissions.

For common resource sharing scopes:

| Sharing scope | Description |
| --- | --- |
| Only me / me | Only the owner can access the resource. |
| Team / team | Users who have joined the team may access the resource if the resource-level permission rules allow it. |

When a team owner invites a user, the relationship is first created with the `invite` role. After the invited user accepts the invitation, the role changes to `normal`.
