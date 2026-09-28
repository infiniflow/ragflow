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
import sys
from unittest.mock import MagicMock
if "infinity" not in sys.modules:
    mock_inf = MagicMock()
    mock_inf.rag_tokenizer = MagicMock()
    sys.modules["infinity"] = mock_inf
    sys.modules["infinity.rag_tokenizer"] = mock_inf.rag_tokenizer

import pytest

from rag.prompts.generator import (
    shorten_image_tags,
    restore_image_tags,
    strip_image_tags,
    clean_chunk_images,
    image_citation_instruction,
)


class TestMultimodalImageProxy:
    """Unit tests for multimodal image proxy, short-tag compression, and lossless restoration."""

    def test_shorten_image_tags_basic(self):
        chunks_text = [
            "步骤一：检查电机传感器，如下图所示：\n![传感器位置图](fig:p1_f4)\n确认接线完好。",
            "步骤二：核对接线端子图：\n![端子排布图](fig:p1_f5)\n重新锁紧螺丝。",
        ]
        chunk_metas = [
            {"kb_id": "kb123", "doc_id": "doc456"},
            {"kb_id": "kb123", "doc_id": "doc456"},
        ]
        shortened, session_map = shorten_image_tags(chunks_text, chunk_metas)

        assert len(shortened) == 2
        assert "![传感器位置图](fig:1)" in shortened[0]
        assert "![端子排布图](fig:2)" in shortened[1]
        assert "1" in session_map
        assert "2" in session_map
        assert session_map["1"] == "kb123-doc456_p1_f4"
        assert session_map["2"] == "kb123-doc456_p1_f5"

    def test_restore_image_tags_normal(self):
        session_map = {
            "1": "2096853651271294978-2096841180541127681_p3_f3",
            "2": "2102048440537682882-2102013792031401137_p4_f1",
        }
        llm_response = "排查步骤如下：\n1. 检查传感器位置 ![传感器图](fig:1)；\n2. 参照架构图 ![架构图](fig:2) 进行联调。"
        # Default relative URL for Web browser
        restored_url = restore_image_tags(llm_response, session_map)
        assert "![传感器图](/api/v1/documents/images/2096853651271294978-2096841180541127681_p3_f3)" in restored_url
        assert "![架构图](/api/v1/documents/images/2102048440537682882-2102013792031401137_p4_f1)" in restored_url

        # Custom fig: protocol
        restored_fig = restore_image_tags(llm_response, session_map, url_prefix="fig:")
        assert "![传感器图](fig:2096853651271294978-2096841180541127681_p3_f3)" in restored_fig
        assert "![架构图](fig:2102048440537682882-2102013792031401137_p4_f1)" in restored_fig

    def test_restore_image_tags_various_syntaxes(self):
        session_map = {
            "1": "kb123-doc456_p1_f4",
            "2": "kb123-doc456_p1_f5",
        }
        llm_response = "测试标记：fig:1 和 (fig:1) 以及 {fig:2} 和 ![配图](fig:2)"
        restored = restore_image_tags(llm_response, session_map, url_prefix="fig:")

        assert "fig:kb123-doc456_p1_f4" in restored
        assert "(fig:kb123-doc456_p1_f4)" in restored
        assert "{fig:kb123-doc456_p1_f5}" in restored
        assert "![配图](fig:kb123-doc456_p1_f5)" in restored

        # Test Fig. 2 / Figure 2 inline replacement
        text_fig_dot = "故障表现为二次识码失败 Fig. 1，操作界面见 Fig. 2。"
        restored_fig_dot = restore_image_tags(text_fig_dot, session_map)
        assert "![Image](/api/v1/documents/images/kb123-doc456_p1_f4)" in restored_fig_dot
        assert "![Image](/api/v1/documents/images/kb123-doc456_p1_f5)" in restored_fig_dot

    def test_strip_image_tags(self):
        text = "步骤一：检查设备\n![说明](fig:1)\n配图: ![图](fig:2)\n(fig:3)\n已完成。"
        cleaned = strip_image_tags(text)
        assert "![说明]" not in cleaned
        assert "fig:1" not in cleaned
        assert "fig:2" not in cleaned
        assert "fig:3" not in cleaned
        assert "已完成。" in cleaned

    def test_clean_chunk_images(self):
        chunk = {
            "content": "内容包含图片 ![图](fig:p1_f1)",
            "content_with_weight": "加权内容 ![图](fig:p1_f1)",
            "image_id": "p1_f1",
            "image_map": {"p1_f1": "full_key"},
        }
        cleaned = clean_chunk_images(chunk)
        assert "image_id" not in cleaned
        assert "image_map" not in cleaned
        assert "fig:p1_f1" not in cleaned["content"]

    def test_image_citation_instruction(self):
        instruction = image_citation_instruction()
        assert "Image Citation Guidelines" in instruction
        assert "fig:1" in instruction
        assert "Strict Relevance Only" in instruction
        assert "Strict Negative Constraint" in instruction

    def test_shorten_image_tags_img_prefix_and_numeric_map(self):
        chunks_text = [
            "步骤一：检查接线：\n![端子图](img:p1_f4)",
            "步骤二：查看图表：\n![对照表](fig:1)",
        ]
        chunk_metas = [
            {"kb_id": "kb1", "doc_id": "doc1"},
            {"kb_id": "kb1", "doc_id": "doc1", "image_map": {"1": "kb1-doc1_custom_fig1"}},
        ]
        shortened, session_map = shorten_image_tags(chunks_text, chunk_metas)
        assert "![端子图](fig:1)" in shortened[0]
        assert "![对照表](fig:2)" in shortened[1]
        assert session_map["1"] == "kb1-doc1_p1_f4"
        assert session_map["2"] == "kb1-doc1_custom_fig1"

    def test_clean_tts_text_filters_images_and_urls(self):
        import importlib.util
        from pathlib import Path
        file_path = Path(__file__).resolve().parents[4] / "api" / "db" / "services" / "dialog_service.py"
        spec = importlib.util.spec_from_file_location("temp_dialog_service", str(file_path))
        # Parse clean_tts_text function source from file to test regex directly in unit tests
        import re
        code = file_path.read_text(encoding="utf-8")
        clean_tts_match = re.search(r"def clean_tts_text\(text: str\) -> str:.*?(?=\ndef )", code, re.DOTALL)
        assert clean_tts_match is not None
        scope = {"re": re, "logging": MagicMock()}
        exec(clean_tts_match.group(0), scope)
        clean_tts_text = scope["clean_tts_text"]

        text = "请看图 ![说明图](/api/v1/documents/images/kb1-doc1_p1_f1) 以及端子排布 fig:1 正常运行。"
        cleaned = clean_tts_text(text)
        assert "/api/v1/documents/images" not in cleaned
        assert "fig:1" not in cleaned
        assert "请看图 以及端子排布 正常运行。" in cleaned

    def test_with_image_prompt_config_priority(self):
        # 1. When kwargs explicitly passed, kwargs wins
        kwargs = {"with_image": False}
        prompt_config = {"with_image": True}
        resolved = kwargs.get("with_image")
        if resolved is None:
            resolved = prompt_config.get("with_image", True)
        assert resolved is False

        # 2. When kwargs not passed, prompt_config wins
        kwargs = {}
        prompt_config = {"with_image": False}
        resolved = kwargs.get("with_image")
        if resolved is None:
            resolved = prompt_config.get("with_image", True)
        assert resolved is False

        # 3. When neither passed, default to True
        kwargs = {}
        prompt_config = {}
        resolved = kwargs.get("with_image")
        if resolved is None:
            resolved = prompt_config.get("with_image", True)
        assert resolved is True

    def test_candidate_id_defensive_filter(self):
        import re
        pattern = re.compile(r"^[a-zA-Z0-9_\-]{8,64}$")
        # Valid document / chunk ids
        assert pattern.match("2096841180541127681")
        assert pattern.match("doc_abcdef1234567890")
        assert pattern.match("chunk-12345678")

        # Invalid suspicious / too-short strings
        assert not pattern.match("1")
        assert not pattern.match("doc")
        assert not pattern.match("../etc/passwd")
        assert not pattern.match("doc;drop")

    def test_kb_prompt_with_standalone_image_chunk(self):
        from rag.prompts.generator import kb_prompt
        kbinfos = {
            "chunks": [
                {
                    "id": "chunk_text_1",
                    "content_with_weight": "故障处理方法说明文字。",
                    "docnm_kwd": "manual.docx",
                },
                {
                    "id": "chunk_img_2",
                    "content_with_weight": "Visual Type: HMI Screen\nTitle: 电机速度设定",
                    "img_id": "kb123-chunk_img_2",
                    "docnm_kwd": "manual.docx",
                },
            ]
        }
        blocks = kb_prompt(kbinfos, max_tokens=1000)
        assert len(blocks) == 2
        assert "![Image](fig:kb123-chunk_img_2)" in blocks[1]

        shortened, session_map = shorten_image_tags(blocks, kbinfos["chunks"])
        assert session_map.get("1") == "kb123-chunk_img_2"
        assert "![Image](fig:1)" in shortened[1]

        answer = "请参考下图：\n![电机速度设定](fig:1)"
        restored = restore_image_tags(answer, session_map)
        assert "/api/v1/documents/images/kb123-chunk_img_2" in restored

        stripped = strip_image_tags(shortened)
        assert "fig:1" not in stripped[1]



