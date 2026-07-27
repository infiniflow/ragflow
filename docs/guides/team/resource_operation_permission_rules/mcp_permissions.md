---
sidebar_position: 4
sidebar_label: "MCP permissions"
---

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
