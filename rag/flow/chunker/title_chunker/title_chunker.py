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

from rag.flow.base import ProcessBase
from rag.flow.chunker.title_chunker.group_chunker import GroupTitleChunker
from rag.flow.chunker.title_chunker.hierarchy_chunker import HierarchyTitleChunker
from rag.flow.chunker.title_chunker.schema import TitleChunkerFromUpstream

class TitleChunker(ProcessBase):
    component_name = "TitleChunker"

    async def _invoke(self, **kwargs):
        try:
            from_upstream = TitleChunkerFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        if self._param.method == "hierarchy":
            await HierarchyTitleChunker(self, from_upstream).invoke()
            return

        if self._param.method == "group":
            await GroupTitleChunker(self, from_upstream).invoke()
            return

        self.set_output("_ERROR", f"Unsupported TitleChunker method: {self._param.method}")
