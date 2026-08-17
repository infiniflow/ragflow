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

from copy import deepcopy

GENERATION_CONFIG_KEYS = ("temperature", "top_p", "frequency_penalty", "presence_penalty", "max_tokens")

# Default values for the four LLM generation parameters stored in
# search_config.llm_setting.  When the corresponding ``_enabled`` flag is
# ``False`` (or the key is absent), the default is used instead of whatever
# the user may have stored in ``llm_setting``.
LLM_SETTING_DEFAULTS = {
    "temperature": 0.1,
    "top_p": 0.3,
    "frequency_penalty": 0.7,
    "presence_penalty": 0.4,
}


def resolve_llm_setting(llm_setting):
    """Resolve *llm_setting* values according to their enable flags.

    For each of the four generation parameters the dictionary may carry a
    ``{key}_enabled`` boolean.  When the flag is ``True`` and the value
    exists in *llm_setting*, the user-configured value is kept; otherwise
    the default from :data:`LLM_SETTING_DEFAULTS` is substituted.

    Keys whose name ends with ``_enabled`` are stripped from the result so
    they are never forwarded to the downstream LLM call.
    """
    if not llm_setting:
        return dict(LLM_SETTING_DEFAULTS)

    resolved = {}
    for key, default_val in LLM_SETTING_DEFAULTS.items():
        enabled_key = f"{key}_enabled"
        if llm_setting.get(enabled_key, True) and key in llm_setting:
            resolved[key] = llm_setting[key]
        else:
            resolved[key] = default_val

    # Carry over any extra keys that are not generation parameters and not
    # enable flags (e.g. ``llm_id``, ``model_type``).
    for key, val in llm_setting.items():
        if key not in resolved and not key.endswith("_enabled"):
            resolved[key] = val

    return resolved


def extract_generation_config(req):
    return {key: req[key] for key in GENERATION_CONFIG_KEYS if key in req and req[key] is not None}


def pop_generation_config(req):
    generation_config = extract_generation_config(req)
    for key in GENERATION_CONFIG_KEYS:
        req.pop(key, None)
    return generation_config


def merge_generation_config(dialog, generation_config):
    if not generation_config:
        return
    llm_setting = deepcopy(getattr(dialog, "llm_setting", None) or {})
    llm_setting.update(generation_config)
    dialog.llm_setting = llm_setting
