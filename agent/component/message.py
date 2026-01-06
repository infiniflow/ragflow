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
import asyncio
import nest_asyncio
nest_asyncio.apply()
import inspect
import json
import os
import random
import re
import logging
import tempfile
from functools import partial
from typing import Any

from agent.component.base import ComponentBase, ComponentParamBase
from jinja2 import Template as Jinja2Template

from common.connection_utils import timeout
from common.misc_utils import get_uuid
from common import settings

from api.db.joint_services.memory_message_service import queue_save_to_memory_task


class MessageParam(ComponentParamBase):
    """
    Define the Message component parameters.
    """
    def __init__(self):
        super().__init__()
        self.content = []
        self.stream = True
        self.output_format = None  # default output format
        self.auto_play = False
        self.outputs = {
            "content": {
                "type": "str"
            }
        }

    def check(self):
        self.check_empty(self.content, "[Message] Content")
        self.check_boolean(self.stream, "[Message] stream")
        return True


class Message(ComponentBase):
    component_name = "Message"

    def get_input_elements(self) -> dict[str, Any]:
        return self.get_input_elements_from_text("".join(self._param.content))

    def get_kwargs(self, script:str, kwargs:dict = {}, delimiter:str=None) -> tuple[str, dict[str, str | list | Any]]:
        for k,v in self.get_input_elements_from_text(script).items():
            if k in kwargs:
                continue
            v = v["value"]
            if not v:
                v = ""
            ans = ""
            if isinstance(v, partial):
                iter_obj = v()
                if inspect.isasyncgen(iter_obj):
                    ans = asyncio.run(self._consume_async_gen(iter_obj))
                else:
                    for t in iter_obj:
                        ans += t
            elif isinstance(v, list) and delimiter:
                ans = delimiter.join([str(vv) for vv in v])
            elif not isinstance(v, str):
                try:
                    ans = json.dumps(v, ensure_ascii=False)
                except Exception:
                    pass
            else:
                ans = v
            if not ans:
                ans = ""
            kwargs[k] = ans
            self.set_input_value(k, ans)

        _kwargs = {}
        for n, v in kwargs.items():
            _n = re.sub("[@:.]", "_", n)
            script = re.sub(r"\{%s\}" % re.escape(n), _n, script)
            _kwargs[_n] = v
        return script, _kwargs

    async def _consume_async_gen(self, agen):
        buf = ""
        async for t in agen:
            buf += t
        return buf

    async def _stream(self, rand_cnt:str):
        s = 0
        all_content = ""
        cache = {}
        for r in re.finditer(self.variable_ref_patt, rand_cnt, flags=re.DOTALL):
            if self.check_if_canceled("Message streaming"):
                return

            all_content += rand_cnt[s: r.start()]
            yield rand_cnt[s: r.start()]
            s = r.end()
            exp = r.group(1)
            if exp in cache:
                yield cache[exp]
                all_content += cache[exp]
                continue

            v = self._canvas.get_variable_value(exp)
            if v is None:
                v = ""
            if isinstance(v, partial):
                cnt = ""
                iter_obj = v()
                if inspect.isasyncgen(iter_obj):
                    async for t in iter_obj:
                        if self.check_if_canceled("Message streaming"):
                            return

                        all_content += t
                        cnt += t
                        yield t
                else:
                    for t in iter_obj:
                        if self.check_if_canceled("Message streaming"):
                            return

                        all_content += t
                        cnt += t
                        yield t
                self.set_input_value(exp, cnt)
                continue
            elif inspect.isawaitable(v):
                v = await v
            elif not isinstance(v, str):
                try:
                    v = json.dumps(v, ensure_ascii=False)
                except Exception:
                    v = str(v)
            yield v
            self.set_input_value(exp, v)
            all_content += v
            cache[exp] = v

        if s < len(rand_cnt):
            if self.check_if_canceled("Message streaming"):
                return

            all_content += rand_cnt[s: ]
            yield rand_cnt[s: ]

        self.set_output("content", all_content)
        self._convert_content(all_content)
        await self._save_to_memory(all_content)

    def _is_jinjia2(self, content:str) -> bool:
        patt = [
            r"\{%.*%\}", "{{", "}}"
        ]
        return any([re.search(p, content) for p in patt])

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("Message processing"):
            return

        rand_cnt = random.choice(self._param.content)
        if self._param.stream and not self._is_jinjia2(rand_cnt):
            self.set_output("content", partial(self._stream, rand_cnt))
            return

        rand_cnt, kwargs = self.get_kwargs(rand_cnt, kwargs)
        template = Jinja2Template(rand_cnt)
        try:
            content = template.render(kwargs)
        except Exception:
            pass

        if self.check_if_canceled("Message processing"):
            return

        for n, v in kwargs.items():
            content = re.sub(n, v, content)

        self.set_output("content", content)
        self._convert_content(content)
        self._save_to_memory(content)

    def thoughts(self) -> str:
        return ""

    def _parse_markdown_table_lines(self, table_lines: list):
        """
        Parse a list of Markdown table lines into a pandas DataFrame.
        
        Args:
            table_lines: List of strings, each representing a row in the Markdown table
                        (excluding separator lines like |---|---|)
        
        Returns:
            pandas DataFrame with the table data, or None if parsing fails
        """
        import pandas as pd
        
        if not table_lines:
            return None
        
        rows = []
        headers = None
        
        for line in table_lines:
            # Split by | and clean up
            cells = [cell.strip() for cell in line.split('|')]
            # Remove empty first and last elements from split (caused by leading/trailing |)
            cells = [c for c in cells if c]
            
            if headers is None:
                headers = cells
            else:
                rows.append(cells)
        
        if headers and rows:
            # Ensure all rows have same number of columns as headers
            normalized_rows = []
            for row in rows:
                while len(row) < len(headers):
                    row.append('')
                normalized_rows.append(row[:len(headers)])
            
            return pd.DataFrame(normalized_rows, columns=headers)
        
        return None

    def _convert_content(self, content):
        if not self._param.output_format:
            return

        import pypandoc
        doc_id = get_uuid()

        if self._param.output_format.lower() not in {"markdown", "html", "pdf", "docx", "xlsx"}:
            self._param.output_format = "markdown"

        try:
            if self._param.output_format in {"markdown", "html"}:
                if isinstance(content, str):
                    converted = pypandoc.convert_text(
                        content,
                        to=self._param.output_format,
                        format="markdown",
                    )
                else:
                    converted = pypandoc.convert_file(
                        content,
                        to=self._param.output_format,
                        format="markdown",
                    )

                binary_content = converted.encode("utf-8")

            elif self._param.output_format == "xlsx":
                import pandas as pd
                from io import BytesIO

                # Debug: log the content being parsed
                logging.info(f"XLSX Parser: Content length={len(content) if content else 0}, first 500 chars: {content[:500] if content else 'None'}")
                
                # Try to parse ALL Markdown tables from the content
                # Each table will be written to a separate sheet
                tables = []  # List of (sheet_name, dataframe)
                
                if isinstance(content, str):
                    lines = content.strip().split('\n')
                    logging.info(f"XLSX Parser: Total lines={len(lines)}, lines starting with '|': {sum(1 for line in lines if line.strip().startswith('|'))}")
                    current_table_lines = []
                    current_table_title = None
                    pending_title = None
                    in_table = False
                    table_count = 0
                    
                    for i, line in enumerate(lines):
                        stripped = line.strip()
                        
                        # Check for potential table title (lines before a table)
                        # Look for patterns like "Table 1:", "## Table", or markdown headers
                        if not in_table and stripped and not stripped.startswith('|'):
                            # Check if this could be a table title
                            lower_stripped = stripped.lower()
                            if (lower_stripped.startswith('table') or 
                                stripped.startswith('#') or
                                ':' in stripped):
                                pending_title = stripped.lstrip('#').strip()
                        
                        if stripped.startswith('|') and '|' in stripped[1:]:
                            # Check if this is a separator line (|---|---|)
                            cleaned = stripped.replace(' ', '').replace('|', '').replace('-', '').replace(':', '')
                            if cleaned == '':
                                continue  # Skip separator line
                            
                            if not in_table:
                                # Starting a new table
                                in_table = True
                                current_table_lines = []
                                current_table_title = pending_title
                                pending_title = None
                            
                            current_table_lines.append(stripped)
                        
                        elif in_table and not stripped.startswith('|'):
                            # End of current table - save it
                            if current_table_lines:
                                df = self._parse_markdown_table_lines(current_table_lines)
                                if df is not None and not df.empty:
                                    table_count += 1
                                    # Generate sheet name
                                    if current_table_title:
                                        # Clean and truncate title for sheet name
                                        sheet_name = current_table_title[:31]
                                        sheet_name = sheet_name.replace('/', '_').replace('\\', '_').replace('*', '').replace('?', '').replace('[', '').replace(']', '').replace(':', '')
                                    else:
                                        sheet_name = f"Table_{table_count}"
                                    tables.append((sheet_name, df))
                            
                            # Reset for next table
                            in_table = False
                            current_table_lines = []
                            current_table_title = None
                            
                            # Check if this line could be a title for the next table
                            if stripped:
                                lower_stripped = stripped.lower()
                                if (lower_stripped.startswith('table') or 
                                    stripped.startswith('#') or
                                    ':' in stripped):
                                    pending_title = stripped.lstrip('#').strip()
                    
                    # Don't forget the last table if content ends with a table
                    if in_table and current_table_lines:
                        df = self._parse_markdown_table_lines(current_table_lines)
                        if df is not None and not df.empty:
                            table_count += 1
                            if current_table_title:
                                sheet_name = current_table_title[:31]
                                sheet_name = sheet_name.replace('/', '_').replace('\\', '_').replace('*', '').replace('?', '').replace('[', '').replace(']', '').replace(':', '')
                            else:
                                sheet_name = f"Table_{table_count}"
                            tables.append((sheet_name, df))
                
                # Fallback: if no tables found, create single sheet with content
                if not tables:
                    df = pd.DataFrame({"Content": [content if content else ""]})
                    tables = [("Data", df)]

                # Write all tables to Excel, each in a separate sheet
                excel_io = BytesIO()
                with pd.ExcelWriter(excel_io, engine='openpyxl') as writer:
                    used_names = set()
                    for sheet_name, df in tables:
                        # Ensure unique sheet names
                        original_name = sheet_name
                        counter = 1
                        while sheet_name in used_names:
                            suffix = f"_{counter}"
                            sheet_name = original_name[:31-len(suffix)] + suffix
                            counter += 1
                        used_names.add(sheet_name)
                        df.to_excel(writer, sheet_name=sheet_name, index=False)
                
                excel_io.seek(0)
                binary_content = excel_io.read()
                
                logging.info(f"Generated Excel with {len(tables)} sheet(s): {[t[0] for t in tables]}")

            else:  # pdf, docx
                with tempfile.NamedTemporaryFile(suffix=f".{self._param.output_format}", delete=False) as tmp:
                    tmp_name = tmp.name

                try:
                    if isinstance(content, str):
                        pypandoc.convert_text(
                            content,
                            to=self._param.output_format,
                            format="markdown",
                            outputfile=tmp_name,
                        )
                    else:
                        pypandoc.convert_file(
                            content,
                            to=self._param.output_format,
                            format="markdown",
                            outputfile=tmp_name,
                        )

                    with open(tmp_name, "rb") as f:
                        binary_content = f.read()

                finally:
                    if os.path.exists(tmp_name):
                        os.remove(tmp_name)

            settings.STORAGE_IMPL.put(self._canvas._tenant_id, doc_id, binary_content)
            self.set_output("attachment", {
                "doc_id":doc_id,
                "format":self._param.output_format,
                "file_name":f"{doc_id[:8]}.{self._param.output_format}"})

            logging.info(f"Converted content uploaded as {doc_id} (format={self._param.output_format})")

        except Exception as e:
            logging.error(f"Error converting content to {self._param.output_format}: {e}")

    async def _save_to_memory(self, content):
        if not hasattr(self._param, "memory_ids") or not self._param.memory_ids:
            return True, "No memory selected."

        message_dict = {
            "user_id": self._canvas._tenant_id,
            "agent_id": self._canvas._id,
            "session_id": self._canvas.task_id,
            "user_input": self._canvas.get_sys_query(),
            "agent_response": content
        }
        return await queue_save_to_memory_task(self._param.memory_ids, message_dict)
