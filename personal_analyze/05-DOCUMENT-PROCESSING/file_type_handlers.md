# File Type Handlers

## Tong Quan

RAGFlow ho tro nhieu file formats khac nhau, moi format co parser rieng de extract noi dung va metadata. Document duoc xu ly qua unified chunk() function trong `/rag/app/naive.py`, function nay se chon parser phu hop dua tren file extension.

## File Location
```
/deepdoc/parser/          # Individual parsers
/rag/app/naive.py         # Main chunk() function
```

## Supported File Types

```
┌─────────────────────────────────────────────────────────────────┐
│                    FILE TYPE HANDLERS                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐           │
│  │    PDF      │   │   Office    │   │    Text     │           │
│  │  .pdf       │   │ .docx .xlsx │   │ .txt .md    │           │
│  │  .ppt       │   │ .pptx .doc  │   │ .csv .json  │           │
│  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘           │
│         │                 │                 │                   │
│         ▼                 ▼                 ▼                   │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐           │
│  │ DeepDOC     │   │ python-docx │   │ Direct      │           │
│  │ MinerU      │   │ openpyxl    │   │ Read        │           │
│  │ Docling     │   │ python-pptx │   │             │           │
│  │ TCADP       │   │ tika        │   │             │           │
│  │ VisionLLM   │   │             │   │             │           │
│  └─────────────┘   └─────────────┘   └─────────────┘           │
│                                                                  │
│  ┌─────────────┐   ┌─────────────┐                              │
│  │    Web      │   │   Image     │                              │
│  │ .html .htm  │   │ .jpg .png   │                              │
│  │             │   │ .tiff       │                              │
│  └──────┬──────┘   └──────┬──────┘                              │
│         │                 │                                      │
│         ▼                 ▼                                      │
│  ┌─────────────┐   ┌─────────────┐                              │
│  │BeautifulSoup│   │ Vision LLM  │                              │
│  │ html5lib    │   │             │                              │
│  └─────────────┘   └─────────────┘                              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Main Entry Point

```python
# /rag/app/naive.py
def chunk(filename, binary=None, from_page=0, to_page=100000,
          lang="Chinese", callback=None, **kwargs):
    """
    Main chunking function for all file types.

    Args:
        filename: File name with extension
        binary: File binary content
        from_page: Start page (for paginated docs)
        to_page: End page
        lang: Language hint (Chinese/English)
        callback: Progress callback(progress, message)
        **kwargs: Additional options (parser_config, tenant_id, etc.)

    Returns:
        List of tokenized chunks ready for indexing
    """
    parser_config = kwargs.get("parser_config", {
        "chunk_token_num": 512,
        "delimiter": "\n!?。；！？",
        "layout_recognize": "DeepDOC"
    })

    # Route to appropriate handler based on extension
    if re.search(r"\.pdf$", filename, re.IGNORECASE):
        return _handle_pdf(...)
    elif re.search(r"\.docx$", filename, re.IGNORECASE):
        return _handle_docx(...)
    elif re.search(r"\.(csv|xlsx?)$", filename, re.IGNORECASE):
        return _handle_excel(...)
    # ... more handlers
```

## PDF Handler

### Parser Selection

```python
# PDF Parser options in /rag/app/naive.py
PARSERS = {
    "deepdoc":   by_deepdoc,    # Default: OCR + Layout + TSR
    "mineru":    by_mineru,     # MinerU external parser
    "docling":   by_docling,    # Docling external parser
    "tcadp":     by_tcadp,      # Tencent Cloud ADP
    "plaintext": by_plaintext,  # Plain text or Vision LLM
}

def by_deepdoc(filename, binary=None, from_page=0, to_page=100000,
               lang="Chinese", callback=None, **kwargs):
    """
    DeepDOC parser: RAGFlow's native PDF parser.

    Features:
    - OCR with PaddleOCR
    - Layout detection with Detectron2
    - Table structure recognition
    - Figure extraction with Vision LLM
    """
    pdf_parser = PdfParser()
    sections, tables = pdf_parser(
        filename if not binary else binary,
        from_page=from_page,
        to_page=to_page,
        callback=callback
    )

    # Optional: Vision LLM for figure understanding
    tables = vision_figure_parser_pdf_wrapper(
        tbls=tables,
        callback=callback,
        **kwargs
    )

    return sections, tables, pdf_parser

def by_plaintext(filename, binary=None, from_page=0, to_page=100000,
                 callback=None, **kwargs):
    """
    Plain text or Vision LLM parser.

    Options:
    - "Plain Text": Extract text only, no layout
    - Vision LLM: Use multimodal LLM for understanding
    """
    if kwargs.get("layout_recognizer", "") == "Plain Text":
        pdf_parser = PlainParser()
    else:
        vision_model = LLMBundle(
            kwargs["tenant_id"],
            LLMType.IMAGE2TEXT,
            llm_name=kwargs.get("layout_recognizer", "")
        )
        pdf_parser = VisionParser(vision_model=vision_model, **kwargs)

    sections, tables = pdf_parser(
        filename if not binary else binary,
        from_page=from_page,
        to_page=to_page,
        callback=callback
    )
    return sections, tables, pdf_parser
```

### PdfParser Class

```python
# /deepdoc/parser/pdf_parser.py
class RAGFlowPdfParser:
    """
    Main PDF parser with full document understanding.

    Pipeline:
    1. Image extraction (pdfplumber)
    2. OCR (PaddleOCR)
    3. Layout detection (Detectron2)
    4. Table structure recognition
    5. Text merging (XGBoost)
    6. Table/figure extraction
    """

    def __call__(self, filename, from_page=0, to_page=100000,
                 zoomin=3, callback=None):
        # 1. Extract page images
        self.__images__(filename, zoomin, from_page, to_page, callback)

        # 2. Run OCR
        self.__ocr(callback, 0.4, 0.63)

        # 3. Layout detection
        self._layouts_rec(zoomin)

        # 4. Table structure
        self._table_transformer_job(zoomin)

        # 5. Text merging
        self._text_merge(zoomin=zoomin)
        self._naive_vertical_merge()
        self._concat_downward()
        self._final_reading_order_merge()

        # 6. Extract tables/figures
        tbls = self._extract_table_figure(True, zoomin, True, True)

        return [(b["text"], self._line_tag(b, zoomin))
                for b in self.boxes], tbls
```

## DOCX Handler

```python
# /deepdoc/parser/docx_parser.py
class RAGFlowDocxParser:
    """
    Microsoft Word (.docx) parser.

    Features:
    - Paragraph extraction with styles
    - Table extraction with structure
    - Embedded image extraction
    - Heading hierarchy for table context
    """

    def __call__(self, fnm, from_page=0, to_page=100000):
        self.doc = Document(fnm) if isinstance(fnm, str) \
                   else Document(BytesIO(fnm))

        pn = 0  # Current page
        lines = []

        # Extract paragraphs
        for p in self.doc.paragraphs:
            if pn > to_page:
                break
            if from_page <= pn < to_page:
                if p.text.strip():
                    # Get embedded images
                    current_image = self.get_picture(self.doc, p)
                    lines.append((
                        self._clean(p.text),
                        [current_image],
                        p.style.name if p.style else ""
                    ))

            # Track page breaks
            for run in p.runs:
                if 'lastRenderedPageBreak' in run._element.xml:
                    pn += 1

        # Extract tables with context
        tbls = []
        for i, tb in enumerate(self.doc.tables):
            title = self._get_nearest_title(i, fnm)
            html = self._table_to_html(tb, title)
            tbls.append(((None, html), ""))

        return lines, tbls

    def get_picture(self, document, paragraph):
        """
        Extract embedded images from paragraph.

        Handles:
        - Inline images (blip elements)
        - Multiple images (concat together)
        - Image format errors (graceful skip)
        """
        imgs = paragraph._element.xpath('.//pic:pic')
        if not imgs:
            return None

        res_img = None
        for img in imgs:
            embed = img.xpath('.//a:blip/@r:embed')
            if not embed:
                continue

            try:
                related_part = document.part.related_parts[embed[0]]
                image_blob = related_part.image.blob
                image = Image.open(BytesIO(image_blob)).convert('RGB')

                if res_img is None:
                    res_img = image
                else:
                    res_img = concat_img(res_img, image)
            except Exception:
                continue

        return res_img
```

## Excel Handler

```python
# /deepdoc/parser/excel_parser.py
class RAGFlowExcelParser:
    """
    Excel/CSV parser.

    Supports:
    - .xlsx, .xls (openpyxl, pandas)
    - .csv (pandas)
    - Multiple sheets
    - HTML and Markdown output
    """

    def __call__(self, fnm):
        """
        Parse Excel to natural language descriptions.

        Output format:
        "Header1: Value1; Header2: Value2 ——SheetName"
        """
        wb = self._load_excel_to_workbook(fnm)
        res = []

        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            rows = list(ws.rows)

            if not rows:
                continue

            # First row as headers
            ti = list(rows[0])

            # Process data rows
            for r in rows[1:]:
                fields = []
                for i, c in enumerate(r):
                    if not c.value:
                        continue
                    t = str(ti[i].value) if i < len(ti) else ""
                    t += ("：" if t else "") + str(c.value)
                    fields.append(t)

                line = "; ".join(fields)
                if sheetname.lower().find("sheet") < 0:
                    line += " ——" + sheetname
                res.append(line)

        return res

    def html(self, fnm, chunk_rows=256):
        """
        Convert to HTML tables with chunking.

        Splits large tables into chunks of chunk_rows rows.
        """
        wb = self._load_excel_to_workbook(fnm)
        tb_chunks = []

        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            rows = list(ws.rows)

            # Build header row
            tb_rows_0 = "<tr>"
            for t in list(rows[0]):
                tb_rows_0 += f"<th>{escape(str(t.value or ''))}</th>"
            tb_rows_0 += "</tr>"

            # Chunk data rows
            for chunk_i in range((len(rows) - 1) // chunk_rows + 1):
                tb = f"<table><caption>{sheetname}</caption>"
                tb += tb_rows_0

                start = 1 + chunk_i * chunk_rows
                end = min(start + chunk_rows, len(rows))

                for r in rows[start:end]:
                    tb += "<tr>"
                    for c in r:
                        tb += f"<td>{escape(str(c.value or ''))}</td>"
                    tb += "</tr>"
                tb += "</table>\n"
                tb_chunks.append(tb)

        return tb_chunks
```

## PowerPoint Handler

```python
# /deepdoc/parser/ppt_parser.py
class RAGFlowPptParser:
    """
    PowerPoint (.pptx) parser.

    Features:
    - Slide-by-slide extraction
    - Shape hierarchy (text frames, tables, groups)
    - Bulleted list formatting
    - Embedded table extraction
    """

    def __call__(self, fnm, from_page, to_page, callback=None):
        ppt = Presentation(fnm) if isinstance(fnm, str) \
              else Presentation(BytesIO(fnm))

        txts = []
        self.total_page = len(ppt.slides)

        for i, slide in enumerate(ppt.slides):
            if i < from_page:
                continue
            if i >= to_page:
                break

            texts = []
            # Sort shapes by position (top-to-bottom, left-to-right)
            for shape in sorted(slide.shapes,
                              key=lambda x: (
                                  (x.top or 0) // 10,
                                  x.left or 0
                              )):
                txt = self._extract(shape)
                if txt:
                    texts.append(txt)

            txts.append("\n".join(texts))

        return txts

    def _extract(self, shape):
        """
        Extract text from shape recursively.

        Handles:
        - Text frames with paragraphs
        - Tables (shape_type == 19)
        - Group shapes (shape_type == 6)
        """
        # Text frame
        if hasattr(shape, 'has_text_frame') and shape.has_text_frame:
            texts = []
            for paragraph in shape.text_frame.paragraphs:
                if paragraph.text.strip():
                    texts.append(self._get_bulleted_text(paragraph))
            return "\n".join(texts)

        shape_type = shape.shape_type

        # Table
        if shape_type == 19:
            tb = shape.table
            rows = []
            for i in range(1, len(tb.rows)):
                rows.append("; ".join([
                    f"{tb.cell(0, j).text}: {tb.cell(i, j).text}"
                    for j in range(len(tb.columns))
                    if tb.cell(i, j)
                ]))
            return "\n".join(rows)

        # Group shape
        if shape_type == 6:
            texts = []
            for p in sorted(shape.shapes,
                          key=lambda x: (x.top // 10, x.left)):
                t = self._extract(p)
                if t:
                    texts.append(t)
            return "\n".join(texts)

        return ""
```

## HTML Handler

```python
# /deepdoc/parser/html_parser.py
class RAGFlowHtmlParser:
    """
    HTML parser using BeautifulSoup.

    Features:
    - Block tag detection (p, div, h1-h6, table, etc.)
    - Script/style removal
    - Table extraction
    - Heading hierarchy to markdown
    """

    BLOCK_TAGS = [
        "h1", "h2", "h3", "h4", "h5", "h6",
        "p", "div", "article", "section", "aside",
        "ul", "ol", "li",
        "table", "pre", "code", "blockquote",
        "figure", "figcaption"
    ]

    TITLE_TAGS = {
        "h1": "#", "h2": "##", "h3": "###",
        "h4": "####", "h5": "#####", "h6": "######"
    }

    def __call__(self, fnm, binary=None, chunk_token_num=512):
        if binary:
            encoding = find_codec(binary)
            txt = binary.decode(encoding, errors="ignore")
        else:
            with open(fnm, "r", encoding=get_encoding(fnm)) as f:
                txt = f.read()

        return self.parser_txt(txt, chunk_token_num)

    @classmethod
    def parser_txt(cls, txt, chunk_token_num):
        """
        Parse HTML text to chunks.

        Process:
        1. Clean HTML (remove scripts, styles, comments)
        2. Recursively extract text from body
        3. Merge blocks by block_id
        4. Chunk by token limit
        """
        soup = BeautifulSoup(txt, "html5lib")

        # Remove unwanted elements
        for tag in soup.find_all(["style", "script"]):
            tag.decompose()

        # Extract text recursively
        temp_sections = []
        cls.read_text_recursively(
            soup.body, temp_sections,
            chunk_token_num=chunk_token_num
        )

        # Merge and chunk
        block_txt_list, table_list = cls.merge_block_text(temp_sections)
        sections = cls.chunk_block(block_txt_list, chunk_token_num)

        # Add tables
        for table in table_list:
            sections.append(table.get("content", ""))

        return sections
```

## Text Handler

```python
# /deepdoc/parser/txt_parser.py
class RAGFlowTxtParser:
    """
    Plain text parser with delimiter-based chunking.

    Supports:
    - .txt, .py, .js, .java, .c, .cpp, .h, .php,
      .go, .ts, .sh, .cs, .kt, .sql
    """

    def __call__(self, fnm, binary=None, chunk_token_num=128,
                 delimiter="\n!?;。；！？"):
        txt = get_text(fnm, binary)
        return self.parser_txt(txt, chunk_token_num, delimiter)

    @classmethod
    def parser_txt(cls, txt, chunk_token_num=128,
                   delimiter="\n!?;。；！？"):
        """
        Split text by delimiters and chunk by token count.
        """
        cks = [""]
        tk_nums = [0]

        # Parse delimiter (support regex patterns)
        dels = cls._parse_delimiter(delimiter)
        secs = re.split(r"(%s)" % dels, txt)

        for sec in secs:
            if re.match(f"^{dels}$", sec):
                continue
            cls._add_chunk(sec, cks, tk_nums, chunk_token_num)

        return [[c, ""] for c in cks]
```

## Markdown Handler

```python
# /deepdoc/parser/markdown_parser.py
class RAGFlowMarkdownParser:
    """
    Markdown parser with element extraction.

    Features:
    - Heading hierarchy detection
    - Table extraction (separate or inline)
    - Image URL extraction and loading
    - Code block handling
    """

    def __call__(self, filename, binary=None, separate_tables=True,
                 delimiter=None, return_section_images=False):
        if binary:
            encoding = find_codec(binary)
            txt = binary.decode(encoding, errors="ignore")
        else:
            with open(filename, "r") as f:
                txt = f.read()

        # Extract tables
        remainder, tables = self.extract_tables_and_remainder(
            f'{txt}\n',
            separate_tables=separate_tables
        )

        # Extract elements with metadata
        extractor = MarkdownElementExtractor(txt)
        image_refs = self.extract_image_urls_with_lines(txt)
        element_sections = extractor.extract_elements(
            delimiter,
            include_meta=True
        )

        # Process sections with images
        sections = []
        section_images = []
        image_cache = {}

        for element in element_sections:
            content = element["content"]
            start_line = element["start_line"]
            end_line = element["end_line"]

            # Find images in section
            urls_in_section = [
                ref["url"] for ref in image_refs
                if start_line <= ref["line"] <= end_line
            ]

            imgs = []
            if urls_in_section:
                imgs, image_cache = self.load_images_from_urls(
                    urls_in_section, image_cache
                )

            combined_image = None
            if imgs:
                combined_image = reduce(concat_img, imgs) \
                                if len(imgs) > 1 else imgs[0]

            sections.append((content, ""))
            section_images.append(combined_image)

        # Convert tables to HTML
        tbls = []
        for table in tables:
            html = markdown(table, extensions=['markdown.extensions.tables'])
            tbls.append(((None, html), ""))

        if return_section_images:
            return sections, tbls, section_images
        return sections, tbls
```

## JSON Handler

```python
# /deepdoc/parser/json_parser.py
class RAGFlowJsonParser:
    """
    JSON/JSONL parser.

    Supports:
    - .json (single object or array)
    - .jsonl, .ldjson (line-delimited JSON)
    - Nested object flattening
    """

    def __call__(self, binary, chunk_token_num=512):
        txt = binary.decode('utf-8', errors='ignore')

        # Try parsing as JSONL first
        lines = txt.strip().split('\n')
        results = []

        for line in lines:
            try:
                obj = json.loads(line)
                flat = self._flatten(obj)
                results.append(self._to_text(flat))
            except json.JSONDecodeError:
                # Try as full JSON
                try:
                    data = json.loads(txt)
                    if isinstance(data, list):
                        for item in data:
                            flat = self._flatten(item)
                            results.append(self._to_text(flat))
                    else:
                        flat = self._flatten(data)
                        results.append(self._to_text(flat))
                except:
                    pass
                break

        return self._chunk(results, chunk_token_num)
```

## File Extension Routing

```python
# In /rag/app/naive.py chunk() function
FILE_HANDLERS = {
    # PDF
    r"\.pdf$": _handle_pdf,

    # Microsoft Office
    r"\.docx$": _handle_docx,
    r"\.doc$": _handle_doc,  # Requires tika
    r"\.pptx?$": _handle_ppt,
    r"\.(csv|xlsx?)$": _handle_excel,

    # Text
    r"\.(txt|py|js|java|c|cpp|h|php|go|ts|sh|cs|kt|sql)$": _handle_txt,
    r"\.(md|markdown)$": _handle_markdown,

    # Web
    r"\.(htm|html)$": _handle_html,

    # Data
    r"\.(json|jsonl|ldjson)$": _handle_json,
}

def chunk(filename, binary=None, ...):
    for pattern, handler in FILE_HANDLERS.items():
        if re.search(pattern, filename, re.IGNORECASE):
            return handler(filename, binary, ...)

    raise NotImplementedError(
        "file type not supported yet"
    )
```

## Configuration

```python
# Parser configuration options
parser_config = {
    # Chunking
    "chunk_token_num": 512,        # Max tokens per chunk
    "delimiter": "\n!?。；！？",   # Chunk boundaries
    "overlapped_percent": 0,       # Chunk overlap

    # PDF specific
    "layout_recognize": "DeepDOC", # DeepDOC, MinerU, Plain Text, etc.
    "analyze_hyperlink": True,     # Extract URLs from documents

    # Excel specific
    "html4excel": False,           # Output as HTML tables
}
```

## Related Files

- `/deepdoc/parser/__init__.py` - Parser exports
- `/deepdoc/parser/pdf_parser.py` - PDF parser
- `/deepdoc/parser/docx_parser.py` - Word parser
- `/deepdoc/parser/excel_parser.py` - Excel/CSV parser
- `/deepdoc/parser/ppt_parser.py` - PowerPoint parser
- `/deepdoc/parser/html_parser.py` - HTML parser
- `/deepdoc/parser/txt_parser.py` - Text parser
- `/deepdoc/parser/markdown_parser.py` - Markdown parser
- `/deepdoc/parser/json_parser.py` - JSON parser
- `/rag/app/naive.py` - Main chunk() function
