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

import logging
from typing import Dict, List, Optional
from datetime import datetime

from api.db.db_models import ConversationMemory
from api.db.services.common_service import CommonService
from api.db.services.llm_service import TenantLLMService
from api.utils import get_uuid
from mem0 import Memory
from api.db import LLMType
import os
# from langchain_voyageai import VoyageAIEmbeddings



class MemoryService(CommonService):
    model = ConversationMemory
    
    def __init__(self):
        self._memory_clients = {}  # Cache for memory clients per tenant

    def _get_memory_config(self, tenant_id: str) -> Dict:
        """Get memory configuration for a tenant, using their LLM settings"""
        try:
            # Get the tenant's chat LLM configuration for memory
            model_config = TenantLLMService.get_model_config(tenant_id, LLMType.CHAT.value)
            print(f"Model config for tenant {tenant_id}: {model_config}")  # Debugging line to check model config
            
            # Get the tenant's embedding model configuration
            embedding_config = TenantLLMService.get_model_config(tenant_id, LLMType.EMBEDDING.value)
            print(f"Embedding config for tenant {tenant_id}: {embedding_config}")  # Debugging line to check embedding config
            
            config = {
                "llm": {
                    "provider": "openai",  # Default to openai format
                    "config": {
                        "model": model_config.get("llm_name", "gpt-3.5-turbo"),
                        "temperature": 0.2,
                        "max_tokens": 2000,
                    }
                }
            }
            
            # Map different providers to appropriate config for LLM
            factory = model_config.get("llm_factory", "OpenAI")
            print(f"Factory for tenant {tenant_id}: {factory}")  # Debugging line to check factory
            if factory == "OpenAI":
                config["llm"]["provider"] = "openai"
                config["llm"]["config"]["api_key"] = model_config.get("api_key")
                if model_config.get("api_base"):
                    config["llm"]["config"]["base_url"] = model_config.get("api_base")
            elif factory in ["Tongyi-Qianwen", "QWen"]:
                config["llm"]["provider"] = "qwen"
                config["llm"]["config"]["api_key"] = model_config.get("api_key")
            elif factory == "Zhipu":
                config["llm"]["provider"] = "zhipuai"
                config["llm"]["config"]["api_key"] = model_config.get("api_key")
            elif factory == "DeepSeek":
                config["llm"]["provider"] = "deepseek"
                config["llm"]["config"]["model"] = model_config.get("llm_name", "deepseek-chat")
                config["llm"]["config"]["api_key"] = model_config.get("api_key")
                config["llm"]["config"]["deepseek_base_url"] = "https://api.deepseek.com"
            else:
                # For other providers, try OpenAI-compatible format
                config["llm"]["provider"] = "openai"
                config["llm"]["config"]["api_key"] = model_config.get("api_key")
                if model_config.get("api_base"):
                    config["llm"]["config"]["base_url"] = model_config.get("api_base")

        #     # Map different providers to appropriate config for Embedding
        #     embedding_factory = embedding_config.get("llm_factory", "OpenAI")
        #     print(f"Embedding factory for tenant {tenant_id}: {embedding_factory}")  # Debugging line to check embedding factory
        #     if embedding_factory == "OpenAI":
        #         config["embedder"]["provider"] = "openai"
        #         config["embedder"]["config"]["api_key"] = embedding_config.get("api_key")
        #         if embedding_config.get("api_base"):
        #             config["embedder"]["config"]["base_url"] = embedding_config.get("api_base")
        #     elif embedding_factory in ["Tongyi-Qianwen", "QWen"]:
        #         config["embedder"]["provider"] = "dashscope"
        #         config["embedder"]["config"]["api_key"] = embedding_config.get("api_key")
        #     elif embedding_factory == "Zhipu":
        #         config["embedder"]["provider"] = "zhipuai"
        #         config["embedder"]["config"]["api_key"] = embedding_config.get("api_key")
        #     elif embedding_factory == "Voyage AI":
        #         # Initialize a LangChain embeddings model directly
        #         config["embedder"]["provider"] = "langchain"
        #         config["embedder"]["model"] = VoyageAIEmbeddings(
        #             api_key=embedding_config.get("api_key"),
        #             model_name=embedding_config.get("llm_name", "voyageai-embed-text")
        #         )
        #     elif embedding_factory == "Ollama":
        #         config["embedder"]["provider"] = "ollama"
        #         config["embedder"]["config"]["model"] = embedding_config.get("llm_name", "nomic-embed-text")
        #         if embedding_config.get("api_base"):
        #             config["embedder"]["config"]["base_url"] = embedding_config.get("api_base", "http://localhost:11434")
        #     elif embedding_factory in ["BAAI", "HuggingFace"]:
        #         config["embedder"]["provider"] = "huggingface"
        #         config["embedder"]["config"]["model"] = embedding_config.get("llm_name", "BAAI/bge-small-en")
        #     else:
        #         # For other providers, try OpenAI-compatible format
        #         config["embedder"]["provider"] = "openai"
        #         config["embedder"]["config"]["api_key"] = embedding_config.get("api_key")
        #         config["embedder"]["config"]["model"] = embedding_config.get("llm_name", "text-embedding-ada-002")
        #         if embedding_config.get("api_base"):
        #             config["embedder"]["config"]["base_url"] = embedding_config.get("api_base")

            return config
            
        except Exception as e:
            logging.error(f"Failed to get memory config for tenant {tenant_id}: {str(e)}")
            # Return a basic config as fallback
            return {
                "llm": {
                    "provider": "openai",
                    "config": {
                        "model": "gpt-3.5-turbo",
                        "temperature": 0.2,
                        "max_tokens": 2000,
                    }
                },
            }

    def _get_memory_client(self, tenant_id: str) -> Memory:
        """Get or create a memory client for a tenant"""
        if tenant_id not in self._memory_clients:
            config = self._get_memory_config(tenant_id)
            try:
                print(f"Creating memory client for tenant {tenant_id} with config: {config}")  # Debugging line to check config
                self._memory_clients[tenant_id] = Memory.from_config(config)
            except Exception as e:
                logging.error(f"Failed to create memory client for tenant {tenant_id}: {str(e)}")
                raise
        return self._memory_clients[tenant_id]

    def add_memory(self, tenant_id: str, user_id: str, dialog_id: str, message: str, metadata: Optional[Dict] = None) -> Optional[str]:
        """Add a memory for a user in a specific dialog"""
        try:
            memory_client = self._get_memory_client(tenant_id)
            
            # Create memory with mem0
            memory_result = memory_client.add(
                messages=[{"role": "user", "content": message}],
                user_id=f"{dialog_id}_{user_id}",
                metadata=metadata or {}
            )
            print(f"Memory result: {memory_result}")  # Debugging line to check memory result

            # Store memory reference in database
            memory_id = memory_result.get("id") if memory_result else None
            if memory_id:
                conversation_memory = ConversationMemory(
                    id=get_uuid(),
                    dialog_id=dialog_id,
                    user_id=user_id,
                    memory_id=memory_id,
                    content=message,
                    metadata=metadata or {},
                    relevance_score=0.0
                )
                conversation_memory.save()
                
            return memory_id
            
        except Exception as e:
            logging.error(f"Failed to add memory: {str(e)}")
            return None

    def get_relevant_memories(self, tenant_id: str, user_id: str, dialog_id: str, query: str, limit: int = 5) -> List[Dict]:
        """Get relevant memories for a user query"""
        try:
            memory_client = self._get_memory_client(tenant_id)
            
            # Search memories using mem0
            search_result = memory_client.search(
                query=query,
                user_id=f"{dialog_id}_{user_id}",
                limit=limit
            )
            
            # Extract memories from the search result
            memories = search_result.get("results", []) if isinstance(search_result, dict) else []
            
            relevant_memories = []
            for memory in memories:
                relevant_memories.append({
                    "id": memory.get("id"),
                    "content": memory.get("memory", ""),
                    "score": memory.get("score", 0.0),
                    "metadata": memory.get("metadata", {}),
                    "created_at": memory.get("created_at")
                })
                
            return relevant_memories
            
        except Exception as e:
            logging.error(f"Failed to get relevant memories: {str(e)}")
            return []

    def get_all_memories(self, tenant_id: str, user_id: str, dialog_id: str) -> List[Dict]:
        """Get all memories for a user in a specific dialog"""
        try:
            memory_client = self._get_memory_client(tenant_id)
            memories_response = memory_client.get_all(user_id=f"{dialog_id}_{user_id}")

            # Extract the actual memories from the response
            if isinstance(memories_response, dict) and "results" in memories_response:
                memories = memories_response["results"]
            else:
                memories = memories_response if isinstance(memories_response, list) else []

            return [
                {
                    "id": memory.get("id"),
                    "content": memory.get("memory", ""),
                    "metadata": memory.get("metadata", {}),
                    "created_at": memory.get("created_at")
                }
                for memory in memories
            ]
            
        except Exception as e:
            logging.error(f"Failed to get all memories: {str(e)}")
            return []

    def delete_memory(self, tenant_id: str, user_id: str, dialog_id: str, memory_id: str) -> bool:
        """Delete a specific memory"""
        try:
            memory_client = self._get_memory_client(tenant_id)
            
            # Delete from mem0
            memory_client.delete(memory_id=memory_id)
            
            # Delete from database
            ConversationMemory.delete().where(
                ConversationMemory.memory_id == memory_id,
                ConversationMemory.user_id == user_id,
                ConversationMemory.dialog_id == dialog_id
            ).execute()
            
            return True
            
        except Exception as e:
            logging.error(f"Failed to delete memory: {str(e)}")
            return False

    def clear_memories(self, tenant_id: str, user_id: str, dialog_id: str) -> bool:
        """Clear all memories for a user in a specific dialog"""
        try:
            memory_client = self._get_memory_client(tenant_id)
            
            # Delete all memories from mem0
            memories = memory_client.get_all(user_id=f"{dialog_id}_{user_id}")
            for memory in memories:
                memory_client.delete(memory_id=memory.get("id"))
            
            # Delete from database
            ConversationMemory.delete().where(
                ConversationMemory.user_id == user_id,
                ConversationMemory.dialog_id == dialog_id
            ).execute()
            
            return True
            
        except Exception as e:
            logging.error(f"Failed to clear memories: {str(e)}")
            return False

    def update_memory(self, tenant_id: str, user_id: str, dialog_id: str, memory_id: str, content: str) -> bool:
        """Update a specific memory"""
        try:
            memory_client = self._get_memory_client(tenant_id)
            
            # Update memory in mem0
            memory_client.update(memory_id=memory_id, data=content)
            
            # Update in database
            ConversationMemory.update(content=content).where(
                ConversationMemory.memory_id == memory_id,
                ConversationMemory.user_id == user_id,
                ConversationMemory.dialog_id == dialog_id
            ).execute()
            
            return True
            
        except Exception as e:
            logging.error(f"Failed to update memory: {str(e)}")
            return False

    def get_memory_stats(self, tenant_id: str, user_id: str, dialog_id: str) -> Dict:
        """Get memory statistics for a user in a specific dialog"""
        try:
            memories = self.get_all_memories(tenant_id, user_id, dialog_id)
            return {
                "total_memories": len(memories),
                "enabled": True,
                "last_updated": datetime.now().isoformat() if memories else None
            }
            
        except Exception as e:
            logging.error(f"Failed to get memory stats: {str(e)}")
            return {"total_memories": 0, "enabled": False, "error": str(e)}

# Global memory service instance
memory_service = MemoryService()
