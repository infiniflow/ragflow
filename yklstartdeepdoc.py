#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
DeepDoc 批量文档解析脚本
基于 RAGFlow 官方解析调度机制，充分发挥各解析器功能。

PDF 解析流水线（parse_into_bboxes 内部）：
  1. OCR / pdfplumber 文本提取（乱码回退 OCR）
  2. LayoutRecognizer (ONNX) 版面分析 → layout_type + layoutno
  3. TableStructureRecognizer 表格检测与结构识别 → HTML
  4. _text_merge          水平合并（同行 + 同 layoutno + 同 col_id）
  5. _concat_downward     xgboost 智能垂直拼接（核心文本拼接）
  6. _naive_vertical_merge 规则兜底合并
  7. _extract_table_figure 提取表格/图表图片 + OCR 内部文字
  8. crop                  为每个 block 截图

本脚本在 parse_into_bboxes 返回后额外做：
  - 先提取 figure/table 图片到 images/ 目录（避免重复保存）
  - 再序列化输出为 json / markdown / text / html
  - 可选：移除目录页、移除页眉页脚、多列重排、展平媒体

已修复的依赖兼容性问题（RAGFlow _concat_downward 被禁用时期的 bit rot）：
  - rag_tokenizer.tag() 返回 bool → pdf_parser.py:176-177
  - datrie.Trie 缺失 has_keys_with_prefix → rag_tokenizer.py monkey-patch
  - 空字符串取 [-1] IndexError → pdf_parser.py:1157
"""

import os
import sys
import re
import argparse
import traceback
import json
import logging
from pathlib import Path
from datetime import datetime
from typing import List, Dict, Any, Optional

# 添加 ragflow 到 Python 路径
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "")))

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
    handlers=[
        logging.StreamHandler(sys.stdout),
    ]
)
logger = logging.getLogger(__name__)

# 导入 DeepDoc 解析器
from deepdoc.parser import (
    PdfParser, PlainParser, DocxParser, ExcelParser,
    HtmlParser, MarkdownParser, TxtParser, PptParser, EpubParser
)
from deepdoc.parser.pdf_parser import RAGFlowPdfParser

# 检查 xgboost 版本兼容性
def _check_xgboost_compatibility():
    """检查 xgboost 版本是否能加载旧格式模型"""
    try:
        import xgboost as xgb
        version = xgb.__version__
        major = int(version.split('.')[0])
        minor = int(version.split('.')[1])

        model_path = os.path.join(
            os.path.dirname(__file__), "rag", "res", "deepdoc", "updown_concat_xgb.model"
        )
        if not os.path.exists(model_path):
            return True, version, None  # 模型不存在，跳过检查

        # XGBoost 3.1+ 不再支持旧二进制格式
        if major > 3 or (major == 3 and minor >= 1):
            return False, version, "XGBoost 3.1+ removed support for old binary format"

        # 3.0.x 和 2.x 应该兼容，但实际测试加载
        try:
            b = xgb.Booster()
            b.set_param({"device": "cpu"})
            b.load_model(model_path)
            return True, version, None
        except Exception as e:
            return False, version, str(e)
    except ImportError:
        return False, "not installed", "xgboost not installed"

_xgb_ok, _xgb_ver, _xgb_err = _check_xgboost_compatibility()
if not _xgb_ok:
    logger.warning("=" * 60)
    logger.warning("XGBoost 版本兼容性问题 detected!")
    logger.warning(f"当前版本: {_xgb_ver}")
    logger.warning(f"错误: {_xgb_err[:80] if _xgb_err else 'unknown'}")
    logger.warning("")
    logger.warning("解决方案:")
    logger.warning("  pip install xgboost==3.0.5")
    logger.warning("")
    logger.warning("PDF 解析仍可继续，但文本垂直拼接将使用 fallback 方法")
    logger.warning("=" * 60)
else:
    logger.info(f"XGBoost {_xgb_ver} 兼容性检查通过，模型加载正常")


class DeepDocDocumentProcessor:
    """
    DeepDoc 文档批量处理器 - 基于 RAGFlow 官方解析调度
    """

    # 支持的文件格式映射
    SUPPORTED_FORMATS = {
        ".pdf": {"parser": "pdf", "class": PdfParser, "desc": "PDF (OCR + 版面分析 + 表格识别)"},
        ".docx": {"parser": "docx", "class": DocxParser, "desc": "Word 文档"},
        ".xlsx": {"parser": "excel", "class": ExcelParser, "desc": "Excel 表格"},
        ".xls": {"parser": "excel", "class": ExcelParser, "desc": "Excel 表格"},
        ".csv": {"parser": "excel", "class": ExcelParser, "desc": "CSV 表格"},
        ".html": {"parser": "html", "class": HtmlParser, "desc": "HTML 网页"},
        ".htm": {"parser": "html", "class": HtmlParser, "desc": "HTML 网页"},
        ".md": {"parser": "markdown", "class": MarkdownParser, "desc": "Markdown"},
        ".markdown": {"parser": "markdown", "class": MarkdownParser, "desc": "Markdown"},
        ".txt": {"parser": "txt", "class": TxtParser, "desc": "纯文本"},
        ".ppt": {"parser": "ppt", "class": PptParser, "desc": "PPT 演示文稿"},
        ".pptx": {"parser": "ppt", "class": PptParser, "desc": "PPT 演示文稿"},
        ".epub": {"parser": "epub", "class": EpubParser, "desc": "EPUB 电子书"},
    }

    def __init__(self, output_dir="./deepdoc_outputs", output_format="json",
                 zoomin=3, return_html=True, from_page=0, to_page=100000,
                 parse_method="deepdoc", lang="Chinese",
                 remove_toc=False, remove_header_footer=False,
                 flatten_media=False, enable_multi_column=False,
                 save_all_images=False, extract_figures=True):
        """
        初始化处理器

        Args:
            output_dir: 输出目录
            output_format: 输出格式 (json/markdown/text/html)
            zoomin: PDF 缩放倍数
            return_html: 是否返回 HTML 表格
            from_page: PDF 起始页码
            to_page: PDF 结束页码
            parse_method: PDF 解析方法 (deepdoc/plain_text)
            lang: OCR 语言
            remove_toc: 是否移除目录页
            remove_header_footer: 是否移除页眉页脚
            flatten_media: 是否将图片/表格展平为文本
            enable_multi_column: 是否启用多列排版重排
            save_all_images: 是否保存所有文本块的截图（默认 False，只保存 figure/table 图片）
            extract_figures: 是否将 PDF 中的图片/图表单独提取到 images 目录（默认 True）
        """
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.output_format = output_format
        self.zoomin = zoomin
        self.return_html = return_html
        self.from_page = from_page
        self.to_page = to_page
        self.parse_method = parse_method
        self.lang = lang
        self.remove_toc = remove_toc
        self.remove_header_footer = remove_header_footer
        self.flatten_media = flatten_media
        self.enable_multi_column = enable_multi_column
        self.save_all_images = save_all_images
        self.extract_figures = extract_figures

        # 初始化解析器缓存
        self._parsers = {}
        self._pdf_parser = None

    def _get_parser(self, file_ext):
        """获取或创建对应格式的解析器"""
        parser_info = self.SUPPORTED_FORMATS.get(file_ext.lower())
        if not parser_info:
            return None

        if file_ext not in self._parsers:
            logger.info(f"初始化 {file_ext} 解析器 ({parser_info['desc']})...")
            self._parsers[file_ext] = parser_info["class"]()

        return self._parsers[file_ext]

    def _progress_callback(self, progress, msg=""):
        """PDF 解析进度回调"""
        if msg:
            logger.info(f"  [{progress*100:5.1f}%] {msg}")
        else:
            logger.info(f"  [{progress*100:5.1f}%]")

    def _save_json_output(self, data: List[Dict], output_path: Path, file_stem: str):
        """保存 JSON 格式输出，处理 PIL Image 对象和 numpy 类型"""
        import numpy as np

        json_path = output_path / f"{file_stem}.json"

        def convert_value(value):
            """递归转换不可序列化的类型"""
            if isinstance(value, np.ndarray):
                return value.tolist()
            elif isinstance(value, (np.integer, np.floating)):
                return float(value) if isinstance(value, np.floating) else int(value)
            elif isinstance(value, dict):
                return {k: convert_value(v) for k, v in value.items()}
            elif isinstance(value, list):
                return [convert_value(v) for v in value]
            elif hasattr(value, 'save'):  # PIL Image
                return None
            else:
                return value

        # 需要保存图片的 layout_type（仅在 --save_all_images 时保存文本块截图）
        IMAGE_LAYOUT_TYPES = {"figure", "table", "figure caption", "table caption"}
        text_images_saved = 0
        text_images_skipped = 0

        # 序列化前处理
        serializable_data = []
        for idx, item in enumerate(data):
            item_copy = {}
            for key, value in item.items():
                if key == "image":
                    # 情况1: _extract_figures_to_dir 已将 PIL Image 替换为字符串路径
                    if isinstance(value, str):
                        item_copy["image_path"] = value
                    # 情况2: 仍是 PIL Image 对象（仅 --save_all_images 时保存文本块截图）
                    elif value is not None and hasattr(value, 'save'):
                        layout_type = item.get("layout_type", "text")
                        if self.save_all_images and layout_type not in IMAGE_LAYOUT_TYPES:
                            img_path = output_path / f"{file_stem}_image_{idx}.png"
                            try:
                                value.save(img_path)
                                item_copy["image_path"] = str(img_path.name)
                                text_images_saved += 1
                            except Exception as e:
                                logger.warning(f"保存图片失败: {e}")
                        else:
                            item_copy["image_path"] = None
                            if hasattr(value, 'save'):
                                text_images_skipped += 1
                    else:
                        item_copy["image_path"] = None
                else:
                    item_copy[key] = convert_value(value)

            serializable_data.append(item_copy)

        if text_images_skipped > 0:
            logger.info(f"  - 跳过 {text_images_skipped} 个文本块截图（使用 --save_all_images 保存全部）")

        with open(json_path, "w", encoding="utf-8") as f:
            json.dump(serializable_data, f, ensure_ascii=False, indent=2)
        return json_path

    def _save_markdown_output(self, data: List[Dict], output_path: Path, file_stem: str):
        """保存 Markdown 格式输出"""
        md_path = output_path / f"{file_stem}.md"
        lines = []
        for item in data:
            layout_type = item.get("layout_type", "text")
            text = item.get("text", "")
            if not text:
                continue

            if layout_type == "title":
                lines.append(f"\n# {text}\n")
            elif layout_type == "table":
                lines.append(f"\n## 表格\n{text}\n")
            elif layout_type in ("figure", "image"):
                lines.append(f"\n## 图片\n{text}\n")
            else:
                lines.append(text)

        with open(md_path, "w", encoding="utf-8") as f:
            f.write("\n".join(lines))
        return md_path

    def _save_text_output(self, data: List[Dict], output_path: Path, file_stem: str):
        """保存纯文本输出"""
        txt_path = output_path / f"{file_stem}.txt"
        lines = []
        for item in data:
            text = item.get("text", "")
            if text:
                lines.append(text)

        with open(txt_path, "w", encoding="utf-8") as f:
            f.write("\n\n".join(lines))
        return txt_path

    def _save_html_output(self, data: List[Dict], output_path: Path, file_stem: str):
        """保存 HTML 格式输出"""
        html_path = output_path / f"{file_stem}.html"
        html_lines = [
            "<!DOCTYPE html>",
            "<html><head><meta charset='utf-8'><title>解析结果</title></head><body>",
        ]
        for item in data:
            layout_type = item.get("layout_type", "text")
            text = item.get("text", "")
            if not text:
                continue

            if layout_type == "title":
                html_lines.append(f"<h1>{text}</h1>")
            elif layout_type == "table":
                html_lines.append(f"<div class='table'>{text}</div>")
            elif layout_type in ("figure", "image"):
                html_lines.append(f"<div class='figure'>{text}</div>")
            else:
                html_lines.append(f"<p>{text}</p>")

        html_lines.append("</body></html>")

        with open(html_path, "w", encoding="utf-8") as f:
            f.write("\n".join(html_lines))
        return html_path

    def _save_output(self, data: List[Dict], output_path: Path, file_stem: str):
        """根据配置格式保存输出"""
        saved = []
        if self.output_format in ("json", "all"):
            saved.append(self._save_json_output(data, output_path, file_stem))
        if self.output_format in ("markdown", "all"):
            saved.append(self._save_markdown_output(data, output_path, file_stem))
        if self.output_format in ("text", "all"):
            saved.append(self._save_text_output(data, output_path, file_stem))
        if self.output_format in ("html", "all"):
            saved.append(self._save_html_output(data, output_path, file_stem))
        return saved

    def _extract_figures_to_dir(self, data: List[Dict], output_dir: Path, file_stem: str):
        """
        将解析结果中的图表（figure/table）单独提取到 images 子目录。
        这是用户通常最关心的内容——原始文档中的彩色图片、图表等。

        Args:
            data: bboxes 列表
            output_dir: 输出根目录
            file_stem: 文件名（不含扩展名）

        Returns:
            saved image paths list, statistics dict
        """
        images_dir = output_dir / "images"
        images_dir.mkdir(parents=True, exist_ok=True)

        saved = []
        stats = {"figure": 0, "table": 0, "figure_caption": 0, "table_caption": 0, "other": 0}

        for idx, item in enumerate(data):
            layout_type = item.get("layout_type", "text")
            image = item.get("image")

            if image is None or not hasattr(image, 'save'):
                continue

            # 只提取有意义的图片类型
            if layout_type in ("figure", "table", "figure caption", "table caption"):
                # 命名规则：类型_页码_序号.png
                page = item.get("page_number", 0)
                img_name = f"{file_stem}_{layout_type.replace(' ', '_')}_p{page}_{idx:03d}.png"
                img_path = images_dir / img_name
                try:
                    image.save(img_path)
                    saved.append(str(img_path))
                    # 将 PIL Image 替换为相对路径字符串，后续 JSON 输出直接引用
                    item["image"] = f"images/{img_name}"
                    if layout_type in stats:
                        stats[layout_type] += 1
                    else:
                        stats["other"] += 1
                    logger.debug(f"  提取图片: {img_name}")
                except Exception as e:
                    logger.warning(f"  提取图片失败 [{layout_type}] #{idx}: {e}")
            else:
                # 非图表类型（text/title 等），丢弃 PIL Image 避免后续重复保存
                item["image"] = None

        if saved:
            logger.info(f"\n图片提取统计:")
            logger.info(f"  - 图表 (figure): {stats['figure']} 个")
            logger.info(f"  - 表格 (table):  {stats['table']} 个")
            logger.info(f"  - 图标题:        {stats['figure_caption']} 个")
            logger.info(f"  - 表标题:        {stats['table_caption']} 个")
            logger.info(f"  - 总计:          {len(saved)} 个")
            logger.info(f"  - 保存位置:      {images_dir}")
        else:
            logger.info(f"\n未检测到可提取的图表（layout_type 中无 figure/table 类型）")
            logger.info(f"提示: 版面分析模型可能未检测到图片。尝试降低检测阈值或检查 PDF 质量。")

        return saved, stats

    def _merge_by_layoutno(self, bboxes: List[Dict]) -> List[Dict]:
        """
        将相同 layoutno 的文本块合并为完整段落。

        RAGFlowPdfParser 的 _concat_downward (xgboost) 已重新启用，
        本方法作为可选的兜底策略：按 layoutno 强制合并相邻的 text/title 块。
        默认不调用，如需启用，在 parse_pdf_deepdoc 中取消注释即可。

        Args:
            bboxes: 原始 bboxes 列表

        Returns:
            合并后的 bboxes 列表
        """
        if not bboxes:
            return bboxes

        merged = []
        # 只合并这些类型的块
        MERGEABLE_TYPES = {"text", "title"}

        for b in bboxes:
            lt = b.get("layout_type", "text")
            lno = b.get("layoutno", "")

            # 不可合并的类型直接追加
            if lt not in MERGEABLE_TYPES or not lno:
                merged.append(b)
                continue

            # 尝试与上一个块合并：同 page、同 layoutno、同 col_id、同类型
            if merged:
                prev = merged[-1]
                prev_lt = prev.get("layout_type", "text")
                prev_lno = prev.get("layoutno", "")

                if (prev_lt == lt
                        and prev_lno == lno
                        and prev.get("page_number") == b.get("page_number")
                        and prev.get("col_id") == b.get("col_id")):
                    # 合并文本
                    prev_text = prev.get("text", "").strip()
                    curr_text = b.get("text", "").strip()
                    if prev_text and curr_text:
                        # 判断是否需要加空格（中英文混排）
                        if (re.match(r"[a-zA-Z0-9]$", prev_text[-1])
                                and re.match(r"^[a-zA-Z0-9]", curr_text[0])):
                            prev["text"] = prev_text + " " + curr_text
                        else:
                            prev["text"] = prev_text + curr_text
                    else:
                        prev["text"] = prev_text + curr_text
                    # 扩展边界
                    prev["x0"] = min(prev["x0"], b["x0"])
                    prev["x1"] = max(prev["x1"], b["x1"])
                    prev["bottom"] = b["bottom"]
                    continue

            merged.append(b)

        merged_count = len(bboxes) - len(merged)
        if merged_count > 0:
            logger.info(f"  - layoutno 合并: {merged_count} 个文本块 → 段落已整合")
        return merged

    def parse_pdf_deepdoc(self, pdf_path, output_dir=None):
        """
        使用 RAGFlowPdfParser.parse_into_bboxes() 解析 PDF
        这是 RAGFlow 官方推荐的解析方式，支持进度回调和分页
        """
        pdf_path = Path(pdf_path)
        file_stem = pdf_path.stem

        if output_dir is None:
            output_dir = self.output_dir / file_stem
        else:
            output_dir = Path(output_dir) / file_stem
        output_dir.mkdir(parents=True, exist_ok=True)

        logger.info(f"\n{'='*60}")
        logger.info(f"解析 PDF (DeepDoc): {pdf_path.name}")
        logger.info(f"{'='*60}")
        logger.info(f"输出目录: {output_dir}")
        logger.info(f"解析方法: {self.parse_method}")
        logger.info(f"页码范围: {self.from_page} - {self.to_page}")
        logger.info(f"缩放倍数: {self.zoomin}")

        try:
            # 使用 RAGFlowPdfParser 的 parse_into_bboxes 方法
            # 这是 rag/flow/parser/parser.py 中 _pdf() 使用的官方方法
            parser = RAGFlowPdfParser()

            with open(pdf_path, "rb") as f:
                binary = f.read()

            logger.info("开始解析...")

            if self.parse_method == "deepdoc":
                # 使用完整的 DeepDoc 解析流程（带回调）
                bboxes = parser.parse_into_bboxes(
                    binary,
                    callback=self._progress_callback,
                    zoomin=self.zoomin,
                    from_page=self.from_page,
                    to_page=self.to_page,
                )

                # 处理多列排版
                if self.enable_multi_column:
                    from rag.flow.parser.pdf_chunk_metadata import reorder_multi_column_bboxes
                    bboxes = reorder_multi_column_bboxes(parser, bboxes)

                # 移除目录页
                if self.remove_toc:
                    from rag.flow.parser.utils import remove_toc_pdf
                    if parser.outlines and parser.outlines[0][2] == 1:
                        bboxes = remove_toc_pdf(bboxes, parser.outlines)

                # 移除页眉页脚
                if self.remove_header_footer:
                    bboxes = [b for b in bboxes if not b.get("layout_type", "").lower() in ("header", "footer", "number")]

                # 展平媒体为文本
                if self.flatten_media:
                    for b in bboxes:
                        b["doc_type_kwd"] = "text"

                # （可选）按 layoutno 合并同一段落的文本块
                # xgboost _concat_downward 已重新启用，此兜底策略默认关闭
                # 如需启用，取消下一行注释：
                # bboxes = self._merge_by_layoutno(bboxes)

                logger.info(f"\n解析完成！")
                logger.info(f"  - 文本块: {len(bboxes)} 个")

                # 统计各类元素
                layout_counts = {}
                for b in bboxes:
                    lt = b.get("layout_type", "text")
                    layout_counts[lt] = layout_counts.get(lt, 0) + 1
                for lt, count in sorted(layout_counts.items()):
                    logger.info(f"  - {lt}: {count} 个")

                # 先提取图表图片到 images/ 目录，并将 bbox 中的 PIL Image
                # 替换为相对路径字符串，后续 JSON 输出直接引用，避免重复保存
                if self.extract_figures:
                    fig_paths, fig_stats = self._extract_figures_to_dir(bboxes, output_dir, file_stem)
                    layout_counts["_extracted_images"] = len(fig_paths)
                    layout_counts["_figure_stats"] = fig_stats
                else:
                    fig_paths = []

                # 保存输出（image_path 直接引用 images/ 中的文件，不再重复保存）
                saved = self._save_output(bboxes, output_dir, file_stem)
                saved.extend(fig_paths)

                # 保存大纲信息
                if parser.outlines:
                    outline_path = output_dir / "outlines.json"
                    with open(outline_path, "w", encoding="utf-8") as f:
                        json.dump(parser.outlines, f, ensure_ascii=False, indent=2)
                    saved.append(outline_path)

                # 保存元数据
                meta = {
                    "file": str(pdf_path),
                    "pages": parser.total_page if hasattr(parser, 'total_page') else "unknown",
                    "blocks": len(bboxes),
                    "layout_types": layout_counts,
                    "outlines": len(parser.outlines) if parser.outlines else 0,
                    "parse_method": "deepdoc",
                    "zoomin": self.zoomin,
                }
                meta_path = output_dir / "metadata.json"
                with open(meta_path, "w", encoding="utf-8") as f:
                    json.dump(meta, f, ensure_ascii=False, indent=2)
                saved.append(meta_path)

                logger.info(f"\n输出文件:")
                for s in saved:
                    logger.info(f"  - {s}")

                return {
                    "file": str(pdf_path),
                    "blocks": bboxes,
                    "output_dir": str(output_dir),
                    "saved_files": [str(s) for s in saved],
                }

            else:
                # 使用纯文本解析（PlainParser）
                plain_parser = PlainParser()
                lines, _ = plain_parser(binary, from_page=self.from_page, to_page=self.to_page)

                bboxes = [{"text": t, "layout_type": "text"} for t, _ in lines]

                logger.info(f"\n解析完成！")
                logger.info(f"  - 文本行: {len(bboxes)} 个")

                saved = self._save_output(bboxes, output_dir, file_stem)

                meta = {
                    "file": str(pdf_path),
                    "lines": len(bboxes),
                    "parse_method": "plain_text",
                }
                meta_path = output_dir / "metadata.json"
                with open(meta_path, "w", encoding="utf-8") as f:
                    json.dump(meta, f, ensure_ascii=False, indent=2)
                saved.append(meta_path)

                return {
                    "file": str(pdf_path),
                    "blocks": bboxes,
                    "output_dir": str(output_dir),
                    "saved_files": [str(s) for s in saved],
                }

        except Exception as e:
            logger.error(f"解析 {pdf_path.name} 时出错: {e}")
            traceback.print_exc()
            return None

    def parse_pdf_legacy(self, pdf_path, output_dir=None):
        """
        使用传统的 PdfParser.__call__() 解析 PDF
        保留向后兼容性，支持图像提取
        """
        pdf_path = Path(pdf_path)
        file_stem = pdf_path.stem

        if output_dir is None:
            output_dir = self.output_dir / file_stem
        else:
            output_dir = Path(output_dir) / file_stem
        output_dir.mkdir(parents=True, exist_ok=True)

        # 创建子目录结构
        text_dir = output_dir / "texts"
        table_dir = output_dir / "tables"
        image_dir = output_dir / "images"
        text_dir.mkdir(exist_ok=True)
        table_dir.mkdir(exist_ok=True)
        image_dir.mkdir(exist_ok=True)

        logger.info(f"\n{'='*60}")
        logger.info(f"解析 PDF (Legacy): {pdf_path.name}")
        logger.info(f"{'='*60}")

        try:
            parser = PdfParser()

            # 使用传统调用方式
            texts, tables = parser(
                str(pdf_path),
                need_image=True,
                zoomin=self.zoomin,
                return_html=self.return_html,
            )

            logger.info(f"\n解析完成！")
            logger.info(f"  - 文本段落: {len(texts)} 个")
            logger.info(f"  - 表格: {len(tables)} 个")

            # 保存文本结果
            text_output = text_dir / "texts.json"
            with open(text_output, "w", encoding="utf-8") as f:
                json.dump(texts, f, ensure_ascii=False, indent=2)

            # 保存表格结果
            if tables:
                tables_serializable = []
                for table in tables:
                    if isinstance(table, tuple):
                        img, table_dict = table
                        table = table_dict
                        if img and hasattr(img, 'save'):
                            img_path = image_dir / f"table_{len(tables_serializable)}.png"
                            img.save(img_path)
                            if isinstance(table, dict):
                                table["image_path"] = str(img_path.name)
                    elif isinstance(table, list):
                        table = {"data": table}
                    elif not isinstance(table, dict):
                        table = {"value": str(table)}

                    table_copy = {}
                    for key, value in table.items():
                        if key == "image":
                            img_path = image_dir / f"table_{len(tables_serializable)}.png"
                            if hasattr(value, 'save'):
                                value.save(img_path)
                            table_copy["image_path"] = str(img_path.name)
                        elif isinstance(value, (str, int, float, bool, list, dict, type(None))):
                            table_copy[key] = value
                        else:
                            table_copy[key] = str(value)
                    tables_serializable.append(table_copy)

                table_output = table_dir / "tables.json"
                with open(table_output, "w", encoding="utf-8") as f:
                    json.dump(tables_serializable, f, ensure_ascii=False, indent=2)

                for i, table in enumerate(tables_serializable):
                    if "html" in table:
                        html_file = table_dir / f"table_{i}.html"
                        with open(html_file, "w", encoding="utf-8") as f:
                            f.write(table["html"])

            # 保存纯文本
            text_plain = text_dir / "content.txt"
            with open(text_plain, "w", encoding="utf-8") as f:
                for text in texts:
                    if isinstance(text, dict):
                        f.write(f"{text.get('text', '')}\n\n")
                    elif isinstance(text, str):
                        f.write(f"{text}\n\n")
                    else:
                        f.write(f"{str(text)}\n\n")

            # 保存结构化文本
            text_structured = text_dir / "content_structured.txt"
            with open(text_structured, "w", encoding="utf-8") as f:
                for text in texts:
                    if isinstance(text, dict):
                        layout_type = text.get("layout_type", "text")
                        page = text.get("page_number", 0)
                        f.write(f"[{layout_type}] (Page {page})\n")
                        f.write(f"{text.get('text', '')}\n")
                    elif isinstance(text, str):
                        f.write(f"[text] (Page 0)\n")
                        f.write(f"{text}\n")
                    else:
                        f.write(f"[text] (Page 0)\n")
                        f.write(f"{str(text)}\n")
                    f.write("-" * 50 + "\n\n")

            # 生成 README
            readme_file = output_dir / "README.txt"
            with open(readme_file, "w", encoding="utf-8") as f:
                f.write(f"PDF 解析结果: {pdf_path.name}\n")
                f.write(f"{'='*50}\n\n")
                f.write(f"统计:\n")
                f.write(f"  - 文本段落: {len(texts)} 个\n")
                f.write(f"  - 表格: {len(tables)} 个\n")

            logger.info(f"\n输出文件:")
            logger.info(f"  [文本] {text_output}")
            logger.info(f"  [文本] {text_plain}")
            logger.info(f"  [文本] {text_structured}")
            if tables:
                logger.info(f"  [表格] {table_output}")

            return {
                "file": str(pdf_path),
                "texts": texts,
                "tables": tables,
                "output_dir": str(output_dir),
            }

        except Exception as e:
            logger.error(f"解析 {pdf_path.name} 时出错: {e}")
            traceback.print_exc()
            return None

    def parse_document(self, file_path, output_dir=None):
        """
        解析通用文档（非 PDF）
        使用 RAGFlow 官方解析器的正确调用方式
        """
        file_path = Path(file_path)
        file_ext = file_path.suffix
        file_stem = file_path.stem

        if output_dir is None:
            output_dir = self.output_dir / file_stem
        output_dir = Path(output_dir)
        output_dir.mkdir(parents=True, exist_ok=True)

        logger.info(f"\n{'='*60}")
        logger.info(f"解析文档: {file_path.name}")
        logger.info(f"{'='*60}")

        parser = self._get_parser(file_ext)
        if not parser:
            logger.error(f"不支持的文件格式: {file_ext}")
            return None

        try:
            # 读取文件内容
            with open(file_path, "rb") as f:
                binary = f.read()

            logger.info(f"使用 {file_ext} 解析器...")

            # 根据不同格式调用正确的解析方法
            parser_type = self.SUPPORTED_FORMATS[file_ext.lower()]["parser"]

            if parser_type == "docx":
                # DocxParser: __call__(fnm) -> (secs, tbls)
                # secs = [(paragraph_text, style_name), ...]
                # tbls = [table_html, ...]
                secs, tbls = parser(binary)
                results = []
                for text, style in secs:
                    if text.strip():
                        results.append({"text": text, "layout_type": "text"})
                for table_html in tbls:
                    if table_html:
                        results.append({"text": table_html, "layout_type": "table"})

            elif parser_type == "excel":
                # ExcelParser: 支持多种输出格式
                if self.output_format == "html":
                    htmls = parser.html(binary, 1000000000)
                    results = [{"text": htmls[0], "layout_type": "table"}]
                elif self.output_format == "markdown":
                    md = parser.markdown(binary)
                    results = [{"text": md, "layout_type": "text"}]
                else:
                    texts = parser(binary)
                    results = [{"text": t, "layout_type": "text"} for t in texts if t]

            elif parser_type == "ppt":
                # PptParser: ppt_parser(blob, from_page, to_page, callback)
                texts = parser(binary, 0, 100000, None)
                results = [{"text": t, "layout_type": "text"} for t in texts if t.strip()]

            elif parser_type == "html":
                # HtmlParser: __call__(fnm, chunk_token_num)
                sections = parser(binary, 512)
                results = [{"text": s, "layout_type": "text"} for s in sections if s]

            elif parser_type == "markdown":
                # MarkdownParser: markdown_parser(name, blob, separate_tables=True)
                sections, tables, section_images = parser(
                    binary,
                    separate_tables=True,
                    return_section_images=True,
                )
                results = []
                for idx, (section_text, _) in enumerate(sections):
                    item = {"text": section_text, "layout_type": "text"}
                    if section_images and len(section_images) > idx and section_images[idx] is not None:
                        item["layout_type"] = "figure"
                    results.append(item)
                for table in tables:
                    if table and table[0] and table[0][1]:
                        results.append({"text": table[0][1], "layout_type": "table"})

            elif parser_type == "epub":
                # EpubParser: __call__(fnm)
                sections = parser(binary)
                results = [{"text": s, "layout_type": "text"} for s in sections if s]

            elif parser_type == "txt":
                # TxtParser: __call__(fnm, chunk_token_num, delimiter)
                sections = parser(binary, 128, "\n!?;。；！？")
                results = [{"text": s[0], "layout_type": "text"} for s in sections if s[0]]

            else:
                # 通用处理
                texts = parser(binary)
                if isinstance(texts, list):
                    results = [{"text": str(t), "layout_type": "text"} for t in texts if t]
                else:
                    results = [{"text": str(texts), "layout_type": "text"}]

            logger.info(f"\n解析完成！")
            logger.info(f"  - 文本块: {len(results)} 个")

            # 保存输出
            saved = self._save_output(results, output_dir, file_stem)

            logger.info(f"\n输出文件:")
            for s in saved:
                logger.info(f"  - {s}")

            return {
                "file": str(file_path),
                "blocks": results,
                "output_dir": str(output_dir),
                "saved_files": [str(s) for s in saved],
            }

        except Exception as e:
            logger.error(f"解析 {file_path.name} 时出错: {e}")
            traceback.print_exc()
            return None

    def process_batch(self, input_path, output_dir=None):
        """
        批量处理文档

        Args:
            input_path: 输入路径（文件或文件夹）
            output_dir: 输出目录
        """
        input_path = Path(input_path)

        # 收集所有文件
        all_files = []
        if input_path.is_dir():
            for ext in self.SUPPORTED_FORMATS.keys():
                all_files.extend(input_path.glob(f"*{ext}"))
                all_files.extend(input_path.glob(f"*{ext.upper()}"))
            all_files = sorted(set(all_files))
        else:
            if input_path.suffix.lower() in self.SUPPORTED_FORMATS:
                all_files = [input_path]
            else:
                logger.error(f"不支持的文件格式: {input_path.suffix}")
                return []

        if not all_files:
            logger.warning(f"在 {input_path} 中没有找到支持的文件")
            logger.info(f"支持的格式: {', '.join(self.SUPPORTED_FORMATS.keys())}")
            return []

        logger.info(f"\n{'#'*60}")
        logger.info(f"DeepDoc 批量文档解析")
        logger.info(f"{'#'*60}")
        logger.info(f"输出格式: {self.output_format}")
        logger.info(f"PDF 解析方法: {self.parse_method}")
        logger.info(f"找到 {len(all_files)} 个文件:")
        for f in all_files:
            parser_info = self.SUPPORTED_FORMATS.get(f.suffix.lower(), {})
            logger.info(f"  - {f.name} ({parser_info.get('desc', 'unknown')})")
        logger.info("")

        # 批量处理
        results = []
        for i, file_path in enumerate(all_files, 1):
            logger.info(f"\n处理 [{i}/{len(all_files)}]: {file_path.name}")

            try:
                file_ext = file_path.suffix.lower()

                if file_ext == ".pdf":
                    if self.parse_method == "legacy":
                        # 使用传统方式（支持图像提取）
                        result = self.parse_pdf_legacy(file_path, output_dir=output_dir)
                    else:
                        # 使用官方推荐的 parse_into_bboxes
                        result = self.parse_pdf_deepdoc(file_path, output_dir=output_dir)
                else:
                    result = self.parse_document(file_path, output_dir=output_dir)

                if result:
                    results.append(result)

            except Exception as e:
                logger.error(f"处理 {file_path.name} 时出错: {e}")
                traceback.print_exc()
                continue

        # 生成汇总报告
        self._generate_report(results)

        logger.info(f"\n{'#'*60}")
        logger.info(f"批量处理完成！")
        logger.info(f"成功处理: {len(results)}/{len(all_files)} 个文件")
        logger.info(f"{'#'*60}")

        return results

    def _generate_report(self, results):
        """生成处理报告"""
        report_file = self.output_dir / f"report_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"

        summary = []
        for result in results:
            if result:
                summary.append({
                    "file": result["file"],
                    "output_dir": result["output_dir"],
                    "block_count": len(result.get("blocks", [])),
                    "saved_files": result.get("saved_files", []),
                })

        with open(report_file, "w", encoding="utf-8") as f:
            json.dump(summary, f, ensure_ascii=False, indent=2)

        logger.info(f"\n处理报告已保存: {report_file}")


def main():
    parser = argparse.ArgumentParser(
        description="DeepDoc 批量文档解析工具 - 基于 RAGFlow 官方解析调度",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  # 解析单个 PDF（自动提取图表到 images/ 目录）
  python yklstartdeepdoc.py --inputs=document.pdf

  # 批量解析文件夹
  python yklstartdeepdoc.py --inputs=./my_documents/

  # 不单独提取图表图片
  python yklstartdeepdoc.py --inputs=./pdfs/ --no_extract_figures

  # 保存所有文本块截图（默认只保存 figure/table 图片）
  python yklstartdeepdoc.py --inputs=./docs/ --save_all_images

  # 使用纯文本模式解析 PDF（更快，无 OCR）
  python yklstartdeepdoc.py --inputs=./pdfs/ --parse_method=plain_text

  # 输出 Markdown 格式
  python yklstartdeepdoc.py --inputs=./docs/ --output_format=markdown

  # 启用多列排版重排
  python yklstartdeepdoc.py --inputs=./papers/ --enable_multi_column

输出目录结构:
  deepdoc_outputs/<文件名>/
  ├── <文件名>.json          # 结构化解析结果
  ├── <文件名>.md            # Markdown 格式
  ├── <文件名>.txt           # 纯文本
  ├── metadata.json           # 解析统计
  ├── outlines.json           # PDF 大纲/书签
  └── images/                 # 提取的图表图片（单独的文件夹）
      ├── xxx_figure_p1_000.png
      ├── xxx_table_p3_015.png
      └── ...

支持的格式:
  PDF (.pdf), Word (.docx), Excel (.xlsx, .xls, .csv),
  HTML (.html, .htm), Markdown (.md, .markdown),
  TXT (.txt), PPT (.ppt, .pptx), EPUB (.epub)
        """
    )

    parser.add_argument("--inputs", required=True,
                       help="输入文件或文件夹路径")
    parser.add_argument("--output_dir", default="./deepdoc_outputs",
                       help="输出目录 (默认: ./deepdoc_outputs)")
    parser.add_argument("--output_format", default="json",
                       choices=["json", "markdown", "text", "html", "all"],
                       help="输出格式 (默认: json)")
    parser.add_argument("--parse_method", default="deepdoc",
                       choices=["deepdoc", "plain_text", "legacy"],
                       help="PDF 解析方法: deepdoc=完整OCR+版面分析, plain_text=纯文本提取, legacy=传统模式(支持图像提取) (默认: deepdoc)")
    parser.add_argument("--zoomin", type=int, default=3,
                       help="PDF 缩放倍数，影响图像分辨率 (默认: 3)")
    parser.add_argument("--return_html", action="store_true", default=True,
                       help="返回 HTML 格式的表格 (默认: True)")
    parser.add_argument("--from_page", type=int, default=0,
                       help="PDF 起始页码 (默认: 0)")
    parser.add_argument("--to_page", type=int, default=100000,
                       help="PDF 结束页码 (默认: 100000)")
    parser.add_argument("--lang", default="Chinese",
                       help="OCR 语言 (默认: Chinese)")
    parser.add_argument("--remove_toc", action="store_true",
                       help="移除目录页内容")
    parser.add_argument("--remove_header_footer", action="store_true",
                       help="移除页眉页脚")
    parser.add_argument("--flatten_media", action="store_true",
                       help="将图片/表格展平为文本")
    parser.add_argument("--enable_multi_column", action="store_true",
                       help="启用多列排版重排")
    parser.add_argument("--save_all_images", action="store_true",
                       help="保存所有文本块的截图（默认只保存 figure/table 类型的图片）")
    parser.add_argument("--no_extract_figures", action="store_true",
                       help="不单独提取图表图片到 images 目录")
    parser.add_argument("--verbose", "-v", action="store_true",
                       help="显示详细日志")

    args = parser.parse_args()

    # 设置日志级别
    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    # 初始化处理器
    processor = DeepDocDocumentProcessor(
        output_dir=args.output_dir,
        output_format=args.output_format,
        zoomin=args.zoomin,
        return_html=args.return_html,
        from_page=args.from_page,
        to_page=args.to_page,
        parse_method=args.parse_method,
        lang=args.lang,
        remove_toc=args.remove_toc,
        remove_header_footer=args.remove_header_footer,
        flatten_media=args.flatten_media,
        enable_multi_column=args.enable_multi_column,
        save_all_images=args.save_all_images,
        extract_figures=not args.no_extract_figures,
    )

    # 执行批量处理
    results = processor.process_batch(
        input_path=args.inputs,
        output_dir=args.output_dir,
    )

    return results


if __name__ == "__main__":
    main()
