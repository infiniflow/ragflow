"""
Run RAGFlowPdfParser on real/snapshot PDFs and produce:
  - output/py/{variant}/text/{pdf}.txt  — per-box text (for diff with Go output)
  - real_pdf_results_py.json            — per-PDF pipeline stage stats

Usage:
    cd /home/shenyushi/cc-workspace/ragflow
    python3 internal/parser/tools/dump_py_results.py [--count N] [--real] [--snapshots]
"""
import json, logging, os, re, sys, time, traceback, unicodedata, warnings
from collections import Counter

warnings.filterwarnings("ignore", message="Number of distinct clusters")
logging.basicConfig(level=logging.WARNING, force=True)
logging.getLogger('deepdoc').setLevel(logging.WARNING)
logging.getLogger('PIL').setLevel(logging.WARNING)
from pathlib import Path

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
sys.path.insert(0, PROJECT_ROOT)

SCRIPT_DIR = Path(os.path.dirname(os.path.abspath(__file__))).parent / "testdata"
REAL_PDFS_DIR = SCRIPT_DIR / "real_pdfs"
SNAPSHOT_PDFS_DIR = SCRIPT_DIR / "pdfs"

# DLA+TSR always on. SKIP_OCR=1 skips image OCR only (DLA+TSR kept).
_py_variant = "noocr" if os.getenv("SKIP_OCR") == "1" else "ocr"
TEXT_OUT_DIR = SCRIPT_DIR / "output" / "py" / _py_variant / "text"
TEXT_OUT_DIR.mkdir(parents=True, exist_ok=True)

TABLES_OUT_DIR = SCRIPT_DIR / "output" / "py" / _py_variant / "tables"
TABLES_OUT_DIR.mkdir(parents=True, exist_ok=True)

DLA_OUT_DIR = SCRIPT_DIR / "output" / "py" / _py_variant / "dla"
DLA_OUT_DIR.mkdir(parents=True, exist_ok=True)

TSR_RAW_OUT_DIR = SCRIPT_DIR / "output" / "py" / _py_variant / "tsr_raw"
TSR_RAW_OUT_DIR.mkdir(parents=True, exist_ok=True)

CHARSPY_OUT_DIR = SCRIPT_DIR / "charspy"
CHARSPY_OUT_DIR.mkdir(exist_ok=True)


def _dump_dla_tsr_intermediates(name, parser):
    """Write DLA layout regions and TSR raw components for diff with Go."""
    # DLA layout regions per page
    dla_path = DLA_OUT_DIR / f"{name}.json"
    if not dla_path.exists():
        dla_out = []
        if hasattr(parser, 'page_layout') and parser.page_layout:
            for pn, regions in enumerate(parser.page_layout):
                dla_out.append({
                    "page": pn,
                    "regions": [
                        {
                            "type": r["type"],
                            "x0": r["x0"], "x1": r["x1"],
                            "top": r["top"], "bottom": r["bottom"],
                        }
                        for r in regions
                    ],
                })
        with open(dla_path, "w", encoding="utf-8") as f:
            json.dump(dla_out, f, ensure_ascii=False)

    # TSR raw components per table
    tsr_path = TSR_RAW_OUT_DIR / f"{name}.json"
    if not tsr_path.exists():
        tsr_out = []
        if hasattr(parser, 'tb_cpns') and parser.tb_cpns:
            for t in parser.tb_cpns:
                tsr_out.append({
                    "table_index": t.get("table_index", t.get("layoutno", 0)),
                    "page": t.get("pn", 0),
                    "label": t.get("label", ""),
                    "x0": t.get("x0", 0), "y0": t.get("top", 0),
                    "x1": t.get("x1", 0), "y1": t.get("bottom", 0),
                    "text": t.get("text", ""),
                })
        with open(tsr_path, "w", encoding="utf-8") as f:
            json.dump(tsr_out, f, ensure_ascii=False)


def html_to_rows(html):
    """Extract cell text grid from <table> HTML produced by construct_table.
    This is the golden data — matching Go's rowsToHTML output structure."""
    rows = []
    for tr in re.findall(r'<tr>(.*?)</tr>', html, re.DOTALL):
        cells = re.findall(r'<t[dh][^>]*>(.*?)</t[dh]>', tr, re.DOTALL)
        rows.append(cells)
    return rows



# Dump table boxes with R/C annotations for Go pipeline parity test.
# Captured BEFORE _extract_table_figure replaces boxes with HTML.
TABLE_BOXES_OUT_DIR = SCRIPT_DIR / "output" / "py" / _py_variant / "table_boxes"
TABLE_BOXES_OUT_DIR.mkdir(parents=True, exist_ok=True)

def dump_table_boxes(parser, name):
    """Export boxes with R/C/H/SP annotations to JSON for Go parity testing."""
    if not hasattr(parser, 'boxes'):
        return
    table_boxes = []
    for b in parser.boxes:
        if b.get("layout_type") != "table":
            continue
        table_boxes.append({
            "x0": b.get("x0", 0), "x1": b.get("x1", 0),
            "top": b.get("top", 0), "bottom": b.get("bottom", 0),
            "text": b.get("text", ""),
            "page_number": b.get("page_number", 0),
            "R": b.get("R", -1), "C": b.get("C", -1),
            "H": b.get("H", -1), "SP": b.get("SP", -1),
            "layout_type": b.get("layout_type", ""),
        })
    if table_boxes:
        path = TABLE_BOXES_OUT_DIR / f"{name}.json"
        with open(path, "w", encoding="utf-8") as f:
            json.dump(table_boxes, f, ensure_ascii=False, indent=2)

def count_chars(parser):
    if hasattr(parser, "page_chars") and parser.page_chars:
        return sum(len(ch) for ch in parser.page_chars)
    return 0


def process(pdf_path, do_text=True, do_stats=True, auto_rotate=False):
    """Run production pipeline, return (text, stats, tables, page_chars)."""
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser

    name = Path(pdf_path).name
    parser = RAGFlowPdfParser()
    t0 = time.time()

    # Main entry: parse_into_bboxes runs the full text pipeline.
    boxes = parser.parse_into_bboxes(str(pdf_path), zoomin=3)
    n_pages = len(parser.page_images) if hasattr(parser, 'page_images') else 0
    n_chars = count_chars(parser)
    n_box = len(boxes) if boxes else 0
    box0_type = type(boxes[0]).__name__ if boxes else 'empty'
    has_en = hasattr(parser, 'is_english') and parser.is_english
    print(f"  DEBUG {name}: is_english={has_en} pages={n_pages} chars={n_chars} boxes={n_box} boxes[0]={box0_type}")

    elapsed = time.time() - t0
    # DLA/TSR intermediates (after parse_into_bboxes, page_layout/tb_cpns are available).
    _dump_dla_tsr_intermediates(name, parser)
    dump_table_boxes(parser, name)

    # Text output
    lines = []
    if do_text:
        for b in boxes:
            t = b.get("text", "").strip()
            if t:
                lines.append(f"{t}\n")

    boxes_final = len(boxes)
    text_len = sum(len(b.get("text", "").strip()) for b in boxes if b.get("text", "").strip())
    chars_total = count_chars(parser)
    tables_count = len(parser.tb_cpns) if hasattr(parser, 'tb_cpns') else 0

    # Read stage-level metrics collected by _parse_loaded_window_into_bboxes.
    boxes_initial = getattr(parser, 'boxes_initial', boxes_final)
    boxes_text_merge = getattr(parser, 'boxes_text_merge', boxes_final)
    boxes_vertical_merge = getattr(parser, 'boxes_vertical_merge', boxes_final)
    boxes_final_stage = getattr(parser, 'boxes_final', boxes_final)  # before insert_table_figures

    stats = {
        "pages": parser.total_page if hasattr(parser, "total_page") else len(parser.page_images),
        "chars": chars_total,
        "boxes_initial": boxes_initial,
        "boxes_text_merge": boxes_text_merge,
        "boxes_vertical_merge": boxes_vertical_merge,
        "sections": boxes_final_stage,
        "tables": tables_count,
        "boxes_final": boxes_final,
        "text_len": text_len,
        "is_english": bool(parser.is_english) if hasattr(parser, "is_english") else None,
        "time_s": round(elapsed, 2),
    }

    # Table output: extract rows from the HTML <table> produced by construct_table.
    # This is the golden data — same source as Python's final section text.
    table_items = []
    for b in boxes:
        if not b.get("text", "").startswith("<table>"):
            continue
        html = b.get("text", "")
        rows = html_to_rows(html)
        positions = [{
            "page": b.get("page_number", 0),
            "left": b.get("x0", 0),
            "right": b.get("x1", 0),
            "top": b.get("top", 0),
            "bottom": b.get("bottom", 0),
        }]
        table_items.append({"positions": positions, "rows": rows})

    tables_out = {
        "pdf": Path(pdf_path).name,
        "tables": len(table_items),
        "results": table_items,
        "time_s": round(elapsed, 2),
    }

    page_chars = parser.page_chars if hasattr(parser, "page_chars") else []
    return "".join(lines), stats, tables_out, page_chars


def run_set(pdf_dir, name_prefix, count=None):
    pdfs = sorted(Path(pdf_dir).glob("*.pdf"), key=lambda p: p.stat().st_size)
    if count:
        pdfs = pdfs[:count]

    text_results = {}  # name -> text
    stats_results = []

    total_chars = 0
    for i, p in enumerate(pdfs):
        name = p.name
        label = f"[{i+1}/{len(pdfs)}] {name}"
        text_path = TEXT_OUT_DIR / f"{name}.txt"

        if text_path.exists():
            with open(text_path) as f:
                cached_text = f.read()
            total_chars += len(cached_text)

            # Read tables count from cached output/py/{variant}/tables/{name}.json
            tables_count = 0
            tables_path = TABLES_OUT_DIR / f"{name}.json"
            if tables_path.exists():
                try:
                    with open(tables_path) as f:
                        td = json.load(f)
                    tables_count = td.get("tables", 0)
                except (json.JSONDecodeError, KeyError):
                    pass

            # Read stage metadata from #@meta line at end of text file.
            prev = {"pages": 0, "chars": len(cached_text), "sections": 0, "tables": tables_count,
                    "boxes_initial": 0, "boxes_text_merge": 0, "boxes_vertical_merge": 0}
            meta_idx = cached_text.rfind("\n#@meta")
            if meta_idx >= 0:
                try:
                    meta = json.loads(cached_text[meta_idx+7:])
                    prev["chars"] = meta.get("chars", prev["chars"])
                    prev["boxes_initial"] = meta.get("boxes_initial", 0)
                    prev["boxes_text_merge"] = meta.get("boxes_text_merge", 0)
                    prev["boxes_vertical_merge"] = meta.get("boxes_vertical_merge", 0)
                    prev["sections"] = meta.get("sections", meta.get("boxes_final", 0))
                except (json.JSONDecodeError, KeyError):
                    pass

            stats_results.append({
                "file": name, "pages": prev["pages"], "chars": prev["chars"],
                "text_len": len(cached_text),
                "boxes_initial": prev["boxes_initial"],
                "boxes_text_merge": prev["boxes_text_merge"],
                "boxes_vertical_merge": prev["boxes_vertical_merge"],
                "sections": prev["sections"],
                 "is_english": None, "time_s": 0,
                "_cached": True,
            })
            print(f"{time.strftime('%H:%M:%S')} {label} SKIP (cached, {prev['chars']} chars, {prev['sections']} sections, {prev['tables']} tables)")
            continue

        try:
            text, stats, tables_out, page_chars = process(str(p))
        except Exception as e:
            print(f"    ERROR: {e}")
            traceback.print_exc()
            stats_results.append({"file": name, "error": str(e)})
            continue

        stats["file"] = name
        stats_results.append(stats)

        if text:
            meta = json.dumps({
                "chars": stats["chars"],
                "boxes_initial": stats["boxes_initial"],
                "boxes_text_merge": stats["boxes_text_merge"],
                "boxes_vertical_merge": stats["boxes_vertical_merge"],
                "sections": stats["sections"],
            }, ensure_ascii=False)
            with open(text_path, "w") as f:
                f.write(text)
                if not text.endswith("\n"):
                    f.write("\n")
                f.write("#@meta")
                f.write(meta)
                f.write("\n")

        # Write output/py/{variant}/tables/{name}.json
        tables_path = TABLES_OUT_DIR / f"{name}.json"
        with open(tables_path, "w") as f:
            json.dump(tables_out, f, ensure_ascii=False, indent=2)

        # Write charspy/{name}.json
        charspy_path = CHARSPY_OUT_DIR / f"{name}.json"
        if not charspy_path.exists():
            char_fields = ["text", "x0", "x1", "top", "bottom", "fontname", "size"]
            chars_out = {"pages": [[{k: c[k] for k in char_fields if k in c} for c in pg] for pg in page_chars]}
            with open(charspy_path, "w") as f:
                json.dump(chars_out, f, ensure_ascii=False)

        total_chars += stats["text_len"]

        print(f"{time.strftime('%H:%M:%S')} {label} — OK "
              f"chars={stats['chars']} text={stats['text_len']} "
              f"boxes_initial={stats['boxes_initial']} "
              f"text_merge={stats['boxes_text_merge']} "
              f"vertical_merge={stats['boxes_vertical_merge']} "
              f"final={stats['sections']} "
              f"tables={stats.get('tables', 0)} "
              f"({stats['time_s']}s)")

    print(f"\n{name_prefix}: {len(stats_results)} PDFs, {total_chars} chars")
    return stats_results


# ── compare-go helpers ──────────────────────────────────────────────

TEXT_GO_DIR = SCRIPT_DIR / "output" / "go" / _py_variant / "text"  # matches Go output path
# When comparing, Go may use a different variant. Try both.
_COMPARE_GO_DIRS = [
    SCRIPT_DIR / "output" / "go" / "ocr" / "text",
    SCRIPT_DIR / "output" / "go" / "noocr" / "text",
]
WHITESPACE_RE = re.compile(r"\s+")


def _extract_chars(text):
    """Extract non-whitespace characters, NFC-normalised, as a Counter."""
    cleaned = WHITESPACE_RE.sub("", text)
    normalised = unicodedata.normalize("NFKC", cleaned)
    return Counter(normalised)


def _dice_similarity(py_chars, go_chars):
    """Sørensen-Dice coefficient × 100 for two character Counters."""
    all_keys = set(py_chars) | set(go_chars)
    if not all_keys:
        return 100.0
    common = sum(min(py_chars[k], go_chars[k]) for k in all_keys)
    total = sum(py_chars.values()) + sum(go_chars.values())
    return round(common * 2 * 100 / total, 2)


def compare_with_go(threshold=95.0):
    """Compare Go and Python output per-PDF character similarity."""
    # Try multiple Go directories to find matching text files.
    go_files = {}
    for d in _COMPARE_GO_DIRS:
        if d.is_dir():
            go_files.update({p.name.replace(".txt", ""): p for p in d.glob("*.txt")})
        if go_files:
            break
    py_files = {p.name.replace(".txt", ""): p for p in TEXT_OUT_DIR.glob("*.txt")}
    common = sorted(set(go_files) & set(py_files))

    if not common:
        print(f"No matching files found between {TEXT_OUT_DIR}/ and Go dirs {[str(d) for d in _COMPARE_GO_DIRS if d.is_dir()]}")
        return

    passed, failed = 0, 0
    per_pdf = []

    for name in common:
        py_text = py_files[name].read_text(encoding="utf-8")
        go_text = go_files[name].read_text(encoding="utf-8")
        py_chars = _extract_chars(py_text)
        go_chars = _extract_chars(go_text)
        score = _dice_similarity(py_chars, go_chars)
        ok = score >= threshold

        if ok:
            passed += 1
            status = "PASS"
        else:
            failed += 1
            status = "FAIL"

        py_only = [(ch, py_chars[ch]) for ch in py_chars if ch not in go_chars]
        go_only = [(ch, go_chars[ch]) for ch in go_chars if ch not in py_chars]
        py_only.sort(key=lambda x: -x[1])
        go_only.sort(key=lambda x: -x[1])

        per_pdf.append({
            "file": name,
            "score": score,
            "pass": ok,
            "py_total": sum(py_chars.values()),
            "go_total": sum(go_chars.values()),
            "py_only_top5": [(ch, cnt) for ch, cnt in py_only[:5]],
            "go_only_top5": [(ch, cnt) for ch, cnt in go_only[:5]],
        })

        extra = ""
        if not ok:
            extra = f"  py_only: {py_only[:5]}  go_only: {go_only[:5]}"
        print(f"  [{status}] {name}: {score:.1f}% (py={sum(py_chars.values())} go={sum(go_chars.values())}){extra}")

    total = passed + failed
    print(f"\n=== {passed}/{total} passed ({100*passed//total if total else 0}%) at threshold {threshold}% ===")

    out_path = SCRIPT_DIR / "char_similarity.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump({"threshold": threshold, "passed": passed, "failed": failed,
                    "total": total, "results": per_pdf}, f, ensure_ascii=False, indent=2)
    print(f"Details saved to {out_path}")


def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--count", type=int, default=0, help="Limit to first N PDFs")
    ap.add_argument("--real", action="store_true", help="Process real_pdfs/")
    ap.add_argument("--compare-go", action="store_true", help="Compare Go vs Python output char similarity")
    ap.add_argument("--threshold", type=float, default=95.0, help="Pass threshold %% for --compare-go (default 95)")
    ap.add_argument("--single", type=str, default="", help="Process a single PDF by filename")
    args = ap.parse_args()

    if args.single:
        pdf_path = REAL_PDFS_DIR / args.single
        if not pdf_path.exists():
            print(f"PDF not found: {pdf_path}")
            sys.exit(1)
        print(f"=== Single PDF: {args.single} ===\n")
        text, stats, tables_out, _ = process(str(pdf_path))
        name = pdf_path.name
        text_path = TEXT_OUT_DIR / f"{name}.txt"
        tables_path = TABLES_OUT_DIR / f"{name}.json"
        if text:
            meta = json.dumps({
                "chars": stats["chars"],
                "boxes_initial": stats["boxes_initial"],
                "boxes_text_merge": stats["boxes_text_merge"],
                "boxes_vertical_merge": stats["boxes_vertical_merge"],
                "sections": stats["sections"],
            }, ensure_ascii=False)
            with open(text_path, "w") as f:
                f.write(text)
                if not text.endswith("\n"):
                    f.write("\n")
                f.write("#@meta")
                f.write(meta)
                f.write("\n")
        with open(tables_path, "w") as f:
            json.dump(tables_out, f, ensure_ascii=False, indent=2)
        print(f"{time.strftime('%H:%M:%S')} {args.single} — OK "
              f"chars={stats['chars']} text={stats['text_len']} "
              f"boxes_initial={stats['boxes_initial']} "
              f"text_merge={stats['boxes_text_merge']} "
              f"vertical_merge={stats['boxes_vertical_merge']} "
              f"final={stats['sections']} "
              f"tables={stats.get('tables', 0)} "
              f"({stats['time_s']}s)")
        return

    if args.compare_go:
        compare_with_go(args.threshold)
        return

    if args.real:
        print("=== Real PDFs ===\n")
        run_set(REAL_PDFS_DIR, "real",
                count=args.count if args.count else None)
    else:
        args.real = True
        print("=== Real PDFs ===\n")
        run_set(REAL_PDFS_DIR, "real",
                count=args.count if args.count else None)


if __name__ == "__main__":
    main()
