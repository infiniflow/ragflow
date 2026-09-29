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

from rag.graphrag.entity_resolution import EntityResolution


def _resolver():
    return EntityResolution(llm_invoker=None)


def test_is_similarity_matches_within_half_edit_distance():
    resolver = _resolver()

    assert resolver.is_similarity("microsoft", "microsfot") is True
    assert resolver.is_similarity("apple", "orange") is False


def test_is_similarity_identical_strings():
    resolver = _resolver()

    assert resolver.is_similarity("openai", "openai") is True
