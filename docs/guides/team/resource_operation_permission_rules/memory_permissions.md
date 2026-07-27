---
sidebar_position: 5
sidebar_label: "Memory permissions"
---

## Memory permissions

Memory settings include a sharing scope. Memory permissions cover memory configuration, memory messages, SDK-related message access, and message status operations.

### Memory operations

| Resource | Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- | --- |
| MEMORY | Create memory | Y | Y | Y | Y | Y |
| MEMORY | List memory | X | Y | Y | Y | Y |
| MEMORY | View memory configuration | X | Y | Y | Y | Y |
| MEMORY | View memory message list | X | Y | Y | Y | Y |
| MEMORY | Modify memory basic configuration | X | X | X | Y | Y |
| MEMORY | Delete memory | X | X | X | X | Y |

Creating a memory creates a new resource. The frontend creation entry is on the memories list page. Listing memories returns accessible memories, including private and shared memories. Modifying memory basic configuration includes fields such as name, model, and prompts.

### Message operations

| Resource | Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- | --- |
| MESSAGE | Search messages | X | Y | Y | Y | Y |
| MESSAGE | Get a specified message | X | Y | Y | Y | Y |
| MESSAGE | View message content | X | Y | Y | Y | Y |
| MESSAGE | Add message to memory | X | X | Y | Y | Y |
| MESSAGE | Delete a single message | X | X | Y | Y | Y |
| MESSAGE | Update a single message status | X | X | Y | Y | Y |

Message search can be performed by memory, Agent, or session. Getting specified messages applies to memory messages and SDK availability. Adding a message to memory has no direct frontend button and is mainly used by the backend SDK.
