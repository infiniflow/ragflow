---
sidebar_position: 2
slug: /team_management
sidebar_custom_props: {
  categoryIcon: LucideLogOut
}
---
# Team management

Manage team members, team invitations, joined teams, departments, and groups.

---

By default, each RAGFlow user is assigned a single team named after the user's name. RAGFlow allows you to invite RAGFlow users to your team. Your team members can help you:

- Upload documents to your shared datasets.
- Parse documents in your shared datasets.
- Use your shared Agents.

:::tip NOTE
- Team members cannot invite users to your team. Only the team owner can invite users or remove members.
- Sharing added models with team members is only available in RAGFlow Enterprise.
:::

## Enter the Team page

Click your avatar in the upper-right corner of the page, then select **Team** in the left-side panel to access the **Team** page.

The page title is displayed as the current user's nickname plus **workspace**. On this page, you can view members in the current workspace and teams that the current user has joined.

## View current workspace members

The **Team members** list displays members in the current workspace. The owner is not included in the ordinary member list returned by the backend member query.

The list contains the following fields:

| Field | Description |
| --- | --- |
| Name | The member's nickname. |
| Update date | The time when the membership record was last updated. |
| Email | The member's email address. |
| Role | The member's team role. `normal` is displayed as **Member**, `invite` is displayed as **Invite**, and `owner` is displayed as **Owner**. |
| Action | Operations available for the current member, such as removing a member when permitted. |

You can search members by nickname or email address, and sort members by **Update date**.

Only the workspace owner can query and manage the full member list for the workspace.

## Invite members

The team owner can invite RAGFlow users to join the team.

To invite a member:

1. Open the **Team** page.
2. In the **Team members** area, click **Invite**.
3. Enter the email address of the user to invite.
4. Click **OK**.

The email address must belong to an existing RAGFlow user. If the email address does not match any user, the system returns a user-not-found error. If the user has already joined the team or has already been invited, the system rejects the duplicate invitation.

After the invitation is created, the membership record is stored with the `invite` role. The invited user must accept the invitation before becoming a normal team member.

## Remove members

The team owner can remove members from the team.

To remove a member:

1. Open the **Team** page.
2. Find the member in the **Team members** list.
3. Click the delete action in the **Action** column.
4. Confirm the operation.

The workspace owner cannot be removed from the workspace. Removing a member does not delete the member's personal resources, but it affects the member's access to resources that are shared only within the team.

## View joined teams

The **Joined teams** list displays other workspaces that the current user has joined or has been invited to join.

The list contains the following fields:

| Field | Description |
| --- | --- |
| Name | The workspace owner's nickname. |
| Update date | The time when the membership record was last updated. |
| Email | The workspace owner's email address. |
| Action | Operations available for the joined team, such as accepting, declining, or leaving. |

You can search joined teams by the owner nickname or email address, and sort joined teams by **Update date**.

## Accept or decline an invitation

When another team owner invites you to join a team, you can process the invitation on the **Team** page.

If the membership role is `invite`, the **Joined teams** list displays **Agree** and **Refuse** operations.

- Click **Agree** to accept the invitation. The system changes the relationship role from `invite` to `normal`.
- Click **Refuse** to decline the invitation. The system deletes the invitation relationship.

Only an invitation relationship can be accepted. After accepting an invitation, you can access team-shared resources in that workspace when those resources also satisfy their sharing scope and operation permission rules.

## Leave a joined team

A user with the `normal` role can leave a joined team if the team is not the user's own workspace.

To leave a joined team:

1. Open the **Team** page.
2. Find the target team in the **Joined teams** list.
3. Click the leave action.
4. Confirm the operation.

After leaving a team, the team no longer appears as a joined team. The user can no longer access resources that are only shared within that team. The user's own workspace does not provide a leave operation.

## Manage departments

In RAGFlow Enterprise, administrators can create departments according to the enterprise's actual organization structure. Departments can be used as collaborator objects when configuring resource permissions.

### Add or remove department members

Administrators can add users to a department or remove users from a department. Members in a department can inherit the corresponding resource permission when the department is added as a collaborator.

### Move departments

Administrators can move departments to adjust the organization hierarchy. Department names and levels may vary between enterprises.

### Manage groups

In RAGFlow Enterprise, administrators can create groups to organize users for permission configuration and resource collaboration. A group can be added as a collaborator, and users in the group can inherit the group's corresponding permission.

## Resource sharing from the team perspective

Joining a team does not automatically grant access to all resources in that team. The resource itself must be shared with the team or authorized to the user.

For knowledge bases, the **Permissions** field can be set to **Only me** or **Team**. The default value is **Only me**. When the value is changed to **Team**, joined team members can see and use the knowledge base according to permission rules.

Knowledge base access follows these rules:

- If the current user belongs to the owner tenant, access is allowed.
- If the knowledge base permission is **Only me**, non-owners cannot access it.
- If the knowledge base permission is **Team** and the user belongs to the tenant, access is allowed.
- Deleting a knowledge base is stricter and generally requires the current user to be the creator.

Agent sharing also uses **Only me** and **Team**. The permission field is located in the Agent settings dialog. The default value is **Only me**.

Memory sharing is configured in the memory's advanced settings. **Only me** maps to `me`, and **Team** maps to `team`. The backend also defines an `all` value, but the UI exposes **Only me** and **Team**.

## Enterprise role permissions

Enterprise role permissions are separate from the team page.

The Enterprise role page supports listing roles, creating roles, editing role descriptions, deleting roles, and configuring available actions by resource type. The fixed action types include **enable**, **read**, **write**, and **share**.

Enterprise roles are different from team roles such as `owner`, `invite`, and `normal`.

## FAQ

### Why does inviting a member fail?

Common causes include:

- The email address does not belong to an existing RAGFlow user.
- The user has already joined the team.
- The user has already been invited.
- The current user is not the team owner.

### Why can't an invited user see team resources?

The invited user must first click **Agree** to join the team. The target resource must also be set to **Team** or otherwise authorized to the user.

### Why can a team member only see some knowledge bases after joining a team?

Only knowledge bases whose permissions allow team access are visible to joined team members.

### Why can't a team member delete a shared knowledge base?

Deleting a shared knowledge base generally requires the current user to be the creator of the knowledge base. Team sharing grants access and use permissions, but it does not automatically grant delete permission.

### Are Team roles the same as Enterprise roles?

No. Team roles describe membership in a workspace. Enterprise roles describe administrative resource action permissions.