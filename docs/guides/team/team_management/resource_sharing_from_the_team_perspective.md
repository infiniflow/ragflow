---
sidebar_position: 9
sidebar_label: Resource Sharing from the Team Perspective
title: Resource Sharing from the Team Perspective
---

# Resource Sharing from the Team Perspective

Joining a team does not automatically grant access to all resources in that team. The resource itself must be shared with the team or authorized to the user.

For knowledge bases, the **Permissions** field can be set to **Only me** or **Team**. The default value is **Only me**. When the value is changed to **Team**, joined team members can see and use the knowledge base according to permission rules.

Knowledge base access follows these rules:

- If the current user belongs to the owner tenant, access is allowed.
- If the knowledge base permission is **Only me**, non-owners cannot access it.
- If the knowledge base permission is **Team** and the user belongs to the tenant, access is allowed.
- Deleting a knowledge base is stricter and generally requires the current user to be the creator.

Agent sharing also uses **Only me** and **Team**. The permission field is located in the Agent settings dialog. The default value is **Only me**.

Memory sharing is configured in the memory's advanced settings. **Only me** maps to `me`, and **Team** maps to `team`. The backend also defines an `all` value, but the UI exposes **Only me** and **Team**.
