---
sidebar_position: 31
slug: /chunker_title_component
sidebar_custom_props: {
  categoryIcon: LucideBlocks
}
---
# Title chunker component

A component that splits texts into chunks by heading level.

---

A **Token chunker** component is a text splitter that uses specified heading level as delimiter to define chunk boundaries and create chunks.

## Scenario

A **Title chunker** component is optional, usually placed immediately after **Parser**.

:::caution WARNING
Placing a **Title chunker** after a **Token chunker** is invalid and will cause an error. Please note that this restriction is not currently system-enforced and requires your attention.
:::

## Configurations

### Hierarchy

Specifies the heading level to define chunk boundaries: 

- H1
- H2
- H3 (Default)
- H4

Click **+ Add** to add heading levels here or update the corresponding **Regular Expressions** fields for custom heading patterns.

### Output

The global variable name for the output of the **Title chunker** component, which can be referenced by subsequent components in the ingestion pipeline.

- Default: `chunks`
- Type: `Array<Object>`