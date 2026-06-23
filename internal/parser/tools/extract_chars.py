"""Extract raw chars from PDFs for Go pipeline comparison.

Usage: uv run python3 tools/extract_chars.py
"""
import json, os, sys

HERE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "testdata")
PDF_DIR = os.path.join(HERE, "pdfs")
OUT_DIR = os.path.join(HERE, "snapshots")

def main():
    import pdfplumber

    for fname in sorted(os.listdir(PDF_DIR)):
        if not fname.endswith(".pdf"):
            continue
        name = fname.replace(".pdf", "")
        out_path = os.path.join(OUT_DIR, f"{name}_chars.json")

        with pdfplumber.open(os.path.join(PDF_DIR, fname)) as pdf:
            all_chars = []
            for pg_idx, page in enumerate(pdf.pages):
                page_chars = []
                for c in page.chars:
                    page_chars.append({
                        "x0": round(float(c["x0"]), 3),
                        "x1": round(float(c["x1"]), 3),
                        "top": round(float(c["top"]), 3),
                        "bottom": round(float(c["bottom"]), 3),
                        "text": c.get("text", ""),
                        "fontname": c.get("fontname", ""),
                        "page_number": pg_idx,
                        "ncs": c.get("ncs", ""),
                        "stroking_color": c.get("stroking_color", None),
                        "non_stroking_color": c.get("non_stroking_color", None),
                    })
                all_chars.extend(page_chars)
            with open(out_path, "w") as f:
                json.dump(all_chars, f, indent=2, ensure_ascii=False)
            print(f"{name}: {len(all_chars)} chars -> {out_path}")

if __name__ == "__main__":
    main()
