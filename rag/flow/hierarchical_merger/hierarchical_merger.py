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

import asyncio
import logging
import random
import re
from copy import deepcopy
from functools import partial

from common.misc_utils import get_uuid
from rag.utils.base64_image import id2image, image2id
from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.hierarchical_merger.schema import HierarchicalMergerFromUpstream
from rag.nlp import concat_img
from common import settings


class HierarchicalMergerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.levels = []
        self.hierarchy = None

    def check(self):
        self.check_empty(self.levels, "Hierarchical setups.")
        self.check_empty(self.hierarchy, "Hierarchy number.")

    def get_input_form(self) -> dict[str, dict]:
        return {}


class HierarchicalMerger(ProcessBase):
    component_name = "HierarchicalMerger"

    async def _invoke(self, **kwargs):
        try:
            from_upstream = HierarchicalMergerFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Start to merge hierarchically.")
        if from_upstream.output_format in ["markdown", "text", "html"]:
            if from_upstream.output_format == "markdown":
                payload = from_upstream.markdown_result
            elif from_upstream.output_format == "text":
                payload = from_upstream.text_result
            else:  # == "html"
                payload = from_upstream.html_result

            if not payload:
                payload = ""

            lines = [ln for ln in payload.split("\n") if ln]
        else:
            arr = from_upstream.chunks if from_upstream.output_format == "chunks" else from_upstream.json_result
            lines = [o.get("text", "") for o in arr]
            sections, section_images = [], []
            for o in arr or []:
                sections.append((o.get("text", ""), o.get("position_tag", "")))
                section_images.append(o.get("img_id"))

        matches = []
        for txt in lines:
            good = False
            for lvl, regs in enumerate(self._param.levels):
                for reg in regs:
                    if re.search(reg, txt):
                        matches.append(lvl)
                        good = True
                        break
                if good:
                    break
            if not good:
                matches.append(len(self._param.levels))
        assert len(matches) == len(lines), f"{len(matches)} vs. {len(lines)}"

        root = {
            "level": -1,
            "index": -1,
            "texts": [],
            "children": []
        }
        for i, m in enumerate(matches):
            if m == 0:
                root["children"].append({
                    "level": m,
                    "index": i,
                    "texts": [],
                    "children": []
                })
            elif m == len(self._param.levels):
                def dfs(b):
                    if not b["children"]:
                        b["texts"].append(i)
                    else:
                        dfs(b["children"][-1])
                dfs(root)
            else:
                def dfs(b):
                    nonlocal m, i
                    if not b["children"] or  m == b["level"] + 1:
                        b["children"].append({
                            "level": m,
                            "index": i,
                            "texts": [],
                            "children": []
                        })
                        return
                    dfs(b["children"][-1])

                dfs(root)

        all_pathes = []
        def dfs(n, path, depth):
            nonlocal all_pathes
            if not n["children"] and path:
                all_pathes.append(path)

            for nn in n["children"]:
                if depth < self._param.hierarchy:
                    _path = deepcopy(path)
                else:
                    _path = path
                _path.extend([nn["index"], *nn["texts"]])
                dfs(nn, _path, depth+1)

                if depth == self._param.hierarchy:
                    all_pathes.append(_path)

        dfs(root, [], 0)

        if root["texts"]:
            all_pathes.insert(0, root["texts"])
        if from_upstream.output_format in ["markdown", "text", "html"]:
            cks = []
            for path in all_pathes:
                txt = ""
                for i in path:
                    txt += lines[i] + "\n"
                cks.append(txt)

            self.set_output("chunks", [{"text": c} for c in cks if c])
        else:
            cks = []
            images = []
            for path in all_pathes:
                txt = ""
                img = None
                for i in path:
                    txt += lines[i] + "\n"
                    concat_img(img, id2image(section_images[i], partial(settings.STORAGE_IMPL.get, tenant_id=self._canvas._tenant_id)))
                cks.append(txt)
                images.append(img)

            cks = [
                {
                    "text": RAGFlowPdfParser.remove_tag(c),
                    "image": img,
                    "positions": RAGFlowPdfParser.extract_positions(c),
                }
                for c, img in zip(cks, images)
            ]
            tasks = []
            for d in cks:
                tasks.append(asyncio.create_task(image2id(d, partial(settings.STORAGE_IMPL.put, tenant_id=self._canvas._tenant_id), get_uuid())))
            try:
                await asyncio.gather(*tasks, return_exceptions=False)
            except Exception as e:
                logging.error(f"Error in image2id: {e}")
                for t in tasks:
                    t.cancel()
                await asyncio.gather(*tasks, return_exceptions=True)
                raise

            self.set_output("chunks", cks)

        self.callback(1, "Done.")
