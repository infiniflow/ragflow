---
sidebar_position: -7
title: Set Metadata
sidebar_label: Set Metadata
slug: /set_metadata
sidebar_custom_props: {
  categoryIcon: LucideCode
}
---
# Set Metadata

Manually add metadata to an uploaded file

---

On the **Dataset** page of your dataset, you can add metadata to any uploaded file. This approach enables you to 'tag' additional information like URL, author, date, and more to an existing file. In an AI-powered chat, such information will be sent to the LLM with the retrieved chunks for content generation.

For example, if you have a dataset of HTML files and want the LLM to cite the source URL when responding to your query, add a `"url"` parameter to each file's metadata.

![Set metadata](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_metadata.png)

:::tip NOTE
Ensure that your metadata is in JSON format; otherwise, your updates will not be applied.
:::

![Input metadata](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/input_metadata.jpg)

## Related APIs

[Retrieve chunks](../../references/http_api_reference.md#retrieve-chunks)

## Frequently Asked Questions

### Can I Set Metadata for Multiple Documents at Once?

From v0.23.0 onwards, you can set metadata for each document individually or have the LLM auto-generate metadata for multiple files. See [Extract metadata](./advanced/auto_metadata.md) for details.
