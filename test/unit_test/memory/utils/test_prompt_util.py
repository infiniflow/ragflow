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

from memory.utils.prompt_util import PromptAssembler


def test_semantic_output_format_requires_valid_at_timestamp():
    prompt = PromptAssembler.assemble_system_prompt({"memory_type": ["semantic"]})

    assert '"valid_at": "timestamp — use the conversation time when the fact has no date of its own"' in prompt
    assert '"valid_at": "timestamp or empty"' not in prompt
