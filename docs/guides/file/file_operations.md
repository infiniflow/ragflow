---
sidebar_position: 2
slug: /file_management/file_operations
sidebar_label: File Operations
title: File Operations
sidebar_custom_props: {
  categoryIcon: LucideFolderDot
}
---

# File Operations

After you hover over a file row, the **Operations** column displays the available operation buttons. The available operations for folders and regular files are slightly different. For files from external data sources, some operations, such as moving, renaming, or deleting, may be restricted by the data source permissions.

![File operations](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/file_operations.jpg)

**Add to Knowledge Base**: Applies to files. Associates the file with one or more knowledge bases, allowing the knowledge base to use the file as a data source.

**Move**: Applies to files or folders. After selecting a target folder and saving, the system moves the selected item to the target directory.

**Rename**: Applies to files or folders. When renaming, it is recommended to retain information that identifies the file format and source to avoid name confusion within the same directory.

**Download**: Applies to files. Downloads the original file. Folder rows usually do not provide a download button.

**Preview or View**: Applies to previewable files. Some document types display a view entry, allowing you to open or preview the file content within the page.

**Delete**: Applies to files or folders. A confirmation dialog appears before deletion. After confirmation, the file is removed from the current directory.

Deleting files or folders may affect knowledge base data sources that are already associated with them. Before deleting, confirm that the file is no longer used by business workflows, knowledge base parsing, or agent retrieval.

