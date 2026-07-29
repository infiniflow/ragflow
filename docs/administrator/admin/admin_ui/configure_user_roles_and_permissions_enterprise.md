---
sidebar_position: 6
sidebar_label: "Configure User Roles and Permissions (Enterprise Edition)"
---

# Configure User Roles and Permissions (Enterprise Edition)

## Maintain Roles

Enterprise Edition maintains roles and permissions through the **Roles** page. Administrators can view existing roles on this page and create new roles through **New role**.

The current page displays a permission matrix under each role. Visible resources include `Dataset`, `Chat`, `Agent`, `Search`, `File`, `Team`, `Memory`, and `Model provider`. Visible actions include `Enable`, `Read`, `Write`, and `Share`.

Administrators should configure roles according to user responsibilities and should not grant ordinary users resource permissions beyond what their work requires.

![Maintain Roles](https://raw.githubusercontent.com/Yannnnnnny/ragflow-docs/main/images/maintain_roles.jpg)

![Role Management](https://raw.githubusercontent.com/Yannnnnnny/ragflow-docs/main/images/role_management.jpg)

**Note:** Modifying role permissions affects users' access scope for knowledge bases, chat, Agent, search, files, teams, memory, model providers, and other resources.

## Assign Roles to Users

In Enterprise Edition, a user's `Role` can be assigned on the **User management** page. Administrators can click the target user's current `Role` and select a new role from the role list.

1. Go to the **User management** page.
2. Find the target user and open the role dropdown in the `Role` column.
3. Select the new role.

![Assign Roles To Users](https://raw.githubusercontent.com/Yannnnnnny/ragflow-docs/main/images/assign_roles_to_users.jpg)
