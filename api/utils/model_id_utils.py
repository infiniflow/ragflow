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
MODEL_ID_FIELDS = {
    "llm_id",
    "embd_id",
    "rerank_id",
    "asr_id",
    "img2txt_id",
    "tts_id",
    "ocr_id",
    "embedding_model",
}
SEARCH_CONFIG_MODEL_ID_FIELDS = {"chat_id"}


def normalize_model_id_for_response(model_id):
    if not isinstance(model_id, str) or not model_id:
        return model_id

    parts = model_id.split("@")
    if len(parts) == 2 and all(parts):
        return f"{parts[0]}@default@{parts[1]}"
    return model_id


def normalize_model_ids_for_response(data):
    if isinstance(data, dict):
        normalized = {}
        for key, value in data.items():
            if key == "search_config" and isinstance(value, dict):
                normalized[key] = normalize_model_ids_for_response_with_extra_fields(value, SEARCH_CONFIG_MODEL_ID_FIELDS)
            elif key in MODEL_ID_FIELDS:
                normalized[key] = normalize_model_id_for_response(value)
            else:
                normalized[key] = normalize_model_ids_for_response(value)
        return normalized

    if isinstance(data, list):
        return [normalize_model_ids_for_response(item) for item in data]

    if isinstance(data, tuple):
        return tuple(normalize_model_ids_for_response(item) for item in data)

    return data


def normalize_model_ids_for_response_with_extra_fields(data, extra_model_id_fields):
    if isinstance(data, dict):
        normalized = {}
        model_id_fields = MODEL_ID_FIELDS | set(extra_model_id_fields)
        for key, value in data.items():
            if key in model_id_fields:
                normalized[key] = normalize_model_id_for_response(value)
            else:
                normalized[key] = normalize_model_ids_for_response(value)
        return normalized

    if isinstance(data, list):
        return [normalize_model_ids_for_response_with_extra_fields(item, extra_model_id_fields) for item in data]

    if isinstance(data, tuple):
        return tuple(normalize_model_ids_for_response_with_extra_fields(item, extra_model_id_fields) for item in data)

    return data
