# Table header decisions

Source parsers classify column headers before segmentation. `TableData.HeaderRows`
is the resulting number of leading header rows; zero keeps every row as data.
Renderers and chunkers consume that decision without inferring another header.

- Native HTML/Markdown declarations and applicable Excel Table declarations are
  authoritative. Row-scoped headings are not column headers.
- Recognition-generated headings are predictions. OCR Markdown producers use
  `parser.NewOCRMarkdownResult` so conversion retains that distinction.
- Without native declarations, the shared detector uses column-type contrast,
  sampled style boundaries, merge structure, and common field labels. Missing
  values do not supply type-contrast evidence. Numbers in scientific notation
  remain numeric data.
- Ambiguous text-only tables stay headerless. For example,
  `Product,Region / Widget,North / Gadget,South` has no reliable distinction
  between a header and a first record. Defaulting every text row to a header
  would discard valid records from Q&A extraction. No user header configuration
  or additional model call is required.

XLSX currently treats a worksheet as one table, segmented around images. An
Excel Table declaration is applied only when its rectangle covers the sheet
table, optionally after single-cell preamble rows. Local or multiple Excel
Table declarations cannot define headers for unrelated cells; those cases emit
a warning and use sheet-level inference. Independent region extraction is not
implemented. Frozen panes are not treated as proof of a header.

After changing header rules, re-parse documents to apply them. Re-parsing can
change chunk text, chunk IDs, chunk counts, and retrieval results. Q&A extraction
excludes confirmed header rows; headerless tables retain their first record.
Header decisions are logged at debug level, and unsupported XLSX metadata or
merge geometry is reported through parser warnings.
