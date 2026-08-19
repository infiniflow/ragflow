#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

"""Sentence-boundary definition shared by the title and token chunkers.

Kept dependency-free so both chunkers can reuse the exact same split without
pulling in heavier parser/tokenizer modules.
"""

import re

# Boundaries used to re-split an oversized chunk into sentences. Covers
# Chinese/English period, exclamation mark, question mark, the Chinese
# variants, and the newline. The capturing group keeps the delimiter attached
# to the preceding sentence so re-merged text preserves original boundaries.
SENTENCE_BOUNDARY_PATTERN = r"([。!?？；！\n]|\. )"

SENTENCE_BOUNDARY_RE = re.compile(SENTENCE_BOUNDARY_PATTERN, re.DOTALL)
