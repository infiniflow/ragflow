# Table Structure Recognition (TSR)

## Tong Quan

Table Structure Recognition (TSR) la component xu ly cau truc bang trong documents. No phan tich cac vung table da duoc detect boi Layout Recognizer de xac dinh rows, columns, cells va cau truc header. Ket qua duoc su dung de chuyen bang thanh HTML hoac natural language format.

## File Location
```
/deepdoc/vision/table_structure_recognizer.py
```

## Architecture

```
                  TABLE STRUCTURE RECOGNITION PIPELINE

                       Table Image Region
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    TABLE TRANSFORMER                             │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Model: tsr.onnx (TableTransformer)                     │   │
│  │  Detected Elements:                                      │   │
│  │  • table              • table column header              │   │
│  │  • table column       • table projected row header       │   │
│  │  • table row          • table spanning cell              │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                   STRUCTURE ALIGNMENT                            │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  • Align rows: left & right edges                       │   │
│  │  • Align columns: top & bottom edges                    │   │
│  │  • Handle spanning cells                                │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    TABLE CONSTRUCTION                            │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  • Map OCR boxes to cells                               │   │
│  │  • Identify header rows                                 │   │
│  │  • Calculate colspan/rowspan                            │   │
│  │  • Output: HTML table or Natural language              │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## TSR Labels

| Label | Description |
|-------|-------------|
| table | Overall table boundary |
| table column | Vertical column dividers |
| table row | Horizontal row dividers |
| table column header | Header row(s) at top |
| table projected row header | Row headers on left side |
| table spanning cell | Merged cells (colspan/rowspan) |

## Core Implementation

### TableStructureRecognizer Class

```python
class TableStructureRecognizer(Recognizer):
    """
    Recognize table structure (rows, columns, cells).

    Uses TableTransformer model to detect:
    - Row and column boundaries
    - Header regions
    - Spanning (merged) cells
    """

    labels = [
        "table",
        "table column",
        "table row",
        "table column header",
        "table projected row header",
        "table spanning cell",
    ]

    def __init__(self):
        model_dir = os.path.join(
            get_project_base_directory(),
            "rag/res/deepdoc"
        )
        super().__init__(self.labels, "tsr", model_dir)

    def __call__(self, images, thr=0.2):
        """
        Detect table structure in images.

        Args:
            images: List of cropped table images
            thr: Confidence threshold

        Returns:
            List of table structures with aligned rows/columns
        """
        # Run inference
        tbls = super().__call__(images, thr)

        res = []
        for tbl in tbls:
            # Convert to internal format
            lts = [{
                "label": b["type"],
                "score": b["score"],
                "x0": b["bbox"][0],
                "x1": b["bbox"][2],
                "top": b["bbox"][1],
                "bottom": b["bbox"][-1],
            } for b in tbl]

            if not lts:
                continue

            # Align row boundaries (left & right)
            lts = self._align_rows(lts)

            # Align column boundaries (top & bottom)
            lts = self._align_columns(lts)

            res.append(lts)

        return res
```

### Row/Column Alignment

```python
def _align_rows(self, lts):
    """
    Align row boundaries to consistent left/right edges.

    Process:
    1. Find all row and header elements
    2. Calculate mean left/right position
    3. Adjust elements to align
    """
    # Get row elements
    row_elements = [b for b in lts
                   if b["label"].find("row") > 0 or
                      b["label"].find("header") > 0]

    if not row_elements:
        return lts

    # Calculate alignment positions
    left_positions = [b["x0"] for b in row_elements]
    right_positions = [b["x1"] for b in row_elements]

    left = np.mean(left_positions) if len(left_positions) > 4 \
           else np.min(left_positions)
    right = np.mean(right_positions) if len(right_positions) > 4 \
            else np.max(right_positions)

    # Align rows
    for b in lts:
        if b["label"].find("row") > 0 or b["label"].find("header") > 0:
            if b["x0"] > left:
                b["x0"] = left
            if b["x1"] < right:
                b["x1"] = right

    return lts

def _align_columns(self, lts):
    """
    Align column boundaries to consistent top/bottom edges.
    """
    # Get column elements
    col_elements = [b for b in lts if b["label"] == "table column"]

    if not col_elements:
        return lts

    # Calculate alignment positions
    top_positions = [b["top"] for b in col_elements]
    bottom_positions = [b["bottom"] for b in col_elements]

    top = np.median(top_positions) if len(top_positions) > 4 \
          else np.min(top_positions)
    bottom = np.median(bottom_positions) if len(bottom_positions) > 4 \
             else np.max(bottom_positions)

    # Align columns
    for b in lts:
        if b["label"] == "table column":
            if b["top"] > top:
                b["top"] = top
            if b["bottom"] < bottom:
                b["bottom"] = bottom

    return lts
```

### Table Construction

```python
@staticmethod
def construct_table(boxes, is_english=False, html=True, **kwargs):
    """
    Construct table from OCR boxes with structure info.

    Args:
        boxes: OCR boxes with row/column assignments
        is_english: Language setting
        html: Output HTML (True) or natural language (False)

    Returns:
        HTML string or list of natural language descriptions
    """
    # 1. Extract and remove caption
    cap = ""
    i = 0
    while i < len(boxes):
        if TableStructureRecognizer.is_caption(boxes[i]):
            cap += boxes[i]["text"]
            boxes.pop(i)
        else:
            i += 1

    if not boxes:
        return []

    # 2. Classify block types
    for b in boxes:
        b["btype"] = TableStructureRecognizer.blockType(b)

    max_type = Counter([b["btype"] for b in boxes]).most_common(1)[0][0]

    # 3. Sort and assign row numbers
    rowh = [b["R_bott"] - b["R_top"] for b in boxes if "R" in b]
    rowh = np.min(rowh) if rowh else 0
    boxes = Recognizer.sort_R_firstly(boxes, rowh / 2)

    boxes[0]["rn"] = 0
    rows = [[boxes[0]]]
    btm = boxes[0]["bottom"]

    for b in boxes[1:]:
        b["rn"] = len(rows) - 1
        lst_r = rows[-1]

        # Check if new row
        if lst_r[-1].get("R", "") != b.get("R", "") or \
           (b["top"] >= btm - 3 and
            lst_r[-1].get("R", "-1") != b.get("R", "-2")):
            btm = b["bottom"]
            b["rn"] += 1
            rows.append([b])
            continue

        btm = (btm + b["bottom"]) / 2.0
        rows[-1].append(b)

    # 4. Sort and assign column numbers
    colwm = [b["C_right"] - b["C_left"] for b in boxes if "C" in b]
    colwm = np.min(colwm) if colwm else 0

    boxes = Recognizer.sort_C_firstly(boxes, colwm / 2)
    boxes[0]["cn"] = 0
    cols = [[boxes[0]]]
    right = boxes[0]["x1"]

    for b in boxes[1:]:
        b["cn"] = len(cols) - 1
        lst_c = cols[-1]

        # Check if new column
        if b["x0"] >= right and \
           lst_c[-1].get("C", "-1") != b.get("C", "-2"):
            right = b["x1"]
            b["cn"] += 1
            cols.append([b])
            continue

        right = (right + b["x1"]) / 2.0
        cols[-1].append(b)

    # 5. Build table matrix
    tbl = [[[] for _ in range(len(cols))] for _ in range(len(rows))]
    for b in boxes:
        tbl[b["rn"]][b["cn"]].append(b)

    # 6. Identify header rows
    hdset = set()
    for i in range(len(tbl)):
        cnt, h = 0, 0
        for j, arr in enumerate(tbl[i]):
            if not arr:
                continue
            cnt += 1
            if any([a.get("H") for a in arr]) or \
               (max_type == "Nu" and arr[0]["btype"] != "Nu"):
                h += 1
        if h / cnt > 0.5:
            hdset.add(i)

    # 7. Calculate spans
    tbl = TableStructureRecognizer._cal_spans(boxes, rows, cols, tbl, html)

    # 8. Output
    if html:
        return TableStructureRecognizer._html_table(cap, hdset, tbl)
    else:
        return TableStructureRecognizer._desc_table(cap, hdset, tbl, is_english)
```

### Block Type Classification

```python
@staticmethod
def blockType(b):
    """
    Classify cell content type.

    Types:
    - Dt: Date (2024-01-01, 2024年1月)
    - Nu: Number (123, 45.6, -78%)
    - Ca: Code/ID (ABC-123, XYZ_456)
    - En: English text
    - NE: Number + English mix
    - Sg: Single character
    - Nr: Person name
    - Tx: Short text (3-12 tokens)
    - Lx: Long text (>12 tokens)
    - Ot: Other
    """
    patt = [
        # Date patterns
        ("^(20|19)[0-9]{2}[年/-][0-9]{1,2}[月/-][0-9]{1,2}日*$", "Dt"),
        (r"^(20|19)[0-9]{2}年$", "Dt"),
        (r"^(20|19)[0-9]{2}[年-][0-9]{1,2}月*$", "Dt"),
        ("^[0-9]{1,2}[月-][0-9]{1,2}日*$", "Dt"),
        (r"^第*[一二三四1-4]季度$", "Dt"),
        (r"^(20|19)[0-9]{2}年*[一二三四1-4]季度$", "Dt"),
        (r"^(20|19)[0-9]{2}[ABCDE]$", "Dt"),

        # Number patterns
        ("^[0-9.,+%/ -]+$", "Nu"),

        # Code patterns
        (r"^[0-9A-Z/\._~-]+$", "Ca"),

        # English text
        (r"^[A-Z]*[a-z' -]+$", "En"),

        # Number + English mix
        (r"^[0-9.,+-]+[0-9A-Za-z/$￥%<>（）()' -]+$", "NE"),

        # Single character
        (r"^.{1}$", "Sg"),
    ]

    for p, n in patt:
        if re.search(p, b["text"].strip()):
            return n

    # Tokenize and classify
    tks = [t for t in rag_tokenizer.tokenize(b["text"]).split() if len(t) > 1]

    if len(tks) > 3:
        return "Tx" if len(tks) < 12 else "Lx"

    if len(tks) == 1 and rag_tokenizer.tag(tks[0]) == "nr":
        return "Nr"

    return "Ot"
```

### HTML Output

```python
@staticmethod
def _html_table(cap, hdset, tbl):
    """
    Convert table to HTML format.

    Features:
    - Caption support
    - Header rows (<th>)
    - Colspan/rowspan attributes
    """
    html = "<table>"

    if cap:
        html += f"<caption>{cap}</caption>"

    for i in range(len(tbl)):
        row = "<tr>"
        txts = []

        for j, arr in enumerate(tbl[i]):
            if arr is None:  # Spanned cell
                continue

            if not arr:
                row += "<td></td>" if i not in hdset else "<th></th>"
                continue

            # Get cell text
            h = min(np.min([c["bottom"] - c["top"] for c in arr]) / 2, 10)
            txt = " ".join([c["text"] for c in
                          Recognizer.sort_Y_firstly(arr, h)])
            txts.append(txt)

            # Build span attributes
            sp = ""
            if arr[0].get("colspan"):
                sp = f"colspan={arr[0]['colspan']}"
            if arr[0].get("rowspan"):
                sp += f" rowspan={arr[0]['rowspan']}"

            # Add cell
            if i in hdset:
                row += f"<th {sp}>{txt}</th>"
            else:
                row += f"<td {sp}>{txt}</td>"

        if row != "<tr>":
            row += "</tr>"
            html += "\n" + row

    html += "\n</table>"
    return html
```

### Natural Language Output

```python
@staticmethod
def _desc_table(cap, hdr_rowno, tbl, is_english):
    """
    Convert table to natural language format.

    Output format:
    "Header1: Value1; Header2: Value2 ——from 'Table Caption'"

    This format is better for:
    - RAG retrieval
    - LLM understanding
    - Semantic search
    """
    clmno = len(tbl[0])
    rowno = len(tbl)

    # Build headers dictionary
    headers = {}
    for r in sorted(list(hdr_rowno)):
        headers[r] = ["" for _ in range(clmno)]
        for i in range(clmno):
            if tbl[r][i]:
                txt = " ".join([a["text"].strip() for a in tbl[r][i]])
                headers[r][i] = txt

    # Merge hierarchical headers
    de = "的" if not is_english else " for "
    # ... header merging logic

    # Generate row descriptions
    row_txt = []
    for i in range(rowno):
        if i in hdr_rowno:
            continue

        rtxt = []
        # Find nearest header row
        r = 0
        if headers:
            _arr = [(i - r, r) for r, _ in headers.items() if r < i]
            if _arr:
                _, r = min(_arr, key=lambda x: x[0])

        # Build row text with headers
        for j in range(clmno):
            if not tbl[i][j]:
                continue
            txt = "".join([a["text"].strip() for a in tbl[i][j]])
            if not txt:
                continue

            ctt = headers[r][j] if r in headers else ""
            if ctt:
                ctt += "："
            ctt += txt
            if ctt:
                rtxt.append(ctt)

        if rtxt:
            row_txt.append("; ".join(rtxt))

    # Add caption attribution
    if cap:
        from_ = " in " if is_english else "来自"
        row_txt = [t + f"\t——{from_}"{cap}"" for t in row_txt]

    return row_txt
```

### Span Calculation

```python
@staticmethod
def _cal_spans(boxes, rows, cols, tbl, html=True):
    """
    Calculate colspan and rowspan for merged cells.

    Process:
    1. Find boxes marked as spanning cells
    2. Calculate which rows/columns they span
    3. Mark spanned cells as None (for HTML) or merge content
    """
    # Calculate row/column boundaries
    clft = [np.mean([c.get("C_left", c["x0"]) for c in cln]) for cln in cols]
    crgt = [np.mean([c.get("C_right", c["x1"]) for c in cln]) for cln in cols]
    rtop = [np.mean([c.get("R_top", c["top"]) for c in row]) for row in rows]
    rbtm = [np.mean([c.get("R_btm", c["bottom"]) for c in row]) for row in rows]

    for b in boxes:
        if "SP" not in b:  # Not a spanning cell
            continue

        b["colspan"] = [b["cn"]]
        b["rowspan"] = [b["rn"]]

        # Find spanned columns
        for j in range(len(clft)):
            if j == b["cn"]:
                continue
            if clft[j] + (crgt[j] - clft[j]) / 2 < b["H_left"]:
                continue
            if crgt[j] - (crgt[j] - clft[j]) / 2 > b["H_right"]:
                continue
            b["colspan"].append(j)

        # Find spanned rows
        for j in range(len(rtop)):
            if j == b["rn"]:
                continue
            if rtop[j] + (rbtm[j] - rtop[j]) / 2 < b["H_top"]:
                continue
            if rbtm[j] - (rbtm[j] - rtop[j]) / 2 > b["H_bott"]:
                continue
            b["rowspan"].append(j)

    # Update table with spans
    # ... merge spanned cells, mark as None for HTML

    return tbl
```

## Ascend NPU Support

```python
def _run_ascend_tsr(self, image_list, thr=0.2, batch_size=16):
    """
    Run TSR on Huawei Ascend NPU.

    Uses .om model format and ais_bench for inference.
    """
    from ais_bench.infer.interface import InferSession

    model_file_path = os.path.join(model_dir, "tsr.om")
    device_id = int(os.getenv("ASCEND_LAYOUT_RECOGNIZER_DEVICE_ID", 0))

    session = InferSession(device_id=device_id, model_path=model_file_path)

    results = []
    for batch_images in batched(image_list, batch_size):
        inputs_list = self.preprocess(batch_images)
        for ins in inputs_list:
            output_list = session.infer(feeds=[ins["image"]], mode="static")
            bb = self.postprocess(output_list, ins, thr)
            results.append(bb)

    return results
```

## Configuration

```python
# Model selection
TABLE_STRUCTURE_RECOGNIZER_TYPE = "onnx"  # onnx, ascend

# Detection parameters
TSR_PARAMS = {
    "threshold": 0.2,      # Confidence threshold
    "batch_size": 16,      # Inference batch size
}

# Output format
TABLE_OUTPUT = {
    "html": True,          # HTML format (default)
    "desc": False,         # Natural language descriptions
}
```

## Integration with PDF Parser

```python
# In pdf_parser.py
def _table_transformer_job(self, zoomin):
    """
    Run TSR on detected table regions.

    Process:
    1. Find all boxes with layout_type == "table"
    2. Crop table regions from page images
    3. Run TSR to get structure
    4. Map OCR boxes to cells
    """
    self.tsr = TableStructureRecognizer()

    # Group tables by page
    table_boxes = [b for b in self.boxes if b.get("layout_type") == "table"]

    for tb in table_boxes:
        # Crop table image
        page_img = self.page_images[tb["page_number"]]
        table_img = page_img.crop((
            tb["x0"] * zoomin,
            tb["top"] * zoomin,
            tb["x1"] * zoomin,
            tb["bottom"] * zoomin
        ))

        # Run TSR
        structure = self.tsr([np.array(table_img)])[0]

        # Map structure to OCR boxes
        self._map_structure_to_boxes(tb, structure)
```

## Related Files

- `/deepdoc/vision/table_structure_recognizer.py` - TSR implementation
- `/deepdoc/vision/recognizer.py` - Base recognizer class
- `/rag/res/deepdoc/tsr.onnx` - TSR ONNX model
- `/deepdoc/parser/pdf_parser.py` - PDF parser integration
