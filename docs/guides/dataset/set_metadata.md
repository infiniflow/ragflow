---
sidebar_position: -7
slug: /set_metadata
---

# Set metadata

Add metadata to an uploaded file

---

On the **Dataset** page of your dataset, you can add metadata to any uploaded file. This approach enables you to 'tag' additional information like URL, author, date, and more to an existing file. In an AI-powered chat, such information will be sent to the LLM with the retrieved chunks for content generation.

For example, if you have a dataset of HTML files and want the LLM to cite the source URL when responding to your query, add a `"url"` parameter to each file's metadata.

![Set metadata](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_metadata.jpg)

:::tip NOTE
Ensure that your metadata is in JSON format; otherwise, your updates will not be applied.
:::

![Input metadata](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/input_metadata.jpg)

## Related APIs

[Retrieve chunks](../../references/http_api_reference.md#retrieve-chunks)

## Frequently asked questions

### Can I set metadata for multiple documents at once?

No, you must set metadata *individually* for each document, as RAGFlow does not support batch setting of metadata. If you still consider this feature essential, please [raise an issue](https://github.com/infiniflow/ragflow/issues) explaining your use case and its importance.