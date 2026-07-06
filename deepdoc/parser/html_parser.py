# -*- coding: utf-8 -*-
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

from rag.nlp import find_codec, rag_tokenizer
import logging
import re
import uuid
import chardet
from bs4 import BeautifulSoup, NavigableString, Tag, Comment
import html


def get_encoding(file):
    with open(file, "rb") as f:
        tmp = chardet.detect(f.read())
        return tmp["encoding"]


BLOCK_TAGS = ["h1", "h2", "h3", "h4", "h5", "h6", "p", "div", "article", "section", "aside", "ul", "ol", "li", "table", "pre", "code", "blockquote", "figure", "figcaption"]
TITLE_TAGS = {"h1": "#", "h2": "##", "h3": "###", "h4": "####", "h5": "#####", "h6": "######"}


class RAGFlowHtmlParser:
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
        if not isinstance(txt, str):
            raise TypeError("txt type should be string!")

        temp_sections = []
        soup = BeautifulSoup(txt, "html.parser")
        # delete <style> tag
        for style_tag in soup.find_all(["style", "script"]):
            style_tag.decompose()
        # delete <script> tag in <div>
        for div_tag in soup.find_all("div"):
            for script_tag in div_tag.find_all("script"):
                script_tag.decompose()
        # delete inline style
        for tag in soup.find_all(True):
            if "style" in tag.attrs:
                del tag.attrs["style"]
        # delete HTML comment
        for comment in soup.find_all(string=lambda text: isinstance(text, Comment)):
            comment.extract()

        root = soup.body or soup
        if soup.body is None:
            logging.debug("html_parser: parsing HTML fragment without <body>; falling back to soup root")
        cls.read_text_recursively(root, temp_sections, chunk_token_num=chunk_token_num)
        block_txt_list, table_list = cls.merge_block_text(temp_sections)
        sections = cls.chunk_block(block_txt_list, chunk_token_num=chunk_token_num)
        for table in table_list:
            sections.append(table.get("content", ""))
        return sections

    @classmethod
    def split_table(cls, html_table, chunk_token_num=512):
        soup = BeautifulSoup(html_table, "html.parser")
        rows = soup.find_all("tr")
        tables = []
        current_table = []
        current_count = 0
        table_str_list = []
        for row in rows:
            tks_str = rag_tokenizer.tokenize(str(row))
            token_count = len(tks_str.split(" ")) if tks_str else 0
            if current_count + token_count > chunk_token_num:
                tables.append(current_table)
                current_table = []
                current_count = 0
            current_table.append(row)
            current_count += token_count
        if current_table:
            tables.append(current_table)

        for table_rows in tables:
            new_table = soup.new_tag("table")
            for row in table_rows:
                new_table.append(row)
            table_str_list.append(str(new_table))

        return table_str_list

    @classmethod
    def read_text_recursively(cls, element, parser_result, chunk_token_num=512, parent_name=None, block_id=None):
        if isinstance(element, NavigableString):
            content = element.strip()

            def is_valid_html(content):
                try:
                    soup = BeautifulSoup(content, "html.parser")
                    return bool(soup.find())
                except Exception:
                    return False

            return_info = []
            if content:
                if is_valid_html(content):
                    soup = BeautifulSoup(content, "html.parser")
                    child_info = cls.read_text_recursively(soup, parser_result, chunk_token_num, element.name, block_id)
                    parser_result.extend(child_info)
                else:
                    info = {"content": element.strip(), "tag_name": "inner_text", "metadata": {"block_id": block_id}}
                    if parent_name:
                        info["tag_name"] = parent_name
                    return_info.append(info)
            return return_info
        elif isinstance(element, Tag):
            if str.lower(element.name) == "table":
                table_info_list = []
                table_id = str(uuid.uuid1())
                table_list = [html.unescape(str(element))]
                for t in table_list:
                    table_info_list.append({"content": t, "tag_name": "table", "metadata": {"table_id": table_id, "index": table_list.index(t)}})
                return table_info_list
            else:
                if str.lower(element.name) in BLOCK_TAGS:
                    block_id = str(uuid.uuid1())
                for child in element.children:
                    child_info = cls.read_text_recursively(child, parser_result, chunk_token_num, element.name, block_id)
                    parser_result.extend(child_info)
        return []

    @classmethod
    def merge_block_text(cls, parser_result):
        block_content = []
        current_content = ""
        table_info_list = []
        last_block_id = None
        for item in parser_result:
            content = item.get("content")
            tag_name = item.get("tag_name")
            title_flag = tag_name in TITLE_TAGS
            block_id = item.get("metadata", {}).get("block_id")
            if block_id:
                if title_flag:
                    content = f"{TITLE_TAGS[tag_name]} {content}"
                if last_block_id != block_id:
                    if last_block_id is not None:
                        block_content.append(current_content)
                    current_content = content
                    last_block_id = block_id
                else:
                    current_content += (" " if current_content else "") + content
            else:
                if tag_name == "table":
                    table_info_list.append(item)
                else:
                    current_content += (" " if current_content else "") + content
        if current_content:
            block_content.append(current_content)
        return block_content, table_info_list

    # Characters from scripts written without spaces between words (CJK, kana,
    # Hangul). These must be split per-character, since whitespace is not a
    # usable word boundary for them.
    _SPACELESS = (
        "぀-ヿ"  # Hiragana, Katakana
        "㐀-䶿"  # CJK Extension A
        "一-鿿"  # CJK Unified Ideographs
        "豈-﫿"  # CJK Compatibility Ideographs
        "가-힯"  # Hangul syllables
    )
    _ATOM_RE = re.compile(r"[{s}]|[^\s{s}]+|\s+".format(s=_SPACELESS))

    @classmethod
    def _token_count(cls, text):
        if not text:
            return 0
        tks_str = rag_tokenizer.tokenize(text)
        return len(tks_str.split(" ")) if tks_str else 0

    @classmethod
    def _split_oversized_block(cls, block, chunk_token_num):
        # Split the ORIGINAL text into pieces of at most chunk_token_num tokens,
        # preserving the source characters. Break on whitespace for
        # space-delimited scripts and per-character for scripts that have no
        # spaces (e.g. Chinese), so both are split without mangling the text.
        pieces = []
        current = ""
        current_tokens = 0
        # Spaceless scripts yield many repeated single-character atoms, so cache
        # the token count per distinct atom to avoid re-tokenizing each one.
        token_cache = {}

        def atom_token_count(atom):
            if atom.isspace():
                return 0
            if atom not in token_cache:
                token_cache[atom] = cls._token_count(atom)
            return token_cache[atom]

        for atom in cls._ATOM_RE.findall(block):
            atom_tokens = atom_token_count(atom)
            if current and current_tokens + atom_tokens > chunk_token_num:
                pieces.append(current)
                current = ""
                current_tokens = 0
            if atom_tokens > chunk_token_num and not atom.isspace():
                # A single atom longer than the budget (e.g. a very long
                # unbroken token): fall back to fixed character windows.
                logging.debug(
                    "html_parser: atom of %d chars exceeds chunk_token_num=%d; falling back to character windows",
                    len(atom),
                    chunk_token_num,
                )
                for i in range(0, len(atom), chunk_token_num):
                    pieces.append(atom[i : i + chunk_token_num])
                continue
            current += atom
            current_tokens += atom_tokens
        if current:
            pieces.append(current)
        logging.debug(
            "html_parser: split oversized block of %d chars into %d pieces",
            len(block),
            len(pieces),
        )
        return pieces

    @classmethod
    def chunk_block(cls, block_txt_list, chunk_token_num=512):
        chunks = []
        current_block = ""
        current_token_count = 0

        for block in block_txt_list:
            block_token_count = cls._token_count(block)
            if block_token_count > chunk_token_num:
                if current_block:
                    chunks.append(current_block)
                    current_block = ""
                    current_token_count = 0
                chunks.extend(cls._split_oversized_block(block, chunk_token_num))
            else:
                if current_token_count + block_token_count <= chunk_token_num:
                    current_block += ("\n" if current_block else "") + block
                    current_token_count += block_token_count
                else:
                    chunks.append(current_block)
                    current_block = block
                    current_token_count = block_token_count

        if current_block:
            chunks.append(current_block)

        return chunks
