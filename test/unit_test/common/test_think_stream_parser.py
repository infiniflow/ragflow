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

import asyncio
import io
import sys
import unittest
from contextlib import redirect_stdout
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from api.db.services.dialog_service import _stream_with_think_delta


CASES = [
    (
        "minimax",
        {
            "min_tokens": 16,
            "chunks": [
                '<think>The user has sent a simple greeting "hello". I should respond in a friendly and helpful manner.Hello!',
                "</think>\n\n How can I help",
                " you today?",
            ],
            "expected": {
                "think": 'The user has sent a simple greeting "hello". I should respond in a friendly and helpful manner.Hello!',
                "answer": "\n\n How can I help you today?",
            },
        },
    ),
    (
        "deepseek",
        {
            "min_tokens": 16,
            "chunks": [
                "<think>We</think>",
                " need</think>",
                " to</think>",
                " respond</think>",
                " to</think>",
                " the</think>",
                " user</think>",
                "'s</think>",
                " greeting</think>",
                ' "</think>',
                "hello</think>",
                '".</think>',
                " The</think>",
                " assistant</think>",
                " should</think>",
                " be</think>",
                " friendly</think>",
                " and</think>",
                " helpful</think>",
                ".</think>",
                " A</think>",
                " simple</think>",
                " greeting</think>",
                " back</think>",
                " is</think>",
                " appropriate</think>",
                ",</think>",
                " perhaps</think>",
                " with</think>",
                " an</think>",
                " offer</think>",
                " of</think>",
                " assistance</think>",
                ".</think>",
                "Hello",
                "!",
                " How",
                " can",
                " I",
                " assist",
                " you",
                " today",
                "?",
            ],
            "expected": {
                "think": 'We need to respond to the user\'s greeting "hello". The assistant should be friendly and helpful. A simple greeting back is appropriate, perhaps with an offer of assistance.',
                "answer": "Hello! How can I assist you today?",
            },
        },
    ),
    (
        "deepseek_repeat",
        {
            "min_tokens": 16,
            "chunks": [
                "<think>We</think>",
                " need</think>",
                " to</think>",
                " respond</think>",
                " to</think>",
                " the</think>",
                " user</think>",
                "'s</think>",
                ' "</think>',
                "hello</think>",
                '"</think>',
                " again</think>",
                ".</think>",
                " The</think>",
                " user</think>",
                " just</think>",
                " said</think>",
                ' "</think>',
                "hello</think>",
                '"</think>',
                " after</think>",
                " I</think>",
                " already</think>",
                " responded</think>",
                ".</think>",
                " Possibly</think>",
                " they</think>",
                "'re</think>",
                " testing</think>",
                " or</think>",
                " just</think>",
                " greeting</think>",
                " again</think>",
                ".</think>",
                " I</think>",
                "'ll</think>",
                " respond</think>",
                " in</think>",
                " a</think>",
                " friendly</think>",
                " manner</think>",
                ",</think>",
                " perhaps</think>",
                " acknowledging</think>",
                " the</think>",
                " repeated</think>",
                " greeting</think>",
                " and</think>",
                " inviting</think>",
                " them</think>",
                " to</think>",
                " ask</think>",
                " something</think>",
                ".</think>",
                "Hello",
                " again",
                "!",
                " How",
                " can",
                " I",
                " help",
                " you",
                " today",
                "?",
            ],
            "expected": {
                "think": 'We need to respond to the user\'s "hello" again. The user just said "hello" after I already responded. Possibly they\'re testing or just greeting again. I\'ll respond in a friendly manner, perhaps acknowledging the repeated greeting and inviting them to ask something.',
                "answer": "Hello again! How can I help you today?",
            },
        },
    ),
    (
        "answer_then_think",
        {
            "min_tokens": 16,
            "chunks": [
                "前言",
                " ",
                "<think>内部推理一</think>",
                "最终回答",
                "。",
            ],
            "expected": {
                "think": "内部推理一",
                "answer": "前言 最终回答。",
                "markers": ["<think>", "</think>"],
            },
        },
    ),
    (
        "close_pending_eof",
        {
            "min_tokens": 16,
            "chunks": [
                "<think>先思考完毕</think>答案在这里",
            ],
            "expected": {
                "think": "先思考完毕",
                "answer": "答案在这里",
                "markers": ["<think>", "</think>"],
            },
        },
    ),
    (
        "mixed_boundary",
        {
            "min_tokens": 16,
            "chunks": [
                "前缀",
                "<think>理由A</think>答案A",
                " 后缀",
            ],
            "expected": {
                "think": "理由A",
                "answer": "前缀答案A 后缀",
                "markers": ["<think>", "</think>"],
            },
        },
    ),
    (
        "think_only_eof",
        {
            "min_tokens": 16,
            "chunks": [
                "<think>只输出思考，不输出最终答案",
                "，并且流在这里结束",
            ],
            "expected": {
                "think": "只输出思考，不输出最终答案，并且流在这里结束",
                "answer": "",
                "markers": ["<think>"],
            },
        },
    ),
    (
        "double_think_blocks",
        {
            "min_tokens": 16,
            "chunks": [
                "<think>第一段推理</think>答案A",
                " <think>第二段推理</think>答案B",
            ],
            "expected": {
                "think": "第一段推理第二段推理",
                "answer": "答案A 答案B",
                "markers": ["<think>", "</think>", "<think>", "</think>"],
            },
        },
    ),
    (
        "nested_or_malformed_tags",
        {
            "min_tokens": 16,
            "chunks": [
                "<think><think>重复开始</think></think>",
                "答案",
                "</think>",
                "尾巴",
            ],
            "expected": {
                "think": "重复开始",
                "answer": "答案尾巴",
                "markers": ["<think>", "</think>", "</think>"],
            },
        },
    ),
    (
        "tiny_think_chunks",
        {
            "min_tokens": 16,
            "chunks": [
                "<think>",
                "A",
                "B",
                "C",
                "D",
                "E",
                "</think>",
                "答",
                "案",
                "输",
                "出",
            ],
            "expected": {
                "think": "ABCDE",
                "answer": "答案输出",
                "markers": ["<think>", "</think>"],
            },
        },
    ),
    (
        "think_then_answer_then_think",
        {
            "min_tokens": 16,
            "chunks": [
                "<think>第一轮推理</think>第一轮答案",
                " <think>第二轮推理</think>第二轮答案",
            ],
            "expected": {
                "think": "第一轮推理第二轮推理",
                "answer": "第一轮答案 第二轮答案",
                "markers": ["<think>", "</think>", "<think>", "</think>"],
            },
        },
    ),
]


async def _iter_chunks(chunks):
    for chunk in chunks:
        yield chunk


async def _collect_case(chunks, min_tokens):
    think_parts = []
    answer_parts = []
    markers = []
    section = "answer"

    async for kind, value, _state in _stream_with_think_delta(_iter_chunks(chunks), min_tokens=min_tokens):
        if kind == "marker":
            markers.append(value)
            section = "think" if value == "<think>" else "answer"
            continue
        if section == "think":
            think_parts.append(value)
        else:
            answer_parts.append(value)

    return "".join(think_parts), "".join(answer_parts), markers


class TestThinkStreamParser(unittest.TestCase):
    def test_think_stream_parser_cases(self):
        for case_name, case in CASES:
            with self.subTest(case=case_name):
                buf = io.StringIO()
                with redirect_stdout(buf):
                    think_text, answer_text, markers = asyncio.run(_collect_case(case["chunks"], case["min_tokens"]))

                expected = case["expected"]
                self.assertEqual(think_text, expected["think"], case_name)
                self.assertEqual(answer_text, expected["answer"], case_name)
                if "markers" in expected:
                    self.assertEqual(markers, expected["markers"], case_name)
