"""
Dump Python parse_into_bboxes text output for comparison with Go.

Usage:
    SKIP_OCR=1 PYTHONPATH=. uv run python3 internal/parser/tools/dump_text_output.py [count]
"""
import json, os, sys, time
from pathlib import Path

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))))
from deepdoc.parser.pdf_parser import RAGFlowPdfParser

SCRIPT_DIR = Path(os.path.dirname(os.path.abspath(__file__))).parent / "testdata"
PDF_DIR = SCRIPT_DIR / "real_pdfs"
_engine = "ocr" if os.getenv("DEEPDOC_URL") and not os.getenv("SKIP_OCR") else "noocr"
OUT_DIR = SCRIPT_DIR / "output" / "py" / _engine / "text"
OUT_DIR.mkdir(parents=True, exist_ok=True)

count = int(sys.argv[1]) if len(sys.argv) > 1 else None


def process(pdf_path):
    try:
        parser = RAGFlowPdfParser()
        boxes = parser.parse_into_bboxes(str(pdf_path), zoomin=3)
        lines = []
        for b in boxes:
            t = b.get("text", "").strip()
            if t:
                lines.append(f"{t}\n")
        return "".join(lines)
    except Exception as e:
        print(f"    ERROR: {e}")
        return ""


def main():
    pdfs = sorted(PDF_DIR.glob("*.pdf"))
    if count:
        pdfs = pdfs[:count]

    total_chars = 0
    for i, p in enumerate(pdfs):
        name = p.name
        out_path = OUT_DIR / f"{name}.txt"
        if out_path.exists():
            with open(out_path) as f:
                n = len(f.read())
            total_chars += n
            print(f"[{i+1}/{len(pdfs)}] {name} — SKIP ({n} chars)")
            continue

        t0 = time.time()
        text = process(p)
        with open(out_path, "w") as f:
            f.write(text)
        total_chars += len(text)
        print(f"[{i+1}/{len(pdfs)}] {name} — {len(text)} chars ({time.time()-t0:.1f}s)")

    print(f"\nDone. {total_chars} chars total. Output: {OUT_DIR}/")


if __name__ == "__main__":
    main()
