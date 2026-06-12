# RAGFlow Task Executor：Python → Go 迁移设计

## 概述

当前 task executor（`rag/svr/task_executor.py`，~1926 行）纯 Python。Go 侧（`internal/`）已有 tokenizer（CGo）、MinIO、ES/Infinity、LLM（30+ provider）等完整基础设施。

**原则**：能迁尽迁。Python 仅保留 pdfplumber 渲染 + ONNX OCR 两个硬卡点。

**跨语言边界**：`sections + tables + page_images`。Python 输出这三样，Go 接手全部 NLP + 后处理 + 入库。

---

## 1. 不可迁移部分 — 逐文件/逐函数

### 1.1 `deepdoc/parser/pdf_parser.py`（2080 行）

`RAGFlowPdfParser` 是 PDF 解析主类，核心入口 `__images__()` 调 pdfplumber 渲染页面位图，其余函数处理 `page.chars`（逐字符 dict）和 ONNX 推理。

| 函数 | 行数 | 可迁？ | 说明 |
|------|------|--------|------|
| `__images__()` | ~150 | ❌ | `pdfplumber.open()` + `page.to_image()` → page_images / page_chars — **唯一硬卡点** |
| `__ocr()` | ~200 | ❌ | ONNX 文字识别 |
| `_layouts_rec()` | ~400 | ❌ | ONNX 布局识别 |
| `_table_transformer_job()` | ~200 | ❌ | ONNX 表格识别 |
| `_evaluate_table_orientation()` | ~50 | ❌ | 依赖 page_images + ONNX |
| `_ocr_rotated_tables()` | ~150 | ❌ | 同上 |
| `_text_merge()` | ~65 | ❌ | 输入 pdfplumber char dict（`x0/x1/top/bottom`） |
| `_assign_column()` | ~60 | ❌ | 同上 |
| `_concat_downward()` | ~100 | ❌ | 同上 |
| `_filter_forpages()` | ~45 | ❌ | 同上 |
| `_merge_with_same_bullet()` | ~25 | ❌ | 同上 |
| `_final_reading_order_merge()` | ~20 | ❌ | 同上 |
| `_naive_vertical_merge()` | ~80 | ❌ | 同上 |
| `__filterout_scraps()` | ~60 | ❌ | 同上 |
| `_line_tag()` | ~14 | ❌ | 同上 |
| `proj_match()` | ~23 | ❌ | 同上 |
| `_updown_concat_features()` | ~70 | ❌ | 输入 pdfplumber char dict + `rag_tokenizer` |
| `_extract_table_figure()` | ~200 | ❌ | 依赖 page_images + page_chars |
| `parse_into_bboxes()` | ~40 | ❌ | 布局分析入口，依赖上面所有 |
| `_parse_loaded_window_into_bboxes()` | ~100 | ❌ | 同上 |
| `total_page_number()` (static) | ~6 | ✅ | 调 pdfplumber 取页数 → **Go `pdfcpu.PageCount()`**，Go 侧独立实现 |
| `sort_X_by_page()` (static) | ~10 | ❌ | 调用方在 `_text_merge()`（不可迁） |
| `__char_width()`, `__height()`, `_x_dis()`, `_y_dis()` | ~10 | ❌ | 调用方 `_updown_concat_features()` → `_text_merge()`（不可迁） |
| `_match_proj()` | ~10 | ❌ | 同上 |
| `_has_color()` | ~6 | ❌ | 调用方 `__images__()`（不可迁） |
| `_is_garbled_char()` (static) | ~20 | ❌ | 调用方 `_is_garbled_text()` → `_is_garbled_by_font_encoding()` → `__images__()`（不可迁） |
| `_is_garbled_text()` (static) | ~20 | ❌ | 同上 |
| `_has_subset_font_prefix()` (static) | ~5 | ❌ | 同上 |
| `_is_garbled_by_font_encoding()` (static) | ~45 | ❌ | 调用方 `__images__()`（不可迁） |
| `_offset_position_tag()` (static) | ~8 | ❌ | 调用方 `_to_global_boxes()` → `parse_into_bboxes()`（不可迁） |
| `_to_global_boxes()` | ~15 | ❌ | 调用方 `parse_into_bboxes()`（不可迁） |
| **`crop()`** | ~110 | **✅** | 调用方迁到 Go（`tokenize_chunks`）→ **base64 绕过（附录 A），Go `image`** |
| **`extract_positions()`** (static) | ~6 | **✅** | 调用方迁到 Go（`crop` 和 `append_context2table`）→ **Go `regexp`** |
| **`remove_tag()`** (static) | ~2 | **✅** | 调用方迁到 Go（`naive_merge`）→ **Go `regexp`** |
| `get_position()` | ~20 | ❌ | 调用方 `__filterout_scraps()`（不可迁） |
| **合计** | **2080** | | **核心可迁 ~124 行（crop + extract_positions + remove_tag + total_page_number），其余全部不可迁** |
| `__call__()` | ~20 | ❌ | 入口，聚合 `__images__()` + `parse_into_bboxes()` |
| **合计** | **2080** | | **核心可迁 ~135 行，工具函数 ~145 行（随 pdfplumber 替换迁移），其余 1800 行不可迁** |

### 1.2 `deepdoc/parser/mineru_parser.py`（~900 行）

继承 `RAGFlowPdfParser`，核心是 HTTP 调外部 MinerU 服务 + JSON 后处理。pdfplumber 仅用于 fallback。

| 函数 | 行数 | 可迁？ | 说明 |
|------|------|--------|------|
| `parse_pdf()` / `_build_payload()` / `_send_request()` 等 HTTP 逻辑 | ~450 | ✅ | HTTP client + JSON 解析 → **Go `http.Client`** |
| `_extract_zip_no_root()` / Zip 处理 | ~60 | ✅ | → **Go `archive/zip`** |
| `_transfer_to_sections()` / `_transfer_to_tables()` 等结果转换 | ~150 | ✅ | JSON → sections 纯逻辑 |
| `LANGUAGE_TO_MINERU_MAP` 等配置枚举 | ~80 | ✅ | → **Go const/map** |
| `check_installation()` 等工具方法 | ~30 | ✅ | |
| `__images__()` | ~15 | ❌ | pdfplumber fallback |
| `crop()` | ~100 | **✅** | base64 绕过（附录 A） |
| **合计** | **~900** | | **~870 行可迁 (97%)** |

### 1.3 `deepdoc/parser/paddleocr_parser.py`（~700 行）

同上模式，HTTP 调外部 PaddleOCR 服务。

| 函数 | 行数 | 可迁？ | 说明 |
|------|------|--------|------|
| `parse_pdf()` / `_send_request()` / `_prepare_file_data()` 等 HTTP 逻辑 | ~350 | ✅ | → **Go `http.Client`** |
| `_transfer_to_sections()` / `_transfer_to_tables()` | ~100 | ✅ | JSON → sections 纯逻辑 |
| `PaddleOCRConfig` / `PaddleOCRVLConfig` 等配置类 | ~80 | ✅ | → **Go struct** |
| `check_installation()` 等工具方法 | ~30 | ✅ | |
| `_remove_images_from_markdown()` / `_normalize_bbox()` | ~10 | ✅ | 纯逻辑 |
| `__images__()` | ~10 | ❌ | pdfplumber fallback |
| `crop()` | ~100 | **✅** | base64 绕过（附录 A） |
| `extract_positions()` (static) | ~6 | **✅** | → **Go `regexp`**（与 pdf_parser 重复实现） |
| **合计** | **~700** | | **~600 行可迁 (86%)** |

### 1.4 `deepdoc/parser/opendataloader_parser.py`（~430 行）

同上模式，HTTP 调外部 OpenDataLoader 服务。

| 函数 | 行数 | 可迁？ | 说明 |
|------|------|--------|------|
| `parse_pdf()` / HTTP 逻辑 | ~150 | ✅ | → **Go `http.Client`** |
| `_iter_elements()` / `_element_text()` / `_element_html()` | ~70 | ✅ | JSON 递归遍历纯逻辑 |
| `_bbox_from_element()` / `_as_float()` 等 BBox 工具 | ~70 | ✅ | 纯坐标处理 |
| `check_installation()` 等工具 | ~15 | ✅ | |
| `__images__()` | ~15 | ❌ | pdfplumber fallback |
| `crop()` | ~100 | **✅** | base64 绕过（附录 A） |
| **合计** | **~430** | | **~320 行可迁 (74%)** |

### 1.5 `deepdoc/vision/`（10 文件）

| 文件 | 依赖 | 可迁？ |
|------|------|--------|
| `ocr.py`, `layout_recognizer.py`, `table_structure_recognizer.py` | `onnxruntime` | ❌ |
| `recognizer.py`, `t_ocr.py`, `t_recognizer.py` | `onnxruntime` | ❌ |
| `operators.py` | `cv2` + `numpy` | ❌ |
| `postprocess.py` | `cv2` + `numpy` | ❌ |
| `seeit.py` | `cv2` + `numpy` | ❌ |

**合计：全部 10 文件不可迁。** ONNX 模型是 RAGFlow 核心竞争力，Go onnxruntime CGO 绑定不成熟，预处理/后处理深耦合 numpy。

### 1.6 `deepdoc/parser/figure_parser.py`（281 行）

| 函数 | 行数 | 可迁？ | 说明 |
|------|------|--------|------|
| `vision_figure_parser_pdf_wrapper()` | ~80 | **✅** | table images base64 绕过（附录 B），Go LLM |
| `vision_figure_parser_docx_wrapper()` | ~50 | ✅ | Go `image` + Go LLM |
| `vision_figure_parser_docx_wrapper_naive()` | ~60 | ✅ | 同上 |
| `vision_figure_parser_figure_xlsx_wrapper()` | ~40 | ✅ | 同上 |
| `vision_figure_parser_figure_data_wrapper()` | ~15 | ✅ | 纯 PIL→tuple 转换，Go `image` |
| `VisionFigureParser` 类 | ~36 | ✅ | Go LLM 调用 |
| **合计** | **281** | | **281 行全部可迁** |

### 1.7 `deepdoc/parser/utils.py`（55 行）

| 函数 | 行数 | 可迁？ | 说明 |
|------|------|--------|------|
| `get_text()` | ~13 | ✅ | 编码检测 + decode → Go `charset.DetermineEncoding` |
| `extract_pdf_outlines()` | ~42 | ✅ | pypdf→**Go `pdfcpu` 读 outline 树** |
| **合计** | **55** | | **55 行全部可迁** |

### 1.8 其余不可迁文件

| 文件 | 原因 |
|------|------|
| `rag/app/picture.py`（134行） | 依赖 `deepdoc.vision.OCR` |
| `api/utils/file_utils.py` | pdfplumber + Ghostscript（API 层用） |

### 1.9 不可迁移汇总

```
deepdoc/parser/pdf_parser.py              ← __images__() + ONNX + pdfplumber char dict 布局分析
deepdoc/vision/ 10 文件                    ← onnxruntime + cv2
rag/app/picture.py                        ← 依赖 deepdoc.vision.OCR
api/utils/file_utils.py                   ← pdfplumber + Ghostscript

共 13 文件（从最初 18 文件缩减 5 个）
Python 保留总行数: ~2200 行（从 ~5600 行缩减 61%）
```

---

## 2. 可迁移部分 — 逐函数

### 2.1 `rag/nlp/__init__.py` — NLP 后处理

| 函数 | 行数 | Go 替代 |
|------|------|---------|
| `tokenize()` | ~6 | `internal/tokenizer.Tokenize()` + `FineGrainedTokenize()`（已有） |
| `split_with_pattern()` | ~25 | `internal/tokenizer` + Go `regexp` |
| `tokenize_chunks()` | ~25 | `internal/tokenizer`；crop 从 page_images 裁剪（附录 A） |
| `tokenize_chunks_with_images()` | ~20 | `internal/tokenizer` |
| `doc_tokenize_chunks_with_images()` | ~25 | `internal/tokenizer` |
| `tokenize_table()` | ~20 | `internal/tokenizer` |
| `naive_merge()` | ~55 | `tiktoken-go` + Go `regexp` |
| `naive_merge_with_images()` | ~70 | 同上 + Go `image` |
| `naive_merge_docx()` | ~100 | 同上 + Go `image` |
| `concat_img()` | ~40 | Go `image`（`NewRGBA` + `draw.Draw`） |
| `add_positions()` | ~10 | Go struct 赋值 |
| `append_context2table_image4pdf()` | ~100 | Python 侧执行（sections 输出前），不跨边界 |
| `attach_media_context()` | ~40 | 非 PDF 用，Go 重写 |
| **合计** | **~556** | **全部可迁** |

### 2.2 `rag/svr/task_executor.py` — 后处理管线

| 函数 | 行数 | Go 替代 |
|------|------|---------|
| `build_chunks()` — `chunker.chunk()` 调用 | ~70 | Go 自己调 parser + NLP |
| `build_chunks()` — `upload_to_minio()` / `image2id()` | ~80 | Go `image.Decode` → `jpeg.Encode` → `MinioStorage.Put()` |
| `build_chunks()` — `doc_keyword_extraction()` | ~30 | Go LLM（Layer 4） |
| `build_chunks()` — `doc_question_proposal()` | ~30 | Go LLM |
| `build_chunks()` — `gen_metadata_task()` | ~30 | Go LLM |
| `build_chunks()` — `doc_content_tagging()` | ~50 | Go LLM |
| `do_handle_task()` — `embedding()` | ~70 | Go Embedding API |
| `do_handle_task()` — `insert_chunks()` | ~80 | Go `engine.InsertChunks()`（已有） |
| `build_TOC()` | ~45 | Go LLM（Layer 4） |
| `init_kb()` | ~5 | Go dao |
| `do_handle_task()` 主流程 | ~200 | Go executor 主循环 |
| `collect()` — Redis Stream 消费 | ~60 | **NATS JetStream** |
| `get_storage_binary()` | ~10 | Go `MinioStorage.Get()`（已有） |
| `set_progress()` | ~25 | Go `sendTaskProgress()`（已有） |
| `report_status()` | ~80 | Go `heartbeatLoop()`（已有） |
| `RAPTOR` / `GraphRAG` / `Memory` | ~600 | **后续 Phase 处理** |
| **合计（除去 RAPTOR/GraphRAG/Memory）** | **~900** | **全部可迁** |

### 2.3 `rag/prompts/generator.py` — LLM 富化

| 函数 | Go 替代 |
|------|---------|
| `keyword_extraction()` | Go LLM + `text/template` |
| `question_proposal()` | 同上 |
| `gen_metadata()` | 同上 |
| `content_tagging()` | 同上 + `json_repair` |
| `run_toc_from_text()` | 同上 + `internal/tokenizer` |
| `message_fit_in()` | Go `tiktoken-go` |
| `split_chunks()` | Go `tiktoken-go` |
| `gen_json()` / `assign_toc_levels()` | Go LLM |

**前置**：`rag/prompts/` 下 Jinja2 模板（`.md` / `.j2`）→ Go `text/template`（`{{ var }}` → `{{ .Var }}`，`{% for %}` → `{{ range }}`），机械操作。

### 2.4 非 PDF 文件解析

| 文件 | Python | 行数 | Go 替代 |
|------|--------|------|---------|
| `deepdoc/parser/txt_parser.py` | `RAGFlowTxtParser` | 67 | Go 标准库 |
| `deepdoc/parser/markdown_parser.py` | `RAGFlowMarkdownParser` | 459 | Go `regexp` |
| `deepdoc/parser/html_parser.py` | `RAGFlowHtmlParser` | 212 | `golang.org/x/net/html` |
| `deepdoc/parser/epub_parser.py` | `RAGFlowEpubParser` | 145 | Go `archive/zip` + `encoding/xml` |
| `deepdoc/parser/json_parser.py` | `RAGFlowJsonParser` | 179 | Go `encoding/json` |
| `rag/app/naive.py` 内联 DOCX 类 | `Docx` | ~200 | `gooxml` + Go `image` |
| `deepdoc/parser/docx_parser.py` | `RAGFlowDocxParser` | 185 | 同上 |
| `deepdoc/parser/excel_parser.py` | `RAGFlowExcelParser` | 322 | `excelize` |
| `rag/app/naive.py` DOC 路径 | Tika HTTP | — | Go HTTP 调 Tika |

---

## 3. 跨语言边界与绕过方案

### 3.1 边界

Python 服务输出 `sections + tables + page_images`，Go 接手全部后续。四个 parser 的 `crop()` 都依赖 `self.page_images`（pdfplumber 渲染的 PIL Image 列表），通过 base64 编码跨过边界。

### 3.2 `crop()` 统一绕过

**Python 侧**（四个 parser 返回前统一执行）：

```python
page_images_b64 = {}
if hasattr(pdf_parser, "page_images") and pdf_parser.page_images:
    for i, img in enumerate(pdf_parser.page_images):
        buf = BytesIO(); img.save(buf, format="PNG")
        page_images_b64[str(i)] = base64.b64encode(buf.getvalue()).decode()
```

**Go 侧**（`internal/service/nlp/image.go`）：

- `decodePageImage(b64) → image.Image` — base64 解码 + `image.Decode`
- `cropChunkImage(chunkText, pageImages, zoom) → image.Image` — 解析 @@坐标 → `SubImage` 裁剪 → `draw.Draw` 拼接
- `concatImages(images, gap) → *image.RGBA` — 垂直拼接
- `convertToJPEG(img) → []byte` — 对标 PIL image2id

Go 标准库全覆盖：`encoding/base64`、`image`、`image/draw`、`image/jpeg`。详见附录 A。

### 3.3 `vision_figure_parser_pdf_wrapper` 绕过

Python 返回 tables 时把 PIL Image 转 base64。Go 侧解码 → 解析位置标签获取上下文 → 调视觉 LLM → 追加描述。

### 3.4 `image2id` PIL 依赖

```go
func convertToJPEG(img image.Image) ([]byte, error) {
    var buf bytes.Buffer
    jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
    return buf.Bytes(), nil
}
```

---

## 4. Python PDF Parse Service 接口

### 4.1 请求

```
POST /api/v1/parse
Content-Type: multipart/form-data
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `file` | binary | PDF 文件字节 |
| `config` | JSON string | 解析配置（parser_id, language, name, from_page, to_page, doc_id, kb_id, tenant_id, parser_config.layout_recognizer, parser_config.table_context_size, parser_config.image_context_size） |

### 4.2 成功响应

```json
{
  "success": true,
  "sections": [["text...@@0-1\t50.0\t300.0\t200.0\t400.0##", "@@0\t..."], ...],
  "tables": [[["base64_png_or_null", ["row1","row2"]], [[0, 100.0, 200.0, 300.0, 400.0]]], ...],
  "page_images": { "0": "iVBORw0KGgo...", "1": "iVBORw0KGgo..." },
  "section_count": 128,
  "duration_ms": 3200
}
```

### 4.3 错误响应

```json
{ "success": false, "error": "ParserError", "message": "PDF 损坏" }
```

---

## 5. 迁移路径（自底向上）

```
Layer 7: 清理 Python 残留    最后
Layer 6: 外部解析器客户端
Layer 5: PDF 集成
Layer 4: LLM 富化
Layer 3: 复杂解析器 DOCX/XLSX
Layer 2: 简单解析器 TXT/HTML/...
Layer 1: NLP 原语             最先（Go 侧纯逻辑）
Layer 0: 基础设施 ✅
```

| Layer | 内容 | 新增 Go 文件 | 状态 |
|-------|------|-------------|------|
| **0** | tokenizer、MinIO、ES/Infinity、LLM、NATS | `internal/cache/nats.go` | ✅ 几乎就绪，+NATS |
| **1** | NLP 原语：merge、tokenize、image、position、token_counter | `internal/service/nlp/{merge,tokenize,image,position}.go` + `internal/common/token_counter.go` | ❌ 待建 |
| **2** | 简单解析器：TXT/MD/HTML/EPUB/JSON/DOC | `internal/service/nlp/parser_{txt,markdown,html,epub,json,doc}.go` | ❌ 待建 |
| **3** | 复杂解析器：DOCX/XLSX + 视觉 LLM | `internal/service/nlp/parser_{docx,xlsx}.go` + Go LLM wrapper | ❌ 待建 |
| **4** | LLM 富化：关键词/问题/元数据/标签/TOC | `internal/service/enrich/` | ❌ 待建 |
| **5** | PDF 集成：Python 服务 → Go NLP + 后处理 + 入库 | `api/pdf_parse_server.py` + `internal/service/parse_client.go` | ❌ 待建 |
| **6** | 外部解析器：MinerU/PaddleOCR/OpenDataLoader HTTP client → Go | — | ❌ 待建 |
| **7** | 清理：删 Python 已迁代码，下 executor 进程 | — | ❌ 最后 |

---

## 6. 部署拓扑

```
┌──────────────────────┐  HTTP (仅 PDF)  ┌──────────────────────────┐
│  Go Task Executor #1  │────────────────→│  Python PDF Parse Svc    │
│  Go Task Executor #N  │────────────────→│  (gunicorn, N workers)   │
│  Layer 0-4 内建       │                 │  每 worker:               │
│                       │                 │  - ONNX 模型实例          │
│                       │                 │  - pdfplumber 实例        │
└──────────────────────┘                 └──────────────────────────┘
  NATS ◄── 消费任务
  MinIO ◄── 文件 + 图片
  ES/Infinity ◄── chunk CRUD
  LLM API ◄── 富化 + Embedding
```

---

## 附录 A：`crop()` Go 实现细节

见正文 3.2 节及下文。

```go
// decodePageImage converts base64 PNG to image.Image
func decodePageImage(b64 string) (image.Image, error) {
    data, err := base64.StdEncoding.DecodeString(b64)
    if err != nil { return nil, err }
    img, _, err := image.Decode(bytes.NewReader(data))
    return img, err
}

// cropRegion crops a rectangle from a page image (like PIL .crop)
func cropRegion(page image.Image, left, top, right, bottom, zoom float64) image.Image {
    rect := image.Rect(
        int(left*zoom), int(top*zoom),
        int(right*zoom), int(bottom*zoom),
    )
    return page.(interface{ SubImage(image.Rectangle) image.Image }).SubImage(rect)
}

// concatImages vertically concatenates images (like PIL Image.new + paste)
func concatImages(images []image.Image, gap int) *image.RGBA {
    totalH, maxW := gap*(len(images)-1), 0
    for _, img := range images {
        totalH += img.Bounds().Dy()
        if w := img.Bounds().Dx(); w > maxW { maxW = w }
    }
    canvas := image.NewRGBA(image.Rect(0, 0, maxW, totalH))
    draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.Gray{245}}, image.Point{}, draw.Src)
    y := 0
    for _, img := range images {
        draw.Draw(canvas, image.Rect(0, y, maxW, y+img.Bounds().Dy()), img, image.Point{}, draw.Over)
        y += img.Bounds().Dy() + gap
    }
    return canvas
}

// cropChunkImage crops+concatenates page regions based on @@ position tags
func cropChunkImage(text string, pageImages map[int]image.Image, zoom float64) (*image.RGBA, []Position) {
    poss := extractPositions(text)
    // add context above/below, handle cross-page, crop each region...
    // return concatImages(regions, gap), positions
}
```

## 附录 B：`vision_figure_parser_pdf_wrapper` Go 实现

Python 返回 tables 时图片已 base64 编码。Go 侧：

```go
func enhanceTablesWithVision(tables []TableItem, sections [][2]string, visionModel LLMClient) []TableItem {
    for i, t := range tables {
        if t.ImageB64 == "" { continue }
        img, _ := decodePageImage(t.ImageB64)
        context := extractContextFromSections(sections, t.Positions, contextSize)
        prompt := buildVisionPrompt(context)
        description := visionModel.DescribeImage(img, prompt)
        tables[i].Rows = append(tables[i].Rows, []string{description})
    }
    return tables
}
```

## 附录 C：LiteParse 替换 pdfplumber（可选，Phase 2+）

[LiteParse](https://github.com/run-llama/liteparse)（Rust / Apache 2.0 / PDFium）可有朝一日替换 pdfplumber。

接入方式：

| 方案 | 描述 | 难度 |
|------|------|------|
| CLI 子进程 | Go `os/exec` 调 `lit parse - --format json` | 低 |
| Python HTTP | `pip install liteparse` → Quart 微服务 | 中 |
| Rust HTTP | 编译 axum 静态二进制 | 中 |

LiteParse 支持 `--ocr-server-url` 接入外部 OCR。RAGFlow ONNX OCR 包装为 HTTP server（~50行 Python），LiteParse 通过 PDFium 渲染 + 提取文本，通过 HTTP OCR server 做 OCR——pdfplumber 被完全替代，ONNX 保持不变。

```bash
lit parse --format json --ocr-server-url http://localhost:9385/ocr input.pdf
```
