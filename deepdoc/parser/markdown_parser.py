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

class RAGFlowMarkdownParser:
    def __init__(self, chunk_token_num=128):
        self.chunk_token_num = int(chunk_token_num)

    def extract_tables_and_remainder(self, markdown_text):
        # Standard Markdown table
        table_pattern = re.compile(
            r'''
            (?:\n|^)                     
            (?:\|.*?\|.*?\|.*?\n)        
            (?:\|(?:\s*[:-]+[-| :]*\s*)\|.*?\n) 
            (?:\|.*?\|.*?\|.*?\n)+
            ''', re.VERBOSE)
        tables = table_pattern.findall(markdown_text)
        remainder = table_pattern.sub('', markdown_text)

        # Borderless Markdown table
        no_border_table_pattern = re.compile(
            r'''
            (?:\n|^)                 
            (?:\S.*?\|.*?\n)
            (?:(?:\s*[:-]+[-| :]*\s*).*?\n)
            (?:\S.*?\|.*?\n)+
            ''', re.VERBOSE)
        no_border_tables = no_border_table_pattern.findall(remainder)
        tables.extend(no_border_tables)
        remainder = no_border_table_pattern.sub('', remainder)

        return remainder, tables
