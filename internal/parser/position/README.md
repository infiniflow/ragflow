# Source locations

`position.Location` carries the coordinate vocabulary together with its source
identity. `schema.ChunkDoc` stores it as `source_location` between ingestion
components. An absent location is valid; a location cannot contain both page
and spreadsheet geometry or conflicting sheet identities.

| Location | Geometry | Conventions |
|---|---|---|
| Page | `IndexBoxes`, `PreviewBoxes`, optional page metadata | Page numbers are 1-based; coordinates retain the parser's coordinate space |
| Spreadsheet | Sheet identity and `CellRange` selections | Sheet, rows and columns are 1-based; range endpoints are inclusive |

Parser backend fields are normalized by `FromParserMap` at the parser boundary.
Runtime JSON has one location object. `Extra` cannot reintroduce backend
position fields or override the typed object. DeepDoc's 0-to-1 page conversion
still belongs to its parser; this package does not increment page numbers.

Index and preview boxes have different consumers. Indexing consumes only
`IndexBoxes`. Cropping and reading order prefer `PreviewBoxes`, falling back to
index boxes when preview geometry is absent. Preview-only geometry does not
create index positions. A page-only rectangle can carry an indexable page
number without supplying an image crop region.

`Clone`, `SelectRows`, `WithoutIndex`, and `SlicePage` return independent
locations. `Append` adds geometry to an owned destination; a nil destination
clones its source. Chunkers clone parser data before appending, so source and
sibling chunks remain independent. `MergePage` clones, sorts and deduplicates
page boxes; ordinary append preserves source order. Spreadsheet ranges are
never interpolated as page rectangles. Incompatible merges remain unlocated
instead of recovering a partial selection from a later input.

Chunkers retain their existing strategies: General inherits coarse geometry
when splitting text, while Token and title token caps slice page geometry.
Overlap can carry prior geometry without replacing the current chunk's page
metadata. One's whole-document merge carries no individual item's partial
selection. This model change does not improve PDF highlight precision.

At the index boundary, geometry is projected to existing `position_int`,
`page_num_int` and `top_int` columns. Spreadsheet projection preserves QA's
zero-based source row in `top_int`. The internal location is then removed.
Public debug results, per-node result views and JSON exports project locations
to existing preview fields without mutating runtime state or user metadata.
No database or retrieval API format migration is needed. Existing incorrect
stored spreadsheet positions require reparsing affected documents.

Validate through `build.sh`:

```sh
bash build.sh --test ./internal/parser/position/ ./internal/parser/parser/ ./internal/ingestion/component/schema/ ./internal/ingestion/component/chunker/ ./internal/ingestion/component/ ./internal/ingestion/task/indexdoc/ ./internal/ingestion/task/
bash build.sh --test -race -run 'Test.*(Crop|Clone|Location|Spreadsheet|XLSXChunkers)' ./internal/parser/position/ ./internal/ingestion/component/chunker/ ./internal/ingestion/component/schema/
```
