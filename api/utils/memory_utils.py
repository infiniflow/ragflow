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

from typing import Dict, List, Optional


def format_memory_context(memories: List[Dict]) -> str:
    """Format memories into a context string for LLM"""
    if not memories:
        return ""
    
    formatted_memories = []
    for memory in memories:
        content = memory.get("content", "")
        score = memory.get("score", 0.0)
        if content:
            formatted_memories.append(f"- {content} (relevance: {score:.2f})")
    
    if not formatted_memories:
        return ""
    
    return f"""
### Previous Context from Memory:
{chr(10).join(formatted_memories)}

The above information represents relevant context from previous conversations. Use this context to provide more personalized and coherent responses.
"""


def should_store_memory(message: str, response: str, config: Dict) -> bool:
    """Determine if a conversation should be stored as memory"""
    if not config.get("enabled", False):
        return False
    
    # Basic filtering criteria
    min_length = config.get("min_message_length", 10)
    if len(message) < min_length or len(response) < min_length:
        return False
    
    # Skip if message is too generic
    generic_patterns = ["hello", "hi", "thank you", "thanks", "bye", "goodbye"]
    if message.lower().strip() in generic_patterns:
        return False
    
    return True


def extract_memory_content(message: str, response: str) -> str:
    """Extract meaningful content to store as memory"""
    # For now, store the user message and a summary of the response
    return f"User asked: {message}\nContext: {response[:200]}..."


def validate_memory_config(config: Dict) -> Dict:
    """Validate and normalize memory configuration"""
    default_config = {
        "enabled": True,
        "max_memories": 5,
        "threshold": 0.7,
        "store_interval": 3,
        "min_message_length": 10
    }
    
    if not isinstance(config, dict):
        return default_config
    
    # Validate and set defaults
    validated_config = default_config.copy()
    validated_config.update(config)
    
    # Ensure numeric values are within reasonable bounds
    validated_config["max_memories"] = max(1, min(validated_config["max_memories"], 50))
    validated_config["threshold"] = max(0.0, min(validated_config["threshold"], 1.0))
    validated_config["store_interval"] = max(1, min(validated_config["store_interval"], 100))
    validated_config["min_message_length"] = max(1, min(validated_config["min_message_length"], 1000))
    
    return validated_config
