#!/usr/bin/env python3
"""Update SILICONFLOW models in conf/llm_factories.json.

Usage:
    SILICONFLOW_API_KEY=sk-... ./scripts/update_siliconflow_models.py
    ./scripts/update_siliconflow_models.py --input /tmp/siliconflow_models.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from urllib.parse import urlencode
import urllib.request
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_FACTORIES_PATH = REPO_ROOT / "conf" / "llm_factories.json"
SILICONFLOW_MODELS_URL = "https://api.siliconflow.cn/v1/models"

DEFAULT_SUB_TYPES = (
    "chat",
    "embedding",
    "reranker",
    "text-to-image",
    "image-to-image",
    "speech-to-text",
    "text-to-video",
)

# RAGFlow currently does not have model_type values for image/video generation.
UNSUPPORTED_GENERATION_SUBTYPES = {
    "text-to-image",
    "image-to-image",
    "image-to-video",
    "text-to-video",
}

MAX_TOKENS_OVERRIDES = {
    "deepseek-ai/DeepSeek-V4-Pro": 1000000,
    "deepseek-ai/DeepSeek-V4-Flash": 1000000,
    "MiniMaxAI/MiniMax-M2.5": 197000,
    "Pro/MiniMaxAI/MiniMax-M2.5": 197000,
    "Pro/zai-org/GLM-5.1": 205000,
    "Pro/zai-org/GLM-5": 205000,
    "Pro/zai-org/GLM-4.7": 205000,
    "Pro/moonshotai/Kimi-K2.6": 262000,
    "Pro/moonshotai/Kimi-K2.5": 262000,
    "stepfun-ai/Step-3.5-Flash": 262000,
    "deepseek-ai/DeepSeek-V3.2": 164000,
    "Pro/deepseek-ai/DeepSeek-V3.2": 164000,
    "deepseek-ai/DeepSeek-V3.1-Terminus": 164000,
    "Pro/deepseek-ai/DeepSeek-V3.1-Terminus": 164000,
    "Qwen/Qwen2.5-72B-Instruct-128K": 128000,
}

SILICONFLOW_IMAGE2TEXT_CHAT_MODELS = {
    "Pro/moonshotai/Kimi-K2.6",
    "PaddlePaddle/PaddleOCR-VL-1.5",
    "Qwen/Qwen3-VL-32B-Instruct",
    "Qwen/Qwen3-VL-32B-Thinking",
    "Qwen/Qwen3-VL-8B-Instruct",
    "Qwen/Qwen3-VL-8B-Thinking",
    "Qwen/Qwen3-VL-30B-A3B-Instruct",
    "Qwen/Qwen3-VL-30B-A3B-Thinking",
    "Qwen/Qwen3-Omni-30B-A3B-Instruct",
    "Qwen/Qwen3-Omni-30B-A3B-Thinking",
    "Qwen/Qwen3-Omni-30B-A3B-Captioner",
    "deepseek-ai/DeepSeek-OCR",
    "zai-org/GLM-4.5V",
    "Pro/moonshotai/Kimi-K2.5",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Update the SILICONFLOW provider models in conf/llm_factories.json."
    )
    parser.add_argument(
        "--input",
        type=Path,
        help="Path to a saved SiliconFlow /v1/models JSON response. If omitted, fetch online.",
    )
    parser.add_argument(
        "--factories",
        type=Path,
        default=DEFAULT_FACTORIES_PATH,
        help=f"Path to llm_factories.json. Defaults to {DEFAULT_FACTORIES_PATH}.",
    )
    parser.add_argument(
        "--api-key",
        default=os.getenv("SILICONFLOW_API_KEY"),
        help="SiliconFlow API key. Defaults to SILICONFLOW_API_KEY. Never written to disk.",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print the converted model list summary without writing llm_factories.json.",
    )
    parser.add_argument(
        "--sub-types",
        nargs="+",
        default=list(DEFAULT_SUB_TYPES),
        help="SiliconFlow sub_type query values to fetch in online mode.",
    )
    return parser.parse_args()


def load_models_response(args: argparse.Namespace) -> dict[str, Any]:
    if args.input:
        text = args.input.read_text()
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            json_start = text.find("{")
            if json_start < 0:
                raise
            return json.loads(text[json_start:])

    if not args.api_key:
        raise SystemExit(
            "Missing SiliconFlow API key. Set SILICONFLOW_API_KEY or pass --api-key, "
            "or use --input with a saved /v1/models response."
        )

    models_by_id: dict[str, dict[str, Any]] = {}
    for sub_type in args.sub_types:
        response = fetch_models(args.api_key, {"sub_type": sub_type})
        items = response.get("data") or []
        if not isinstance(items, list):
            print(
                f"skip sub_type={sub_type}: unexpected data={items!r}",
                file=sys.stderr,
            )
            continue
        if not items:
            print(f"skip sub_type={sub_type}: empty response", file=sys.stderr)
        for model in items:
            model_id = get_model_id(model)
            model["_siliconflow_sub_type"] = sub_type
            models_by_id[model_id] = model
    return {"object": "list", "data": list(models_by_id.values())}


def fetch_models(api_key: str, params: dict[str, str] | None = None) -> dict[str, Any]:
    url = SILICONFLOW_MODELS_URL
    if params:
        url = f"{url}?{urlencode(params)}"
    request = urllib.request.Request(
        url,
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.loads(response.read().decode("utf-8"))


def get_model_id(model: dict[str, Any]) -> str:
    model_id = model.get("id")
    if not isinstance(model_id, str) or not model_id:
        raise ValueError(f"Invalid model id in response item: {model!r}")
    return model_id


def infer_sub_type(model: dict[str, Any]) -> str:
    queried_sub_type = model.get("_siliconflow_sub_type")
    if isinstance(queried_sub_type, str) and queried_sub_type:
        return queried_sub_type

    sub_type = model.get("sub_type")
    if isinstance(sub_type, str) and sub_type:
        return sub_type

    name = get_model_id(model)
    lower = name.lower()
    if "embedding" in lower or lower in {"baai/bge-m3", "pro/baai/bge-m3"} or "bge-large" in lower or "bce-embedding" in lower:
        return "embedding"
    if "reranker" in lower or "rerank" in lower or "bce-reranker" in lower:
        return "reranker"
    if "sensevoicesmall" in lower or "telespeechasr" in lower:
        return "speech-to-text"
    if "cosyvoice" in lower or "moss-ttsd" in lower:
        return "text-to-speech"
    if any(token in lower for token in ("z-image", "ernie-image", "qwen-image", "kolors")):
        return "text-to-image"
    if "wan2.2-i2v" in lower:
        return "image-to-video"
    if "wan2.2-t2v" in lower:
        return "text-to-video"
    if any(token in lower for token in ("vl", "ocr", "4.5v")):
        return "image-to-text"
    return "chat"


def to_ragflow_model_type(model: dict[str, Any]) -> str | None:
    sub_type = infer_sub_type(model)
    if sub_type in UNSUPPORTED_GENERATION_SUBTYPES:
        return None
    if sub_type == "chat":
        if model.get("type") == "image" or is_vision_chat_model(get_model_id(model)):
            return "image2text"
        return "chat"
    if sub_type == "embedding":
        return "embedding"
    if sub_type == "reranker":
        return "rerank"
    if sub_type == "speech-to-text":
        return "speech2text"
    if sub_type == "text-to-speech":
        return "tts"
    if sub_type == "image-to-text":
        return "image2text"
    return "chat"


def is_vision_chat_model(name: str) -> bool:
    return name in SILICONFLOW_IMAGE2TEXT_CHAT_MODELS


def max_tokens_for(name: str, model_type: str) -> int:
    lower = name.lower()
    if name in MAX_TOKENS_OVERRIDES:
        return MAX_TOKENS_OVERRIDES[name]
    if model_type in {"embedding", "rerank"}:
        if "qwen3" in lower:
            return 32000
        if "bge-m3" in lower or "reranker-v2-m3" in lower:
            return 8192
        return 512
    if model_type == "speech2text":
        return 26214400
    if model_type == "tts":
        return 2048
    if model_type == "image2text":
        return 128000 if "qwen3-vl" in lower or "omni" in lower else 32000
    if any(token in lower for token in ("qwen3.6", "qwen3.5", "qwen3-32b", "qwen3-30b", "qwen3-coder")):
        return 128000
    if "qwen3-14b" in lower or "qwen3-8b" in lower:
        return 128000
    if "qwen2.5" in lower:
        return 32000
    if "deepseek-r1" in lower or "deepseek-v3" in lower:
        return 64000
    return 32000


def tag_size(tokens: int) -> str:
    if tokens >= 1000000:
        return "1M"
    if tokens >= 1000:
        return f"{tokens // 1000}k"
    return str(tokens)


def tags_for(model_type: str, tokens: int) -> str:
    if model_type == "embedding":
        return f"TEXT EMBEDDING,{tag_size(tokens)}"
    if model_type == "rerank":
        return f"TEXT RE-RANK,{tag_size(tokens)}"
    if model_type == "image2text":
        return f"LLM,IMAGE2TEXT,{tag_size(tokens)}"
    if model_type == "speech2text":
        return "SPEECH2TEXT"
    if model_type == "tts":
        return "TTS"
    return f"LLM,CHAT,{tag_size(tokens)}"


def convert_models(response: dict[str, Any]) -> tuple[list[dict[str, Any]], list[str]]:
    converted = []
    skipped = []
    for model in response.get("data", []):
        name = get_model_id(model)
        model_type = to_ragflow_model_type(model)
        if model_type is None:
            skipped.append(name)
            continue

        max_tokens = max_tokens_for(name, model_type)
        converted.append(
            {
                "llm_name": name,
                "tags": tags_for(model_type, max_tokens),
                "max_tokens": max_tokens,
                "model_type": ["image2text", "chat"] if model_type == "image2text" else model_type,
                "is_tools": model_type == "chat",
            }
        )
    return converted, skipped


def update_factories(path: Path, models: list[dict[str, Any]]) -> None:
    data = json.loads(path.read_text())
    for factory in data["factory_llm_infos"]:
        if factory.get("name") == "SILICONFLOW":
            factory["tags"] = "LLM,TEXT EMBEDDING,TEXT RE-RANK,IMAGE2TEXT,TTS,SPEECH2TEXT"
            factory["llm"] = models
            path.write_text(json.dumps(data, ensure_ascii=False, indent=4) + "\n")
            return
    raise SystemExit("SILICONFLOW provider not found in llm_factories.json")


def print_summary(models: list[dict[str, Any]], skipped: list[str]) -> None:
    counts: dict[str, int] = {}
    for model in models:
        model_type = model["model_type"]
        model_types = model_type if isinstance(model_type, list) else [model_type]
        for item in model_types:
            counts[item] = counts.get(item, 0) + 1
    print(f"converted={len(models)} skipped={len(skipped)}")
    print("model_type_counts=" + json.dumps(counts, ensure_ascii=False, sort_keys=True))
    if skipped:
        print("skipped_unsupported=" + json.dumps(skipped, ensure_ascii=False))


def main() -> int:
    args = parse_args()
    response = load_models_response(args)
    models, skipped = convert_models(response)
    print_summary(models, skipped)
    if not args.dry_run:
        update_factories(args.factories, models)
        json.loads(args.factories.read_text())
    return 0


if __name__ == "__main__":
    sys.exit(main())
