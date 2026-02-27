#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""
简历解析模块 (对标 SmartResume Pipeline 架构优化)

核心优化点 (参考 arXiv:2510.09722):
    1. PDF 文本融合: metadata + OCR 双路径提取并融合
    2. 版面感知重建: YOLOv10 版面分割 + 层级排序 + 行号索引
    3. 并行任务分解: 基本信息/工作经历/教育背景 三路并行 LLM 提取
    4. 索引指针机制: LLM 返回行号范围而非生成全文，减少幻觉
    5. 四阶段后处理: 源文本回提取、领域规范化、上下文去重、源文本验证

兼容性:
    - chunk(filename, binary, callback, **kwargs) 签名保持不变
    - 与 task_executor.py 中 FACTORY[ParserType.RESUME.value] 兼容
"""

import json
import re
import random
import unicodedata
import concurrent.futures
from io import BytesIO
from typing import Optional

import logging as logger
from rag.nlp import rag_tokenizer
from common.string_utils import remove_redundant_spaces
from deepdoc.parser.utils import get_text

# json_repair 用于修复 LLM 返回的不规范 JSON（参考 SmartResume 的容错策略）
try:
    import json_repair
except ImportError:
    json_repair = None

# YOLOv10 布局检测器（延迟初始化，避免未使用时加载模型）
_layout_recognizer = None


def _get_layout_recognizer():
    """
    获取 YOLOv10 布局检测器单例（延迟加载）

    使用项目已有的 deepdoc LayoutRecognizer，基于 layout.onnx 模型。

    返回:
        LayoutRecognizer 实例，加载失败时返回 None
    """
    global _layout_recognizer
    if _layout_recognizer is None:
        try:
            from deepdoc.vision import LayoutRecognizer
            _layout_recognizer = LayoutRecognizer("layout")
            logger.info("YOLOv10 布局检测器加载成功")
        except Exception as e:
            logger.warning(f"YOLOv10 布局检测器加载失败，将降级为启发式排序: {e}")
            _layout_recognizer = False  # 标记为加载失败，避免重复尝试
    return _layout_recognizer if _layout_recognizer is not False else None

# ==================== 常量定义 ====================

# 简历字段中禁止作为选择字段的列表
FORBIDDEN_SELECT_FIELDS = [
    "name_pinyin_kwd", "edu_first_fea_kwd", "degree_kwd",
    "sch_rank_kwd", "edu_fea_kwd"
]

# 字段名到中文描述的映射表（用于 chunk 构建）
FIELD_MAP = {
    "name_kwd": "姓名/名字",
    "name_pinyin_kwd": "姓名拼音/名字拼音",
    "gender_kwd": "性别（男，女）",
    "age_int": "年龄/岁/年纪",
    "phone_kwd": "电话/手机/微信",
    "email_tks": "email/e-mail/邮箱",
    "position_name_tks": "职位/职能/岗位/职责",
    "expect_city_names_tks": "期望城市",
    "work_exp_flt": "工作年限/工作年份/N年经验/毕业了多少年",
    "corporation_name_tks": "最近就职(上班)的公司/上一家公司",
    "first_school_name_tks": "第一学历毕业学校",
    "first_degree_kwd": "第一学历",
    "highest_degree_kwd": "最高学历",
    "first_major_tks": "第一学历专业",
    "edu_first_fea_kwd": "第一学历标签",
    "degree_kwd": "过往学历",
    "major_tks": "学过的专业/过往专业",
    "school_name_tks": "学校/毕业院校",
    "sch_rank_kwd": "学校标签",
    "edu_fea_kwd": "教育标签",
    "corp_nm_tks": "就职过的公司/之前的公司/上过班的公司",
    "edu_end_int": "毕业年份",
    "industry_name_tks": "所在行业",
    "birth_dt": "生日/出生年份",
    "expect_position_name_tks": "期望职位/期望职能/期望岗位",
    "skill_tks": "技能/技术栈/编程语言/框架/工具",
    "language_tks": "语言能力/外语水平",
    "certificate_tks": "证书/资质/认证",
    "project_tks": "项目经验/项目名称",
    "work_desc_tks": "工作职责/工作描述",
    "project_desc_tks": "项目描述/项目职责",
    "self_evaluation_tks": "自我评价/个人优势/个人总结",
}


# ==================== 并行 LLM 提取的 Prompt 模板 ====================
# 参考 SmartResume 的任务分解策略，将提取拆分为三个独立子任务
# 每个 Prompt 末尾添加 /no_think 标记，抑制推理模型的思考输出

SYSTEM_PROMPT = """你是一个专业的简历分析助手。你的任务是将给定的简历文本转换为 JSON 输出。
(如果有中英文简历同时出现时，只关注中文简历)
严格按照 JSON 格式返回结果，不要有任何其他文字。"""

# 子任务1: 基本信息提取（参考 SmartResume BASIC_INFO_PROMPT 精细字段定义）
BASIC_INFO_PROMPT = """请从以下带行号索引的简历文本中提取基本信息。

{indexed_text}

提取如下信息到 JSON，若某些字段不存在则输出 "" 空或 0:
{{
  "name_kwd": "",
  "gender_kwd": "",
  "age_int": 0,
  "phone_kwd": "",
  "email_tks": "",
  "birth_dt": "",
  "work_exp_flt": 0,
  "current_location": "",
  "expect_city_names_tks": [],
  "expect_position_name_tks": [],
  "skill_tks": [],
  "language_tks": [],
  "certificate_tks": [],
  "self_evaluation_tks": ""
}}

字段说明:
- name_kwd: 姓名，如"张三"
- gender_kwd: 男/女，若不存在则不填
- age_int: 当前年龄，整数
- phone_kwd: 电话/手机，请保留原文中的形式，保留国家码区号括号
- email_tks: 邮箱，如 "xxx@qq.com"
- birth_dt: 出生年月，如 "1996-11"
- work_exp_flt: 工作年限，浮点数
- current_location: 现居地/当前城市，不要从工作经历中推测，要写明现居地
- expect_city_names_tks: 期望工作城市列表，简历中需要明确说明是期望城市
- expect_position_name_tks: 期望职位列表
- skill_tks: 技能/技术栈列表
- language_tks: 语言能力列表
- certificate_tks: 证书/资质列表
- self_evaluation_tks: 自我评价/个人优势/个人总结，完整提取原文内容

只返回 JSON。 /no_think"""

# 子任务2: 工作经历提取（使用索引指针机制，参考 SmartResume 的 jobDescription_refer_index_range）
WORK_EXP_PROMPT = """请从以下带行号索引的简历文本中提取工作经历。

{indexed_text}

提取为 JSON，每段工作经历包含:
{{
  "workExperience": [
    {{
      "company": "",
      "position": "",
      "internship": 0,
      "start_date": "",
      "end_date": "",
      "desc_lines": [start_index, end_index]
    }}
  ]
}}

字段说明:
- company: 公司全称（含括号内地区信息），如"阿里巴巴(中国)有限公司"
- position: 职位名称，遵循原文不要编造或推测
- internship: 该段经历是否是实习，是实习为1，不是为0
- start_date: 入职时间，格式为 %Y.%m 或 %Y，如 "2024.1"
- end_date: 离职时间，若至今填写"至今"，若不存在填写""
- desc_lines: [起始行号, 结束行号]，工作描述对应的行号范围（整数数组）
  - 指工作经历描述的原文引用段落 index 范围，包括工作成果、业绩、主要工作、技术栈等
  - 不包括 company、position、start_date、end_date 所在行
  - 尽可能写全，直到下一段工作经历为止
  - 如果不存在就写 []

示例:
[22]: 阿里巴巴 2021.11-2022.11 高级工程师
[23]: 工作描述: 从事地推工作完成xx业绩
[24]: 在地推任务中考核为A
则 desc_lines 应为 [23, 24]

只返回 JSON。 /no_think"""

# 子任务3: 教育背景提取
EDUCATION_PROMPT = """请从以下带行号索引的简历文本中提取教育背景。

{indexed_text}

提取为 JSON:
{{
  "education": [
    {{
      "school": "",
      "major": "",
      "degree": "",
      "department": "",
      "start_date": "",
      "end_date": "",
      "desc_lines": [start_index, end_index]
    }}
  ]
}}

字段说明:
- school: 学校全称，如"厦门大学"，中英文都可以
- major: 专业，如"机械工程"
- degree: 学位，本科/硕士/博士/专科/高中/初中，若不存在则填""
- department: 系/学院，如"信息工程系"
- start_date: 开始时间，格式为 %Y.%m 或 %Y
- end_date: 结束时间，若至今填写"至今"，若不存在填写""
- desc_lines: [起始行号, 结束行号]，教育描述对应的行号范围（可选）
  - 包括课程成绩、研究方向、GPA、荣誉奖项等
  - 不存在则填 []

只返回 JSON。 /no_think"""

# 子任务4: 项目经验提取（使用索引指针机制，与工作经历类似）
PROJECT_EXP_PROMPT = """请从以下带行号索引的简历文本中提取项目经验。

{indexed_text}

提取为 JSON，每段项目经验包含:
{{
  "projectExperience": [
    {{
      "project_name": "",
      "role": "",
      "start_date": "",
      "end_date": "",
      "desc_lines": [start_index, end_index]
    }}
  ]
}}

字段说明:
- project_name: 项目名称
- role: 担任角色/职责，如"项目负责人"、"后端开发"
- start_date: 开始时间，格式为 %Y.%m 或 %Y
- end_date: 结束时间，若至今填写"至今"，若不存在填写""
- desc_lines: [起始行号, 结束行号]，项目描述对应的行号范围（整数数组）
  - 指项目描述的原文引用段落 index 范围，包括项目内容、技术栈、成果等
  - 不包括 project_name、role、start_date、end_date 所在行
  - 尽可能写全，直到下一段项目经验为止
  - 如果不存在就写 []

只返回 JSON。 /no_think"""

# LLM 调用最大重试次数（参考 SmartResume 的重试策略）
_LLM_MAX_RETRIES = 2


# ==================== 第一阶段: PDF 文本融合与版面重建 ====================



def _extract_metadata_text(binary: bytes) -> list[dict]:
    """
    从 PDF 元数据提取文本块（含坐标信息）

    优先使用 extract_text() 获取完整行文本（行重建质量更高），
    仅在 extract_text 失败时降级到 extract_words 逐词提取。

    参数:
        binary: PDF 文件二进制内容
    返回:
        文本块列表，每个元素包含 text, x0, top, x1, bottom, page 字段
    """
    try:
        import pdfplumber
        blocks = []
        with pdfplumber.open(BytesIO(binary)) as pdf:
            for page_idx, page in enumerate(pdf.pages):
                page_width = page.width or 600
                # 优先使用 extract_text(layout=True) 保留空间布局
                page_text = None
                try:
                    page_text = page.extract_text(layout=True)
                except Exception:
                    pass
                if not page_text or not page_text.strip():
                    # 降级到普通 extract_text
                    try:
                        page_text = page.extract_text()
                    except Exception:
                        pass

                if page_text and page_text.strip():
                    # 按行拆分，为每行分配虚拟坐标（用于后续排序）
                    raw_lines = page_text.split("\n")
                    line_height = 16  # 虚拟行高
                    for i, line in enumerate(raw_lines):
                        cleaned = line.strip()
                        if not cleaned:
                            continue
                        # 根据行首缩进估算 x0 坐标
                        leading_spaces = len(line) - len(line.lstrip())
                        x0 = min(leading_spaces * 6, page_width * 0.4)
                        blocks.append({
                            "text": cleaned,
                            "x0": x0,
                            "top": i * line_height,
                            "x1": page_width,
                            "bottom": i * line_height + line_height - 2,
                            "page": page_idx,
                        })
                else:
                    # 最终降级: 使用 extract_words 逐词提取
                    words = page.extract_words(
                        keep_blank_chars=False, use_text_flow=True
                    )
                    for w in words:
                        blocks.append({
                            "text": w.get("text", ""),
                            "x0": float(w.get("x0", 0)),
                            "top": float(w.get("top", 0)),
                            "x1": float(w.get("x1", 0)),
                            "bottom": float(w.get("bottom", 0)),
                            "page": page_idx,
                        })

                # 提取页面中的表格内容
                # 很多简历使用表格布局（如个人信息栏），extract_text 会丢失这些内容
                # 参考 naive.py 中 Docx 类的表格处理方式，将表格单元格转为文本行
                try:
                    tables = page.extract_tables()
                    if tables:
                        # 计算当前页已有 blocks 的最大 top 值，表格行追加在后面
                        page_blocks = [b for b in blocks if b["page"] == page_idx]
                        max_top = max((b["top"] for b in page_blocks), default=0) + 20
                        row_height = 16

                        for table in tables:
                            for row in table:
                                if not row:
                                    continue
                                # 过滤空单元格，合并为一行文本
                                cells = [str(c).strip() for c in row if c and str(c).strip()]
                                if not cells:
                                    continue
                                row_text = " | ".join(cells)
                                # 去重: 如果该行文本已被 extract_text 提取过则跳过
                                # 使用简单的子串匹配避免重复
                                is_dup = False
                                for pb in page_blocks:
                                    # 任一单元格内容已出现在已有文本中，视为重复
                                    if all(c in pb["text"] for c in cells[:2]):
                                        is_dup = True
                                        break
                                if is_dup:
                                    continue
                                blocks.append({
                                    "text": row_text,
                                    "x0": 0,
                                    "top": max_top,
                                    "x1": page_width,
                                    "bottom": max_top + row_height - 2,
                                    "page": page_idx,
                                })
                                max_top += row_height
                except Exception as e:
                    logger.debug(f"PDF 表格提取跳过 (页 {page_idx}): {e}")
        return blocks
    except Exception as e:
        logger.warning(f"PDF 元数据提取失败: {e}")
        return []



def _extract_ocr_text(binary: bytes) -> list[dict]:
    """
    使用 OCR 从 PDF 提取文本块（含坐标信息）

    适用于扫描件或图片型 PDF，提取图片区域中的文本。

    参数:
        binary: PDF 文件二进制内容
    返回:
        文本块列表，每个元素包含 text, x0, top, x1, bottom, page 字段
    """
    try:
        import pdfplumber
        from deepdoc.vision.ocr import OCR
        import numpy as np

        ocr = OCR()
        blocks = []

        with pdfplumber.open(BytesIO(binary)) as pdf:
            for page_idx, page in enumerate(pdf.pages):
                img = page.to_image(resolution=216)
                page_img = np.array(img.annotated)
                ocr_result = ocr(page_img)
                if not ocr_result:
                    continue
                for box_info in ocr_result:
                    if isinstance(box_info, (list, tuple)) and len(box_info) >= 2:
                        coords = box_info[0]  # 坐标点
                        text_info = box_info[1]
                        text = text_info[0] if isinstance(text_info, (list, tuple)) else str(text_info)
                        if text.strip() and isinstance(coords, (list, tuple)) and len(coords) >= 4:
                            # 从四个角点提取边界框
                            xs = [p[0] for p in coords if isinstance(p, (list, tuple))]
                            ys = [p[1] for p in coords if isinstance(p, (list, tuple))]
                            if xs and ys:
                                blocks.append({
                                    "text": text.strip(),
                                    "x0": min(xs), "top": min(ys),
                                    "x1": max(xs), "bottom": max(ys),
                                    "page": page_idx,
                                })
        return blocks
    except Exception as e:
        logger.warning(f"OCR 提取失败: {e}")
        return []


def _fuse_text_blocks(meta_blocks: list[dict], ocr_blocks: list[dict]) -> list[dict]:
    """
    融合 PDF 元数据文本和 OCR 文本（参考 SmartResume 的 Content Fusion 策略）

    对于元数据已覆盖的区域，优先使用元数据文本（更准确）。
    对于元数据未覆盖的区域（图片区域），使用 OCR 文本补充。

    参数:
        meta_blocks: 元数据提取的文本块
        ocr_blocks: OCR 提取的文本块
    返回:
        融合后的文本块列表
    """
    if not ocr_blocks:
        return meta_blocks
    if not meta_blocks:
        return ocr_blocks

    fused = list(meta_blocks)

    # 对每个 OCR 块，检查是否与元数据块重叠
    for ocr_b in ocr_blocks:
        is_covered = False
        for meta_b in meta_blocks:
            if meta_b["page"] != ocr_b["page"]:
                continue
            # 计算 IoU 判断是否重叠
            x_overlap = max(0, min(meta_b["x1"], ocr_b["x1"]) - max(meta_b["x0"], ocr_b["x0"]))
            y_overlap = max(0, min(meta_b["bottom"], ocr_b["bottom"]) - max(meta_b["top"], ocr_b["top"]))
            overlap_area = x_overlap * y_overlap
            ocr_area = max(1, (ocr_b["x1"] - ocr_b["x0"]) * (ocr_b["bottom"] - ocr_b["top"]))
            if overlap_area / ocr_area > 0.5:
                is_covered = True
                break
        if not is_covered:
            fused.append(ocr_b)

    return fused


def _layout_aware_reorder(blocks: list[dict]) -> list[dict]:
    """
    版面感知的层级排序（参考 SmartResume 的 Hierarchical Re-ordering）

    两级排序策略:
    1. 段间排序: 先按页码，再按 Y 坐标（从上到下），同行按 X 坐标（从左到右）
    2. 段内排序: 在每个逻辑段内按阅读顺序排列

    对于多栏简历，通过检测 X 坐标的聚类来识别栏位，
    然后按栏位顺序排列。

    参数:
        blocks: 文本块列表（含坐标信息）
    返回:
        排序后的文本块列表
    """
    if not blocks:
        return blocks

    # 按页分组
    pages = {}
    for b in blocks:
        pg = b.get("page", 0)
        pages.setdefault(pg, []).append(b)

    sorted_blocks = []
    for pg in sorted(pages.keys()):
        page_blocks = pages[pg]

        # 检测是否为多栏布局: 通过 X 坐标中位数判断
        if len(page_blocks) > 5:
            x_centers = [(b["x0"] + b["x1"]) / 2 for b in page_blocks]
            x_min, x_max = min(x_centers), max(x_centers)
            page_width = x_max - x_min if x_max > x_min else 1

            # 简单的双栏检测: 如果文本块明显分布在左右两侧
            mid_x = (x_min + x_max) / 2
            left_count = sum(1 for x in x_centers if x < mid_x - page_width * 0.1)
            right_count = sum(1 for x in x_centers if x > mid_x + page_width * 0.1)

            if left_count > 3 and right_count > 3:
                # 多栏布局: 先左栏后右栏，各栏内从上到下
                left_blocks = [b for b in page_blocks if (b["x0"] + b["x1"]) / 2 < mid_x]
                right_blocks = [b for b in page_blocks if (b["x0"] + b["x1"]) / 2 >= mid_x]
                left_blocks.sort(key=lambda b: (b["top"], b["x0"]))
                right_blocks.sort(key=lambda b: (b["top"], b["x0"]))
                sorted_blocks.extend(left_blocks)
                sorted_blocks.extend(right_blocks)
                continue

        # 单栏布局: 从上到下，同行从左到右
        page_blocks.sort(key=lambda b: (b["top"], b["x0"]))
        sorted_blocks.extend(page_blocks)

    return sorted_blocks


def _layout_detect_reorder(blocks: list[dict], binary: bytes) -> list[dict]:
    """
    使用 YOLOv10 布局检测进行版面感知排序（参考 SmartResume 的 Layout-Aware Reordering）

    流程:
    1. 将 PDF 每页渲染为图片
    2. 用 YOLOv10 检测布局区域（标题、正文、表格等）
    3. 将文本块归属到检测出的布局区域
    4. 按区域位置做层级排序（区域中心 Y → 区域中心 X → 块内 Y → 块内 X）

    检测失败时自动降级到启发式排序。

    参数:
        blocks: 文本块列表（含坐标信息）
        binary: PDF 文件二进制内容（用于渲染页面图片）
    返回:
        排序后的文本块列表
    """
    if not blocks:
        return blocks

    recognizer = _get_layout_recognizer()
    if recognizer is None:
        logger.info("布局检测器不可用，降级为启发式排序")
        return _layout_aware_reorder(blocks)

    try:
        import pdfplumber
        from PIL import Image
        import numpy as np

        # 按页分组文本块
        pages_blocks = {}
        for b in blocks:
            pg = b.get("page", 0)
            pages_blocks.setdefault(pg, []).append(b)

        # 渲染每页为图片，并准备 OCR 格式的输入
        page_indices = sorted(pages_blocks.keys())
        image_list = []
        ocr_res_per_page = []

        with pdfplumber.open(BytesIO(binary)) as pdf:
            for pg in page_indices:
                if pg >= len(pdf.pages):
                    continue
                page = pdf.pages[pg]
                # 渲染为 PIL Image（scale_factor=3 与 LayoutRecognizer 默认一致）
                pil_img = page.to_image(resolution=72 * 3).annotated
                image_list.append(pil_img)

                # 将该页的文本块转为 LayoutRecognizer 需要的格式
                page_bxs = []
                for b in pages_blocks[pg]:
                    page_bxs.append({
                        "x0": float(b["x0"]),
                        "top": float(b["top"]),
                        "x1": float(b["x1"]),
                        "bottom": float(b["bottom"]),
                        "text": b["text"],
                        "page": pg,
                    })
                ocr_res_per_page.append(page_bxs)

        if not image_list:
            return _layout_aware_reorder(blocks)

        # 调用 YOLOv10 布局检测 + 文本块标注
        # LayoutRecognizer.__call__ 会为每个文本块打上 layout_type 和 layoutno
        tagged_blocks, page_layouts = recognizer(
            image_list, ocr_res_per_page, scale_factor=3, thr=0.2, drop=False
        )

        if not tagged_blocks:
            logger.warning("布局检测未返回结果，降级为启发式排序")
            return _layout_aware_reorder(blocks)

        # 按 layoutno 分组排序:
        # 1. 有 layoutno 的块按区域位置排序（区域 Y → 区域 X → 块内 Y → 块内 X）
        # 2. 无 layoutno 的块按原始坐标排序
        def _sort_key(b):
            layoutno = b.get("layoutno", "")
            layout_type = b.get("layout_type", "")
            # header 排最前，footer 排最后
            if layout_type == "header":
                return (0, 0, b.get("top", 0), b.get("x0", 0))
            if layout_type == "footer":
                return (9999, 0, b.get("top", 0), b.get("x0", 0))
            # 其他按 top → x0 排序
            return (1, b.get("top", 0), b.get("x0", 0), 0)

        tagged_blocks.sort(key=_sort_key)

        # 恢复 page 字段（LayoutRecognizer 可能不保留）
        for b in tagged_blocks:
            if "page" not in b:
                b["page"] = 0

        logger.info(f"YOLOv10 布局检测完成，共 {len(tagged_blocks)} 个文本块，"
                    f"检测到 {sum(len(pl) for pl in page_layouts)} 个布局区域")
        return tagged_blocks

    except Exception as e:
        logger.warning(f"YOLOv10 布局检测排序失败，降级为启发式排序: {e}")
        return _layout_aware_reorder(blocks)



def _build_indexed_text(blocks: list[dict]) -> tuple[str, list[str], list[dict]]:
    """
    构建带行号索引的文本（参考 SmartResume 的 Indexed Linearization）

    将排序后的文本块合并为行，并为每行添加唯一索引号。
    包含乱码行过滤逻辑，以及字段标签拆分修复。
    同时保留每行对应的坐标信息，用于后续写入 chunk 的 position_int 等字段。

    参数:
        blocks: 排序后的文本块列表
    返回:
        (indexed_text, lines, line_positions) 元组:
        - indexed_text: 带行号的文本字符串
        - lines: 原始行文本列表（不含行号）
        - line_positions: 每行对应的坐标信息，格式为
          {"page": int, "x0": float, "x1": float, "top": float, "bottom": float}
    """
    if not blocks:
        return "", [], []

    # 将相邻的文本块合并为行（基于 Y 坐标接近度）
    # 同时记录每行的边界框（取所有块的外接矩形）
    raw_lines = []
    raw_positions = []  # 每行对应的坐标
    current_line_parts = []
    current_line_blocks = []  # 当前行包含的所有块
    current_top = blocks[0].get("top", 0)
    threshold = 10  # Y 坐标差异阈值

    def _merge_line_position(line_blocks: list[dict]) -> dict:
        """合并一行中所有块的坐标为外接矩形"""
        return {
            "page": line_blocks[0].get("page", 0),
            "x0": min(b.get("x0", 0) for b in line_blocks),
            "x1": max(b.get("x1", 0) for b in line_blocks),
            "top": min(b.get("top", 0) for b in line_blocks),
            "bottom": max(b.get("bottom", 0) for b in line_blocks),
        }

    for b in blocks:
        if abs(b.get("top", 0) - current_top) > threshold and current_line_parts:
            raw_lines.append(" ".join(current_line_parts))
            raw_positions.append(_merge_line_position(current_line_blocks))
            current_line_parts = []
            current_line_blocks = []
            current_top = b.get("top", 0)
        current_line_parts.append(b["text"])
        current_line_blocks.append(b)

    if current_line_parts:
        raw_lines.append(" ".join(current_line_parts))
        raw_positions.append(_merge_line_position(current_line_blocks))

    # 过滤空行和乱码行（同步过滤坐标）
    lines = []
    line_positions = []
    for line, pos in zip(raw_lines, raw_positions):
        line = line.strip()
        if not line:
            continue
        # 乱码检测: 如果一行中有效字符（中文/ASCII字母/数字/常见标点）占比过低则跳过
        if not _is_valid_line(line):
            continue
        lines.append(line)
        line_positions.append(pos)

    # 修复字段标签拆分问题（如 "名：陈晓俐 姓" -> "姓名：陈晓俐"）
    # 坐标不受影响，保持原位置
    lines = _fix_split_labels(lines)

    # 构建带行号索引的文本
    indexed_parts = [f"[{i}]: {line}" for i, line in enumerate(lines)]
    indexed_text = "\n".join(indexed_parts)

    return indexed_text, lines, line_positions

def _is_valid_line(line: str) -> bool:
    """
    检测一行文本是否为有效内容（非乱码）

    通过计算有效字符占比来判断：中文、ASCII字母、数字、常见标点。
    占比低于 40% 且行长度大于 3 的视为乱码行。

    参数:
        line: 待检测的文本行
    返回:
        True 表示有效行，False 表示乱码行
    """
    if len(line) <= 3:
        # 短行可能是姓名等有效内容，保留
        return True
    # 有效字符: 中文、ASCII字母数字、常见标点和空格
    # 有效字符: 中文、ASCII字母数字、常见标点和空格
    valid_chars = re.findall(
        r'[\u4e00-\u9fa5a-zA-Z0-9\s@.,:;!?()（）【】\-_/\\|·•、，。：；！？\u201c\u201d\u2018\u2019《》]',
        line
    )
    ratio = len(valid_chars) / len(line) if len(line) > 0 else 0
    return ratio >= 0.4


def _fix_split_labels(lines: list[str]) -> list[str]:
    """
    修复字段标签拆分问题

    某些 PDF 排版会将字段标签拆分到行首行尾，例如:
    - "名：陈晓俐 姓" -> 应修复为 "姓名：陈晓俐"
    - "别：男 性" -> 应修复为 "性别：男"

    参数:
        lines: 原始行文本列表
    返回:
        修复后的行文本列表
    """
    # 常见的被拆分的字段标签模式: (行尾部分, 行首部分) -> 完整标签
    split_patterns = {
        ("姓", "名"): "姓名",
        ("性", "别"): "性别",
        ("年", "龄"): "年龄",
        ("电", "话"): "电话",
        ("邮", "箱"): "邮箱",
        ("学", "历"): "学历",
        ("专", "业"): "专业",
        ("地", "址"): "地址",
        ("籍", "贯"): "籍贯",
        ("民", "族"): "民族",
    }

    fixed = []
    for line in lines:
        # 检测行内的拆分模式: "X：内容 Y" 其中 (Y, X) 是拆分对
        for (suffix_char, prefix_char), full_label in split_patterns.items():
            # 模式: "prefix_char：内容 suffix_char" (标签前半在行首，后半在行尾)
            pattern = rf'^({re.escape(prefix_char)})\s*[:：]\s*(.+?)\s+{re.escape(suffix_char)}\s*$'
            m = re.match(pattern, line)
            if m:
                content = m.group(2).strip()
                line = f"{full_label}：{content}"
                break
            # 模式: "suffix_char 内容 prefix_char：" (标签后半在行首，前半在行尾)
            pattern2 = rf'^{re.escape(suffix_char)}\s*[:：]?\s*(.+?)\s+{re.escape(prefix_char)}\s*$'
            m2 = re.match(pattern2, line)
            if m2:
                content = m2.group(1).strip()
                line = f"{full_label}：{content}"
                break
        fixed.append(line)
    return fixed





def extract_text(filename: str, binary: bytes) -> tuple[str, list[str], list[dict]]:
    """
    根据文件类型提取文本内容（Pipeline 第一阶段）

    PDF 文件使用双路径融合 + 版面重建 + 行号索引。
    其他格式降级到简单文本提取。

    参数:
        filename: 文件名
        binary: 文件二进制内容
    返回:
        (indexed_text, lines, line_positions) 元组:
        - indexed_text: 带行号索引的文本
        - lines: 原始行文本列表
        - line_positions: 每行坐标信息列表（非 PDF 格式为空列表）
    """
    fname_lower = filename.lower()

    try:
        if fname_lower.endswith(".pdf"):
            # 双路径提取
            meta_blocks = _extract_metadata_text(binary)
            ocr_blocks = []
            # 如果元数据提取文本过少，启用 OCR 补充
            meta_text_len = sum(len(b["text"]) for b in meta_blocks)
            if meta_text_len < 100:
                logger.info("PDF 元数据文本过少，启用 OCR 补充提取")
                ocr_blocks = _extract_ocr_text(binary)

            # 文本融合
            fused_blocks = _fuse_text_blocks(meta_blocks, ocr_blocks)

            # 版面感知排序（优先使用 YOLOv10 布局检测，失败时降级为启发式）
            sorted_blocks = _layout_detect_reorder(fused_blocks, binary)

            # 构建带行号索引的文本（含坐标信息）
            return _build_indexed_text(sorted_blocks)

        elif fname_lower.endswith(".docx"):
            from docx import Document
            doc = Document(BytesIO(binary))
            lines = [p.text.strip() for p in doc.paragraphs if p.text.strip()]

            # 提取 DOCX 中的表格内容
            # 参考 naive.py 中 Docx 类的表格处理方式
            # 很多简历使用表格布局存放个人信息，仅遍历 paragraphs 会丢失这些内容
            for table in doc.tables:
                for row in table.rows:
                    cells = []
                    for cell in row.cells:
                        cell_text = cell.text.strip()
                        if cell_text:
                            cells.append(cell_text)
                    if not cells:
                        continue
                    row_text = " | ".join(cells)
                    # 去重: 如果该行文本已存在于 lines 中则跳过
                    if row_text not in lines:
                        lines.append(row_text)

            indexed = "\n".join(f"[{i}]: {line}" for i, line in enumerate(lines))
            # DOCX 无坐标信息，返回空列表
            return indexed, lines, []

        else:
            text = get_text(filename, binary)
            lines = [line.strip() for line in text.split("\n") if line.strip()]
            indexed = "\n".join(f"[{i}]: {line}" for i, line in enumerate(lines))
            return indexed, lines, []

    except Exception as e:
        logger.exception(f"文本提取失败: {filename}")
        return "", [], []


# ==================== 第二阶段: 并行 LLM 结构化提取 ====================


def _clean_llm_json_response(response: str) -> str:
    """
    清理 LLM 返回的 JSON 响应

    使用 SmartResume 的轻量字符串提取策略:
    1. 移除 markdown 代码块标记
    2. 移除 <think>...</think> 思考标签（推理模型可能输出）
    3. text.find("{") 和 text.rfind("}") 定位有效 JSON 块

    参数:
        response: LLM 原始响应文本
    返回:
        清理后的 JSON 字符串
    """
    text = response.strip()
    # 移除 markdown 代码块标记
    text = text.replace("```json", "").replace("```", "").strip()
    # 移除推理模型的思考标签
    text = re.sub(r'<think>.*?</think>', '', text, flags=re.DOTALL).strip()
    # 清理转义引号（SmartResume 的处理方式）
    text = text.replace('\\"', '"')
    # 使用 SmartResume 的策略: 定位第一个 { 和最后一个 }
    start = text.find("{")
    end = text.rfind("}")
    if start != -1 and end != -1 and end > start:
        return text[start:end + 1]
    return text


def _parse_json_with_repair(text: str) -> dict:
    """
    解析 JSON 字符串，失败时尝试修复（参考 SmartResume 的 json_repair 策略）

    修复策略:
    1. 标准 json.loads
    2. 替换 Python 风格的布尔值/None
    3. 使用 json_repair 库修复

    参数:
        text: JSON 字符串
    返回:
        解析后的字典
    异常:
        json.JSONDecodeError: 所有修复策略均失败时抛出
    """
    # 第一次尝试: 标准解析
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass

    # 第二次尝试: 替换 Python 风格的值（参考 SmartResume）
    repaired = text.replace("'", '"')
    repaired = repaired.replace('True', 'true')
    repaired = repaired.replace('False', 'false')
    repaired = repaired.replace('None', 'null')
    try:
        return json.loads(repaired)
    except json.JSONDecodeError:
        pass

    # 第三次尝试: 使用 json_repair 库
    if json_repair is not None:
        try:
            return json_repair.loads(text)
        except Exception:
            pass

    # 所有策略失败
    raise json.JSONDecodeError("所有 JSON 修复策略均失败", text, 0)


def _call_llm(prompt: str, tenant_id , lang: str) -> Optional[dict]:
    """
    调用 LLM 并解析 JSON 响应（参考 SmartResume 的重试 + 容错策略）

    重试机制:
    - 最多重试 _LLM_MAX_RETRIES 次
    - 重试时提高 temperature 并随机化 seed，增加输出多样性
    - JSON 解析失败时使用 json_repair 修复

    参数:
        prompt: 用户提示词
        lang: 语言
    返回:
        解析后的字典，失败返回 None
    """
    try:
        from api.db.services.llm_service import LLMBundle
        from common.constants import LLMType

        llm = LLMBundle(tenant_id, llm_type=LLMType.CHAT, lang=lang)

        for attempt in range(_LLM_MAX_RETRIES + 1):
            try:
                # 重试时提高 temperature 增加多样性（参考 SmartResume）
                temperature = 0.1 if attempt == 0 else 1.0
                gen_conf = {"temperature": temperature, "max_tokens": 2048}
                if attempt > 0:
                    gen_conf["seed"] = random.randint(0, 1000000)

                response = llm.chat(
                    system=SYSTEM_PROMPT,
                    history=[{"role": "user", "content": prompt}],
                    gen_conf=gen_conf,
                )
                cleaned = _clean_llm_json_response(response)
                return _parse_json_with_repair(cleaned)

            except json.JSONDecodeError as e:
                if attempt < _LLM_MAX_RETRIES:
                    logger.info(f"LLM JSON 解析失败(第{attempt + 1}次)，准备重试: {e}")
                    continue
                else:
                    logger.warning(f"LLM JSON 解析失败(已用尽重试): {e}")
                    return None

    except Exception as e:
        logger.warning(f"LLM 调用失败: {e}")
        return None


def _normalize_for_comparison(text: str) -> str:
    """
    归一化文本用于比较（参考 SmartResume 的 _normalize_for_comparison）

    统一全角/半角、去除空白、Unicode 归一化，
    使得 "阿里巴巴" 和 "阿 里 巴 巴" 能匹配。

    参数:
        text: 原始文本
    返回:
        归一化后的文本
    """
    if not text:
        return ""
    # Unicode NFKC 归一化（全角转半角等）
    text = unicodedata.normalize("NFKC", text)
    # 去除所有空白字符
    text = re.sub(r'\s+', '', text)
    return text.lower()

def _calc_single_exp_years(start_str: str, end_str: str) -> float:
    """
    计算单段经历的年限

    参数:
        start_str: 起始日期字符串
        end_str: 结束日期字符串（"至今" 等表示当前）
    返回:
        年限（浮点数，保留1位小数），无法计算返回 0
    """
    from datetime import datetime

    start_str = str(start_str).strip()
    end_str = str(end_str).strip()
    if not start_str:
        return 0

    start_date = _parse_date_str(start_str)
    if not start_date:
        return 0

    if end_str in ("至今", "现在", "present", "Present", "now", "Now", ""):
        end_date = datetime.now()
    else:
        end_date = _parse_date_str(end_str)
        if not end_date:
            end_date = datetime.now()

    months = (end_date.year - start_date.year) * 12 + (end_date.month - start_date.month)
    if months <= 0:
        return 0
    return round(months / 12.0, 1)


def _calculate_work_years(experiences: list[dict]) -> float:
    """
    根据每段工作经历的起止日期计算总工作年限

    参数:
        experiences: 工作经历列表，每项包含 start_date、end_date 字段
    返回:
        总工作年限（浮点数），无法计算时返回 0
    """
    total = 0.0
    for exp in experiences:
        total += _calc_single_exp_years(
            exp.get("start_date", ""), exp.get("end_date", "")
        )
    return round(total, 1)


def _parse_date_str(date_str: str) -> Optional["datetime"]:
    """
    解析日期字符串，支持多种常见格式

    支持格式:
    - 2024.1 / 2024.01
    - 2024-1 / 2024-01
    - 2024/1 / 2024/01
    - 2024年1月
    - 2024（仅年份，默认1月）

    参数:
        date_str: 日期字符串
    返回:
        datetime 对象，解析失败返回 None
    """
    from datetime import datetime

    date_str = date_str.strip()
    # 尝试匹配 年.月 / 年-月 / 年/月 / 年年月月
    patterns = [
        (r"((?:19|20)\d{2})[.\-/年](\d{1,2})", "%Y-%m"),
        (r"^((?:19|20)\d{2})$", "%Y"),
    ]
    for pattern, _ in patterns:
        m = re.search(pattern, date_str)
        if m:
            try:
                year = int(m.group(1))
                month = int(m.group(2)) if len(m.groups()) > 1 else 1
                # 月份范围校验
                if month < 1 or month > 12:
                    month = 1
                return datetime(year, month, 1)
            except (ValueError, IndexError):
                continue
    return None




def _extract_description_from_range(
        index_range: list, lines: list[str],
        company: str = "", position: str = ""
) -> str:
    """
    从原文中按索引范围回提取描述（参考 SmartResume 的 _extract_description_from_range）

    关键改进:
    - 过滤掉同时包含公司名和职位名的行（避免将标题行混入描述）
    - 边界安全检查

    参数:
        index_range: [起始行号, 结束行号]
        lines: 原始行文本列表
        company: 公司名称（用于过滤标题行）
        position: 职位名称（用于过滤标题行）
    返回:
        提取的描述文本
    """
    if not index_range or len(index_range) != 2:
        return ""

    start_idx, end_idx = int(index_range[0]), int(index_range[1])

    # 边界安全检查
    if start_idx < 0 or end_idx >= len(lines) or start_idx > end_idx:
        return ""

    extracted_lines = lines[start_idx:end_idx + 1]

    # 过滤掉同时包含公司名和职位名的行（参考 SmartResume）
    if company or position:
        norm_company = _normalize_for_comparison(company)
        norm_position = _normalize_for_comparison(position)
        filtered = []
        for line in extracted_lines:
            norm_line = _normalize_for_comparison(line)
            # 如果一行同时包含公司名和职位名，大概率是标题行，跳过
            if norm_company and norm_position and norm_company in norm_line and norm_position in norm_line:
                continue
            # 如果一行完全等于公司名或职位名，也跳过
            if norm_line == norm_company or norm_line == norm_position:
                continue
            filtered.append(line)
        extracted_lines = filtered

    if not extracted_lines:
        return ""

    return "\n".join(line.strip() for line in extracted_lines if line.strip())


def _extract_basic_info(indexed_text: str,tenant_id, lang: str) -> Optional[dict]:
    """提取基本信息（子任务1）

    基本信息通常在简历开头，截取前 8000 字符即可覆盖
    """
    prompt = BASIC_INFO_PROMPT.format(indexed_text=indexed_text[:8000])
    return _call_llm(prompt,tenant_id, lang)


def _extract_work_experience(indexed_text: str,tenant_id, lang: str) -> Optional[dict]:
    """提取工作经历（子任务2，使用索引指针）

    工作经历可能分布在简历中后部，使用完整文本避免截断
    """
    prompt = WORK_EXP_PROMPT.format(indexed_text=indexed_text)
    return _call_llm(prompt,tenant_id, lang)


def _extract_education(indexed_text: str,tenant_id, lang: str) -> Optional[dict]:
    """提取教育背景（子任务3）

    教育背景通常在简历末尾，必须使用完整文本避免截断
    简历文本一般不超过 30K 字符，对 LLM 上下文窗口来说完全可以承受
    """
    prompt = EDUCATION_PROMPT.format(indexed_text=indexed_text)
    return _call_llm(prompt,tenant_id, lang)


def _extract_project_experience(indexed_text: str,tenant_id, lang: str) -> Optional[dict]:
    """提取项目经验（子任务4，使用索引指针）

    项目经验可能分布在简历中后部，使用完整文本避免截断
    """
    prompt = PROJECT_EXP_PROMPT.format(indexed_text=indexed_text)
    return _call_llm(prompt, lang)


def parse_with_llm(indexed_text: str, lines: list[str], tenant_id, lang: str) -> Optional[dict]:
    """
    使用并行任务分解策略提取简历信息（参考 SmartResume Section 3.2）

    将提取任务分解为四个独立子任务并行执行:
    1. 基本信息（姓名、电话、技能、自我评价等）
    2. 工作经历（公司、职位、描述行号范围）
    3. 教育背景（学校、专业、学历）
    4. 项目经验（项目名称、角色、描述行号范围）

    参数:
        indexed_text: 带行号索引的简历文本
        lines: 原始行文本列表（用于索引回提取）
        lang: 语言
    返回:
        合并后的结构化简历字典，失败返回 None
    """
    try:
        # 并行执行四个子任务
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
            future_basic = executor.submit(_extract_basic_info, indexed_text,tenant_id, lang)
            future_work = executor.submit(_extract_work_experience, indexed_text,tenant_id, lang)
            future_edu = executor.submit(_extract_education, indexed_text,tenant_id, lang)
            future_project = executor.submit(_extract_project_experience, indexed_text,tenant_id, lang)

            basic_info = future_basic.result(timeout=60)
            work_exp = future_work.result(timeout=60)
            education = future_edu.result(timeout=60)
            project_exp = future_project.result(timeout=60)

        # 合并结果
        resume = {}

        # 合并基本信息
        if basic_info:
            resume.update(basic_info)
            logger.info(f"基本信息提取成功: {len(basic_info)} 个字段")

        # 处理工作经历（索引指针回提取）
        if work_exp and "workExperience" in work_exp:
            experiences = work_exp["workExperience"]
            companies = []
            positions = []
            work_descs = []
            # 保存每段经历的详细信息（时间、年限），供 chunk 生成时使用
            work_exp_details = []
            for exp in experiences:
                company = exp.get("company", "")
                position = exp.get("position", "")
                start_date = exp.get("start_date", "")
                end_date = exp.get("end_date", "")
                # 计算该段经历的年限
                years = _calc_single_exp_years(start_date, end_date)
                if company:
                    companies.append(company)
                if position:
                    positions.append(position)
                # 保存每段经历的详细信息
                work_exp_details.append({
                    "company": company,
                    "position": position,
                    "start_date": start_date,
                    "end_date": end_date,
                    "years": years,
                })
                # 索引指针机制: 用行号范围从原文回提取描述
                # 使用 _extract_description_from_range 过滤标题行（参考 SmartResume）
                desc_lines = exp.get("desc_lines", [])
                if isinstance(desc_lines, list) and len(desc_lines) == 2:
                    desc = _extract_description_from_range(
                        desc_lines, lines, company=company, position=position
                    )
                    if desc.strip():
                        work_descs.append(desc.strip())

            if companies:
                resume["corp_nm_tks"] = companies
                resume["corporation_name_tks"] = companies[0]
            if positions:
                resume["position_name_tks"] = positions
            if work_descs:
                resume["work_desc_tks"] = work_descs
            # 保存每段经历详情，供 _build_chunk_document 使用
            if work_exp_details:
                resume["_work_exp_details"] = work_exp_details
            # 根据每段经历的起止日期计算总工作年限（覆盖 LLM 基本信息中的猜测值）
            calculated_years = _calculate_work_years(experiences)
            if calculated_years > 0:
                resume["work_exp_flt"] = calculated_years
            logger.info(f"工作经历提取成功: {len(experiences)} 段, 计算总年限: {calculated_years}")

        # 处理教育背景
        if education and "education" in education:
            edu_list = education["education"]
            schools = []
            majors = []
            degrees = []
            for edu in edu_list:
                if edu.get("school"):
                    schools.append(edu["school"])
                if edu.get("major"):
                    majors.append(edu["major"])
                if edu.get("degree"):
                    degrees.append(edu["degree"])
                # 提取毕业年份
                end_date = edu.get("end_date", "")
                if end_date and not resume.get("edu_end_int"):
                    year_match = re.search(r"(19|20)\d{2}", str(end_date))
                    if year_match:
                        resume["edu_end_int"] = int(year_match.group(0))

            if schools:
                resume["school_name_tks"] = schools
                resume["first_school_name_tks"] = schools[-1]  # 最早的学校通常在最后
            if majors:
                resume["major_tks"] = majors
                resume["first_major_tks"] = majors[-1]
            if degrees:
                resume["degree_kwd"] = degrees
                # 推断最高学历
                degree_rank = {"博士": 5, "硕士": 4, "MBA": 4, "EMBA": 4,
                               "本科": 3, "大专": 2, "专科": 2, "高中": 1}
                highest = max(degrees, key=lambda d: degree_rank.get(d, 0), default="")
                if highest:
                    resume["highest_degree_kwd"] = highest
                resume["first_degree_kwd"] = degrees[-1] if degrees else ""
            logger.info(f"教育背景提取成功: {len(edu_list)} 段")

        # 处理项目经验（索引指针回提取，与工作经历类似）
        if project_exp and "projectExperience" in project_exp:
            projects = project_exp["projectExperience"]
            project_names = []
            project_descs = []
            for proj in projects:
                name = proj.get("project_name", "")
                if name:
                    project_names.append(name)
                # 索引指针机制: 用行号范围从原文回提取项目描述
                desc_lines = proj.get("desc_lines", [])
                if isinstance(desc_lines, list) and len(desc_lines) == 2:
                    desc = _extract_description_from_range(
                        desc_lines, lines, company=name, position=proj.get("role", "")
                    )
                    if desc.strip():
                        project_descs.append(desc.strip())

            if project_names:
                resume["project_tks"] = project_names
            if project_descs:
                resume["project_desc_tks"] = project_descs
            logger.info(f"项目经验提取成功: {len(projects)} 段")

        if not resume.get("name_kwd"):
            resume["name_kwd"] = "未知"

        return resume if len(resume) > 2 else None

    except concurrent.futures.TimeoutError:
        logger.warning("LLM 并行提取超时")
        return None
    except Exception as e:
        logger.warning(f"LLM 并行提取失败: {e}")
        return None


# ==================== 第三阶段: 正则降级解析 ====================



def parse_with_regex(text: str) -> dict:
    """
    使用正则表达式解析简历文本（降级方案）

    当 LLM 解析失败时，使用正则表达式从文本中
    提取基本的结构化信息。

    参数:
        text: 简历文本内容（不含行号索引）
    返回:
        结构化的简历信息字典
    """
    resume: dict = {}
    lines = [line.strip() for line in text.split("\n") if line.strip()]

    # --- 提取姓名 ---
    # 策略1: 从 "姓名：XXX" 格式提取
    for line in lines[:30]:
        name_match = re.search(r'姓\s*名\s*[:：]\s*([\u4e00-\u9fa5]{2,4})', line)
        if name_match:
            resume["name_kwd"] = name_match.group(1)
            break

    # 策略2: 从前20行找独立的中文姓名（2-4字，排除常见标题词）
    if "name_kwd" not in resume:
        title_words = {
            "个人", "简历", "求职", "应聘", "基本", "信息", "概述", "简介",
            "教育", "工作", "经历", "经验", "技能", "项目", "自我", "评价",
            "专业", "技术", "证书", "语言", "能力", "培训", "荣誉", "奖项",
        }
        for line in lines[:20]:
            # 跳过包含常见标题词的行
            if any(w in line for w in title_words):
                continue
            # 跳过包含冒号的行（通常是字段标签行）
            if re.search(r'[:：]', line) and len(line) > 6:
                continue
            # 清理行首尾的英文/数字/空格
            cleaned = re.sub(r"^[A-Za-z_\-\d\s]+\s+", "", line)
            cleaned = re.sub(r"\s+[A-Za-z_\-\d\s]+$", "", cleaned).strip()
            if 2 <= len(cleaned) <= 4 and re.match(r"^[\u4e00-\u9fa5]{2,4}$", cleaned):
                resume["name_kwd"] = cleaned
                break

    # 策略3: 第一行如果较短且无数字，可能是姓名
    if "name_kwd" not in resume and lines:
        first = lines[0].strip()
        if len(first) <= 10 and not re.search(r"\d", first):
            # 提取其中的中文部分
            cn_part = re.findall(r'[\u4e00-\u9fa5]+', first)
            if cn_part and 2 <= len(cn_part[0]) <= 4:
                resume["name_kwd"] = cn_part[0]

    # --- 提取电话号码 ---
    phones = re.findall(r"1[3-9]\d{9}", text)
    if phones:
        resume["phone_kwd"] = phones[0]

    # --- 提取邮箱 ---
    emails = re.findall(r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}", text)
    if emails:
        resume["email_tks"] = emails[0]

    # --- 提取性别 ---
    # 优先从 "性别：男/女" 格式提取
    gender_label = re.search(r'性\s*别\s*[:：]\s*(男|女)', text)
    if gender_label:
        resume["gender_kwd"] = gender_label.group(1)
    else:
        gender_match = re.search(r"(男|女)", text[:500])  # 限制在前500字符
        if gender_match:
            resume["gender_kwd"] = gender_match.group(1)

    # --- 提取年龄 ---
    age_match = re.search(r"(\d{1,2})\s*岁", text)
    if age_match:
        resume["age_int"] = int(age_match.group(1))

    # --- 提取出生日期 ---
    birth_match = re.search(r"(19|20)\d{2}[年/-]\d{1,2}[月/-]\d{1,2}", text)
    if birth_match:
        resume["birth_dt"] = birth_match.group(0)

    # --- 提取学历 ---
    degree_keywords = ["博士", "硕士", "本科", "大专", "专科", "高中", "MBA", "EMBA", "MPA"]
    found_degrees = [d for d in degree_keywords if d in text]
    if found_degrees:
        resume["degree_kwd"] = found_degrees

    # --- 提取学校 ---
    schools = re.findall(r"[\u4e00-\u9fa5]{2,15}(?:大学|学院|职业技术学院)", text)
    if schools:
        resume["school_name_tks"] = list(set(schools))
        resume["first_school_name_tks"] = schools[0]

    # --- 提取专业 ---
    majors = re.findall(r"专业[:：]\s*([\u4e00-\u9fa5]{2,20})", text)
    if majors:
        resume["major_tks"] = majors
        resume["first_major_tks"] = majors[0]

    # --- 提取公司名称 ---
    company_patterns = [
        r"[\u4e00-\u9fa5]{2,20}[（(][\u4e00-\u9fa5]{2,10}[)）](?:科技|信息技术|网络科技)?(?:股份)?有限公司",
        r"[\u4e00-\u9fa5]{4,20}(?:科技|信息技术|网络科技|银行)?(?:股份)?有限公司",
    ]
    companies = []
    for pattern in company_patterns:
        companies.extend(re.findall(pattern, text))

    unique_companies = []
    seen = set()
    filter_verbs = ["完成", "进行", "实施", "负责", "参与", "开发"]
    for c in companies:
        if len(c) < 6 or any(v in c for v in filter_verbs) or c in seen:
            continue
        is_sub = False
        for existing in list(unique_companies):
            if c in existing:
                is_sub = True
                break
            if existing in c:
                unique_companies.remove(existing)
                seen.discard(existing)
        if not is_sub:
            unique_companies.append(c)
            seen.add(c)

    if unique_companies:
        resume["corp_nm_tks"] = unique_companies
        resume["corporation_name_tks"] = unique_companies[0]

    # --- 提取职位（改进：需要上下文约束，减少噪声） ---
    # 策略1: 从 "职位/岗位：XXX" 格式提取
    position_label_matches = re.findall(
        r'(?:职位|岗位|职务|职称|担任)\s*[:：]\s*([\u4e00-\u9fa5a-zA-Z]{2,15})',
        text
    )
    positions = list(position_label_matches)

    # 策略2: 从工作经历段落中提取（紧跟公司名称后的职位）
    for line in lines:
        # 匹配 "公司名 职位名" 或 "日期 公司名 职位名" 模式
        pos_match = re.search(
            r'(?:有限公司|集团|银行)\s+([\u4e00-\u9fa5]{2,8}(?:工程师|经理|总监|主管|专员|设计师|顾问|助理|架构师|分析师|运营|产品))',
            line
        )
        if pos_match:
            positions.append(pos_match.group(1))

    # 策略3: 独立行中的职位关键词（行长度限制，避免匹配描述文本）
    position_suffixes = ["工程师", "经理", "总监", "主管", "专员", "设计师", "顾问",
                         "助理", "架构师", "分析师", "开发者", "负责人"]
    for line in lines:
        if len(line) > 20:
            continue  # 跳过过长的行（通常是描述文本）
        for suffix in position_suffixes:
            match = re.search(rf'([\u4e00-\u9fa5]{{1,6}}{suffix})', line)
            if match:
                pos = match.group(1)
                # 过滤掉明显不是职位的匹配（包含动词）
                if not any(v in pos for v in ["负责", "参与", "完成", "开发了", "设计了"]):
                    positions.append(pos)

    if positions:
        # 去重并保持顺序
        seen_pos = set()
        unique_positions = []
        for p in positions:
            if p not in seen_pos:
                seen_pos.add(p)
                unique_positions.append(p)
        resume["position_name_tks"] = unique_positions

    # --- 提取工作年限 ---
    work_exp_match = re.search(r"(\d+)\s*年.*?经验", text)
    if work_exp_match:
        resume["work_exp_flt"] = float(work_exp_match.group(1))

    # --- 提取毕业年份 ---
    grad_match = re.search(r"((?:19|20)\d{2})\s*年.*?毕业", text)
    if grad_match:
        resume["edu_end_int"] = int(grad_match.group(1))

    if "name_kwd" not in resume:
        resume["name_kwd"] = "未知"

    return resume



# ==================== 第四阶段: 后处理流水线 ====================


def _postprocess_resume(resume: dict, lines: list[str]) -> dict:
    """
    四阶段后处理流水线（参考 SmartResume Section 3.2.3）

    1. 源文本验证: 检查关键字段是否能在原文中找到
    2. 领域规范化: 标准化日期格式、清理公司名称后缀噪声
    3. 上下文去重: 去除重复的公司/学校条目
    4. 字段补全: 确保所有必需字段存在

    参数:
        resume: LLM 提取的原始简历字典
        lines: 原始行文本列表（用于源文本验证）
    返回:
        后处理后的简历字典
    """
    full_text = "\n".join(lines) if lines else ""
    # 归一化全文用于比较（参考 SmartResume 的 _validate_fields_in_text）
    norm_full_text = _normalize_for_comparison(full_text)

    # --- 阶段1: 源文本验证（剪枝幻觉，参考 SmartResume） ---
    if resume.get("name_kwd") and resume["name_kwd"] != "未知":
        norm_name = _normalize_for_comparison(resume["name_kwd"])
        if norm_full_text and norm_name not in norm_full_text:
            logger.warning(f"姓名 '{resume['name_kwd']}' 在原文中未找到，可能是幻觉")
            resume["_name_confidence"] = "low"

    # 验证公司名称（使用归一化比较）
    if resume.get("corp_nm_tks") and norm_full_text:
        verified_companies = []
        for company in resume["corp_nm_tks"]:
            norm_company = _normalize_for_comparison(company)
            # 公司名称至少部分出现在原文中（允许缩写，取前4字比较）
            norm_short = norm_company[:4] if len(norm_company) > 4 else norm_company
            if norm_short in norm_full_text or norm_company in norm_full_text:
                verified_companies.append(company)
            else:
                logger.debug(f"公司 '{company}' 在原文中未找到，已过滤")
        if verified_companies:
            resume["corp_nm_tks"] = verified_companies
            resume["corporation_name_tks"] = verified_companies[0]

    # 验证学校名称（参考 SmartResume 的 _validate_fields_in_text）
    if resume.get("school_name_tks") and norm_full_text:
        verified_schools = []
        for school in resume["school_name_tks"]:
            norm_school = _normalize_for_comparison(school)
            if norm_school and norm_school in norm_full_text:
                verified_schools.append(school)
            else:
                logger.debug(f"学校 '{school}' 在原文中未找到，已过滤")
        if verified_schools:
            resume["school_name_tks"] = verified_schools
            if resume.get("first_school_name_tks"):
                # 确保 first_school 也在验证列表中
                if resume["first_school_name_tks"] not in verified_schools:
                    resume["first_school_name_tks"] = verified_schools[-1]

    # 验证职位名称
    if resume.get("position_name_tks") and norm_full_text:
        verified_positions = []
        for pos in resume["position_name_tks"]:
            norm_pos = _normalize_for_comparison(pos)
            if norm_pos and norm_pos in norm_full_text:
                verified_positions.append(pos)
        if verified_positions:
            resume["position_name_tks"] = verified_positions

    # --- 阶段2: 领域规范化 ---
    # 标准化日期格式
    if resume.get("birth_dt"):
        resume["birth_dt"] = re.sub(r"[年月]", "-", str(resume["birth_dt"])).rstrip("-")

    # 清理电话号码中的非数字字符（保留 + 号）
    if resume.get("phone_kwd"):
        phone = re.sub(r"[^\d+]", "", str(resume["phone_kwd"]))
        if phone:
            resume["phone_kwd"] = phone

    # 标准化性别
    if resume.get("gender_kwd"):
        gender = str(resume["gender_kwd"]).strip()
        if gender in ("male", "Male", "M", "m"):
            resume["gender_kwd"] = "男"
        elif gender in ("female", "Female", "F", "f"):
            resume["gender_kwd"] = "女"

    # --- 阶段3: 上下文去重 ---
    for list_field in ["corp_nm_tks", "school_name_tks", "major_tks",
                       "position_name_tks", "skill_tks"]:
        if isinstance(resume.get(list_field), list):
            # 保持顺序的去重
            seen = set()
            deduped = []
            for item in resume[list_field]:
                item_str = str(item).strip()
                if item_str and item_str not in seen:
                    seen.add(item_str)
                    deduped.append(item_str)
            resume[list_field] = deduped

    # --- 阶段4: 字段补全 ---
    required_fields = [
        "name_kwd", "gender_kwd", "phone_kwd", "email_tks",
        "position_name_tks", "school_name_tks", "major_tks",
    ]
    for field in required_fields:
        if field not in resume:
            if field.endswith("_tks"):
                resume[field] = []
            elif field.endswith("_int") or field.endswith("_flt"):
                resume[field] = 0
            else:
                resume[field] = ""

    # 清理内部标记字段
    resume.pop("_name_confidence", None)

    return resume


# ==================== Pipeline 编排与 Chunk 构建 ====================


def parse_resume(filename: str, binary: bytes,tenant_id, lang: str = "Chinese") -> tuple[dict, list[str], list[dict]]:
    """
    简历解析 Pipeline 编排函数

    执行流程:
        1. 文本提取（双路径融合 + 版面重建 + 行号索引）
        2. 并行 LLM 结构化提取（三路子任务）
        3. 正则降级解析（LLM 失败时）
        4. 四阶段后处理

    参数:
        filename: 文件名
        binary: 文件二进制内容
        lang: 语言，默认 "Chinese"
    返回:
        (resume, lines, line_positions) 三元组:
        - resume: 结构化的简历信息字典
        - lines: 原始行文本列表（用于 chunk 文本匹配定位）
        - line_positions: 每行坐标信息列表（用于写入 chunk 的 position_int 等字段）
    """
    # 第一阶段: 文本提取
    indexed_text, lines, line_positions = extract_text(filename, binary)
    if not indexed_text or not lines:
        logger.warning(f"文本提取为空: {filename}")
        return {"name_kwd": "未知"}, [], []

    # 第二阶段: 并行 LLM 结构化提取
    resume = parse_with_llm(indexed_text, lines, tenant_id , lang)

    # 第三阶段: LLM 失败时降级到正则解析
    if not resume:
        logger.info(f"LLM 解析失败，降级到正则解析: {filename}")
        plain_text = "\n".join(lines)
        resume = parse_with_regex(plain_text)

    # 第四阶段: 后处理流水线
    resume = _postprocess_resume(resume, lines)

    return resume, lines, line_positions


def _build_chunk_document(filename: str, resume: dict,
                          lines: list[str] | None = None,
                          line_positions: list[dict] | None = None) -> list[dict]:
    """
    将结构化简历信息构建为文档块列表

    每个字段生成一个独立的 chunk，包含分词结果和元数据。
    与 task_executor.py 中的 build_chunks 流程兼容。

    关键设计：每个 chunk 都冗余写入关键身份标识字段（姓名、电话、邮箱等），
    这样检索命中任意 chunk 后，可以立刻识别候选人身份，
    同时通过 doc_id 可以拉取同一简历的所有 chunk 获取完整信息。

    参数:
        filename: 文件名
        resume: 结构化的简历信息字典
        lines: 原始行文本列表（来自 extract_text），用于将 chunk 文本匹配回原文行
        line_positions: 每行坐标信息列表（来自 extract_text），
            格式为 [{"page": int, "x0": float, "x1": float, "top": float, "bottom": float}, ...]
    返回:
        文档块列表，每个块包含 content_with_weight, content_ltks,
        position_int, page_num_int, top_int 等字段
    """
    chunks = []
    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)),
    }
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])

    # 提取关键身份标识字段，冗余写入每个 chunk
    # 这些字段体积小但信息密度高，检索命中后可立刻识别候选人
    _IDENTITY_FIELDS = ("name_kwd", "phone_kwd", "email_tks", "gender_kwd",
                        "highest_degree_kwd", "work_exp_flt", "corporation_name_tks")
    identity_meta = {}
    for ik in _IDENTITY_FIELDS:
        iv = resume.get(ik)
        if not iv:
            continue
        if ik.endswith("_tks"):
            identity_meta[ik] = rag_tokenizer.tokenize(
                " ".join(iv) if isinstance(iv, list) else str(iv)
            )
        elif ik.endswith("_kwd"):
            identity_meta[ik] = iv if isinstance(iv, list) else str(iv)
        elif ik.endswith("_flt"):
            try:
                identity_meta[ik] = float(iv)
            except (ValueError, TypeError):
                pass
        else:
            identity_meta[ik] = str(iv)

    # 构建简历摘要文本，附加到每个 chunk 的 content 末尾，提升语义检索召回率
    summary_parts = []
    if resume.get("name_kwd"):
        summary_parts.append(f"姓名:{resume['name_kwd']}")
    if resume.get("phone_kwd"):
        summary_parts.append(f"电话:{resume['phone_kwd']}")
    if resume.get("corporation_name_tks"):
        corp = resume["corporation_name_tks"]
        summary_parts.append(f"公司:{corp if isinstance(corp, str) else ' '.join(corp)}")
    if resume.get("highest_degree_kwd"):
        summary_parts.append(f"学历:{resume['highest_degree_kwd']}")
    if resume.get("work_exp_flt"):
        summary_parts.append(f"经验:{resume['work_exp_flt']}年")
    resume_summary = " | ".join(summary_parts) if summary_parts else ""

    # 需要按元素拆分的列表字段（每段经历/项目单独生成 chunk，避免合并后过大）
    _SPLIT_LIST_FIELDS = {"work_desc_tks", "project_desc_tks"}

    # 基本信息字段集合：这些字段应合并为一个 chunk，避免姓名、手机、邮箱等被拆散
    _BASIC_INFO_FIELDS = {
        "name_kwd", "name_pinyin_kwd", "gender_kwd", "age_int",
        "phone_kwd", "email_tks", "birth_dt", "work_exp_flt",
        "position_name_tks", "expect_city_names_tks",
        "expect_position_name_tks",
    }

    # 教育背景字段集合：学历、学校、专业、标签等应合并为一个 chunk
    _EDUCATION_FIELDS = {
        "first_school_name_tks", "first_degree_kwd", "highest_degree_kwd",
        "first_major_tks", "edu_first_fea_kwd", "degree_kwd", "major_tks",
        "school_name_tks", "sch_rank_kwd", "edu_fea_kwd", "edu_end_int",
    }

    # 技能与证书字段集合：技能、语言、证书信息量小，合并为一个 chunk
    _SKILL_CERT_FIELDS = {
        "skill_tks", "language_tks", "certificate_tks",
    }

    # 工作概况字段集合：就职公司列表、行业、最近公司合并为一个 chunk
    _WORK_OVERVIEW_FIELDS = {
        "corporation_name_tks", "corp_nm_tks", "industry_name_tks",
    }

    # 所有需要合并的字段分组，(字段集合, chunk标题) 的列表
    _MERGE_GROUPS = [
        (_BASIC_INFO_FIELDS, "基本信息"),
        (_EDUCATION_FIELDS, "教育背景"),
        (_SKILL_CERT_FIELDS, "技能与证书"),
        (_WORK_OVERVIEW_FIELDS, "工作概况"),
    ]

    # 收集所有需要合并处理的字段，后续遍历时跳过
    _ALL_MERGED_FIELDS = set()
    for fields_set, _ in _MERGE_GROUPS:
        _ALL_MERGED_FIELDS.update(fields_set)

    # 按分组合并字段，每组生成一个 chunk
    for fields_set, group_title in _MERGE_GROUPS:
        group_parts = []
        group_field_values = {}  # 保存各字段的结构化值，用于写入 chunk
        for field_key in FIELD_MAP:
            if field_key not in fields_set:
                continue
            value = resume.get(field_key)
            if not value:
                continue
            field_desc = FIELD_MAP[field_key]
            if isinstance(value, list):
                text_value = " ".join(str(v) for v in value if v)
            else:
                text_value = str(value)
            if not text_value.strip():
                continue
            group_parts.append(f"{field_desc}: {text_value}")
            group_field_values[field_key] = value

        if not group_parts:
            continue

        content = f"{group_title}\n" + "\n".join(group_parts)
        if resume_summary:
            content += f"\n[{resume_summary}]"
        chunk = {
            "content_with_weight": content,
            "content_ltks": rag_tokenizer.tokenize(content),
            "content_sm_ltks": rag_tokenizer.fine_grained_tokenize(
                rag_tokenizer.tokenize(content)
            ),
        }
        chunk.update(doc)
        # 冗余写入身份标识字段
        for mk, mv in identity_meta.items():
            chunk[mk] = mv
        # 将各字段的结构化值写入 chunk（用于结构化检索）
        for fk, fv in group_field_values.items():
            if fk.endswith("_tks"):
                text_val = " ".join(str(v) for v in fv) if isinstance(fv, list) else str(fv)
                chunk[fk] = rag_tokenizer.tokenize(text_val)
            elif fk.endswith("_kwd"):
                chunk[fk] = fv if isinstance(fv, list) else str(fv)
            elif fk.endswith("_int"):
                try:
                    chunk[fk] = int(fv)
                except (ValueError, TypeError):
                    pass
            elif fk.endswith("_flt"):
                try:
                    chunk[fk] = float(fv)
                except (ValueError, TypeError):
                    pass
            else:
                chunk[fk] = str(fv)
        chunks.append(chunk)

    # 遍历字段映射表，为每个有值的非合并字段生成 chunk
    for field_key, field_desc in FIELD_MAP.items():
        # 已在分组中合并处理的字段，跳过
        if field_key in _ALL_MERGED_FIELDS:
            continue
        value = resume.get(field_key)
        if not value:
            continue

        # 对于工作描述/项目描述等长文本列表，按元素拆分为多个 chunk
        if field_key in _SPLIT_LIST_FIELDS and isinstance(value, list):
            # 获取公司名列表，用于给每段工作描述添加上下文
            corp_list = resume.get("corp_nm_tks", []) if field_key == "work_desc_tks" else []
            project_list = resume.get("project_tks", []) if field_key == "project_desc_tks" else []
            # 获取每段工作经历的详细信息（时间、年限）
            work_details = resume.get("_work_exp_details", []) if field_key == "work_desc_tks" else []

            for idx, item in enumerate(value):
                item_text = str(item).strip()
                if not item_text:
                    continue

                # 为每段描述添加公司/项目名称前缀，提供上下文
                if field_key == "work_desc_tks" and idx < len(work_details):
                    # 使用详细信息构建前缀，包含公司、时间范围、年限
                    detail = work_details[idx]
                    company = detail.get("company", "")
                    start_d = detail.get("start_date", "")
                    end_d = detail.get("end_date", "")
                    years = detail.get("years", 0)
                    # 构建时间范围文本
                    time_parts = []
                    if start_d:
                        time_range = f"{start_d}-{end_d}" if end_d else str(start_d)
                        time_parts.append(time_range)
                    if years > 0:
                        time_parts.append(f"{years}年")
                    time_text = " ".join(time_parts)
                    if company and time_text:
                        content_prefix = f"{field_desc}（{company} {time_text}）"
                    elif company:
                        content_prefix = f"{field_desc}（{company}）"
                    else:
                        content_prefix = f"{field_desc}（第{idx + 1}段）"
                elif field_key == "work_desc_tks" and idx < len(corp_list):
                    content_prefix = f"{field_desc}（{corp_list[idx]}）"
                elif field_key == "project_desc_tks" and idx < len(project_list):
                    content_prefix = f"{field_desc}（{project_list[idx]}）"
                else:
                    content_prefix = f"{field_desc}（第{idx + 1}段）"

                if resume_summary:
                    content = f"{content_prefix}: {item_text}\n[{resume_summary}]"
                else:
                    content = f"{content_prefix}: {item_text}"

                chunk = {
                    "content_with_weight": content,
                    "content_ltks": rag_tokenizer.tokenize(content),
                    "content_sm_ltks": rag_tokenizer.fine_grained_tokenize(
                        rag_tokenizer.tokenize(content)
                    ),
                }
                chunk.update(doc)

                # 冗余写入身份标识字段
                for mk, mv in identity_meta.items():
                    if mk != field_key:
                        chunk[mk] = mv

                # 当前段的分词结果
                chunk[field_key] = rag_tokenizer.tokenize(item_text)
                chunks.append(chunk)
            continue

        # 将列表值合并为文本
        if isinstance(value, list):
            text_value = " ".join(str(v) for v in value if v)
        else:
            text_value = str(value)

        if not text_value.strip():
            continue

        # 构建 chunk 内容: "字段描述: 字段值"，并附加摘要便于语义关联
        if resume_summary and field_key not in ("name_kwd", "phone_kwd"):
            content = f"{field_desc}: {text_value}\n[{resume_summary}]"
        else:
            content = f"{field_desc}: {text_value}"

        chunk = {
            "content_with_weight": content,
            "content_ltks": rag_tokenizer.tokenize(content),
            "content_sm_ltks": rag_tokenizer.fine_grained_tokenize(
                rag_tokenizer.tokenize(content)
            ),
        }
        chunk.update(doc)

        # 冗余写入身份标识字段（不覆盖当前字段自身的值）
        for mk, mv in identity_meta.items():
            if mk != field_key:
                chunk[mk] = mv

        # 将简历字段值写入 chunk 的对应字段（用于结构化检索）
        if field_key.endswith("_tks"):
            chunk[field_key] = rag_tokenizer.tokenize(text_value)
        elif field_key.endswith("_kwd"):
            if isinstance(value, list):
                chunk[field_key] = value
            else:
                chunk[field_key] = text_value
        elif field_key.endswith("_int"):
            try:
                chunk[field_key] = int(value)
            except (ValueError, TypeError):
                pass
        elif field_key.endswith("_flt"):
            try:
                chunk[field_key] = float(value)
            except (ValueError, TypeError):
                pass
        else:
            chunk[field_key] = text_value

        chunks.append(chunk)

    # 如果没有生成任何 chunk，至少生成一个包含姓名的 chunk
    if not chunks:
        name = resume.get("name_kwd", "未知")
        content = f"姓名: {name}"
        chunk = {
            "content_with_weight": content,
            "content_ltks": rag_tokenizer.tokenize(content),
            "content_sm_ltks": rag_tokenizer.fine_grained_tokenize(
                rag_tokenizer.tokenize(content)
            ),
        }
        chunk.update(doc)
        chunks.append(chunk)

    # 为每个 chunk 写入坐标信息（position_int, page_num_int, top_int）
    #
    # 简历的 chunk 是按语义字段拆分的（基本信息、教育背景、工作描述等），
    # 而非按 PDF 物理区域拆分。字段值在 PDF 中可能分散在多处，
    # 用文本匹配反查坐标会导致排序混乱。
    #
    # 因此直接按 chunk 的生成顺序（即语义逻辑顺序）赋予递增坐标，
    # 保证展示排序为：基本信息 → 教育背景 → 技能证书 → 工作概况 → 工作描述 → 项目描述...
    #
    # add_positions 的输入格式: [(page, left, right, top, bottom), ...]
    #   - page 从 0 开始，函数内部 +1 存储
    #   - task_executor 用 page_num_int 和 top_int 做排序（先页码后 Y 坐标）
    from rag.nlp import add_positions

    for i, ck in enumerate(chunks):
        # 所有 chunk 统一放在 page=0，top 按序号递增，保证逻辑排序
        add_positions(ck, [[0, 0, 0, i, i]])

    return chunks


def chunk(filename, binary,tenant_id , from_page=0, to_page=100000,
          lang="Chinese", callback=None, **kwargs):
    """
    简历解析入口函数（与 task_executor.py 兼容）

    该函数是 FACTORY[ParserType.RESUME.value] 注册的入口，
    签名与其他解析器（如 naive.chunk）保持一致。

    参数:
        filename: 文件名
        binary: 文件二进制内容
        from_page: 起始页码（简历解析中未使用）
        to_page: 结束页码（简历解析中未使用）
        lang: 语言，默认 "Chinese"
        callback: 进度回调函数，接收 (progress, message) 参数
        **kwargs: 其他参数（parser_config, kb_id, tenant_id 等）
    返回:
        文档块列表
    """
    if callback is None:
        callback = lambda prog, msg: None

    try:
        callback(0.1, "开始解析简历...")

        # 解析简历
        resume, lines, line_positions = parse_resume(filename, binary,tenant_id, lang)
        callback(0.6, "简历结构化提取完成")

        # 构建文档块（含坐标信息）
        chunks = _build_chunk_document(filename, resume, lines, line_positions)
        callback(0.9, f"文档块构建完成，共 {len(chunks)} 个块")

        callback(1.0, "简历解析完成")
        return chunks

    except Exception as e:
        logger.exception(f"简历解析异常: {filename}")
        callback(-1, f"简历解析失败: {str(e)}")
        return []