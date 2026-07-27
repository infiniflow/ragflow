---
sidebar_position: 6
sidebar_label: "Search permissions"
---

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
