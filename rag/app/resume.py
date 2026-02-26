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
简历解析模块 (参考 SmartResume Pipeline 架构重构)

处理流程:
    1. 文件处理: 根据文件类型(PDF/DOCX/TXT/DOC)提取原始内容
    2. 版面检测: 使用 YOLOv10 进行版面分析 (仅PDF)
    3. 文本提取: OCR + pdfplumber + docx 多路径提取
    4. LLM 结构化提取: 使用 LLMBundle 进行简历信息结构化
    5. 正则降级: LLM 失败时使用正则表达式兜底
    6. Chunk 构建: 生成标准化的文档块

兼容性:
    - chunk(filename, binary, callback, **kwargs) 签名保持不变
    - 与 task_executor.py 中 FACTORY[ParserType.RESUME.value] 兼容
"""

import json
import re
from io import BytesIO
from typing import Optional

import logging as logger
from rag.nlp import rag_tokenizer
from common.string_utils import remove_redundant_spaces
from deepdoc.parser.utils import get_text

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
}

# LLM 结构化提取的系统提示词（参考 SmartResume 的 prompt 设计）
RESUME_EXTRACTION_SYSTEM_PROMPT = """你是一个专业的简历解析助手。你的任务是从简历文本中提取结构化信息。

提取规则:
1. 严格按照 JSON 格式返回，不要有任何其他文字
2. 公司名称要完整，包括括号内的地区信息
3. 数组字段即使只有一个值也要用数组格式
4. 数值字段如果没有信息返回 0
5. 技能要尽可能详细提取，包括编程语言、框架、数据库、中间件、工具等
6. 如果某个字段在文本中找不到，字符串返回空字符串，数组返回空数组"""

RESUME_EXTRACTION_USER_PROMPT = """请从以下简历文本中提取结构化信息:

{text}

请提取以下字段:
- name_kwd: 姓名(字符串)
- gender_kwd: 性别("男"或"女")
- age_int: 年龄(整数)
- phone_kwd: 电话号码(字符串)
- email_tks: 邮箱地址(字符串)
- birth_dt: 出生日期(字符串)
- degree_kwd: 学历列表(数组)
- school_name_tks: 学校名称列表(数组)
- first_school_name_tks: 第一学历学校(字符串)
- major_tks: 专业列表(数组)
- first_major_tks: 第一学历专业(字符串)
- corp_nm_tks: 就职过的公司列表(数组，完整公司名称)
- corporation_name_tks: 最近就职的公司(字符串)
- position_name_tks: 职位名称列表(数组)
- work_exp_flt: 工作年限(浮点数)
- edu_end_int: 毕业年份(整数)
- skill_tks: 技能列表(数组)
- language_tks: 语言能力(数组)
- certificate_tks: 证书列表(数组)
- project_tks: 项目经验名称列表(数组)

只返回 JSON，不要有其他文字。"""


# ==================== 第一阶段: 文件处理与文本提取 ====================


def _extract_text_from_pdf(binary: bytes) -> str:
    """
    从 PDF 文件提取文本

    优先使用 pdfplumber 提取文本，适用于文本型 PDF。
    对于扫描件 PDF，建议配合 OCR 使用。

    参数:
        binary: PDF 文件二进制内容
    返回:
        提取的文本内容
    """
    try:
        import pdfplumber
        with pdfplumber.open(BytesIO(binary)) as pdf:
            text_parts = []
            for page in pdf.pages:
                page_text = page.extract_text() or ""
                text_parts.append(page_text)
            return "\n".join(text_parts)
    except Exception as e:
        logger.warning(f"pdfplumber 提取 PDF 文本失败: {e}")
        return ""


def _extract_text_from_pdf_with_ocr(binary: bytes) -> str:
    """
    使用 YOLOv10 版面检测 + OCR 从 PDF 提取文本

    适用于扫描件或图片型 PDF，利用项目已有的
    LayoutRecognizer4YOLOv10 和 OCR 能力。

    参数:
        binary: PDF 文件二进制内容
    返回:
        提取的文本内容
    """
    try:
        import pdfplumber
        from deepdoc.vision.ocr import OCR
        from deepdoc.vision.layout_recognizer import LayoutRecognizer4YOLOv10
        import numpy as np

        # 初始化 OCR 和版面检测
        ocr = OCR()
        layout_recognizer = LayoutRecognizer4YOLOv10("layout")

        all_text_parts = []

        with pdfplumber.open(BytesIO(binary)) as pdf:
            for page_idx, page in enumerate(pdf.pages):
                # 将页面转为图片
                img = page.to_image(resolution=216)
                page_img = np.array(img.annotated)

                # OCR 检测文本区域
                ocr_result = ocr(page_img)
                if not ocr_result:
                    continue

                # 版面检测，识别文本区域布局
                ocr_boxes = []
                for box_info in ocr_result:
                    if isinstance(box_info, (list, tuple)) and len(box_info) >= 2:
                        text = box_info[1][0] if isinstance(box_info[1], (list, tuple)) else str(box_info[1])
                        ocr_boxes.append({"text": text})

                # 按顺序拼接文本
                page_text = "\n".join([b.get("text", "") for b in ocr_boxes if b.get("text")])
                if page_text.strip():
                    all_text_parts.append(page_text)

        return "\n".join(all_text_parts)
    except Exception as e:
        logger.warning(f"OCR 提取 PDF 文本失败: {e}")
        return ""


def _extract_text_from_docx(binary: bytes) -> str:
    """
    从 DOCX 文件提取文本

    参数:
        binary: DOCX 文件二进制内容
    返回:
        提取的文本内容
    """
    try:
        from docx import Document
        doc = Document(BytesIO(binary))
        paragraphs = [p.text for p in doc.paragraphs if p.text.strip()]
        return "\n".join(paragraphs)
    except Exception as e:
        logger.warning(f"DOCX 文本提取失败: {e}")
        return ""


def extract_text(filename: str, binary: bytes) -> str:
    """
    根据文件类型提取文本内容（Pipeline 第一阶段）

    支持 PDF、DOCX、DOC、TXT 格式。
    PDF 文件会先尝试 pdfplumber 直接提取，
    如果提取文本过少则降级到 OCR 提取。

    参数:
        filename: 文件名
        binary: 文件二进制内容
    返回:
        提取的文本内容
    """
    fname_lower = filename.lower()

    try:
        if fname_lower.endswith(".pdf"):
            # 先尝试 pdfplumber 直接提取
            text = _extract_text_from_pdf(binary)
            # 如果提取文本过少（可能是扫描件），尝试 OCR
            if len(text.strip()) < 50:
                logger.info("pdfplumber 提取文本过少，尝试 OCR 提取")
                ocr_text = _extract_text_from_pdf_with_ocr(binary)
                if len(ocr_text.strip()) > len(text.strip()):
                    text = ocr_text
            return text

        elif fname_lower.endswith(".docx"):
            return _extract_text_from_docx(binary)

        elif fname_lower.endswith((".doc", ".txt")):
            return get_text(filename, binary)

        else:
            return get_text(filename, binary)

    except Exception as e:
        logger.exception(f"文本提取失败: {filename}")
        return ""


# ==================== 第二阶段: LLM 结构化提取 ====================


def _clean_llm_json_response(response: str) -> str:
    """
    清理 LLM 返回的 JSON 响应

    移除 markdown 代码块标记等干扰内容。

    参数:
        response: LLM 原始响应文本
    返回:
        清理后的 JSON 字符串
    """
    text = response.strip()
    # 移除 markdown 代码块标记
    if text.startswith("```"):
        lines = text.split("\n")
        # 去掉首尾的 ``` 行
        if len(lines) > 2:
            text = "\n".join(lines[1:-1])
        text = text.replace("```json", "").replace("```", "").strip()
    return text


def parse_with_llm(text: str,tenant_id, lang="chinese") -> Optional[dict]:
    """
    使用 LLM 解析简历文本（Pipeline 第二阶段）

    参考 SmartResume 的 prompt 设计，使用项目的 LLMBundle
    进行结构化信息提取。

    参数:
        text: 简历文本内容
    返回:
        结构化的简历信息字典，失败返回 None
    """
    try:
        from api.db.services.llm_service import LLMBundle
        from common.constants import LLMType

        llm = LLMBundle(tenant_id, LLMType.IMAGE2TEXT, lang=lang)

        # 构造用户提示词，限制文本长度避免超出 token 限制
        user_prompt = RESUME_EXTRACTION_USER_PROMPT.format(text=text[:4000])

        # 调用 LLM
        response = llm.chat(
            system=RESUME_EXTRACTION_SYSTEM_PROMPT,
            history=[{"role": "user", "content": user_prompt}],
            gen_conf={"temperature": 0.1, "max_tokens": 2048}
        )

        # 解析 JSON 响应
        cleaned = _clean_llm_json_response(response)
        resume = json.loads(cleaned)

        # 确保姓名字段存在
        if not resume.get("name_kwd"):
            resume["name_kwd"] = "未知"

        logger.info(f"LLM 成功解析简历，提取了 {len(resume)} 个字段")
        return resume

    except json.JSONDecodeError as e:
        logger.warning(f"LLM 返回的 JSON 解析失败: {e}")
        return None
    except Exception as e:
        logger.warning(f"LLM 解析简历失败: {e}")
        return None


# ==================== 第三阶段: 正则降级解析 ====================


def parse_with_regex(text: str) -> dict:
    """
    使用正则表达式解析简历文本（降级方案）

    当 LLM 解析失败时，使用正则表达式从文本中
    提取基本的结构化信息。

    参数:
        text: 简历文本内容
    返回:
        结构化的简历信息字典
    """
    resume: dict = {}
    lines = [line.strip() for line in text.split("\n") if line.strip()]

    # --- 提取姓名 ---
    for line in lines[:20]:
        # 清理行首行尾的英文字符
        cleaned = re.sub(r"^[A-Za-z_\-\d\s]+\s+", "", line)
        cleaned = re.sub(r"\s+[A-Za-z_\-\d\s]+$", "", cleaned).strip()
        # 姓名特征: 2-4 个中文字符
        if 2 <= len(cleaned) <= 4 and re.match(r"^[\u4e00-\u9fa5]{2,4}$", cleaned):
            resume["name_kwd"] = cleaned
            break

    if "name_kwd" not in resume and lines:
        first = lines[0]
        if len(first) <= 10 and not re.search(r"\d", first):
            resume["name_kwd"] = first

    # --- 提取电话号码 ---
    phones = re.findall(r"1[3-9]\d{9}", text)
    if phones:
        resume["phone_kwd"] = phones[0]

    # --- 提取邮箱 ---
    emails = re.findall(r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}", text)
    if emails:
        resume["email_tks"] = emails[0]

    # --- 提取性别 ---
    gender_match = re.search(r"(男|女)", text)
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

    # 去重并过滤
    unique_companies = []
    seen = set()
    filter_verbs = ["完成", "进行", "实施", "负责", "参与", "开发"]
    for c in companies:
        if len(c) < 6 or any(v in c for v in filter_verbs) or c in seen:
            continue
        # 保留更长的公司名
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

    # --- 提取职位 ---
    position_keywords = ["经理", "总监", "主管", "专员", "工程师", "开发", "设计师", "顾问", "助理"]
    positions = []
    for kw in position_keywords:
        matches = re.findall(rf"[\u4e00-\u9fa5]{{0,10}}{kw}", text)
        positions.extend(matches)
    if positions:
        resume["position_name_tks"] = list(set(positions))

    # --- 提取工作年限 ---
    work_exp_match = re.search(r"(\d+)\s*年.*?经验", text)
    if work_exp_match:
        resume["work_exp_flt"] = float(work_exp_match.group(1))

    # --- 提取毕业年份 ---
    grad_match = re.search(r"((?:19|20)\d{2})\s*年.*?毕业", text)
    if grad_match:
        resume["edu_end_int"] = int(grad_match.group(1))

    # 默认姓名
    if "name_kwd" not in resume:
        resume["name_kwd"] = "未知"

    return resume


# ==================== 第四阶段: Pipeline 编排 ====================


def _ensure_required_fields(resume: dict) -> dict:
    """
    确保简历字典包含所有必需字段

    对缺失的字段填充默认值，保证下游 chunk 构建不会出错。

    参数:
        resume: 原始简历字典
    返回:
        补充完整的简历字典
    """
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
    return resume


def parse_resume(filename: str, binary: bytes, tenant_id, lang: str="chinese") -> dict:
    """
    简历解析 Pipeline（核心编排函数）

    流程: 文本提取 → LLM 结构化 → 正则降级 → 字段补全

    参数:
        filename: 文件名
        binary: 文件二进制内容
    返回:
        结构化的简历信息字典
    """
    # 第一阶段: 文本提取
    text = extract_text(filename, binary)
    if not text or len(text.strip()) < 10:
        logger.warning(f"无法从文件中提取有效文本: {filename}")
        return {}

    # 第二阶段: LLM 结构化提取
    resume = parse_with_llm(text,tenant_id, lang)

    # 第三阶段: LLM 失败时降级到正则
    if resume is None:
        logger.info("LLM 解析失败，降级到正则表达式解析")
        resume = parse_with_regex(text)

    # 第四阶段: 补全必需字段
    resume = _ensure_required_fields(resume)

    return resume


# ==================== 第五阶段: Chunk 构建 ====================


def _build_chunk_document(filename: str, resume: dict) -> dict:
    """
    将结构化简历信息构建为标准文档块

    参数:
        filename: 文件名
        resume: 结构化的简历信息字典
    返回:
        标准化的文档块字典
    """
    # 构建标题: 姓名-性别-职位-年龄-简历
    titles = []
    for n in ["name_kwd", "gender_kwd", "position_name_tks", "age_int"]:
        v = resume.get(n, "")
        if isinstance(v, list):
            v = v[0] if v else ""
        if "tks" in n:
            v = remove_redundant_spaces(str(v))
        titles.append(str(v))

    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize("-".join(titles) + "-简历"),
    }
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])

    # 构建内容: 字段名-值 对
    pairs = []
    for field_name, field_desc in FIELD_MAP.items():
        v = resume.get(field_name)
        if not v:
            continue
        if isinstance(v, list):
            v = " ".join(str(item) for item in v)
        if "tks" in field_name:
            v = remove_redundant_spaces(str(v))
        # 移除描述中的括号注释
        clean_desc = re.sub(r"（[^（）]+）", "", field_desc)
        pairs.append((clean_desc, str(v)))

    doc["content_with_weight"] = "\n".join(
        [f"{k}: {v}" for k, v in pairs]
    )
    doc["content_ltks"] = rag_tokenizer.tokenize(doc["content_with_weight"])
    doc["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(doc["content_ltks"])

    # 将简历字段写入文档块
    for field_name in FIELD_MAP:
        if field_name not in resume:
            continue
        val = resume[field_name]
        # 列表字段: 非禁止选择字段且长度为1时取第一个元素
        if isinstance(val, list):
            if len(val) == 1 or field_name not in FORBIDDEN_SELECT_FIELDS:
                if len(val) > 0:
                    val = val[0]
        # 对字符串类型的 _tks 字段进行细粒度分词
        if "_tks" in field_name and isinstance(val, str):
            val = rag_tokenizer.fine_grained_tokenize(val)
        doc[field_name] = val

    return doc


# ==================== 对外接口 ====================

# 保留旧接口的兼容性别名
forbidden_select_fields4resume = FORBIDDEN_SELECT_FIELDS


def chunk(filename: str, binary: bytes = None, tenant_id ,lang="Chinese", callback=None, **kwargs) -> list[dict]:
    """
    简历解析入口函数（与 task_executor.py 兼容）

    支持 PDF、DOCX、DOC、TXT 格式。

    参数:
        filename: 文件名
        binary: 文件二进制内容
        callback: 进度回调函数 callback(progress, message)
        **kwargs: 额外参数（保留兼容性）
    返回:
        文档块列表
    异常:
        NotImplementedError: 不支持的文件格式
        Exception: 简历解析失败
    """
    if not re.search(r"\.(pdf|doc|docx|txt)$", filename, flags=re.IGNORECASE):
        raise NotImplementedError("file type not supported yet(pdf supported)")

    if not binary:
        with open(filename, "rb") as f:
            binary = f.read()

    callback(0.2, "Resume parsing is going on...")

    # 执行简历解析 Pipeline
    resume = parse_resume(filename, binary, tenant_id, lang)

    if len(resume.keys()) < 7:
        callback(-1, "Resume is not successfully parsed.")
        raise Exception("Resume parser remote call fail!")

    callback(0.6, "Done parsing. Chunking...")
    logger.debug(f"简历解析结果: {json.dumps(resume, ensure_ascii=False, indent=2)}")

    # 构建文档块
    doc = _build_chunk_document(filename, resume)

    logger.debug(f"简历 chunk 结果: {doc}")
    return [doc]
