---
sidebar_position: 1
sidebar_label: "Knowledge base operation permissions"
---

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
