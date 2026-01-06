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
from typing import Optional, List

from common.constants import MemoryType
from common.time_utils import current_timestamp

class PromptAssembler:

    SYSTEM_BASE_TEMPLATE = """**Memory Extraction Specialist**
You are an expert at analyzing conversations to extract structured memory.

{type_specific_instructions}


**OUTPUT REQUIREMENTS:**
1. Output MUST be valid JSON
2. Follow the specified output format exactly
3. Each extracted item MUST have: content, valid_at, invalid_at
4. Timestamps in {timestamp_format} format
5. Only extract memory types specified above
6. Maximum {max_items} items per type
"""

    TYPE_INSTRUCTIONS = {
        MemoryType.SEMANTIC.name.lower(): """
        **EXTRACT SEMANTIC KNOWLEDGE:**
        - Universal facts, definitions, concepts, relationships
        - Time-invariant, generally true information
        - Examples: "The capital of France is Paris", "Water boils at 100°C"

        **Timestamp Rules for Semantic Knowledge:**
        - valid_at: When the fact became true (e.g., law enactment, discovery)
        - invalid_at: When it becomes false (e.g., repeal, disproven) or empty if still true
        - Default: valid_at = conversation time, invalid_at = "" for timeless facts
        """,

        MemoryType.EPISODIC.name.lower(): """
        **EXTRACT EPISODIC KNOWLEDGE:**
        - Specific experiences, events, personal stories
        - Time-bound, person-specific, contextual
        - Examples: "Yesterday I fixed the bug", "User reported issue last week"

        **Timestamp Rules for Episodic Knowledge:**
        - valid_at: Event start/occurrence time
        - invalid_at: Event end time or empty if instantaneous
        - Extract explicit times: "at 3 PM", "last Monday", "from X to Y"
        """,

        MemoryType.PROCEDURAL.name.lower(): """
        **EXTRACT PROCEDURAL KNOWLEDGE:**
        - Processes, methods, step-by-step instructions
        - Goal-oriented, actionable, often includes conditions
        - Examples: "To reset password, click...", "Debugging steps: 1)..."

        **Timestamp Rules for Procedural Knowledge:**
        - valid_at: When procedure becomes valid/effective
        - invalid_at: When it expires/becomes obsolete or empty if current
        - For version-specific: use release dates
        - For best practices: invalid_at = ""
        """
    }

    OUTPUT_TEMPLATES = {
        MemoryType.SEMANTIC.name.lower(): """
        "semantic": [
            {
                "content": "Clear factual statement",
                "valid_at": "timestamp or empty",
                "invalid_at": "timestamp or empty"
            }
        ]
        """,

        MemoryType.EPISODIC.name.lower(): """
        "episodic": [
            {
                "content": "Narrative event description",
                "valid_at": "event start timestamp",
                "invalid_at": "event end timestamp or empty"
            }
        ]
        """,

        MemoryType.PROCEDURAL.name.lower(): """
        "procedural": [
            {
                "content": "Actionable instructions",
                "valid_at": "procedure effective timestamp",
                "invalid_at": "procedure expiration timestamp or empty"
            }
        ]
        """
    }

    BASE_USER_PROMPT = """
**CONVERSATION:**
{conversation}

**CONVERSATION TIME:** {conversation_time}
**CURRENT TIME:** {current_time}    
"""

    @classmethod
    def assemble_system_prompt(cls, config: dict) -> str:
        types_to_extract = cls._get_types_to_extract(config["memory_type"])

        type_instructions = cls._generate_type_instructions(types_to_extract)

        output_format = cls._generate_output_format(types_to_extract)

        full_prompt = cls.SYSTEM_BASE_TEMPLATE.format(
            type_specific_instructions=type_instructions,
            timestamp_format=config.get("timestamp_format", "ISO 8601"),
            max_items=config.get("max_items_per_type", 5)
        )

        full_prompt += f"\n**REQUIRED OUTPUT FORMAT (JSON):**\n```json\n{{\n{output_format}\n}}\n```\n"

        examples = cls._generate_examples(types_to_extract)
        if examples:
            full_prompt += f"\n**EXAMPLES:**\n{examples}\n"

        return full_prompt

    @staticmethod
    def _get_types_to_extract(requested_types: List[str]) -> List[str]:
        types = set()
        for rt in requested_types:
            if rt in [e.name.lower()  for e in MemoryType] and rt != MemoryType.RAW.name.lower():
                types.add(rt)
        return list(types)

    @classmethod
    def _generate_type_instructions(cls, types_to_extract: List[str]) -> str:
        target_types = set(types_to_extract)
        instructions = [cls.TYPE_INSTRUCTIONS[mt] for mt in target_types]
        return "\n".join(instructions)

    @classmethod
    def _generate_output_format(cls, types_to_extract: List[str]) -> str:
        target_types = set(types_to_extract)
        output_parts = [cls.OUTPUT_TEMPLATES[mt] for mt in target_types]
        return ",\n".join(output_parts)

    @staticmethod
    def _generate_examples(types_to_extract: list[str]) -> str:
        examples = []

        if MemoryType.SEMANTIC.name.lower() in types_to_extract:
            examples.append("""
            **Semantic Example:**
            Input: "Python lists are mutable and support various operations."
            Output: {"semantic": [{"content": "Python lists are mutable data structures", "valid_at": "2024-01-15T10:00:00", "invalid_at": ""}]}
            """)

        if MemoryType.EPISODIC.name.lower() in types_to_extract:
            examples.append("""
            **Episodic Example:**
            Input: "I deployed the new feature yesterday afternoon."
            Output: {"episodic": [{"content": "User deployed new feature", "valid_at": "2024-01-14T14:00:00", "invalid_at": "2024-01-14T18:00:00"}]}
            """)

        if MemoryType.PROCEDURAL.name.lower() in types_to_extract:
            examples.append("""
            **Procedural Example:**
            Input: "To debug API errors: 1) Check logs 2) Verify endpoints 3) Test connectivity."
            Output: {"procedural": [{"content": "API error debugging: 1. Check logs 2. Verify endpoints 3. Test connectivity", "valid_at": "2024-01-15T10:00:00", "invalid_at": ""}]}
            """)

        return "\n".join(examples)

    @classmethod
    def assemble_user_prompt(
            cls,
            conversation: str,
            conversation_time: Optional[str] = None,
            current_time: Optional[str] = None
    ) -> str:
        return cls.BASE_USER_PROMPT.format(
            conversation=conversation,
            conversation_time=conversation_time or "Not specified",
            current_time=current_time or current_timestamp(),
        )

    @classmethod
    def get_raw_user_prompt(cls):
        return cls.BASE_USER_PROMPT
