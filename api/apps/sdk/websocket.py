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
"""
WebSocket SDK API for RAGFlow Streaming Responses

This module provides WebSocket endpoints following the SDK API pattern,
mirroring the structure of session.py for consistency.
"""

import logging
import json
from quart import websocket

# Log WebSocket endpoints registration
logging.info("WebSocket SDK endpoints registered:")
logging.info("  - /api/v1/ws/chats/<chat_id>/completions")
logging.info("  - /api/v1/ws/agents/<agent_id>/completions")

from api.db.services.dialog_service import DialogService
from api.db.services.canvas_service import UserCanvasService
from api.db.services.conversation_service import async_completion as rag_completion
from api.db.services.canvas_service import completion as agent_completion
from api.utils.api_utils import ws_token_required
from common.constants import StatusEnum


async def send_ws_error(error_message, code=500):
    """Send error message to WebSocket client."""
    error_response = {
        "code": code,
        "message": error_message,
        "data": {
            "answer": f"**ERROR**: {error_message}",
            "reference": []
        }
    }
    await websocket.send(json.dumps(error_response, ensure_ascii=False))


async def send_ws_message(data, code=0, message=""):
    """Send message to WebSocket client."""
    response = {
        "code": code,
        "message": message,
        "data": data
    }
    await websocket.send(json.dumps(response, ensure_ascii=False))


@manager.websocket("/ws/chats/<chat_id>/completions")  # noqa: F821
@ws_token_required
async def chat_completions_ws(tenant_id, chat_id):
    """
    WebSocket endpoint for streaming chat completions.
    Follows the same pattern as the HTTP POST /chats/<chat_id>/completions endpoint.
    Uses /ws/ prefix to avoid routing conflicts with HTTP endpoints.
    """
    # Verify chat ownership
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        await send_ws_error(f"You don't own the chat {chat_id}", code=404)
        await websocket.close(1008)
        return
    
    logging.info(f"WebSocket chat connection established for chat_id: {chat_id}, tenant: {tenant_id}")
    
    try:
        while True:
            message = await websocket.receive()
            
            try:
                req = json.loads(message)
            except json.JSONDecodeError as e:
                await send_ws_error(f"Invalid JSON format: {str(e)}", code=400)
                continue
            
            question = req.get("question", "")
            session_id = req.get("session_id")
            stream = req.get("stream", True)
            
            if question is None:
                await send_ws_error("Missing required parameter: question", code=400)
                continue
            
            try:
                if stream:
                    async for response_chunk in rag_completion(
                        tenant_id=tenant_id,
                        chat_id=chat_id,
                        question=question,
                        session_id=session_id,
                        stream=True,
                        **{k: v for k, v in req.items() if k not in ["question", "session_id", "stream"]}
                    ):
                        if response_chunk.startswith("data:"):
                            json_str = response_chunk[5:].strip()
                            try:
                                response_data = json.loads(json_str)
                                await websocket.send(json.dumps(response_data, ensure_ascii=False))
                            except json.JSONDecodeError:
                                continue
                    
                    logging.info(f"Chat completion streamed successfully for chat_id: {chat_id}")
                else:
                    response = None
                    async for resp in rag_completion(
                        tenant_id=tenant_id,
                        chat_id=chat_id,
                        question=question,
                        session_id=session_id,
                        stream=False,
                        **{k: v for k, v in req.items() if k not in ["question", "session_id", "stream"]}
                    ):
                        response = resp
                        break
                    
                    if response:
                        await send_ws_message(response)
                    else:
                        await send_ws_error("No response generated", code=500)
                    
            except Exception as e:
                logging.exception(f"Error during chat completion: {str(e)}")
                await send_ws_error(str(e))
    
    except Exception as e:
        logging.exception(f"WebSocket error: {str(e)}")
        try:
            await send_ws_error(str(e))
        except Exception:
            pass
        await websocket.close(1011)
    
    finally:
        logging.info(f"WebSocket chat connection closed for chat_id: {chat_id}")


@manager.websocket("/ws/agents/<agent_id>/completions")  # noqa: F821
@ws_token_required
async def agent_completions_ws(tenant_id, agent_id):
    """
    WebSocket endpoint for streaming agent completions.
    Follows the same pattern as the HTTP POST /agents/<agent_id>/completions endpoint.
    Uses /ws/ prefix to avoid routing conflicts with HTTP endpoints.
    """
    # Verify agent ownership
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        await send_ws_error(f"You don't own the agent {agent_id}", code=404)
        await websocket.close(1008)
        return
    
    logging.info(f"WebSocket agent connection established for agent_id: {agent_id}, tenant: {tenant_id}")
    
    try:
        while True:
            message = await websocket.receive()
            
            try:
                req = json.loads(message)
            except json.JSONDecodeError as e:
                await send_ws_error(f"Invalid JSON format: {str(e)}", code=400)
                continue
            
            question = req.get("question", "")
            session_id = req.get("session_id")
            stream = req.get("stream", True)
            
            if not question:
                await send_ws_error("Missing required parameter: question", code=400)
                continue
            
            try:
                if stream:
                    async for response_chunk in agent_completion(
                        tenant_id=tenant_id,
                        agent_id=agent_id,
                        question=question,
                        session_id=session_id,
                        stream=True,
                        **{k: v for k, v in req.items() if k not in ["question", "session_id", "stream"]}
                    ):
                        if isinstance(response_chunk, str) and response_chunk.startswith("data:"):
                            json_str = response_chunk[5:].strip()
                            try:
                                response_data = json.loads(json_str)
                                if response_data.get("event") in ["message", "message_end"]:
                                    await websocket.send(json.dumps({
                                        "code": 0,
                                        "message": "",
                                        "data": response_data
                                    }, ensure_ascii=False))
                            except json.JSONDecodeError:
                                continue
                    
                    await send_ws_message(True)
                    logging.info(f"Agent completion streamed successfully for agent_id: {agent_id}")
                else:
                    full_content = ""
                    reference = {}
                    final_ans = None
                    
                    async for response_chunk in agent_completion(
                        tenant_id=tenant_id,
                        agent_id=agent_id,
                        question=question,
                        session_id=session_id,
                        stream=False,
                        **{k: v for k, v in req.items() if k not in ["question", "session_id", "stream"]}
                    ):
                        if isinstance(response_chunk, str) and response_chunk.startswith("data:"):
                            try:
                                ans = json.loads(response_chunk[5:])
                                if ans["event"] == "message":
                                    full_content += ans["data"]["content"]
                                if ans.get("data", {}).get("reference", None):
                                    reference.update(ans["data"]["reference"])
                                final_ans = ans
                            except Exception as e:
                                await send_ws_error(str(e))
                                continue
                    
                    if final_ans:
                        final_ans["data"]["content"] = full_content
                        final_ans["data"]["reference"] = reference
                        await send_ws_message(final_ans)
                    else:
                        await send_ws_error("No response generated", code=500)
                    
            except Exception as e:
                logging.exception(f"Error during agent completion: {str(e)}")
                await send_ws_error(str(e))
    
    except Exception as e:
        logging.exception(f"WebSocket error: {str(e)}")
        try:
            await send_ws_error(str(e))
        except Exception:
            pass
        await websocket.close(1011)
    
    finally:
        logging.info(f"WebSocket agent connection closed for agent_id: {agent_id}")

