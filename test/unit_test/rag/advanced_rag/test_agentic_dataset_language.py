#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
The agentic search path must retrieve with the dataset language.

Index tokens for a diacritic-folding dataset (Slovak, Czech) are folded to
ASCII, so a query that is not folded the same way cannot match them. RAGTools
carries the language and the search tools pass it to the retriever.
"""

from types import SimpleNamespace

import pytest

from rag.advanced_rag.agentic_rag import RAGTools

pytestmark = pytest.mark.p2


class _ChatModel:
    max_length = 8192

    def clone(self):
        return self


def _kb(language):
    return SimpleNamespace(id="kb-1", language=language, parser_config={})


def test_language_comes_from_the_first_dataset():
    assert RAGTools([], _ChatModel(), kbs=[_kb("Slovak")]).language == "Slovak"


def test_language_is_none_without_datasets():
    assert RAGTools([], _ChatModel()).language is None
