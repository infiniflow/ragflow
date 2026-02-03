import os
import re
import pandas as pd

# Configuration
xlsx_path = "Fake News Alerts Database - 1.xlsx"
out_dir = "ragflow_docs"

# Ensure output directory exists
os.makedirs(out_dir, exist_ok=True)

# Read Excel file
# Based on debug output:
# Row 0 (index 0): Main Title "قاعدة بيانات تعلم - الأخبار الكاذبة" (mostly NaNs)
# Row 1 (index 1): Actual Headers (e.g., "غير صحيح كليًا", "مزيف ومفبرك")
# We set header=1 to use the second row as columns.
try:
    df = pd.read_excel(xlsx_path, header=1, dtype=str)
except FileNotFoundError:
    print(f"Error: Could not find '{xlsx_path}' in the current directory.")
    exit(1)

def safe_name(s: str, max_len=80) -> str:
    """Sanitize string to be safe for filenames."""
    if not isinstance(s, str):
        s = str(s)
    # Replace Windows reserved characters and other potentially problematic chars for filenames
    s = re.sub(r"[\\/:*?\"<>|]+", "_", s)
    # Collapse multiple spaces
    s = re.sub(r"\s+", " ", s).strip()
    return (s[:max_len] if len(s) > max_len else s) or "untitled"

doc_count = 0

print(f"Processing {len(df.columns)} columns...")

for col_idx, col_name in enumerate(df.columns):
    # Handle Unnamed columns if any still exist (though header=1 should fix most)
    category = str(col_name).strip()
    
    # If a column header is still "Unnamed: ...", it might be an empty column or part of a merged header that didn't load right.
    # However, for now we assume header=1 gives us the distinct categories the user sees.
    # We will accept it but log it.
    if "Unnamed" in category:
         # Try to see if row 0 had something? No, we skipped it.
         # If it's unnamed, it's likely an empty column between data or similar. 
         # But in the user's debug, columns 0-3 had data.
         pass

    print(f"Processing Category: {category}")
    
    # Drop empty cells for this column
    cells = df[col_name].dropna()
    
    for row_idx, cell in enumerate(cells):
        text = str(cell).strip()
        
        # Skip very short content (likely noise or sub-headers mixed in data)
        if len(text) < 20: 
            continue

        # Determine Title: First non-empty line
        lines = [ln.strip() for ln in text.splitlines() if ln.strip()]
        internal_title = lines[0] if lines else "Untitled"

        # Construct Filename
        # Format: {col_idx}_{Category}_{row_idx}.md
        # Using safe_name to ensure valid filename
        filename = f"{col_idx:02d}_{safe_name(category)}__r{row_idx+1:04d}.md"
        path = os.path.join(out_dir, filename)

        try:
            with open(path, "w", encoding="utf-8") as f:
                f.write(f"Category: {category}\n")
                f.write(f"Title: {internal_title}\n\n")
                f.write(text)
            doc_count += 1
        except Exception as e:
            print(f"Failed to write {filename}: {e}")

print("-" * 30)
print(f"Done! Created {doc_count} documents.")
print(f"Output folder: {os.path.abspath(out_dir)}")
