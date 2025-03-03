---
sidebar_position: 1
slug: /set_metada
---

# Set metadata

Add metadata to an uploaded file

---

On the **Dataset** page of your knowledge base, you can add metadata to any uploaded file. This approach enables you to 'tag' additional information like URL, author, date, and more to an existing file or dataset. In an AI-powered chat, such information will be sent to the LLM with the retrieved chunks for content generation.

For example, if you have a dataset of HTML files and want the LLM to cite the source URL when responding to your query, add a `"url"` parameter to each file's metadata.

![Image](https://github.com/user-attachments/assets/78cb5035-e96c-43f9-82d7-8fef1b68c843)

:::tip NOTE
Ensure that your metadata is in JSON format; otherwise, your updates will not be applied.
:::

![Image](https://github.com/user-attachments/assets/379cf2c5-4e37-4b79-8aeb-53bf8e01d326)