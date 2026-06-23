"""Generate table bbox reference for Go batch TSR comparison.  ~30s."""
import json, pdfplumber, os, sys

d = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'testdata', 'real_pdfs')
r = []
for f in sorted(os.listdir(d)):
    if not f.endswith('.pdf'): continue
    p = os.path.join(d, f); tbls = []
    with pdfplumber.open(p) as pdf:
        for pg, page in enumerate(pdf.pages):
            for t in (page.find_tables() or []):
                tbls.append({'page': pg, 'table_idx': len(tbls),
                             'bbox_pts': [round(v, 1) for v in list(t.bbox)]})
    r.append({'pdf': f, 'results': tbls})
out = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'testdata', 'tsr', 'tsr_bboxes_py.json')
json.dump({'results': r}, open(out, 'w'), ensure_ascii=False, indent=2)
print(f'{len(r)} PDFs, {sum(len(x["results"]) for x in r)} tables -> {out}')
