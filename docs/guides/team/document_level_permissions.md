---
sidebar_position: 4
slug: /document_level_permissions
sidebar_custom_props: {
  categoryIcon: LucideShare2
}
---
# Resource operation permission rules

Resource operation permission rules define what users can do with shared resources.

---

After a resource is shared, operation permissions determine whether a user can view, configure, use, or manage that resource. Different resource types use different operation tables. The resource owner always has the highest permission.

In the tables below, **Y** means the operation is allowed and **X** means the operation is not allowed.

## Knowledge base operation permissions

Knowledge base permissions are determined jointly by knowledge base-level permissions and document-level permissions. Knowledge base permissions include **Read permission**, **Write permission**, and **Manage permission**. Documents in a knowledge base can also have no permission, read permission, write permission, or manage permission.

### File and file permission operations

The following operations require document read permission or higher. If the user has no permission on the target document, the operation is not allowed even when the user has knowledge base-level permission.

| Operation | No document permission | Document read | Document write | Document manage | Owner |
| --- | --- | --- | --- | --- | --- |
| View parsing logs | X | Y | Y | Y | Y |
| View file information | X | Y | Y | Y | Y |
| Download file | X | Y | Y | Y | Y |
| View chunk | X | Y | Y | Y | Y |
| View file configuration | X | Y | Y | Y | Y |
| View metadata | X | Y | Y | Y | Y |
| Preview document | X | Y | Y | Y | Y |

The following operations require document write permission or higher.

| Operation | No document permission | Document read | Document write | Document manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Enable or disable file | X | X | Y | Y | Y |
| Edit metadata | X | X | Y | Y | Y |
| Batch update metadata | X | X | Y | Y | Y |
| Parse or cancel parsing | X | X | Y | Y | Y |
| Enable or disable chunk | X | X | Y | Y | Y |
| Add chunk | X | X | Y | Y | Y |
| Modify chunk | X | X | Y | Y | Y |
| Delete chunk | X | X | Y | Y | Y |

The following operations require document manage permission.

| Operation | No document permission | Document read | Document write | Document manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Modify file configuration | X | X | X | Y | Y |
| Rename file | X | X | X | Y | Y |

### Knowledge base permission operations

The following knowledge base operations are available to users with knowledge base read permission or higher.

| Operation | No KB permission | KB read | KB write | KB manage | Owner |
| --- | --- | --- | --- | --- | --- |
| View knowledge graph | X | Y | Y | Y | Y |
| Duplicate | X | Y | Y | Y | Y |
| View dataset card | X | Y | Y | Y | Y |
| View metadata configuration | X | Y | Y | Y | Y |
| `trace_index` | X | Y | Y | Y | Y |
| Metadata aggregation | X | Y | Y | Y | Y |
| Knowledge base tag aggregation | X | Y | Y | Y | Y |
| View knowledge base parsing overview | X | Y | Y | Y | Y |
| View knowledge base tag | X | Y | Y | Y | Y |

The following knowledge base operations require knowledge base manage permission or owner permission.

| Operation | No KB permission | KB read | KB write | KB manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Delete file | X | X | X | Y | Y |
| Add file | X | X | X | Y | Y |
| Generate RAPTOR | X | X | X | Y | Y |
| Generate index | X | X | X | Y | Y |
| Delete index | X | X | X | Y | Y |
| Generate knowledge graph | X | X | X | Y | Y |
| Delete knowledge graph | X | X | X | Y | Y |
| Re-embed | X | X | X | Y | Y |
| Delete knowledge base tag | X | X | X | Y | Y |
| Rename knowledge base tag | X | X | X | Y | Y |
| Modify knowledge base configuration | X | X | X | Y | Y |
| Delete knowledge base | X | X | X | Y | Y |

### Knowledge base permission judgment order

When a user performs a knowledge base or document operation, permissions are evaluated in the following order:

1. Whether the user has formally joined the team that owns the knowledge base.
2. Whether the knowledge base sharing scope allows team access.
3. Whether the user has knowledge base-level **Read**, **Write**, or **Manage** permission.
4. Whether the user has target document-level **Read**, **Write**, or **Manage** permission.
5. Whether the current operation is allowed by the permission table.

## Chat application permissions

Chat application permissions control viewing, configuration, creation, deletion, generation, and retrieval operations for chat assistants.

### Chat management

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Get chat list | X | Y | Y | Y | Y |
| Get chat details | X | Y | Y | Y | Y |
| Full update of chat configuration | X | X | X | Y | Y |
| Partial update of chat configuration | X | X | X | Y | Y |
| Create chat application configuration | X | X | X | X | Y |
| Batch delete chats through `ids`, `delete_all`, or request-body `chat_id` | X | X | X | X | Y |
| Delete a single chat | X | X | X | X | Y |

### Sessions and messages

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Create session | X | Y | Y | Y | Y |
| List sessions | X | Y | Y | Y | Y |
| Get a single session | X | Y | Y | Y | Y |
| Like, dislike, or submit feedback | X | Y | Y | Y | Y |
| Batch delete sessions | X | X | Y | Y | Y |
| Update session name or information | X | X | Y | Y | Y |
| Delete a message and its reply or reference | X | X | Y | Y | Y |

For read permission, session operations are limited to the user's own sessions. Write permission can cover all sessions. Batch deletion and message deletion with write permission are limited to the user's own sessions; manage permission and owner permission cover all sessions.

### Chat generation and retrieval

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| TTS returns audio stream | X | Y | Y | Y | Y |
| Audio to text | X | Y | Y | Y | Y |
| Main chat completion API, including SSE and non-SSE | X | Y | Y | Y | Y |
| OpenAI-compatible chat completion | X | Y | Y | Y | Y |
| Shared chatbot conversation entry | X | Y | Y | Y | Y |
| Get recommended questions | X | Y | Y | Y | Y |
| Legacy completion API | X | Y | Y | Y | Y |
| Legacy recommended questions | X | Y | Y | Y | Y |

For the main chat completion API, read permission applies to the user's own sessions, while manage permission applies to all sessions.

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

## MCP permissions

MCP permission rules control MCP server creation, connection testing, listing, details, configuration, deletion, and export.

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Create MCP server | Y | Y | Y | Y | Y |
| Test MCP connection | Y | Y | Y | Y | Y |
| Batch import MCP server | Y | Y | Y | Y | Y |
| List visible MCP servers | X | Y | Y | Y | Y |
| View MCP details | X | Y | Y | Y | Y |
| Update MCP configuration | X | X | Y | Y | Y |
| Delete MCP server | X | X | X | X | Y |
| Export or download MCP configuration | X | X | X | X | Y |

Creating an MCP server creates a new resource and does not require an existing MCP permission. The owner permission is created after the resource is created.

Testing MCP connection is currently not strictly restricted by existing permission checks in the backend. If this behavior changes, a permission check should be added at the point where MCP permissions can be changed.

Batch import creates resources and does not use `operator_permission` from an existing MCP server.

Listing visible MCP servers returns visible results, including `operator_permission`. If an MCP server is visible, its details can be read.

Updating MCP configuration includes updating URL, headers, variables, and tools. Manage permission is recommended for this operation.

Deleting an MCP server is recommended to be owner-only. Exporting or downloading MCP configuration is also recommended to be owner-only because the configuration may contain sensitive values such as `authorization_token`.

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

## Search permissions

Search permissions control creating Search applications, listing and viewing Search applications, Q&A, related questions, mind map generation, editing, and deletion.

| Operation | No permission | Read | Write | Manage | Owner |
| --- | --- | --- | --- | --- | --- |
| Create Search | Y | Y | Y | Y | Y |
| Search list | X | Y | Y | Y | Y |
| Search details | X | Y | Y | Y | Y |
| Search Q&A | X | X | Y | Y | Y |
| Get related questions | X | X | Y | Y | Y |
| Generate mind map | X | X | Y | Y | Y |
| Edit Search | X | X | X | Y | Y |
| Delete Search | X | X | X | X | Y |

Creating Search creates a new resource and creates the owner record afterward. The Search list currently relies on the ENABLE role check and is not fully connected to ACL sharing. Search details are currently connected only by ID; changing this to a READ operation is recommended.

For Search Q&A, related questions, and mind map generation, effective use recommends a READ operation check rather than only checking whether the resource can be created.

Editing Search includes adjusting knowledge base configuration and should require manage permission. Deleting Search is recommended to be owner-only, and current behavior is generally consistent with `created_by`.

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

## Permission effective rules

Resource permissions take effect according to the following general rules:

- The resource owner always has all permissions.
- If the sharing scope is **Only me**, only the owner can access the resource.
- If the sharing scope is **Team**, only team members who meet the permission conditions can access the resource.
- In RAGFlow Enterprise, the system checks collaborator configuration for **Member**, **Department**, and **Group** collaborators.
- If finer-grained permissions exist, such as document-level permissions in a knowledge base, those permissions continue to apply on top of resource access permission.