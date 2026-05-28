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

DASHSCOPE_PROVIDERS = {"Tongyi-Qianwen", "Dashscope"}
GPUSTACK_PROVIDER = "GPUStack"


def is_qwen3_thinking_model(model_name: str) -> bool:
    name = (model_name or "").lower()
    return "qwen3" in name and "thinking" in name


def detect_thinking_family(provider: str | None = None) -> str:
    provider = provider or ""

    if provider == GPUSTACK_PROVIDER:
        return "gpustack_multi_engine"
    if provider in DASHSCOPE_PROVIDERS:
        return "dashscope"
    if provider == "Gemini":
        return "gemini"
    return "openai_thinking"


def apply_enable_thinking_policy(
    model_name: str,
    provider: str | None,
    gen_conf: dict | None,
) -> tuple[dict, dict]:
    """Map gen_conf.reasoning (model thinking) to model-family specific request params."""
    conf = dict(gen_conf or {})
    thinking_enabled = conf.pop("reasoning", None)
    if thinking_enabled is None:
        return conf, {}

    family = detect_thinking_family(provider)
    if (
        not thinking_enabled
        and is_qwen3_thinking_model(model_name)
        and family in {"dashscope", "gpustack_multi_engine"}
    ):
        return conf, {}

    if family == "gemini":
        if thinking_enabled:
            conf.setdefault("thinking_budget", 1024)
        else:
            conf["thinking_budget"] = 0
        return conf, {}

    extra_body: dict = {}
    if family in {"gpustack_multi_engine", "openai_thinking"}:
        extra_body["thinking"] = {"type": "enabled" if thinking_enabled else "disabled"}
        if family == "gpustack_multi_engine":
            extra_body["chat_template_kwargs"] = {
                "enable_thinking": thinking_enabled,
                "thinking": thinking_enabled,
            }
    else:
        extra_body["enable_thinking"] = thinking_enabled

    return conf, {"extra_body": extra_body}
