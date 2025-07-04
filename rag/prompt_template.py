import os

BASE_DIR = os.path.dirname(__file__)

PROMPT_DIR = os.path.join(BASE_DIR, "prompts")

_loaded_prompts = {}


def load_prompt(name: str) -> str:
    if name in _loaded_prompts:
        return _loaded_prompts[name]

    path = os.path.join(PROMPT_DIR, f"{name}.md")
    if not os.path.isfile(path):
        raise FileNotFoundError(f"Prompt file '{name}.md' not found in prompts/ directory.")

    with open(path, "r", encoding="utf-8") as f:
        content = f.read().strip()
        _loaded_prompts[name] = content
        return content
