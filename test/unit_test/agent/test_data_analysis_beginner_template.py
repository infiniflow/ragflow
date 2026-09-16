import json
from pathlib import Path


def _data_analysis_prompts(value):
    if isinstance(value, dict):
        for key, child in value.items():
            if key == "sys_prompt" and isinstance(child, str) and "Data Analyst AI assistant" in child:
                yield child
            else:
                yield from _data_analysis_prompts(child)
    elif isinstance(value, list):
        for child in value:
            yield from _data_analysis_prompts(child)


def test_data_analysis_template_keeps_codeexec_prompt_copies_in_sync():
    template_path = Path(__file__).parents[3] / "agent" / "templates" / "data_analysis_beginner_assistant.json"
    template = json.loads(template_path.read_text())
    prompts = list(_data_analysis_prompts(template))

    assert len(prompts) == 2
    assert prompts[0] == prompts[1]
    assert all("CodeExec` (Python)" in prompt for prompt in prompts)
    assert all("Python/SQL/R" not in prompt for prompt in prompts)
    assert all("programmaticallyand" not in prompt for prompt in prompts)
