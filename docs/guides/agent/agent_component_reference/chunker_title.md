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

### Hierarchy or Group

Select how a document is split:

- Hierarchy: Construct a heading tree and produce self-contained chunks, each carrying its full ancestral path (e.g. Part 1 › Chapter 3 › Section 2 + body text). Best for highly structured texts — such as legal statutes, regulations, contracts, and technical specs — where each chunk must be identifiable by its position in the hierarchy.
- Group: Split the document flat at a chosen heading level, merging adjacent small sections to ensure semantic flow. Chunks exclude ancestral path. Best for documents with flowing, contextually connected content — such as books, manuals, reports, and articles — where narrative coherence depends on keeping adjacent paragraphs together.

#### Separate parent-heading content

:::tip NOTE
Available only when **Hierarchy** is selected.
:::

When enabled, chunks include only their heading path and content; content immediately following a parent heading is kept as a separate chunk.

#### Set first chunk as global context

:::tip NOTE
Available only when **Hierarchy** is selected.
:::

Treats the first split as a global heading to maintain consistent context across the document hierarchy. Ideal for resumes where the first section identifies the subject.

#### H3

Specifies the heading level to define chunk boundaries: 

- H1
- H2
- H3 (Default)
- H4
- H5

Click **+ Add regular expressions** to add heading levels here or update the corresponding **Regular Expressions** fields for custom heading patterns.

### Output

The global variable name for the output of the **Title chunker** component, which can be referenced by subsequent components in the ingestion pipeline.

- Default: `chunks`
- Type: `Array<Object>`