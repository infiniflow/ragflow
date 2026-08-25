---
sidebar_position: 3
slug: /file_management/link_knowledge_base
sidebar_label: Link Knowledge Base
title: Link Knowledge Base
sidebar_custom_props: {
  categoryIcon: LucideFolderDot
}
---

# Link Knowledge Base

The file management page allows you to add files directly to a knowledge base. After the association is created, you can continue operations such as parsing, chunking, and retrieval from the knowledge base side. The specific processing status is subject to the knowledge base document page.

![Parse file](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/parse_file.jpg)


## Link a Single File

1. Find the target file in the file list.
2. Hover over the row, then click the **Add to Knowledge Base** icon in the **Operations** column.
3. In the pop-up window, select one or more knowledge bases.
4. Click **Save** to save the association.

![Link a single file](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/link_a_single_file.jpg)

## Batch Link

1. Select multiple files that need to be linked.
2. In the batch operation bar, click **Add to Knowledge Base**.
3. Select the target knowledge base and save.

A single-file operation replaces the knowledge base associations of that file with the current selection. A batch operation adds the selected files to the target knowledge base. If the same file serves multiple knowledge bases, confirm the target knowledge base name before saving.

