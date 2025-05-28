---
sidebar_position: 4
slug: /enable_excel2html
---

# Enable Excel2HTML

Convert complex Excel spreadsheets into HTML tables.

---

When using the **General** chunking method, you can enable the **Excel to HTML** toggle to convert spreadsheet files into HTML tables. If it is disabled, spreadsheet tables will be represented as key-value pairs. For complex tables that cannot be simply represented this way, you must enable this feature.

:::caution WARNING
The feature is disabled by default. If your knowledge base contains spreadsheets with complex tables and you do not enable this feature, RAGFlow will not throw an error but your tables are likely to be garbled.
:::

## Scenarios

Works with complex tables that cannot be represented as key-value pairs. Examples include spreadsheet tables with multiple columns, tables with merged cells, or multiple tables within one sheet. In such cases, consider converting these spreadsheet tables into HTML tables.

## Considerations

- The Excel2HTML feature applies only to spreadsheet files (XLSX or XLS (Excel 97-2003)).
- This feature is associated with the **General** chunking method. In other words, it is available *only when* you select the **General** chunking method.
- When this feature is enabled, spreadsheet tables with more than 12 rows will be split into chunks of 12 rows each.

## Procedure

1. On your knowledge base's **Configuration** page, select **General** as the chunking method.

   _The **Excel to HTML** toggle appears._

2. Enable **Excel to HTML** if your knowledge base contains complex spreadsheet tables that cannot be represented as key-value pairs.
3. Leave **Excel to HTML** disabled if your knowledge base has no spreadsheet tables or if its spreadsheet tables can be represented as key-value pairs.
4. If question-answering regarding complex tables is unsatisfactory, check if **Excel to HTML** is enabled.

## Frequently asked questions

### Should I enable this feature for PDFs with complex tables?

Nope. This feature applies to spreadsheet files only. Enabling **Excel to HTML** does not affect your PDFs.