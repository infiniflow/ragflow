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
from pathlib import Path


def _load_template():
    repo_root = Path(__file__).resolve().parents[1]
    template_path = repo_root / "agent" / "templates" / "data_analysis_beginner_assistant.json"
    return json.loads(template_path.read_text(encoding="utf-8"))


def test_data_analysis_template_keeps_agent_prompts_in_sync():
    template = _load_template()
    agent_params = template["dsl"]["components"]["Agent:SillyStatesRun"]["obj"]["params"]
    agent_node = next(node for node in template["dsl"]["graph"]["nodes"] if node["id"] == "Agent:SillyStatesRun")
    prompt = agent_params["sys_prompt"]

    assert prompt == agent_node["data"]["form"]["sys_prompt"]
    assert "programmatically and" in prompt
    assert "programmaticallyand" not in prompt
    assert "CodeExec` (Python)" in prompt
    assert "CodeExec` (Python/SQL/R)" not in prompt
