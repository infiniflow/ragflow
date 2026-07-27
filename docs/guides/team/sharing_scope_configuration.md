---
sidebar_position: 3
slug: /sharing_scope_configuration
sidebar_custom_props: {
  categoryIcon: LucideShare2
}
---
# Sharing scope configuration

Configure the visible scope of resources in RAGFlow.

---

Sharing scope controls which users can access the current resource. Resources that support team sharing, such as knowledge bases, Agents, chats, Search applications, and memories, can be configured with a sharing scope.

## Open-source edition sharing scope configuration

In the open-source edition, the sharing scope is configured through the **Permissions** field on the resource configuration page. Select one of the following options based on your business needs:

- **Only me**: Only the knowledge base owner can access the knowledge base.
- **Team**: The knowledge base can be shared within the team.

Resources are not shared automatically. To make a resource available to other users, explicitly configure its sharing scope or permissions.

## Enterprise edition sharing scope configuration

RAGFlow Enterprise supports finer-grained team permission management and team permission control. After a resource is set to **Team**, it does not automatically authorize all team members to access the resource.

Administrators can add collaborators to a resource and grant permissions to **Member**, **Department**, or **Group** collaborators. Only authorized users can access the resource.

Departments and groups are created by administrators based on the enterprise's actual organization structure. Their names and hierarchy may vary between enterprises.

Collaborators can be one of the following object types:

| Type | Description |
| --- | --- |
| Member | Specifies an individual team member. |
| Department | Specifies a department. Members in the department can inherit the corresponding permissions. |
| Group | Specifies a user group. Members in the group can inherit the corresponding permissions. |

## Collaborator permission configuration

In RAGFlow Enterprise, you can configure permissions separately for different collaborator objects, including **Member**, **Department**, and **Group**. Each collaborator can have an independent permission level.

To configure collaborator permissions:

1. Open the resource's **Permission Management** page.
2. Click **Add**.
3. Add a member, department, or group as a collaborator.
4. In the collaborator's **Permission** field, select the required permission level.
5. Save the configuration. The permission takes effect immediately.

| Permission | Description |
| --- | --- |
| Read permission | Allows users to view and use the resource, but does not allow them to modify resource content or configuration. |
| Write permission | Allows users to edit resource content, such as adding, modifying, or deleting content in the resource, but does not allow them to modify resource permissions or management configuration. |
| Manage permission | Grants resource management permissions. Users can manage resource configuration, collaborator permissions, and other management operations. |

## Document-level permissions (Enterprise edition)

Document-level permissions refine knowledge base permissions. The knowledge base owner can configure different collaborator permissions for different documents in the same knowledge base.

For example, the same team member may have read permission on a knowledge base but no permission on a specific document in it. The same member may also have write permission on a specific document, allowing operations such as enabling or disabling that document.

### Configure operations

In the **Files** list of a knowledge base, each document row provides a key icon in the **Action** column. Click the key icon to open the document's **Permission Management** dialog.

Use this dialog to configure collaborator permissions for the selected document.

### Document permission levels

Document-level permission management also uses **Read permission**, **Write permission**, and **Manage permission**.

| Permission | Description |
| --- | --- |
| Read permission | Allows read-only operations such as viewing document information, previewing documents, downloading documents, viewing chunks, and viewing parsing logs. |
| Write permission | Allows document status change operations, such as enabling or disabling documents. |
| Manage permission | Allows higher-level document management actions and covers operations within the read and write permission scopes. |

Resource owners have the highest permission on documents.

If a document has not finished parsing or is being parsed, buttons in the **Action** column may be disabled. In this case, even if the user has the required permission, they must wait until parsing is complete before performing the corresponding operation.

## Share knowledge bases

Share a knowledge base by changing its **Permissions** from **Only me** to **Team**.

Team members can upload and parse files in shared knowledge bases.

## Share chat assistants

Sharing chat assistants is currently exclusive to RAGFlow Enterprise.

## Share Agents

To share an Agent:

1. Open the intended Agent in its editing canvas.
2. Click **Management** **>** **Settings** to open the **Agent settings** dialog.
3. Change **Permissions** from **Only me** to **Team**.
4. Click **Save** to apply your changes.

After this configuration is saved, team members can see the shared Agent.

## Share models

Sharing models is currently exclusive to RAGFlow Enterprise.

## Share memories

To share a memory:

1. Navigate to the **Memory** page and open the intended memory.
2. Click **Configurations**.
3. Change **Permissions** from **Only me** to **Team**.
4. Click **Save** to apply your changes.

After this configuration is saved, team members can see the shared memory.