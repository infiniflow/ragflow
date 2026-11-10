---
sidebar_position: 3
slug: /file_management/link_knowledge_base
sidebar_label: Link Dataset
title: Link Dataset
sidebar_custom_props: {
  categoryIcon: LucideFolderDot
}
---

# Link Dataset {#link-knowledge-base}

The file management page allows you to add files directly to a Dataset. After the association is created, you can continue operations such as parsing, chunking, and retrieval from the Dataset side. The specific processing status is subject to the Dataset document page.

## Link a Single File

1. Find the target file in the file list.
2. Hover over the row, then click the **Add to Dataset** icon in the **Action** column.
3. In the pop-up window, select one or more Datasets.
4. Click **Save** to save the association.

![Link a single file](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/link_a_single_file.jpg)

## Batch Link

1. Select multiple files that need to be linked.
2. In the batch operation bar, click **Add to Dataset**.
3. Select the target Dataset and save.

![Batch link files](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/batch_link.jpg)

A single-file operation replaces the Dataset associations of that file with the current selection. A batch operation adds the selected files to the target Dataset. If the same file serves multiple Datasets, confirm the target Dataset name before saving.
