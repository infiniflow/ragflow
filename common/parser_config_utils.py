#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

from typing import Any


def normalize_layout_recognizer(layout_recognizer_raw: Any) -> tuple[Any, str | None]:
    parser_model_name: str | None = None
    layout_recognizer = layout_recognizer_raw

    if isinstance(layout_recognizer_raw, str):
        lowered = layout_recognizer_raw.lower()
        if lowered.endswith("@mineru"):
            parser_model_name = layout_recognizer_raw
            layout_recognizer = "MinerU"
        elif lowered.endswith("@paddleocr"):
            parser_model_name = layout_recognizer_raw
            layout_recognizer = "PaddleOCR"
        elif lowered.endswith("@opendataloader"):
            parser_model_name = layout_recognizer_raw
            layout_recognizer = "OpenDataLoader"
        elif lowered.endswith("@somark"):
            # Keep the full 3-segment form ``<llm_name>@<instance_name>@<provider>``
            # produced by the new Tenant LLM Provider UI (#14595); downstream
            # ``get_model_config_from_provider_instance`` -> ``split_model_name``
            # expects all three segments to locate the provider/instance row.
            parser_model_name = layout_recognizer_raw
            layout_recognizer = "SoMark"

    return layout_recognizer, parser_model_name
