#!/usr/bin/env python3
"""Rename provider name fields in conf/models/*.json to match llm_factories.json."""
import json, os

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
MODELS_DIR = os.path.join(REPO_ROOT, "conf", "models")

# Valid mappings: models current name → llm_factories name
CHANGES = {
    "avian":          "Avian",
    "Baichuan":       "BaiChuan",
    "Baidu":          "BaiduYiyan",
    "CoHere":         "Cohere",
    "FishAudio":      "Fish Audio",
    "Gitee":          "GiteeAI",
    "Google":         "Gemini",
    "JieKouAI":       "Jiekou.AI",
    "lmstudio":       "LM-Studio",
    "localai":        "LocalAI",
    "modelscope":     "ModelScope",
    "Novita":         "NovitaAI",
    "Nvidia":         "NVIDIA",
    "ollama":         "Ollama",
    "SiliconFlow":    "SILICONFLOW",
    "HunYuan":        "Tencent Hunyuan",
    "Aliyun":         "Tongyi-Qianwen",
    "vllm":           "VLLM",
    "Voyage":         "Voyage AI",
    "xinference":     "Xinference",
    "XunFei":         "XunFei Spark",
}

def main():
    # Build current name → filename mapping
    file_map = {}
    for fn in os.listdir(MODELS_DIR):
        if not fn.endswith(".json"):
            continue
        with open(os.path.join(MODELS_DIR, fn)) as f:
            d = json.load(f)
        file_map[d["name"]] = fn

    for old_name, new_name in CHANGES.items():
        fn = file_map[old_name]
        path = os.path.join(MODELS_DIR, fn)
        with open(path) as f:
            d = json.load(f)
        d["name"] = new_name
        with open(path, "w") as f:
            json.dump(d, f, indent=2, ensure_ascii=False)
            f.write("\n")
        print(f'{fn}: "{old_name}" -> "{new_name}"')

if __name__ == "__main__":
    main()
