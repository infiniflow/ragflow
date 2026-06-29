#!/usr/bin/env python3
"""Export raw DOCX body blocks as JSON for parity testing."""

import json, sys, os, base64
from docx import Document
from docx.text.paragraph import Paragraph

A_NS = 'http://schemas.openxmlformats.org/drawingml/2006/main'
R_NS = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
V_NS = 'urn:schemas-microsoft-com:vml'


def extract_image(paragraph, doc):
    el = paragraph._element
    for blip in el.iter('{%s}blip' % A_NS):
        embed = blip.get('{%s}embed' % R_NS)
        if embed and embed in doc.part.rels:
            img = doc.part.rels[embed].target_part
            return (img.content_type, base64.b64encode(img.blob).decode())
    for imagedata in el.iter('{%s}imagedata' % V_NS):
        rid = imagedata.get('{%s}id' % R_NS)
        if rid and rid in doc.part.rels:
            img = doc.part.rels[rid].target_part
            return (img.content_type, base64.b64encode(img.blob).decode())
    return None


def extract_blocks(filepath):
    doc = Document(filepath)
    blocks = []

    for el in doc._element.body:
        if el.tag.endswith('}p'):
            p = Paragraph(el, doc)
            blocks.append({
                "type": "paragraph",
                "text": p.text,
                "style": p.style.name if p.style else "",
                "image": extract_image(p, doc),
            })
        elif el.tag.endswith('}tbl'):
            n = len([b for b in blocks if b["type"] == "table"])
            table = doc.tables[n]
            rows = [[cell.text for cell in row.cells] for row in table.rows]
            blocks.append({"type": "table", "rows": rows})

    return blocks


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <docx_file> [output_dir]", file=sys.stderr)
        sys.exit(1)

    blocks = extract_blocks(sys.argv[1])

    if len(sys.argv) >= 3:
        out_dir = sys.argv[2]
        os.makedirs(out_dir, exist_ok=True)
        basename = os.path.splitext(os.path.basename(sys.argv[1]))[0]
        out_path = os.path.join(out_dir, f"{basename}_blocks.json")
        with open(out_path, "w") as f:
            json.dump(blocks, f, indent=2, ensure_ascii=False)
        print(f"Wrote {out_path} ({len(blocks)} blocks)", file=sys.stderr)
    else:
        json.dump(blocks, sys.stdout, indent=2, ensure_ascii=False)
