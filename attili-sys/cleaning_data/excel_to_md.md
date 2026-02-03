# Excel to Markdown Conversion Plan

## Goal
Convert the Excel database (`attili-sys\cleaning_data\Fake News Alerts Database - 1.xlsx`) into individual Markdown files suitable for RAGFlow ingestion.

## Strategy: One Cell = One Document
To ensure maximum reliability and clarity for the RAG system, each non-empty cell in the Excel sheet will be converted into a standalone `.md` file.

### Data Mapping
- **Category:** The column header.
- **Title:** The first non-empty line of the cell content.
- **Body:** The full text of the cell.

### Output File Naming
Files will be named deterministically to avoid collisions and allow easy tracing back to the source:
`{ColumnIndex}_{CategoryName}__r{RowIndex}.md`

## Implementation Script

This script uses `pandas` to read the Excel file and generates the markdown files in a `ragflow_docs` directory.

```python
import os
import re
import pandas as pd

# Configuration
xlsx_path = "Fake News Alerts Database - 1.xlsx"  # Assumes script is run in the same dir
out_dir = "ragflow_docs"

# Ensure output directory exists
os.makedirs(out_dir, exist_ok=True)

# Read Excel file, ensuring all data is read as string to avoid type issues
# Note: Requires openpyxl installed (pip install openpyxl)
df = pd.read_excel(xlsx_path, dtype=str)

def safe_name(s: str, max_len=80) -> str:
    """Sanitize string to be safe for filenames."""
    # Replace Windows reserved characters
    s = re.sub(r"[\\/:*?\"<>|]+", "_", s)
    # Collapse multiple spaces
    s = re.sub(r"\s+", " ", s).strip()
    return (s[:max_len] if len(s) > max_len else s) or "untitled"

doc_count = 0

print(f"Processing {len(df.columns)} columns...")

for col_idx, col_name in enumerate(df.columns):
    category = str(col_name).strip()
    print(f"Processing Category: {category}")
    
    # Drop empty cells for this column
    cells = df[col_name].dropna()
    
    for row_idx, cell in enumerate(cells):
        text = str(cell).strip()
        
        # Skip very short content which is likely noise
        if len(text) < 30:
            continue

        # Determine Title: First non-empty line
        lines = [ln.strip() for ln in text.splitlines() if ln.strip()]
        internal_title = lines[0] if lines else "Untitled"

        # Construct Filename
        # Format: CC_Category__rRRRR.md (C=Col Index, R=Row Index)
        filename = f"{col_idx:02d}_{safe_name(category)}__r{row_idx+1:04d}.md"
        path = os.path.join(out_dir, filename)

        # Write Markdown Content
        with open(path, "w", encoding="utf-8") as f:
            f.write(f"Category: {category}\n")
            f.write(f"Title: {internal_title}\n\n")
            f.write(text)

        doc_count += 1

print("-" * 30)
print(f"Done! Created {doc_count} documents.")
print(f"Output folder: {os.path.abspath(out_dir)}")
```

## Prerequisites
- Python 3.x
- `pandas`
- `openpyxl`

```bash
pip install pandas openpyxl
```
