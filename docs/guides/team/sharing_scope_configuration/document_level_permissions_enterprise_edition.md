---
sidebar_position: 4
sidebar_label: "Document-level permissions (Enterprise edition)"
---

## Document-level permissions (Enterprise edition)

Document-level permissions refine knowledge base permissions. The knowledge base owner can configure different collaborator permissions for different documents in the same knowledge base.

For example, the same team member may have read permission on a knowledge base but no permission on a specific document in it. The same member may also have write permission on a specific document, allowing operations such as enabling or disabling that document.

### Configure operations

In the **Files** list of a knowledge base, each document row provides a key icon in the **Action** column. Click the key icon to open the document's **Permission Management** dialog.

Use this dialog to configure collaborator permissions for the selected document.

![Configure Operations](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_operations_1.jpg)

![Configure Operations](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_operations_2.jpg)


### Document permission levels

Document-level permission management also uses **Read permission**, **Write permission**, and **Manage permission**.

| Permission | Description |
| --- | --- |
| Read permission | Allows read-only operations such as viewing document information, previewing documents, downloading documents, viewing chunks, and viewing parsing logs. |
| Write permission | Allows document status change operations, such as enabling or disabling documents. |
| Manage permission | Allows higher-level document management actions and covers operations within the read and write permission scopes. |

Resource owners have the highest permission on documents.

If a document has not finished parsing or is being parsed, buttons in the **Action** column may be disabled. In this case, even if the user has the required permission, they must wait until parsing is complete before performing the corresponding operation.
