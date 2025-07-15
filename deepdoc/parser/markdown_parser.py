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

import re

from markdown import markdown

class RAGFlowMarkdownParser:
    def __init__(self, chunk_token_num=128):
        self.chunk_token_num = int(chunk_token_num)

    def extract_tables_and_remainder(self, markdown_text, separate_tables=True):
        tables = []
        working_text = markdown_text

        def replace_tables_with_rendered_html(pattern, table_list, render=True):
            new_text = ""
            last_end = 0
            for match in pattern.finditer(working_text):
                raw_table = match.group()
                table_list.append(raw_table)
                if separate_tables:
                    # Skip this match (i.e., remove it)
                    new_text += working_text[last_end:match.start()] + "\n\n"
                else:
                    # Replace with rendered HTML
                    html_table = markdown(raw_table, extensions=['markdown.extensions.tables']) if render else raw_table
                    new_text += working_text[last_end:match.start()] + html_table + "\n\n"
                last_end = match.end()
            new_text += working_text[last_end:]
            return new_text

        if "|" in markdown_text: # for optimize performance
            # Standard Markdown table
            border_table_pattern = re.compile(
                r'''
                (?:\n|^)
                (?:\|.*?\|.*?\|.*?\n)
                (?:\|(?:\s*[:-]+[-| :]*\s*)\|.*?\n)
                (?:\|.*?\|.*?\|.*?\n)+
            ''', re.VERBOSE)
            working_text = replace_tables_with_rendered_html(border_table_pattern, tables)

            # Borderless Markdown table
            no_border_table_pattern = re.compile(
                r'''
                (?:\n|^)
                (?:\S.*?\|.*?\n)
                (?:(?:\s*[:-]+[-| :]*\s*).*?\n)
                (?:\S.*?\|.*?\n)+
                ''', re.VERBOSE)
            working_text = replace_tables_with_rendered_html(no_border_table_pattern, tables)

        if "<table>" in working_text.lower(): # for optimize performance
            #HTML table extraction - handle possible html/body wrapper tags
            html_table_pattern = re.compile(
            r'''
            (?:\n|^)
            \s*
            (?:
                # case1: <html><body><table>...</table></body></html>
                (?:<html[^>]*>\s*<body[^>]*>\s*<table[^>]*>.*?</table>\s*</body>\s*</html>)
                |
                # case2: <body><table>...</table></body>
                (?:<body[^>]*>\s*<table[^>]*>.*?</table>\s*</body>)
                |
                # case3: only<table>...</table>
                (?:<table[^>]*>.*?</table>)
            )
            \s*
            (?=\n|$)
            ''',
            re.VERBOSE | re.DOTALL | re.IGNORECASE
            )
            def replace_html_tables():
                nonlocal working_text
                new_text = ""
                last_end = 0
                for match in html_table_pattern.finditer(working_text):
                    raw_table = match.group()
                    tables.append(raw_table)
                    if separate_tables:
                        new_text += working_text[last_end:match.start()] + "\n\n"
                    else:
                        new_text += working_text[last_end:match.start()] + raw_table + "\n\n"
                    last_end = match.end()
                new_text += working_text[last_end:]
                working_text = new_text

            replace_html_tables()

        return working_text, tables
