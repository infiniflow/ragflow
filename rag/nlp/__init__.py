#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

import logging
import random
from collections import Counter, defaultdict

from common.token_utils import num_tokens_from_string
import re
import copy
import roman_numbers as r
from word2number import w2n
from cn2an import cn2an
from PIL import Image

import chardet

__all__ = ['rag_tokenizer']

all_codecs = [
    'utf-8', 'gb2312', 'gbk', 'utf_16', 'ascii', 'big5', 'big5hkscs',
    'cp037', 'cp273', 'cp424', 'cp437',
    'cp500', 'cp720', 'cp737', 'cp775', 'cp850', 'cp852', 'cp855', 'cp856', 'cp857',
    'cp858', 'cp860', 'cp861', 'cp862', 'cp863', 'cp864', 'cp865', 'cp866', 'cp869',
    'cp874', 'cp875', 'cp932', 'cp949', 'cp950', 'cp1006', 'cp1026', 'cp1125',
    'cp1140', 'cp1250', 'cp1251', 'cp1252', 'cp1253', 'cp1254', 'cp1255', 'cp1256',
    'cp1257', 'cp1258', 'euc_jp', 'euc_jis_2004', 'euc_jisx0213', 'euc_kr',
    'gb18030', 'hz', 'iso2022_jp', 'iso2022_jp_1', 'iso2022_jp_2',
    'iso2022_jp_2004', 'iso2022_jp_3', 'iso2022_jp_ext', 'iso2022_kr', 'latin_1',
    'iso8859_2', 'iso8859_3', 'iso8859_4', 'iso8859_5', 'iso8859_6', 'iso8859_7',
    'iso8859_8', 'iso8859_9', 'iso8859_10', 'iso8859_11', 'iso8859_13',
    'iso8859_14', 'iso8859_15', 'iso8859_16', 'johab', 'koi8_r', 'koi8_t', 'koi8_u',
    'kz1048', 'mac_cyrillic', 'mac_greek', 'mac_iceland', 'mac_latin2', 'mac_roman',
    'mac_turkish', 'ptcp154', 'shift_jis', 'shift_jis_2004', 'shift_jisx0213',
    'utf_32', 'utf_32_be', 'utf_32_le', 'utf_16_be', 'utf_16_le', 'utf_7', 'windows-1250', 'windows-1251',
    'windows-1252', 'windows-1253', 'windows-1254', 'windows-1255', 'windows-1256',
    'windows-1257', 'windows-1258', 'latin-2'
]


def find_codec(blob):
    detected = chardet.detect(blob[:1024])
    if detected['confidence'] > 0.5:
        if detected['encoding'] == "ascii":
            return "utf-8"

    for c in all_codecs:
        try:
            blob[:1024].decode(c)
            return c
        except Exception:
            pass
        try:
            blob.decode(c)
            return c
        except Exception:
            pass

    return "utf-8"


QUESTION_PATTERN = [
    r"第([零一二三四五六七八九十百0-9]+)问",
    r"第([零一二三四五六七八九十百0-9]+)条",
    r"[\(（]([零一二三四五六七八九十百]+)[\)）]",
    r"第([0-9]+)问",
    r"第([0-9]+)条",
    r"([0-9]{1,2})[\. 、]",
    r"([零一二三四五六七八九十百]+)[ 、]",
    r"[\(（]([0-9]{1,2})[\)）]",
    r"QUESTION (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)",
    r"QUESTION (I+V?|VI*|XI|IX|X)",
    r"QUESTION ([0-9]+)",
]


def has_qbullet(reg, box, last_box, last_index, last_bull, bull_x0_list):
    section, last_section = box['text'], last_box['text']
    q_reg = r'(\w|\W)*?(?:？|\?|\n|$)+'
    full_reg = reg + q_reg
    has_bull = re.match(full_reg, section)
    index_str = None
    if has_bull:
        if 'x0' not in last_box:
            last_box['x0'] = box['x0']
        if 'top' not in last_box:
            last_box['top'] = box['top']
        if last_bull and box['x0'] - last_box['x0'] > 10:
            return None, last_index
        if not last_bull and box['x0'] >= last_box['x0'] and box['top'] - last_box['top'] < 20:
            return None, last_index
        avg_bull_x0 = 0
        if bull_x0_list:
            avg_bull_x0 = sum(bull_x0_list) / len(bull_x0_list)
        else:
            avg_bull_x0 = box['x0']
        if box['x0'] - avg_bull_x0 > 10:
            return None, last_index
        index_str = has_bull.group(1)
        index = index_int(index_str)
        if last_section[-1] == ':' or last_section[-1] == '：':
            return None, last_index
        if not last_index or index >= last_index:
            bull_x0_list.append(box['x0'])
            return has_bull, index
        if section[-1] == '?' or section[-1] == '？':
            bull_x0_list.append(box['x0'])
            return has_bull, index
        if box['layout_type'] == 'title':
            bull_x0_list.append(box['x0'])
            return has_bull, index
        pure_section = section.lstrip(re.match(reg, section).group()).lower()
        ask_reg = r'(what|when|where|how|why|which|who|whose|为什么|为啥|哪)'
        if re.match(ask_reg, pure_section):
            bull_x0_list.append(box['x0'])
            return has_bull, index
    return None, last_index


def index_int(index_str):
    res = -1
    try:
        res = int(index_str)
    except ValueError:
        try:
            res = w2n.word_to_num(index_str)
        except ValueError:
            try:
                res = cn2an(index_str)
            except ValueError:
                try:
                    res = r.number(index_str)
                except ValueError:
                    return -1
    return res


def qbullets_category(sections):
    global QUESTION_PATTERN
    hits = [0] * len(QUESTION_PATTERN)
    for i, pro in enumerate(QUESTION_PATTERN):
        for sec in sections:
            if re.match(pro, sec) and not not_bullet(sec):
                hits[i] += 1
                break
    maximum = 0
    res = -1
    for i, h in enumerate(hits):
        if h <= maximum:
            continue
        res = i
        maximum = h
    return res, QUESTION_PATTERN[res]


BULLET_PATTERN = [[
    r"第[零一二三四五六七八九十百0-9]+(分?编|部分)",
    r"第[零一二三四五六七八九十百0-9]+章",
    r"第[零一二三四五六七八九十百0-9]+节",
    r"第[零一二三四五六七八九十百0-9]+条",
    r"[\(（][零一二三四五六七八九十百]+[\)）]",
], [
    r"第[0-9]+章",
    r"第[0-9]+节",
    r"[0-9]{,2}[\. 、]",
    r"[0-9]{,2}\.[0-9]{,2}[^a-zA-Z/%~-]",
    r"[0-9]{,2}\.[0-9]{,2}\.[0-9]{,2}",
    r"[0-9]{,2}\.[0-9]{,2}\.[0-9]{,2}\.[0-9]{,2}",
], [
    r"第[零一二三四五六七八九十百0-9]+章",
    r"第[零一二三四五六七八九十百0-9]+节",
    r"[零一二三四五六七八九十百]+[ 、]",
    r"[\(（][零一二三四五六七八九十百]+[\)）]",
    r"[\(（][0-9]{,2}[\)）]",
], [
    r"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)",
    r"Chapter (I+V?|VI*|XI|IX|X)",
    r"Section [0-9]+",
    r"Article [0-9]+"
], [
    r"^#[^#]",
    r"^##[^#]",
    r"^###.*",
    r"^####.*",
    r"^#####.*",
    r"^######.*",
]
]


def random_choices(arr, k):
    k = min(len(arr), k)
    return random.choices(arr, k=k)


def not_bullet(line):
    patt = [
        r"0", r"[0-9]+ +[0-9~个只-]", r"[0-9]+\.{2,}"
    ]
    return any([re.match(r, line) for r in patt])


def bullets_category(sections):
    global BULLET_PATTERN
    hits = [0] * len(BULLET_PATTERN)
    for i, pro in enumerate(BULLET_PATTERN):
        for sec in sections:
            sec = sec.strip()
            for p in pro:
                if re.match(p, sec) and not not_bullet(sec):
                    hits[i] += 1
                    break
    maximum = 0
    res = -1
    for i, h in enumerate(hits):
        if h <= maximum:
            continue
        res = i
        maximum = h
    return res


def is_english(texts):
    if not texts:
        return False

    pattern = re.compile(r"[`a-zA-Z0-9\s.,':;/\"?<>!\(\)\-]")

    if isinstance(texts, str):
        texts = list(texts)
    elif isinstance(texts, list):
        texts = [t for t in texts if isinstance(t, str) and t.strip()]
    else:
        return False

    if not texts:
        return False

    eng = sum(1 for t in texts if pattern.fullmatch(t.strip()))
    return (eng / len(texts)) > 0.8


def is_chinese(text):
    if not text:
        return False
    chinese = 0
    for ch in text:
        if '\u4e00' <= ch <= '\u9fff':
            chinese += 1
    if chinese / len(text) > 0.2:
        return True
    return False


def tokenize(d, txt, eng):
    from . import rag_tokenizer
    d["content_with_weight"] = txt
    t = re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", txt)
    d["content_ltks"] = rag_tokenizer.tokenize(t)
    d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])


def split_with_pattern(d, pattern: str, content: str, eng) -> list:
    docs = []

    # Validate and compile regex pattern before use
    try:
        compiled_pattern = re.compile(r"(%s)" % pattern, flags=re.DOTALL)
    except re.error as e:
        logging.warning(f"Invalid delimiter regex pattern '{pattern}': {e}. Falling back to no split.")
        # Fallback: return content as single chunk
        dd = copy.deepcopy(d)
        tokenize(dd, content, eng)
        return [dd]

    txts = [txt for txt in compiled_pattern.split(content)]
    for j in range(0, len(txts), 2):
        txt = txts[j]
        if not txt:
            continue
        if j + 1 < len(txts):
            txt += txts[j + 1]
        dd = copy.deepcopy(d)
        tokenize(dd, txt, eng)
        docs.append(dd)
    return docs


def tokenize_chunks(chunks, doc, eng, pdf_parser=None, child_delimiters_pattern=None):
    res = []
    # wrap up as es documents
    for ii, ck in enumerate(chunks):
        if len(ck.strip()) == 0:
            continue
        logging.debug("-- {}".format(ck))
        d = copy.deepcopy(doc)
        if pdf_parser:
            try:
                d["image"], poss = pdf_parser.crop(ck, need_position=True)
                add_positions(d, poss)
                ck = pdf_parser.remove_tag(ck)
            except NotImplementedError:
                pass
        else:
            add_positions(d, [[ii] * 5])

        if child_delimiters_pattern:
            d["mom_with_weight"] = ck
            res.extend(split_with_pattern(d, child_delimiters_pattern, ck, eng))
            continue

        tokenize(d, ck, eng)
        res.append(d)
    return res


def doc_tokenize_chunks_with_images(chunks, doc, eng, child_delimiters_pattern=None, batch_size=10):
    res = []
    for ii, ck in enumerate(chunks):
        text = ck.get("context_above", "") + ck.get("text") + ck.get("context_below", "")
        if len(text.strip()) == 0:
            continue
        logging.debug("-- {}".format(ck))
        d = copy.deepcopy(doc)
        if ck.get("image"):
            d["image"] = ck.get("image")
        add_positions(d, [[ii] * 5])

        if ck.get("ck_type") == "text":
            if child_delimiters_pattern:
                d["mom_with_weight"] = text
                res.extend(split_with_pattern(d, child_delimiters_pattern, text, eng))
                continue
        elif ck.get("ck_type") == "image":
            d["doc_type_kwd"] = "image"
        elif ck.get("ck_type") == "table":
            d["doc_type_kwd"] = "table"
        tokenize(d, text, eng)
        res.append(d)
    return res


def tokenize_chunks_with_images(chunks, doc, eng, images, child_delimiters_pattern=None):
    res = []
    # wrap up as es documents
    for ii, (ck, image) in enumerate(zip(chunks, images)):
        if len(ck.strip()) == 0:
            continue
        logging.debug("-- {}".format(ck))
        d = copy.deepcopy(doc)
        d["image"] = image
        add_positions(d, [[ii] * 5])
        if child_delimiters_pattern:
            d["mom_with_weight"] = ck
            res.extend(split_with_pattern(d, child_delimiters_pattern, ck, eng))
            continue
        tokenize(d, ck, eng)
        res.append(d)
    return res


def tokenize_table(tbls, doc, eng, batch_size=10):
    res = []
    # add tables
    for (img, rows), poss in tbls:
        if not rows:
            continue
        if isinstance(rows, str):
            d = copy.deepcopy(doc)
            tokenize(d, rows, eng)
            d["content_with_weight"] = rows
            d["doc_type_kwd"] = "table"
            if img:
                d["image"] = img
                if d["content_with_weight"].find("<tr>") < 0:
                    d["doc_type_kwd"] = "image"
            if poss:
                add_positions(d, poss)
            res.append(d)
            continue
        de = "; " if eng else "； "
        for i in range(0, len(rows), batch_size):
            d = copy.deepcopy(doc)
            r = de.join(rows[i:i + batch_size])
            tokenize(d, r, eng)
            d["doc_type_kwd"] = "table"
            if img:
                d["image"] = img
                if d["content_with_weight"].find("<tr>") < 0:
                    d["doc_type_kwd"] = "image"
            add_positions(d, poss)
            res.append(d)
    return res


def attach_media_context(chunks, table_context_size=0, image_context_size=0):
    """
    Attach surrounding text chunk content to media chunks (table/image).
    Best-effort ordering: if positional info exists on any chunk, use it to
    order chunks before collecting context; otherwise keep original order.
    """
    from . import rag_tokenizer

    if not chunks or (table_context_size <= 0 and image_context_size <= 0):
        return chunks

    def is_image_chunk(ck):
        if ck.get("doc_type_kwd") == "image":
            return True

        text_val = ck.get("content_with_weight") if isinstance(ck.get("content_with_weight"), str) else ck.get("text")
        has_text = isinstance(text_val, str) and text_val.strip()
        return bool(ck.get("image")) and not has_text

    def is_table_chunk(ck):
        return ck.get("doc_type_kwd") == "table"

    def is_text_chunk(ck):
        return not is_image_chunk(ck) and not is_table_chunk(ck)

    def get_text(ck):
        if isinstance(ck.get("content_with_weight"), str):
            return ck["content_with_weight"]
        if isinstance(ck.get("text"), str):
            return ck["text"]
        return ""

    def split_sentences(text):
        pattern = r"([.。！？!?；;：:\n])"
        parts = re.split(pattern, text)
        sentences = []
        buf = ""
        for p in parts:
            if not p:
                continue
            if re.fullmatch(pattern, p):
                buf += p
                sentences.append(buf)
                buf = ""
            else:
                buf += p
        if buf:
            sentences.append(buf)
        return sentences

    def get_bounds_by_page(ck):
        bounds = {}
        try:
            if ck.get("position_int"):
                for pos in ck["position_int"]:
                    if not pos or len(pos) < 5:
                        continue
                    pn, _, _, top, bottom = pos
                    if pn is None or top is None:
                        continue
                    top_val = float(top)
                    bottom_val = float(bottom) if bottom is not None else top_val
                    if bottom_val < top_val:
                        top_val, bottom_val = bottom_val, top_val
                    pn = int(pn)
                    if pn in bounds:
                        bounds[pn] = (min(bounds[pn][0], top_val), max(bounds[pn][1], bottom_val))
                    else:
                        bounds[pn] = (top_val, bottom_val)
            else:
                pn = None
                if ck.get("page_num_int"):
                    pn = ck["page_num_int"][0]
                elif ck.get("page_number") is not None:
                    pn = ck.get("page_number")
                if pn is None:
                    return bounds
                top = None
                if ck.get("top_int"):
                    top = ck["top_int"][0]
                elif ck.get("top") is not None:
                    top = ck.get("top")
                if top is None:
                    return bounds
                bottom = ck.get("bottom")
                pn = int(pn)
                top_val = float(top)
                bottom_val = float(bottom) if bottom is not None else top_val
                if bottom_val < top_val:
                    top_val, bottom_val = bottom_val, top_val
                bounds[pn] = (top_val, bottom_val)
        except Exception:
            return {}
        return bounds

    def trim_to_tokens(text, token_budget, from_tail=False):
        if token_budget <= 0 or not text:
            return ""
        sentences = split_sentences(text)
        if not sentences:
            return ""

        collected = []
        remaining = token_budget
        seq = reversed(sentences) if from_tail else sentences
        for s in seq:
            tks = num_tokens_from_string(s)
            if tks <= 0:
                continue
            if tks > remaining:
                collected.append(s)
                break
            collected.append(s)
            remaining -= tks

        if from_tail:
            collected = list(reversed(collected))
        return "".join(collected)

    def find_mid_sentence_index(sentences):
        if not sentences:
            return 0
        total = sum(max(0, num_tokens_from_string(s)) for s in sentences)
        if total <= 0:
            return max(0, len(sentences) // 2)
        target = total / 2.0
        best_idx = 0
        best_diff = None
        cum = 0
        for i, s in enumerate(sentences):
            cum += max(0, num_tokens_from_string(s))
            diff = abs(cum - target)
            if best_diff is None or diff < best_diff:
                best_diff = diff
                best_idx = i
        return best_idx

    def collect_context_from_sentences(sentences, boundary_idx, token_budget):
        prev_ctx = []
        remaining_prev = token_budget
        for s in reversed(sentences[:boundary_idx + 1]):
            if remaining_prev <= 0:
                break
            tks = num_tokens_from_string(s)
            if tks <= 0:
                continue
            if tks > remaining_prev:
                s = trim_to_tokens(s, remaining_prev, from_tail=True)
                tks = num_tokens_from_string(s)
            prev_ctx.append(s)
            remaining_prev -= tks
        prev_ctx.reverse()

        next_ctx = []
        remaining_next = token_budget
        for s in sentences[boundary_idx + 1:]:
            if remaining_next <= 0:
                break
            tks = num_tokens_from_string(s)
            if tks <= 0:
                continue
            if tks > remaining_next:
                s = trim_to_tokens(s, remaining_next, from_tail=False)
                tks = num_tokens_from_string(s)
            next_ctx.append(s)
            remaining_next -= tks
        return prev_ctx, next_ctx

    def extract_position(ck):
        pn = None
        top = None
        left = None
        try:
            if ck.get("page_num_int"):
                pn = ck["page_num_int"][0]
            elif ck.get("page_number") is not None:
                pn = ck.get("page_number")

            if ck.get("top_int"):
                top = ck["top_int"][0]
            elif ck.get("top") is not None:
                top = ck.get("top")

            if ck.get("position_int"):
                left = ck["position_int"][0][1]
            elif ck.get("x0") is not None:
                left = ck.get("x0")
        except Exception:
            pn = top = left = None
        return pn, top, left

    indexed = list(enumerate(chunks))
    positioned_indices = []
    unpositioned_indices = []
    for idx, ck in indexed:
        pn, top, left = extract_position(ck)
        if pn is not None and top is not None:
            positioned_indices.append((idx, pn, top, left if left is not None else 0))
        else:
            unpositioned_indices.append(idx)

    if positioned_indices:
        positioned_indices.sort(key=lambda x: (int(x[1]), int(x[2]), int(x[3]), x[0]))
        ordered_indices = [i for i, _, _, _ in positioned_indices] + unpositioned_indices
    else:
        ordered_indices = [idx for idx, _ in indexed]

    text_bounds = []
    for idx, ck in indexed:
        if not is_text_chunk(ck):
            continue
        bounds = get_bounds_by_page(ck)
        if bounds:
            text_bounds.append((idx, bounds))

    for sorted_pos, idx in enumerate(ordered_indices):
        ck = chunks[idx]
        token_budget = image_context_size if is_image_chunk(ck) else table_context_size if is_table_chunk(ck) else 0
        if token_budget <= 0:
            continue

        prev_ctx = []
        next_ctx = []
        media_bounds = get_bounds_by_page(ck)
        best_idx = None
        best_dist = None
        candidate_count = 0
        if media_bounds and text_bounds:
            for text_idx, bounds in text_bounds:
                for pn, (t_top, t_bottom) in bounds.items():
                    if pn not in media_bounds:
                        continue
                    m_top, m_bottom = media_bounds[pn]
                    if m_bottom < t_top or m_top > t_bottom:
                        continue
                    candidate_count += 1
                    m_mid = (m_top + m_bottom) / 2.0
                    t_mid = (t_top + t_bottom) / 2.0
                    dist = abs(m_mid - t_mid)
                    if best_dist is None or dist < best_dist:
                        best_dist = dist
                        best_idx = text_idx
        if best_idx is None and media_bounds:
            media_page = min(media_bounds.keys())
            page_order = []
            for ordered_idx in ordered_indices:
                pn, _, _ = extract_position(chunks[ordered_idx])
                if pn == media_page:
                    page_order.append(ordered_idx)
            if page_order and idx in page_order:
                pos_in_page = page_order.index(idx)
                if pos_in_page == 0:
                    for neighbor in page_order[pos_in_page + 1:]:
                        if is_text_chunk(chunks[neighbor]):
                            best_idx = neighbor
                            break
                elif pos_in_page == len(page_order) - 1:
                    for neighbor in reversed(page_order[:pos_in_page]):
                        if is_text_chunk(chunks[neighbor]):
                            best_idx = neighbor
                            break
        if best_idx is not None:
            base_text = get_text(chunks[best_idx])
            sentences = split_sentences(base_text)
            if sentences:
                boundary_idx = find_mid_sentence_index(sentences)
                prev_ctx, next_ctx = collect_context_from_sentences(sentences, boundary_idx, token_budget)

        if not prev_ctx and not next_ctx:
            continue

        self_text = get_text(ck)
        pieces = [*prev_ctx]
        if self_text:
            pieces.append(self_text)
        pieces.extend(next_ctx)
        combined = "\n".join(pieces)

        original = ck.get("content_with_weight")
        if "content_with_weight" in ck:
            ck["content_with_weight"] = combined
        elif "text" in ck:
            original = ck.get("text")
            ck["text"] = combined

        if combined != original:
            if "content_ltks" in ck:
                ck["content_ltks"] = rag_tokenizer.tokenize(combined)
            if "content_sm_ltks" in ck:
                ck["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(
                    ck.get("content_ltks", rag_tokenizer.tokenize(combined)))

    if positioned_indices:
        chunks[:] = [chunks[i] for i in ordered_indices]

    return chunks


def append_context2table_image4pdf(sections: list, tabls: list, table_context_size=0, return_context=False):
    from deepdoc.parser import PdfParser
    if table_context_size <=0:
        return [] if return_context else tabls

    page_bucket = defaultdict(list)
    for i, item in enumerate(sections):
        if isinstance(item, (tuple, list)):
            if len(item) > 2:
                txt, _sec_id, poss = item[0], item[1], item[2]
            else:
                txt = item[0] if item else ""
                poss = item[1] if len(item) > 1 else ""
        else:
            txt = item
            poss = ""
        # Normal: (text, "@@...##") from naive parser -> poss is a position tag string.
        # Manual: (text, sec_id, poss_list) -> poss is a list of (page, left, right, top, bottom).
        # Paper: (text_with_@@tag, layoutno) -> poss is layoutno; parse from txt when it contains @@ tags.
        if isinstance(poss, list):
            poss = poss
        elif isinstance(poss, str):
            if "@@" not in poss and isinstance(txt, str) and "@@" in txt:
                poss = txt
            poss = PdfParser.extract_positions(poss)
        else:
            if isinstance(txt, str) and "@@" in txt:
                poss = PdfParser.extract_positions(txt)
            else:
                poss = []
        if isinstance(txt, str) and "@@" in txt:
            txt = re.sub(r"@@[0-9-]+\t[0-9.\t]+##", "", txt).strip()
        for page, left, right, top, bottom in poss:
            if isinstance(page, list):
                page = page[0] if page else 0
            page_bucket[page].append(((left, right, top, bottom), txt))

    def upper_context(page, i):
        txt = ""
        if page not in page_bucket:
            i = -1
        while num_tokens_from_string(txt) < table_context_size:
            if i < 0:
                page -= 1
                if page < 0 or page not in page_bucket:
                    break
                i = len(page_bucket[page]) -1
            blks = page_bucket[page]
            (_, _, _, _), cnt = blks[i]
            txts = re.split(r"([。!?？；！\n]|\. )", cnt, flags=re.DOTALL)[::-1]
            for j in range(0, len(txts), 2):
                txt = (txts[j+1] if j+1<len(txts) else "") + txts[j] + txt
                if num_tokens_from_string(txt) > table_context_size:
                    break
            i -= 1
        return txt

    def lower_context(page, i):
        txt = ""
        if page not in page_bucket:
            return txt
        while num_tokens_from_string(txt) < table_context_size:
            if i >= len(page_bucket[page]):
                page += 1
                if page not in page_bucket:
                    break
                i = 0
            blks = page_bucket[page]
            (_, _, _, _), cnt = blks[i]
            txts = re.split(r"([。!?？；！\n]|\. )", cnt, flags=re.DOTALL)
            for j in range(0, len(txts), 2):
                txt += txts[j] + (txts[j+1] if j+1<len(txts) else "")
                if num_tokens_from_string(txt) > table_context_size:
                    break
            i += 1
        return txt

    res = []
    contexts = []
    for (img, tb), poss in tabls:
        page, left, right, top, bott = poss[0]
        _page, _left, _right, _top, _bott = poss[-1]
        if isinstance(tb, list):
            tb = "\n".join(tb)

        i = 0
        blks = page_bucket.get(page, [])
        _tb = tb
        while i < len(blks):
            if i + 1 >= len(blks):
                if _page > page:
                    page += 1
                    i = 0
                    blks = page_bucket.get(page, [])
                    continue
                upper = upper_context(page, i)
                lower = lower_context(page + 1, 0)
                tb = upper + tb + lower
                contexts.append((upper.strip(), lower.strip()))
                break
            (_, _, t, b), txt = blks[i]
            if b > top:
                break
            (_, _, _t, _b), _txt = blks[i+1]
            if _t < _bott:
                i += 1
                continue

            upper = upper_context(page, i)
            lower = lower_context(page, i)
            tb = upper + tb + lower
            contexts.append((upper.strip(), lower.strip()))
            break

        if _tb == tb:
            upper = upper_context(page, -1)
            lower = lower_context(page + 1, 0)
            tb = upper + tb + lower
            contexts.append((upper.strip(), lower.strip()))
        if len(contexts) < len(res) + 1:
            contexts.append(("", ""))
        res.append(((img, tb), poss))
    return contexts if return_context else res


def add_positions(d, poss):
    if not poss:
        return
    page_num_int = []
    position_int = []
    top_int = []
    for pn, left, right, top, bottom in poss:
        page_num_int.append(int(pn + 1))
        top_int.append(int(top))
        position_int.append((int(pn + 1), int(left), int(right), int(top), int(bottom)))
    d["page_num_int"] = page_num_int
    d["position_int"] = position_int
    d["top_int"] = top_int


def remove_contents_table(sections, eng=False):
    i = 0
    while i < len(sections):
        def get(i):
            nonlocal sections
            return (sections[i] if isinstance(sections[i],
                                              type("")) else sections[i][0]).strip()

        if not re.match(r"(contents|目录|目次|table of contents|致谢|acknowledge)$",
                        re.sub(r"( | |\u3000)+", "", get(i).split("@@")[0], flags=re.IGNORECASE)):
            i += 1
            continue
        sections.pop(i)
        if i >= len(sections):
            break
        prefix = get(i)[:3] if not eng else " ".join(get(i).split()[:2])
        while not prefix:
            sections.pop(i)
            if i >= len(sections):
                break
            prefix = get(i)[:3] if not eng else " ".join(get(i).split()[:2])
        sections.pop(i)
        if i >= len(sections) or not prefix:
            break
        for j in range(i, min(i + 128, len(sections))):
            if not re.match(prefix, get(j)):
                continue
            for _ in range(i, j):
                sections.pop(i)
            break


def make_colon_as_title(sections):
    if not sections:
        return []
    if isinstance(sections[0], type("")):
        return sections
    i = 0
    while i < len(sections):
        txt, layout = sections[i]
        i += 1
        txt = txt.split("@")[0].strip()
        if not txt:
            continue
        if txt[-1] not in ":：":
            continue
        txt = txt[::-1]
        arr = re.split(r"([。？！!?;；]| \.)", txt)
        if len(arr) < 2 or len(arr[1]) < 32:
            continue
        sections.insert(i - 1, (arr[0][::-1], "title"))
        i += 1


def title_frequency(bull, sections):
    bullets_size = len(BULLET_PATTERN[bull])
    levels = [bullets_size + 1 for _ in range(len(sections))]
    if not sections or bull < 0:
        return bullets_size + 1, levels

    for i, (txt, layout) in enumerate(sections):
        for j, p in enumerate(BULLET_PATTERN[bull]):
            if re.match(p, txt.strip()) and not not_bullet(txt):
                levels[i] = j
                break
        else:
            if re.search(r"(title|head)", layout) and not not_title(txt.split("@")[0]):
                levels[i] = bullets_size
    most_level = bullets_size + 1
    for level, c in sorted(Counter(levels).items(), key=lambda x: x[1] * -1):
        if level <= bullets_size:
            most_level = level
            break
    return most_level, levels


def not_title(txt):
    if re.match(r"第[零一二三四五六七八九十百0-9]+条", txt):
        return False
    if len(txt.split()) > 12 or (txt.find(" ") < 0 and len(txt) >= 32):
        return True
    return re.search(r"[,;，。；！!]", txt)


def tree_merge(bull, sections, depth):
    if not sections or bull < 0:
        return sections
    if isinstance(sections[0], type("")):
        sections = [(s, "") for s in sections]

    # filter out position information in pdf sections
    sections = [(t, o) for t, o in sections if
                t and len(t.split("@")[0].strip()) > 1 and not re.match(r"[0-9]+$", t.split("@")[0].strip())]

    def get_level(bull, section):
        text, layout = section
        text = re.sub(r"\u3000", " ", text).strip()

        for i, title in enumerate(BULLET_PATTERN[bull]):
            if re.match(title, text.strip()):
                return i + 1, text
        else:
            if re.search(r"(title|head)", layout) and not not_title(text):
                return len(BULLET_PATTERN[bull]) + 1, text
            else:
                return len(BULLET_PATTERN[bull]) + 2, text

    level_set = set()
    lines = []
    for section in sections:
        level, text = get_level(bull, section)
        if not text.strip("\n"):
            continue

        lines.append((level, text))
        level_set.add(level)

    sorted_levels = sorted(list(level_set))

    if depth <= len(sorted_levels):
        target_level = sorted_levels[depth - 1]
    else:
        target_level = sorted_levels[-1]

    if target_level == len(BULLET_PATTERN[bull]) + 2:
        target_level = sorted_levels[-2] if len(sorted_levels) > 1 else sorted_levels[0]

    root = Node(level=0, depth=target_level, texts=[])
    root.build_tree(lines)

    return [element for element in root.get_tree() if element]


def hierarchical_merge(bull, sections, depth):
    if not sections or bull < 0:
        return []
    if isinstance(sections[0], type("")):
        sections = [(s, "") for s in sections]
    sections = [(t, o) for t, o in sections if
                t and len(t.split("@")[0].strip()) > 1 and not re.match(r"[0-9]+$", t.split("@")[0].strip())]
    bullets_size = len(BULLET_PATTERN[bull])
    levels = [[] for _ in range(bullets_size + 2)]

    for i, (txt, layout) in enumerate(sections):
        for j, p in enumerate(BULLET_PATTERN[bull]):
            if re.match(p, txt.strip()):
                levels[j].append(i)
                break
        else:
            if re.search(r"(title|head)", layout) and not not_title(txt):
                levels[bullets_size].append(i)
            else:
                levels[bullets_size + 1].append(i)
    sections = [t for t, _ in sections]

    # for s in sections: print("--", s)

    def binary_search(arr, target):
        if not arr:
            return -1
        if target > arr[-1]:
            return len(arr) - 1
        if target < arr[0]:
            return -1
        s, e = 0, len(arr)
        while e - s > 1:
            i = (e + s) // 2
            if target > arr[i]:
                s = i
                continue
            elif target < arr[i]:
                e = i
                continue
            else:
                assert False
        return s

    cks = []
    readed = [False] * len(sections)
    levels = levels[::-1]
    for i, arr in enumerate(levels[:depth]):
        for j in arr:
            if readed[j]:
                continue
            readed[j] = True
            cks.append([j])
            if i + 1 == len(levels) - 1:
                continue
            for ii in range(i + 1, len(levels)):
                jj = binary_search(levels[ii], j)
                if jj < 0:
                    continue
                if levels[ii][jj] > cks[-1][-1]:
                    cks[-1].pop(-1)
                cks[-1].append(levels[ii][jj])
            for ii in cks[-1]:
                readed[ii] = True

    if not cks:
        return cks

    for i in range(len(cks)):
        cks[i] = [sections[j] for j in cks[i][::-1]]
        logging.debug("\n* ".join(cks[i]))

    res = [[]]
    num = [0]
    for ck in cks:
        if len(ck) == 1:
            n = num_tokens_from_string(re.sub(r"@@[0-9]+.*", "", ck[0]))
            if n + num[-1] < 218:
                res[-1].append(ck[0])
                num[-1] += n
                continue
            res.append(ck)
            num.append(n)
            continue
        res.append(ck)
        num.append(218)

    return res


def naive_merge(sections: str | list, chunk_token_num=128, delimiter="\n。；！？", overlapped_percent=0):
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
    if not sections:
        return []
    if isinstance(sections, str):
        sections = [sections]
    if isinstance(sections[0], str):
        sections = [(s, "") for s in sections]
    cks = [""]
    tk_nums = [0]

    def add_chunk(t, pos):
        nonlocal cks, tk_nums, delimiter
        tnum = num_tokens_from_string(t)
        if not pos:
            pos = ""
        if tnum < 8:
            pos = ""
        # Ensure that the length of the merged chunk does not exceed chunk_token_num
        if cks[-1] == "" or tk_nums[-1] > chunk_token_num * (100 - overlapped_percent) / 100.:
            if cks:
                overlapped = RAGFlowPdfParser.remove_tag(cks[-1])
                t = overlapped[int(len(overlapped) * (100 - overlapped_percent) / 100.):] + t
            if t.find(pos) < 0:
                t += pos
            cks.append(t)
            tk_nums.append(tnum)
        else:
            if cks[-1].find(pos) < 0:
                t += pos
            cks[-1] += t
            tk_nums[-1] += tnum

    custom_delimiters = [m.group(1) for m in re.finditer(r"`([^`]+)`", delimiter)]
    has_custom = bool(custom_delimiters)
    if has_custom:
        custom_pattern = "|".join(re.escape(t) for t in sorted(set(custom_delimiters), key=len, reverse=True))
        cks, tk_nums = [], []
        for sec, pos in sections:
            split_sec = re.split(r"(%s)" % custom_pattern, sec, flags=re.DOTALL)
            for sub_sec in split_sec:
                if re.fullmatch(custom_pattern, sub_sec or ""):
                    continue
                text = "\n" + sub_sec
                local_pos = pos
                if num_tokens_from_string(text) < 8:
                    local_pos = ""
                if local_pos and text.find(local_pos) < 0:
                    text += local_pos
                cks.append(text)
                tk_nums.append(num_tokens_from_string(text))
        return cks

    for sec, pos in sections:
        add_chunk("\n" + sec, pos)

    return cks


def naive_merge_with_images(texts, images, chunk_token_num=128, delimiter="\n。；！？", overlapped_percent=0):
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
    if not texts or len(texts) != len(images):
        return [], []
    cks = [""]
    result_images = [None]
    tk_nums = [0]

    def add_chunk(t, image, pos=""):
        nonlocal cks, result_images, tk_nums, delimiter
        tnum = num_tokens_from_string(t)
        if not pos:
            pos = ""
        if tnum < 8:
            pos = ""
        # Ensure that the length of the merged chunk does not exceed chunk_token_num
        if cks[-1] == "" or tk_nums[-1] > chunk_token_num * (100 - overlapped_percent) / 100.:
            if cks:
                overlapped = RAGFlowPdfParser.remove_tag(cks[-1])
                t = overlapped[int(len(overlapped) * (100 - overlapped_percent) / 100.):] + t
            if t.find(pos) < 0:
                t += pos
            cks.append(t)
            result_images.append(image)
            tk_nums.append(tnum)
        else:
            if cks[-1].find(pos) < 0:
                t += pos
            cks[-1] += t
            if result_images[-1] is None:
                result_images[-1] = image
            else:
                result_images[-1] = concat_img(result_images[-1], image)
            tk_nums[-1] += tnum

    custom_delimiters = [m.group(1) for m in re.finditer(r"`([^`]+)`", delimiter)]
    has_custom = bool(custom_delimiters)
    if has_custom:
        custom_pattern = "|".join(re.escape(t) for t in sorted(set(custom_delimiters), key=len, reverse=True))
        cks, result_images, tk_nums = [], [], []
        for text, image in zip(texts, images):
            text_str = text[0] if isinstance(text, tuple) else text
            if text_str is None:
                text_str = ""
            text_pos = text[1] if isinstance(text, tuple) and len(text) > 1 else ""
            split_sec = re.split(r"(%s)" % custom_pattern, text_str)
            for sub_sec in split_sec:
                if re.fullmatch(custom_pattern, sub_sec or ""):
                    continue
                text_seg = "\n" + sub_sec
                local_pos = text_pos
                if num_tokens_from_string(text_seg) < 8:
                    local_pos = ""
                if local_pos and text_seg.find(local_pos) < 0:
                    text_seg += local_pos
                cks.append(text_seg)
                result_images.append(image)
                tk_nums.append(num_tokens_from_string(text_seg))
        return cks, result_images

    for text, image in zip(texts, images):
        # if text is tuple, unpack it
        if isinstance(text, tuple):
            text_str = text[0] if text[0] is not None else ""
            text_pos = text[1] if len(text) > 1 else ""
            add_chunk("\n" + text_str, image, text_pos)
        else:
            add_chunk("\n" + (text or ""), image)

    return cks, result_images


def docx_question_level(p, bull=-1):
    txt = re.sub(r"\u3000", " ", p.text).strip()
    if p.style.name.startswith('Heading'):
        return int(p.style.name.split(' ')[-1]), txt
    else:
        if bull < 0:
            return 0, txt
        for j, title in enumerate(BULLET_PATTERN[bull]):
            if re.match(title, txt):
                return j + 1, txt
    return len(BULLET_PATTERN[bull]) + 1, txt


def concat_img(img1, img2):
    if img1 and not img2:
        return img1
    if not img1 and img2:
        return img2
    if not img1 and not img2:
        return None

    if img1 is img2:
        return img1

    if isinstance(img1, Image.Image) and isinstance(img2, Image.Image):
        pixel_data1 = img1.tobytes()
        pixel_data2 = img2.tobytes()
        if pixel_data1 == pixel_data2:
            return img1

    width1, height1 = img1.size
    width2, height2 = img2.size

    new_width = max(width1, width2)
    new_height = height1 + height2
    new_image = Image.new('RGB', (new_width, new_height))

    new_image.paste(img1, (0, 0))
    new_image.paste(img2, (0, height1))
    return new_image

def _build_cks(sections, delimiter):
    cks = []
    tables = []
    images = []

    # extract custom delimiters wrapped by backticks: `##`, `---`, etc.
    custom_delimiters = [m.group(1) for m in re.finditer(r"`([^`]+)`", delimiter)]
    has_custom = bool(custom_delimiters)

    if has_custom:
        # escape delimiters and build alternation pattern, longest first
        custom_pattern = "|".join(
            re.escape(t) for t in sorted(set(custom_delimiters), key=len, reverse=True)
        )
        # capture delimiters so they appear in re.split results
        pattern = r"(%s)" % custom_pattern

    seg = ""
    for text, image, table in sections:
        # normalize text: ensure string and prepend newline for continuity
        if not text:
            text = ""
        else:
            text = "\n" + str(text)

        if table:
            # table chunk
            ck_text = text + str(table)
            idx = len(cks)
            cks.append({
                "text": ck_text,
                "image": image,
                "ck_type": "table",
                "tk_nums": num_tokens_from_string(ck_text),
            })
            tables.append(idx)
            continue

        if image:
            # image chunk (text kept as-is for context)
            idx = len(cks)
            cks.append({
                "text": text,
                "image": image,
                "ck_type": "image",
                "tk_nums": num_tokens_from_string(text),
            })
            images.append(idx)
            continue

        # pure text chunk(s)
        if has_custom:
            split_sec = re.split(pattern, text)
            for sub_sec in split_sec:
                # ① empty or whitespace-only segment → flush current buffer
                if not sub_sec or not sub_sec.strip():
                    if seg and seg.strip():
                        s = seg.strip()
                        cks.append({
                            "text": s,
                            "image": None,
                            "ck_type": "text",
                            "tk_nums": num_tokens_from_string(s),
                        })
                    seg = ""
                    continue

                # ② matched custom delimiter (allow surrounding whitespace)
                if re.fullmatch(custom_pattern, sub_sec.strip()):
                    if seg and seg.strip():
                        s = seg.strip()
                        cks.append({
                            "text": s,
                            "image": None,
                            "ck_type": "text",
                            "tk_nums": num_tokens_from_string(s),
                        })
                    seg = ""
                    continue

                # ③ normal text content → accumulate
                seg += sub_sec
        else:
            # no custom delimiter: emit the text as a single chunk
            if text and text.strip():
                t = text.strip()
                cks.append({
                    "text": t,
                    "image": None,
                    "ck_type": "text",
                    "tk_nums": num_tokens_from_string(t),
                })

    # final flush after loop (only when custom delimiters are used)
    if has_custom and seg and seg.strip():
        s = seg.strip()
        cks.append({
            "text": s,
            "image": None,
            "ck_type": "text",
            "tk_nums": num_tokens_from_string(s),
        })

    return cks, tables, images, has_custom


def _add_context(cks, idx, context_size):
    if cks[idx]["ck_type"] not in ("image", "table"):
        return

    prev = idx - 1
    after = idx + 1
    remain_above = context_size
    remain_below = context_size

    cks[idx]["context_above"] = ""
    cks[idx]["context_below"] = ""

    split_pat = r"([。!?？；！\n]|\. )"

    picked_above = []
    picked_below = []

    def take_sentences_from_end(cnt, need_tokens):
        txts = re.split(split_pat, cnt, flags=re.DOTALL)
        sents = []
        for j in range(0, len(txts), 2):
            sents.append(txts[j] + (txts[j + 1] if j + 1 < len(txts) else ""))
        acc = ""
        for s in reversed(sents):
            acc = s + acc
            if num_tokens_from_string(acc) >= need_tokens:
                break
        return acc

    def take_sentences_from_start(cnt, need_tokens):
        txts = re.split(split_pat, cnt, flags=re.DOTALL)
        acc = ""
        for j in range(0, len(txts), 2):
            acc += txts[j] + (txts[j + 1] if j + 1 < len(txts) else "")
            if num_tokens_from_string(acc) >= need_tokens:
                break
        return acc

    # above
    parts_above = []
    while prev >= 0 and remain_above > 0:
        if cks[prev]["ck_type"] == "text":
            tk = cks[prev]["tk_nums"]
            if tk >= remain_above:
                piece = take_sentences_from_end(cks[prev]["text"], remain_above)
                parts_above.insert(0, piece)
                picked_above.append((prev, "tail", remain_above, tk, piece[:80]))
                remain_above = 0
                break
            else:
                parts_above.insert(0, cks[prev]["text"])
                picked_above.append((prev, "full", remain_above, tk, (cks[prev]["text"] or "")[:80]))
                remain_above -= tk
        prev -= 1

    # below
    parts_below = []
    while after < len(cks) and remain_below > 0:
        if cks[after]["ck_type"] == "text":
            tk = cks[after]["tk_nums"]
            if tk >= remain_below:
                piece = take_sentences_from_start(cks[after]["text"], remain_below)
                parts_below.append(piece)
                picked_below.append((after, "head", remain_below, tk, piece[:80]))
                remain_below = 0
                break
            else:
                parts_below.append(cks[after]["text"])
                picked_below.append((after, "full", remain_below, tk, (cks[after]["text"] or "")[:80]))
                remain_below -= tk
        after += 1

    cks[idx]["context_above"] = "".join(parts_above) if parts_above else ""
    cks[idx]["context_below"] = "".join(parts_below) if parts_below else ""


def _merge_cks(cks, chunk_token_num, has_custom):
    merged = []
    image_idxs = []
    prev_text_ck = -1

    for i in range(len(cks)):
        ck_type = cks[i]["ck_type"]

        if ck_type != "text":
            merged.append(cks[i])
            if ck_type == "image":
                image_idxs.append(len(merged) - 1)
            continue

        if prev_text_ck<0 or merged[prev_text_ck]["tk_nums"] >= chunk_token_num or has_custom:
            merged.append(cks[i])
            prev_text_ck = len(merged) - 1
            continue

        merged[prev_text_ck]["text"] = (merged[prev_text_ck].get("text") or "") + (cks[i].get("text") or "")
        merged[prev_text_ck]["tk_nums"] = merged[prev_text_ck].get("tk_nums", 0) + cks[i].get("tk_nums", 0)

    return merged, image_idxs


def naive_merge_docx(
    sections,
    chunk_token_num = 128,
    delimiter="\n。；！？",
    table_context_size=0,
    image_context_size=0,):

    if not sections:
        return [], []

    cks, tables, images, has_custom = _build_cks(sections, delimiter)

    if table_context_size > 0:
        for i in tables:
            _add_context(cks, i, table_context_size)

    if image_context_size > 0:
        for i in images:
            _add_context(cks, i, image_context_size)
    
    merged_cks, merged_image_idx = _merge_cks(cks, chunk_token_num, has_custom)

    return merged_cks, merged_image_idx


def extract_between(text: str, start_tag: str, end_tag: str) -> list[str]:
    pattern = re.escape(start_tag) + r"(.*?)" + re.escape(end_tag)
    return re.findall(pattern, text, flags=re.DOTALL)


def get_delimiters(delimiters: str):
    dels = []
    s = 0
    for m in re.finditer(r"`([^`]+)`", delimiters, re.I):
        f, t = m.span()
        dels.append(m.group(1))
        dels.extend(list(delimiters[s: f]))
        s = t
    if s < len(delimiters):
        dels.extend(list(delimiters[s:]))

    dels.sort(key=lambda x: -len(x))
    dels = [re.escape(d) for d in dels if d]
    dels = [d for d in dels if d]
    dels_pattern = "|".join(dels)

    return dels_pattern


class Node:
    def __init__(self, level, depth=-1, texts=None):
        self.level = level
        self.depth = depth
        self.texts = texts or []
        self.children = []

    def add_child(self, child_node):
        self.children.append(child_node)

    def get_children(self):
        return self.children

    def get_level(self):
        return self.level

    def get_texts(self):
        return self.texts

    def set_texts(self, texts):
        self.texts = texts

    def add_text(self, text):
        self.texts.append(text)

    def clear_text(self):
        self.texts = []

    def __repr__(self):
        return f"Node(level={self.level}, texts={self.texts}, children={len(self.children)})"

    def build_tree(self, lines):
        stack = [self]
        for level, text in lines:
            if self.depth != -1 and level > self.depth:
                # Beyond target depth: merge content into the current leaf instead of creating deeper nodes
                stack[-1].add_text(text)
                continue

            # Move up until we find the proper parent whose level is strictly smaller than current
            while len(stack) > 1 and level <= stack[-1].get_level():
                stack.pop()

            node = Node(level=level, texts=[text])
            # Attach as child of current parent and descend
            stack[-1].add_child(node)
            stack.append(node)

        return self

    def get_tree(self):
        tree_list = []
        self._dfs(self, tree_list, [])
        return tree_list

    def _dfs(self, node, tree_list, titles):
        level = node.get_level()
        texts = node.get_texts()
        child = node.get_children()

        if level == 0 and texts:
            tree_list.append("\n".join(titles + texts))

        # Titles within configured depth are accumulated into the current path
        if 1 <= level <= self.depth:
            path_titles = titles + texts
        else:
            path_titles = titles

        # Body outside the depth limit becomes its own chunk under the current title path
        if level > self.depth and texts:
            tree_list.append("\n".join(path_titles + texts))

        # A leaf title within depth emits its title path as a chunk (header-only section)
        elif not child and (1 <= level <= self.depth):
            tree_list.append("\n".join(path_titles))

        # Recurse into children with the updated title path
        for c in child:
            self._dfs(c, tree_list, path_titles)
