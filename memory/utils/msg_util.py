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
import re
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


def map_message_to_storage_fields(message: dict) -> dict:
    """
    Map message dictionary fields to Elasticsearch document/Infinity fields.

    :param message: A dictionary containing message details.
    :return: A dictionary formatted for Elasticsearch/Infinity indexing.
    """
    storage_doc = {
        "message_id": message["message_id"],
        "message_type_kwd": message["message_type"],
        "source_id": message["source_id"],
        "memory_id": message["memory_id"],
        "user_id": message["user_id"],
        "agent_id": message["agent_id"],
        "session_id": message["session_id"],
        "valid_at": message["valid_at"],
        "invalid_at": message["invalid_at"],
        "forget_at": message["forget_at"],
        "status_int": 1 if message["status"] else 0,
        "zone_id": 0,
        "content_ltks": message["content"],
        f"content_embed_{len(message['content_embed'])}_vec": message["content_embed"],
    }
    return storage_doc


def get_message_from_storage_doc(doc: dict) -> dict:
    """
    Convert an Elasticsearch/Infinity document back to a message dictionary.

    :param doc: A dictionary representing the Elasticsearch/Infinity document.
    :return: A dictionary formatted as a message.
    """
    embd_field_name = next((key for key in doc.keys() if re.match(r"content_embed_\d+_vec", key)), None)
    message = {
        "message_id": doc["message_id"],
        "message_type": doc["message_type_kwd"],
        "source_id": doc["source_id"] if doc["source_id"] else None,
        "memory_id": doc["memory_id"],
        "user_id": doc.get("user_id", ""),
        "agent_id": doc["agent_id"],
        "session_id": doc["session_id"],
        "zone_id": doc.get("zone_id", 0),
        "valid_at": doc["valid_at"],
        "invalid_at": doc.get("invalid_at", "-"),
        "forget_at": doc.get("forget_at", "-"),
        "status": bool(int(doc["status_int"])),
        "content": doc.get("content_ltks", ""),
        "content_embed": doc.get(embd_field_name, []) if embd_field_name else [],
    }
    if doc.get("id"):
        message["id"] = doc["id"]
    return message
