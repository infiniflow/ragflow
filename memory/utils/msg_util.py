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
import json


def get_json_result_from_llm_response(response_str: str) -> dict:
    """
    Parse the LLM response string to extract JSON content.
    The function looks for the first and last curly braces to identify the JSON part.
    If parsing fails, it returns an empty dictionary.

    :param response_str: The response string from the LLM.
    :return: A dictionary parsed from the JSON content in the response.
    """
    try:
        clean_str = response_str.strip()
        if clean_str.startswith('```json'):
            clean_str = clean_str[7:]  # Remove the starting ```json
        if clean_str.endswith('```'):
            clean_str = clean_str[:-3]  # Remove the ending ```

        return json.loads(clean_str.strip())
    except (ValueError, json.JSONDecodeError):
        return {}
