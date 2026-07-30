---
sidebar_position: 3
sidebar_label: "Agent permissions"
---

## Agent permissions

Agent settings include a sharing scope such as **Only me** or **Team**. Agent operation permissions determine whether the user can view, debug, run, update, or delete an Agent.

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Create Agent | Y | Y | Y | Y | Y |
| List visible Agents and return `operator_permission` | X | Y | Y | Y | Y |
| Get user-visible Agents and tags | X | Y | Y | Y | Y |
| View Agent details | X | Y | Y | Y | Y |
| Access session list | X | Y | Y | Y | Y |
| View a single session | X | Y | Y | Y | Y |
| View component form configuration | X | Y | Y | Y | Y |
| View version list | X | Y | Y | Y | Y |
| Read a single version | X | Y | Y | Y | Y |
| View `webhooktrace` | X | Y | Y | Y | Y |
| View component connection configuration file | X | Y | Y | Y | Y |
| Download Agent-related files | X | Y | Y | Y | Y |
| Get prompts | X | Y | Y | Y | Y |
| Create session | X | X | Y | Y | Y |
| Create a single session | X | X | Y | Y | Y |
| Batch delete sessions | X | X | Y | Y | Y |
| Upload Agent | X | X | Y | Y | Y |
| Component debugging | X | X | Y | Y | Y |
| Test database connection | X | X | Y | Y | Y |
| Read running logs | X | X | Y | Y | Y |
| Chat conversation entry | X | X | Y | Y | Y |
| Chat running entry | X | X | Y | Y | Y |
| Update Agent or DSL | X | X | Y | Y | Y |
| Update canvas tags | X | X | X | Y | Y |
| Delete Agent | X | X | X | X | Y |
| Dataflow or pipeline rerun | - | - | - | - | - |
| Webhook running entry | N/A | N/A | N/A | N/A | N/A |
| Webhook test entry | N/A | N/A | N/A | N/A | N/A |
