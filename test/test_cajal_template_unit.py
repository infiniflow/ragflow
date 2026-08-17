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
import json
import importlib.util
from pathlib import Path


def _load_template_utils():
    repo_root = Path(__file__).resolve().parents[1]
    module_path = repo_root / "api" / "db" / "template_utils.py"
    spec = importlib.util.spec_from_file_location("template_utils", module_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _load_cajal_template():
    repo_root = Path(__file__).resolve().parents[1]
    template_path = repo_root / "agent" / "templates" / "cajal_scientific_paper_agent.json"
    with template_path.open(encoding="utf-8") as template_file:
        return _load_template_utils().normalize_canvas_template_categories(json.load(template_file))


def test_cajal_template_exposes_local_ollama_model_and_agent_categories():
    template = _load_cajal_template()

    assert template["id"] == "41"
    assert template["title"]["en"] == "CAJAL scientific paper agent"
    assert template["canvas_type"] == "Agent"
    assert template["canvas_types"] == ["Agent", "Recommended"]

    agent_params = template["dsl"]["components"]["Agent:NewPumasLick"]["obj"]["params"]
    assert agent_params["llm_id"] == "agnuxo/cajal-4b-p2pclaw@Ollama"
    assert agent_params["max_tokens"] == 32768
    assert "Agnuxo/CAJAL-4B-P2PCLAW" in agent_params["sys_prompt"]
    assert "LaTeX" in agent_params["sys_prompt"]


def test_cajal_template_keeps_retrieval_grounding_and_graph_form_in_sync():
    template = _load_cajal_template()
    agent_params = template["dsl"]["components"]["Agent:NewPumasLick"]["obj"]["params"]
    retrieval_tools = [tool for tool in agent_params["tools"] if tool["component_name"] == "Retrieval"]

    assert len(retrieval_tools) == 1
    assert retrieval_tools[0]["params"]["top_n"] == 10
    assert "ground" in retrieval_tools[0]["params"]["description"].lower()
    assert "{sys.query}" in agent_params["prompts"][0]["content"]

    agent_node = next(node for node in template["dsl"]["graph"]["nodes"] if node["id"] == "Agent:NewPumasLick")
    begin_node = next(node for node in template["dsl"]["graph"]["nodes"] if node["id"] == "begin")

    assert agent_node["data"]["form"]["llm_id"] == agent_params["llm_id"]
    assert agent_node["data"]["form"]["sys_prompt"] == agent_params["sys_prompt"]
    assert "CAJAL" in begin_node["data"]["form"]["prologue"]


def test_cajal_is_registered_as_a_known_ollama_chat_model():
    repo_root = Path(__file__).resolve().parents[1]
    factories_path = repo_root / "conf" / "llm_factories.json"
    factories = json.loads(factories_path.read_text(encoding="utf-8"))
    ollama = next(factory for factory in factories["factory_llm_infos"] if factory["name"] == "Ollama")
    cajal = next(model for model in ollama["llm"] if model["llm_name"] == "agnuxo/cajal-4b-p2pclaw")

    assert cajal["model_type"] == "chat"
    assert cajal["max_tokens"] == 32768
    assert "SCIENTIFIC_WRITING" in cajal["tags"]
