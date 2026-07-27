---
sidebar_position: 7
sidebar_label: "Model permissions"
---

## Model permissions

Model permissions use provider-level and owner-level checks. The table below uses **No permission**, **Read permission**, and **Owner** columns.

| Operation | No permission | Read permission | Owner |
| --- | --- | --- | --- |
| List providers | X | Y | Y |
| Add provider | X | Y | Y |
| Delete provider | X | X | Y |
| View models under a provider | X | X | Y |
| View a single model | X | X | Y |
| Create instance | X | X | Y |
| Verify connection or API key | X | X | Y |
| List instances | X | X | Y |
| View a single instance | X | X | Y |
| Get models under an instance | X | X | Y |
| Create instance model scope | X | X | Y |
| Delete instance model scope | X | X | Y |
| View model support status | X | X | Y |
| Create model support status | X | X | Y |
| Delete model support status | X | X | Y |
| Batch delete instances | X | X | Y |
| View system default model | X | X | Y |
| View current user's default model | X | X | Y |
| Set or get default model | X | X | Y |

Listing providers lists the current user's own providers and system providers. Adding a provider follows the permission matrix. Deleting a provider is not related to ordinary read permissions.

Viewing the system default model must determine whether the default model belongs to the current user. Viewing the current user's default model must determine whether the model is the user's own default value. Setting or getting the default model follows the initial permission design.
