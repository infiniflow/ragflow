#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
WebSocket API for RAGFlow Streaming Responses

This module provides WebSocket endpoints for real-time streaming of chat completions.
WebSocket support is essential for platforms like WeChat Mini Programs that require
persistent bidirectional connections for real-time communication.

Key Features:
- Real-time bidirectional communication via WebSocket
- Support for multiple authentication methods (API Token, User Session)
- Streaming chat completions with incremental responses
- Error handling and connection management
- Compatible with WeChat Mini Programs and other WebSocket clients

WebSocket Message Format:
    Client -> Server (Request):
    {
        "type": "chat",              # Message type (currently supports "chat")
        "chat_id": "xxx",            # Dialog/Chat ID
        "session_id": "xxx",         # Optional: Conversation session ID
        "question": "Hello",         # User's question/message
        "stream": true,              # Optional: Enable streaming (default: true)
        "kb_ids": []                 # Optional: Knowledge base IDs to query
    }

    Server -> Client (Response):
    {
        "code": 0,                   # Status code (0=success, 500=error)
        "message": "",               # Error message (if any)
        "data": {                    # Response data
            "answer": "...",         # Incremental answer text (for streaming)
            "reference": {...},      # Source references
            "id": "xxx",             # Message ID
            "session_id": "xxx"      # Session ID
        }
    }

    Server -> Client (Completion):
    {
        "code": 0,
        "message": "",
        "data": true                 # Indicates completion of streaming
    }

    Server -> Client (Error):
    {
        "code": 500,
        "message": "Error description",
        "data": {
            "answer": "**ERROR**: Error details",
            "reference": []
        }
    }

Connection Lifecycle:
1. Client initiates WebSocket connection with authentication
2. Server validates authentication (API token or user session)
3. Client sends chat message requests
4. Server streams response chunks back to client
5. Server sends completion marker when done
6. Connection remains open for subsequent messages
7. Either party can close the connection
"""

import logging
import json
from quart import websocket
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer

from api.db.db_models import APIToken
from api.db.services.user_service import UserService
from api.db.services.dialog_service import DialogService
from api.db.services.conversation_service import completion
from common.constants import StatusEnum
from common import settings


# -----------------------------------------------------------------------------
# Authentication Helper Functions
# -----------------------------------------------------------------------------

async def authenticate_websocket():
    """
    Authenticate WebSocket connection using multiple methods.
    
    This function attempts to authenticate the WebSocket connection using:
    1. API Token authentication (Bearer token in Authorization header)
    2. User session authentication (Session-based JWT token)
    3. Query parameter authentication (token passed as URL parameter)
    
    Authentication Methods:
    - API Token: Used by external applications, bots, and integrations
    - User Session: Used by web interface and logged-in users
    - Query Parameter: Fallback for clients that can't send headers
    
    Returns:
        tuple: (authenticated: bool, tenant_id: str|None, error_message: str|None)
        
    Examples:
        # API Token authentication
        ws://host/ws/chat?Authorization=Bearer ragflow-xxxxx
        
        # Query parameter authentication
        ws://host/ws/chat?token=ragflow-xxxxx
    """
    tenant_id = None
    error_message = None
    
    # Method 1: Try API Token authentication from Authorization header
    # This is the preferred method for SDK and API integrations
    authorization = websocket.headers.get("Authorization", "")
    
    if authorization:
        try:
            # Parse Bearer token format: "Bearer <token>"
            authorization_parts = authorization.split()
            
            if len(authorization_parts) >= 2:
                token = authorization_parts[1]
                
                # Query database for matching API token
                objs = APIToken.query(token=token)
                
                if objs:
                    # Valid API token found, extract tenant ID
                    tenant_id = objs[0].tenant_id
                    logging.info(f"WebSocket authenticated via API token for tenant: {tenant_id}")
                    return True, tenant_id, None
                else:
                    error_message = "Invalid API token"
                    logging.warning(f"WebSocket authentication failed: {error_message}")
            else:
                error_message = "Invalid Authorization header format. Expected: 'Bearer <token>'"
                logging.warning(f"WebSocket authentication failed: {error_message}")
                
        except Exception as e:
            error_message = f"Error processing API token: {str(e)}"
            logging.error(f"WebSocket authentication error: {error_message}")
    
    # Method 2: Try User Session authentication (JWT token)
    # This is used by the web interface for logged-in users
    try:
        jwt = Serializer(secret_key=settings.SECRET_KEY)
        
        # Try to get authorization from header or query parameter
        auth_token = websocket.headers.get("Authorization") or \
                     websocket.args.get("authorization") or \
                     websocket.args.get("token")
        
        if auth_token:
            try:
                # Decode JWT token to get access token
                access_token = str(jwt.loads(auth_token))
                
                # Validate access token format
                if access_token and len(access_token.strip()) >= 32:
                    # Query user by access token
                    user = UserService.query(
                        access_token=access_token,
                        status=StatusEnum.VALID.value
                    )
                    
                    if user and user[0]:
                        # Valid user session found
                        tenant_id = user[0].id
                        logging.info(f"WebSocket authenticated via user session for user: {user[0].email}")
                        return True, tenant_id, None
                    
            except Exception as e:
                # JWT decoding or validation failed
                logging.debug(f"User session authentication failed: {str(e)}")
    
    except Exception as e:
        logging.error(f"Error in user session authentication: {str(e)}")
    
    # Method 3: Try query parameter authentication
    # Fallback for clients that cannot set custom headers
    token_param = websocket.args.get("token")
    if token_param:
        try:
            objs = APIToken.query(token=token_param)
            if objs:
                tenant_id = objs[0].tenant_id
                logging.info(f"WebSocket authenticated via query parameter for tenant: {tenant_id}")
                return True, tenant_id, None
        except Exception as e:
            logging.error(f"Query parameter authentication error: {str(e)}")
    
    # No valid authentication method succeeded
    if not error_message:
        error_message = "Authentication required. Please provide valid API token or user session."
    
    return False, None, error_message


async def send_error(error_message, code=500):
    """
    Send error message to WebSocket client in standardized format.
    
    Args:
        error_message (str): Human-readable error description
        code (int): Error code (default: 500 for server errors)
        
    Error Response Format:
        {
            "code": 500,
            "message": "Error description",
            "data": {
                "answer": "**ERROR**: Error details",
                "reference": []
            }
        }
    """
    error_response = {
        "code": code,
        "message": error_message,
        "data": {
            "answer": f"**ERROR**: {error_message}",
            "reference": []
        }
    }
    
    await websocket.send(json.dumps(error_response, ensure_ascii=False))
    logging.error(f"WebSocket error sent: {error_message}")


async def send_message(data, code=0, message=""):
    """
    Send message to WebSocket client in standardized format.
    
    Args:
        data: Response data (can be dict, bool, or any JSON-serializable object)
        code (int): Status code (0 for success)
        message (str): Optional status message
        
    Success Response Format:
        {
            "code": 0,
            "message": "",
            "data": {...}
        }
    """
    response = {
        "code": code,
        "message": message,
        "data": data
    }
    
    await websocket.send(json.dumps(response, ensure_ascii=False))


# -----------------------------------------------------------------------------
# WebSocket Endpoint: Chat Completions
# -----------------------------------------------------------------------------

@manager.route("/ws/chat")  # noqa: F821
async def websocket_chat():
    """
    WebSocket endpoint for real-time chat completions with streaming responses.
    
    This endpoint provides a persistent WebSocket connection for interactive chat
    sessions. It supports streaming responses, allowing clients to receive
    incremental updates as the AI generates the response.
    
    Connection URL:
        ws://host/v1/ws/chat
        
    Authentication:
        - Authorization header: "Bearer <api_token>"
        - Query parameter: "?token=<api_token>"
        - User session JWT
        
    Message Flow:
        1. Client connects and authenticates
        2. Client sends chat request message
        3. Server streams response chunks
        4. Server sends completion marker
        5. Connection stays open for more messages
        
    Supported Features:
        - Multi-turn conversations with session tracking
        - Knowledge base integration for RAG
        - Reference/citation tracking
        - Error recovery and graceful degradation
        
    Example Client Code (JavaScript):
        ```javascript
        const ws = new WebSocket('ws://host/v1/ws/chat?token=YOUR_TOKEN');
        
        ws.onopen = () => {
            ws.send(JSON.stringify({
                type: 'chat',
                chat_id: 'your-chat-id',
                question: 'Hello, how are you?',
                stream: true
            }));
        };
        
        ws.onmessage = (event) => {
            const response = JSON.parse(event.data);
            if (response.data === true) {
                console.log('Stream completed');
            } else {
                console.log('Received:', response.data.answer);
            }
        };
        ```
        
    Example Client Code (Python):
        ```python
        import websocket
        import json
        
        def on_message(ws, message):
            data = json.loads(message)
            if data['data'] is True:
                print('Stream completed')
            else:
                print('Received:', data['data']['answer'])
        
        ws = websocket.WebSocketApp(
            'ws://host/v1/ws/chat?token=YOUR_TOKEN',
            on_message=on_message
        )
        
        ws.on_open = lambda ws: ws.send(json.dumps({
            'type': 'chat',
            'chat_id': 'your-chat-id',
            'question': 'Hello!',
            'stream': True
        }))
        
        ws.run_forever()
        ```
    """
    # Step 1: Authenticate the WebSocket connection
    # This ensures only authorized clients can access the chat service
    authenticated, tenant_id, error_msg = await authenticate_websocket()
    
    if not authenticated:
        # Authentication failed - send error and close connection
        await send_error(error_msg, code=401)
        await websocket.close(1008, error_msg)  # 1008 = Policy Violation
        return
    
    # Authentication successful - log connection
    logging.info(f"WebSocket chat connection established for tenant: {tenant_id}")
    
    # Step 2: Connection loop - handle multiple messages over same connection
    # WebSocket connections are persistent, allowing multiple request/response cycles
    try:
        # Keep connection open and process incoming messages
        while True:
            # Wait for message from client
            # This is a blocking call that waits until client sends data
            message = await websocket.receive()
            
            # Parse JSON message from client
            try:
                request_data = json.loads(message)
            except json.JSONDecodeError as e:
                # Invalid JSON format - send error but keep connection open
                await send_error(f"Invalid JSON format: {str(e)}", code=400)
                continue
            
            # Extract message type (currently only 'chat' is supported)
            message_type = request_data.get("type", "chat")
            
            # Step 3: Route message to appropriate handler based on type
            if message_type == "chat":
                # Handle chat completion request
                await handle_chat_request(tenant_id, request_data)
            else:
                # Unknown message type - send error but keep connection open
                await send_error(f"Unknown message type: {message_type}", code=400)
    
    except Exception as e:
        # Unexpected error occurred - log and notify client
        error_message = f"WebSocket error: {str(e)}"
        logging.exception(error_message)
        
        try:
            await send_error(error_message)
        except Exception:
            # Failed to send error (connection may be closed)
            pass
        
        # Close connection with error code
        await websocket.close(1011, "Internal server error")  # 1011 = Internal Error
    
    finally:
        # Connection closed - cleanup and log
        logging.info(f"WebSocket chat connection closed for tenant: {tenant_id}")


async def handle_chat_request(tenant_id, request_data):
    """
    Handle chat completion request received via WebSocket.
    
    This function processes a chat request, validates parameters, retrieves
    the dialog configuration, and streams the AI response back to the client.
    
    Args:
        tenant_id (str): Authenticated tenant/user ID
        request_data (dict): Parsed JSON request from client
        
    Required Request Fields:
        - chat_id (str): Dialog/Chat ID to use for the conversation
        - question (str): User's question or message
        
    Optional Request Fields:
        - session_id (str): Existing conversation session ID (creates new if not provided)
        - stream (bool): Enable streaming responses (default: True)
        - kb_ids (list): Knowledge base IDs to include in retrieval
        - doc_ids (str): Comma-separated document IDs to prioritize
        - files (list): File IDs attached to this message
        
    Processing Steps:
        1. Validate required parameters
        2. Verify dialog ownership and permissions
        3. Create or retrieve conversation session
        4. Stream AI-generated response chunks
        5. Send completion marker
        
    Error Handling:
        - Missing parameters: Returns 400 error
        - Invalid dialog: Returns 404 error
        - Permission denied: Returns 403 error
        - Processing error: Returns 500 error
    """
    try:
        # Step 1: Extract and validate required parameters
        chat_id = request_data.get("chat_id")
        question = request_data.get("question", "")
        session_id = request_data.get("session_id")
        stream = request_data.get("stream", True)
        
        # Validate chat_id is provided
        if not chat_id:
            await send_error("Missing required parameter: chat_id", code=400)
            return
        
        # Validate question is provided (empty questions are allowed for session initialization)
        if question is None:
            await send_error("Missing required parameter: question", code=400)
            return
        
        # Step 2: Verify dialog exists and user has access
        # Check if the dialog belongs to this tenant and is active
        dialog_query = DialogService.query(
            tenant_id=tenant_id,
            id=chat_id,
            status=StatusEnum.VALID.value
        )
        
        if not dialog_query:
            # Dialog not found or user doesn't have permission
            await send_error(f"Dialog not found or access denied: {chat_id}", code=404)
            return
        
        # Step 3: Extract optional parameters for enhanced functionality
        # These parameters customize the retrieval and generation process
        additional_params = {}
        
        # Knowledge base filtering - limit search to specific KBs
        if "kb_ids" in request_data:
            additional_params["kb_ids"] = request_data["kb_ids"]
        
        # Document filtering - prioritize specific documents
        if "doc_ids" in request_data:
            additional_params["doc_ids"] = request_data["doc_ids"]
        
        # File attachments - include files uploaded with this message
        if "files" in request_data:
            additional_params["files"] = request_data["files"]
        
        # Pass through any other custom parameters
        # This allows for future extensibility without code changes
        for key, value in request_data.items():
            if key not in ["type", "chat_id", "question", "session_id", "stream"]:
                if key not in additional_params:
                    additional_params[key] = value
        
        # Step 4: Process chat completion with streaming
        if stream:
            # Streaming mode: Send incremental response chunks
            # This provides a better user experience with real-time feedback
            
            try:
                # Call the completion service which yields response chunks
                # The completion function handles session management, RAG retrieval,
                # LLM generation, and response formatting
                # Note: completion() is a synchronous generator, not async
                for response_chunk in completion(
                    tenant_id=tenant_id,
                    chat_id=chat_id,
                    question=question,
                    session_id=session_id,
                    stream=True,
                    **additional_params
                ):
                    # Parse the SSE-formatted response
                    # completion() returns "data:{json}\n\n" format for compatibility
                    if response_chunk.startswith("data:"):
                        # Extract JSON from SSE format
                        json_str = response_chunk[5:].strip()
                        
                        # Parse and forward to WebSocket client
                        try:
                            response_data = json.loads(json_str)
                            
                            # Send the chunk to WebSocket client
                            await websocket.send(json.dumps(response_data, ensure_ascii=False))
                            
                        except json.JSONDecodeError:
                            # Malformed response chunk - log but continue
                            logging.warning(f"Failed to parse response chunk: {json_str}")
                            continue
                
                # Stream completed successfully
                logging.info(f"Chat completion streamed successfully for chat_id: {chat_id}")
                
            except Exception as e:
                # Error during streaming - send error message
                error_message = f"Error during chat completion: {str(e)}"
                logging.exception(error_message)
                await send_error(error_message)
        
        else:
            # Non-streaming mode: Send complete response at once
            # This is simpler but provides no incremental feedback
            
            try:
                # Get the complete response (completion yields once for non-streaming)
                response = None
                for resp in completion(
                    tenant_id=tenant_id,
                    chat_id=chat_id,
                    question=question,
                    session_id=session_id,
                    stream=False,
                    **additional_params
                ):
                    response = resp
                    break  # Only one response in non-streaming mode
                
                # Send complete response
                if response:
                    await send_message(response)
                else:
                    await send_error("No response generated", code=500)
                
                logging.info(f"Chat completion completed (non-streaming) for chat_id: {chat_id}")
                
            except Exception as e:
                # Error during generation - send error message
                error_message = f"Error during chat completion: {str(e)}"
                logging.exception(error_message)
                await send_error(error_message)
    
    except Exception as e:
        # Unexpected error in request handling
        error_message = f"Error handling chat request: {str(e)}"
        logging.exception(error_message)
        await send_error(error_message)


# -----------------------------------------------------------------------------
# WebSocket Endpoint: Agent Completions (Future Enhancement)
# -----------------------------------------------------------------------------

@manager.route("/ws/agent")  # noqa: F821
async def websocket_agent():
    """
    WebSocket endpoint for agent-based completions with streaming.
    
    This endpoint is similar to websocket_chat but designed for agent-based
    interactions. Agents can have custom tools, workflows, and behaviors
    beyond standard RAG chat.
    
    Note: This is a placeholder for future implementation. The authentication
    and connection handling logic is the same as websocket_chat.
    
    Future Enhancements:
        - Tool calling and function execution
        - Multi-step agent reasoning
        - Agent state management
        - Custom agent workflows
    """
    # Authenticate connection
    authenticated, tenant_id, error_msg = await authenticate_websocket()
    
    if not authenticated:
        await send_error(error_msg, code=401)
        await websocket.close(1008, error_msg)
        return
    
    logging.info(f"WebSocket agent connection established for tenant: {tenant_id}")
    
    # Connection loop
    try:
        while True:
            message = await websocket.receive()
            
            try:
                request_data = json.loads(message)
            except json.JSONDecodeError as e:
                await send_error(f"Invalid JSON format: {str(e)}", code=400)
                continue
            
            # Handle agent completion request
            await handle_agent_request(tenant_id, request_data)
    
    except Exception as e:
        error_message = f"WebSocket error: {str(e)}"
        logging.exception(error_message)
        
        try:
            await send_error(error_message)
        except Exception:
            pass
        
        await websocket.close(1011, "Internal server error")
    
    finally:
        logging.info(f"WebSocket agent connection closed for tenant: {tenant_id}")


async def handle_agent_request(tenant_id, request_data):
    """
    Handle agent completion request received via WebSocket.
    
    This is a placeholder for future agent functionality.
    
    Args:
        tenant_id (str): Authenticated tenant/user ID
        request_data (dict): Parsed JSON request from client
    """
    # TODO: Implement agent-specific logic
    # For now, return a not-implemented error
    await send_error("Agent completions not yet implemented", code=501)
    
    logging.info("Agent request received but not yet implemented")


# -----------------------------------------------------------------------------
# WebSocket Health Check Endpoint
# -----------------------------------------------------------------------------

@manager.route("/ws/health")  # noqa: F821
async def websocket_health():
    """
    WebSocket health check endpoint.
    
    This endpoint allows clients to verify WebSocket connectivity
    without authentication. Useful for monitoring and diagnostics.
    
    The server will echo back any messages received, allowing clients
    to test round-trip latency and connection stability.
    
    Example Usage:
        ```javascript
        const ws = new WebSocket('ws://host/v1/ws/health');
        ws.onopen = () => ws.send('ping');
        ws.onmessage = (e) => console.log('Received:', e.data);
        ```
    """
    logging.info("WebSocket health check connection established")
    
    try:
        # Send initial health status
        await websocket.send(json.dumps({
            "status": "healthy",
            "message": "WebSocket connection established",
            "version": "1.0"
        }))
        
        # Echo messages back to client
        while True:
            message = await websocket.receive()
            
            # Echo the message back
            await websocket.send(json.dumps({
                "echo": message,
                "timestamp": str(logging.time.time())
            }))
    
    except Exception as e:
        logging.info(f"WebSocket health check closed: {str(e)}")
    
    finally:
        logging.info("WebSocket health check connection closed")

