---
sidebar_position: 5
title: Configure the Transformer Component
sidebar_label: Configure the Transformer Component
slug: /configure_transformer_component
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Configure the Transformer Component

The **Transformer** component is designed to bridge the "semantic gap". In general, it uses AI models to add semantic metadata, making your content easier to discover during retrieval.

It has four generation types:

- **Summary**: Creates a concise overview.
- **Keywords**: Extracts key terms.
- **Questions**: Generates questions that each text chunk can answer.
- **Metadata**: Custom metadata extraction.

If you have multiple **Transformer** components, make sure to separate the **Transformer** component for each function, for example, one for summaries and another for keywords.

Key configurations:

Model mode (select one):

- **Improvise**: More creative, suitable for question generation.
- **Precise**: Strictly faithful to the text, suitable for summary and keyword extraction.
- **Balanced**: A middle ground suitable for most scenarios.

Prompt engineering:

- The system prompt for each generation type is open and customizable.

Connection:

- The **Transformer** can be connected after the **Parser** to process the entire document, or after the **Chunker** to process each chunk.

Variable reference:

- Nodes do not automatically obtain content. In the user prompt, manually reference upstream variables by typing `/` and selecting a specific output, such as `/{Parser.output}` or `/{Chunker.output}`.

Chained connection:

- When chaining **Transformer** components, if variables are referenced correctly, the second **Transformer** component processes the output of the first one, for example, generating keywords from a summary.

![Configure The Transformer Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_transformer_component_1.jpg)

![Configure The Transformer Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_transformer_component_2.jpg)

![Configure The Transformer Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_transformer_component_3.jpg)
