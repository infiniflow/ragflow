#!/usr/bin/env python3
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
RAGFlow WebSocket Client Example (Python)

This example demonstrates how to connect to RAGFlow's WebSocket API
and stream chat responses in real-time.

Requirements:
    pip install websocket-client

Usage:
    # Chat mode
    python python_client.py --host ws://localhost \
                          --token your-api-token \
                          --chat-id your-chat-id \
                          --question "What is RAGFlow?"
    
    # Agent mode
    python python_client.py --host ws://localhost \
                          --token your-api-token \
                          --agent-id your-agent-id \
                          --mode agent \
                          --question "What is RAGFlow?"
"""

import argparse
import json
import threading
import websocket


class RAGFlowWebSocketClient:
    """
    WebSocket client for RAGFlow streaming chat completions.
    
    This client demonstrates:
    - Connection establishment with authentication
    - Sending chat requests
    - Receiving and displaying streaming responses
    - Error handling and reconnection
    - Multi-turn conversations
    - Support for both Chat and Agent modes
    """
    
    def __init__(self, host, token, chat_id=None, agent_id=None, mode='chat', debug=False):
        """
        Initialize the WebSocket client.
        
        Args:
            host (str): WebSocket host (e.g., ws://localhost or wss://your-domain.com)
            token (str): API token for authentication
            chat_id (str, optional): Dialog/Chat ID for chat mode
            agent_id (str, optional): Agent ID for agent mode
            mode (str): 'chat' or 'agent' mode
            debug (bool): Enable debug output
        """
        # Construct URL based on mode
        if mode == 'chat' and chat_id:
            self.url = f"{host}/api/v1/ws/chats/{chat_id}/completions?token={token}"
        elif mode == 'agent' and agent_id:
            self.url = f"{host}/api/v1/ws/agents/{agent_id}/completions?token={token}"
        else:
            raise ValueError("Must provide chat_id for chat mode or agent_id for agent mode")
        
        self.chat_id = chat_id
        self.agent_id = agent_id
        self.mode = mode
        self.debug = debug
        self.ws = None
        self.session_id = None  # Track session for multi-turn conversations
        self.current_answer = ""  # Accumulate streaming chunks
        
    def on_message(self, ws, message):
        """
        Handle incoming WebSocket messages.
        
        This callback is invoked for each message received from the server.
        Messages contain incremental response chunks or completion markers.
        
        Args:
            ws: WebSocket connection object
            message (str): JSON message from server
        """
        try:
            # Parse JSON response
            response = json.loads(message)
            
            if self.debug:
                print(f"\n[DEBUG] Received: {json.dumps(response, indent=2)}")
            
            # Check if this is a completion marker
            if response.get('data') is True:
                print("\n\n✓ Stream completed")
                print("-" * 60)
                return
            
            # Check for errors
            if response.get('code', 0) != 0:
                print(f"\n✗ Error {response['code']}: {response.get('message', 'Unknown error')}")
                return
            
            # Extract response data
            data = response.get('data', {})
            
            if isinstance(data, dict):
                # Extract answer chunk
                answer = data.get('answer', '')
                
                # Save session ID for multi-turn conversations
                if 'session_id' in data and not self.session_id:
                    self.session_id = data['session_id']
                    if self.debug:
                        print(f"\n[DEBUG] Session ID: {self.session_id}")
                
                # Display incremental answer
                if answer:
                    print(answer, end='', flush=True)
                    self.current_answer += answer
                
                # Display references if available
                reference = data.get('reference', {})
                if reference and reference.get('chunks'):
                    print(f"\n\n📚 References: {len(reference['chunks'])} sources")
                    if self.debug:
                        for i, chunk in enumerate(reference['chunks'][:3], 1):
                            doc_name = chunk.get('doc_name', 'Unknown')
                            print(f"  {i}. {doc_name}")
        
        except json.JSONDecodeError as e:
            print(f"\n✗ Failed to parse response: {e}")
        except Exception as e:
            print(f"\n✗ Error handling message: {e}")
    
    def on_error(self, ws, error):
        """
        Handle WebSocket errors.
        
        Args:
            ws: WebSocket connection object
            error: Error object or message
        """
        print(f"\n✗ WebSocket error: {error}")
    
    def on_close(self, ws, close_status_code, close_msg):
        """
        Handle WebSocket connection close.
        
        Args:
            ws: WebSocket connection object
            close_status_code (int): Close status code
            close_msg (str): Close message
        """
        if close_status_code == 1000:
            # Normal closure
            print("\n✓ Connection closed normally")
        else:
            # Abnormal closure
            print(f"\n✗ Connection closed: {close_status_code} - {close_msg}")
    
    def on_open(self, ws):
        """
        Handle WebSocket connection open.
        
        This callback is invoked when the connection is established.
        It sends the initial chat message to start the conversation.
        
        Args:
            ws: WebSocket connection object
        """
        print("✓ Connected to RAGFlow")
        print("-" * 60)
    
    def send_message(self, question, session_id=None):
        """
        Send a chat message through the WebSocket.
        
        Args:
            question (str): User's question or message
            session_id (str, optional): Session ID for continuing a conversation
        """
        if not self.ws:
            print("✗ Not connected")
            return False
        
        # Construct chat request message (new SDK-style format)
        message = {
            'question': question,
            'stream': True
        }
        
        # Include session ID if continuing a conversation
        if session_id:
            message['session_id'] = session_id
        
        if self.debug:
            print(f"\n[DEBUG] Sending: {json.dumps(message, indent=2)}")
        
        # Reset answer accumulator
        self.current_answer = ""
        
        # Send message
        try:
            self.ws.send(json.dumps(message))
            print(f"\n💬 Question: {question}\n")
            print("🤖 Answer: ", end='', flush=True)
            return True
        except Exception as e:
            print(f"\n✗ Failed to send message: {e}")
            return False
    
    def connect(self):
        """
        Establish WebSocket connection.
        
        This creates the WebSocket connection and sets up event handlers.
        The connection runs in the main thread (blocking).
        """
        # Enable debug traces if requested
        if self.debug:
            websocket.enableTrace(True)
        
        # Create WebSocket app with event handlers
        self.ws = websocket.WebSocketApp(
            self.url,
            on_open=self.on_open,
            on_message=self.on_message,
            on_error=self.on_error,
            on_close=self.on_close
        )
        
        # Run forever (blocking call)
        self.ws.run_forever()
    
    def close(self):
        """Close the WebSocket connection."""
        if self.ws:
            self.ws.close()


def interactive_mode(client):
    """
    Run interactive mode for multi-turn conversations.
    
    This allows users to have ongoing conversations with the AI
    by typing questions and receiving responses in real-time.
    
    Args:
        client (RAGFlowWebSocketClient): WebSocket client instance
    """
    print("\n" + "=" * 60)
    print("Interactive Mode - Type 'quit' or 'exit' to end")
    print("=" * 60)
    
    def connection_thread():
        """Run WebSocket connection in background thread."""
        client.connect()
    
    # Start connection in background thread
    thread = threading.Thread(target=connection_thread, daemon=True)
    thread.start()
    
    # Wait for connection to establish
    import time
    time.sleep(2)
    
    # Interactive loop
    try:
        while True:
            # Get user input
            question = input("\n\n👤 You: ").strip()
            
            if not question:
                continue
            
            if question.lower() in ['quit', 'exit', 'q']:
                print("\n👋 Goodbye!")
                break
            
            # Send question (continue session if available)
            client.send_message(question, session_id=client.session_id)
            
            # Wait for response to complete
            # In production, you'd use proper async/event handling
            time.sleep(1)
    
    except KeyboardInterrupt:
        print("\n\n👋 Goodbye!")
    
    finally:
        client.close()


def main():
    """
    Main entry point for the WebSocket client example.
    
    Parses command-line arguments and runs the client in either
    single-question or interactive mode.
    """
    # Parse command-line arguments
    parser = argparse.ArgumentParser(
        description='RAGFlow WebSocket Client Example',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Single question (Chat mode)
  python python_client.py --host ws://localhost \\
                         --token your-token \\
                         --chat-id your-chat-id \\
                         --question "What is RAGFlow?"
  
  # Single question (Agent mode)
  python python_client.py --host ws://localhost \\
                         --token your-token \\
                         --agent-id your-agent-id \\
                         --mode agent \\
                         --question "What is RAGFlow?"
  
  # Interactive mode
  python python_client.py --host ws://localhost \\
                         --token your-token \\
                         --chat-id your-chat-id \\
                         --interactive
        """
    )
    
    parser.add_argument(
        '--host',
        required=True,
        help='WebSocket host (e.g., ws://localhost or wss://your-domain.com)'
    )
    
    parser.add_argument(
        '--token',
        required=True,
        help='API token for authentication'
    )
    
    parser.add_argument(
        '--chat-id',
        help='Dialog/Chat ID to use (required for chat mode)'
    )
    
    parser.add_argument(
        '--agent-id',
        help='Agent ID to use (required for agent mode)'
    )
    
    parser.add_argument(
        '--mode',
        choices=['chat', 'agent'],
        default='chat',
        help='Mode: chat or agent (default: chat)'
    )
    
    parser.add_argument(
        '--question',
        help='Question to ask (single question mode)'
    )
    
    parser.add_argument(
        '--session-id',
        help='Session ID to continue existing conversation'
    )
    
    parser.add_argument(
        '--interactive',
        action='store_true',
        help='Enable interactive mode for multi-turn conversations'
    )
    
    parser.add_argument(
        '--debug',
        action='store_true',
        help='Enable debug output'
    )
    
    args = parser.parse_args()
    
    # Validate arguments
    if not args.interactive and not args.question:
        parser.error("Either --question or --interactive must be specified")
    
    if args.mode == 'chat' and not args.chat_id:
        parser.error("--chat-id is required for chat mode")
    if args.mode == 'agent' and not args.agent_id:
        parser.error("--agent-id is required for agent mode")
    
    # Create client
    client = RAGFlowWebSocketClient(
        host=args.host,
        token=args.token,
        chat_id=args.chat_id,
        agent_id=args.agent_id,
        mode=args.mode,
        debug=args.debug
    )
    
    print("\n" + "=" * 60)
    print("RAGFlow WebSocket Client")
    print("=" * 60)
    
    # Run in appropriate mode
    if args.interactive:
        # Interactive mode - ongoing conversation
        interactive_mode(client)
    else:
        # Single question mode
        # Save original on_open method to avoid infinite recursion
        original_on_open = client.on_open
        
        def send_after_connect(ws):
            """Send question after connection is established."""
            original_on_open(ws)  # Call original method
            client.send_message(args.question, session_id=args.session_id)
        
        # Override on_open to send question
        client.on_open = send_after_connect
        
        # Connect and run (blocking)
        try:
            client.connect()
        except KeyboardInterrupt:
            print("\n\n👋 Interrupted")
        finally:
            client.close()


if __name__ == '__main__':
    main()

