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
import requests
from bridge.context import ContextType  # Import Context, ContextType
from bridge.reply import Reply, ReplyType  # Import Reply, ReplyType
from plugins import Plugin, register  # Import Plugin and register
from plugins.event import Event, EventContext, EventAction  # Import event-related classes

@register(name="RAGFlowChat", desc="Use RAGFlow API to chat", version="1.0", author="Your Name")
class RAGFlowChat(Plugin):
    def __init__(self):
        super().__init__()
        # Load plugin configuration
        self.cfg = self.load_config()
        # Bind event handling function
        self.handlers[Event.ON_HANDLE_CONTEXT] = self.on_handle_context
        # Store conversation_id for each user
        self.conversations = {}
        logging.info("[RAGFlowChat] Plugin initialized")

    def on_handle_context(self, e_context: EventContext):
        context = e_context['context']
        if context.type != ContextType.TEXT:
            return  # Only process text messages

        user_input = context.content.strip()
        session_id = context['session_id']

        # Call RAGFlow API to get a reply
        reply_text = self.get_ragflow_reply(user_input, session_id)
        if reply_text:
            reply = Reply()
            reply.type = ReplyType.TEXT
            reply.content = reply_text
            e_context['reply'] = reply
            e_context.action = EventAction.BREAK_PASS  # Skip the default processing logic
        else:
            # If no reply is received, pass to the next plugin or default logic
            e_context.action = EventAction.CONTINUE

    def get_ragflow_reply(self, user_input, session_id):
        # Get API_KEY and host address from the configuration
        api_key = self.cfg.get("api_key")
        host_address = self.cfg.get("host_address")
        user_id = session_id  # Use session_id as user_id

        if not api_key or not host_address:
            logging.error("[RAGFlowChat] Missing configuration")
            return "The plugin configuration is incomplete. Please check the configuration."

        headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json"
        }

        # Step 1: Get or create conversation_id
        conversation_id = self.conversations.get(user_id)
        if not conversation_id:
            # Create a new conversation
            url_new_conversation = f"http://{host_address}/v1/api/new_conversation"
            params_new_conversation = {
                "user_id": user_id
            }
            try:
                response = requests.get(url_new_conversation, headers=headers, params=params_new_conversation)
                logging.debug(f"[RAGFlowChat] New conversation response: {response.text}")
                if response.status_code == 200:
                    data = response.json()
                    if data.get("code") == 0:
                        conversation_id = data["data"]["id"]
                        self.conversations[user_id] = conversation_id
                    else:
                        logging.error(f"[RAGFlowChat] Failed to create conversation: {data.get('message')}")
                        return f"Sorry, unable to create a conversation: {data.get('message')}"
                else:
                    logging.error(f"[RAGFlowChat] HTTP error when creating conversation: {response.status_code}")
                    return f"Sorry, unable to connect to RAGFlow API (create conversation). HTTP status code: {response.status_code}"
            except Exception as e:
                logging.exception("[RAGFlowChat] Exception when creating conversation")
                return f"Sorry, an internal error occurred: {str(e)}"

        # Step 2: Send the message and get a reply
        url_completion = f"http://{host_address}/v1/api/completion"
        payload_completion = {
            "conversation_id": conversation_id,
            "messages": [
                {
                    "role": "user",
                    "content": user_input
                }
            ],
            "quote": False,
            "stream": False
        }

        try:
            response = requests.post(url_completion, headers=headers, json=payload_completion)
            logging.debug(f"[RAGFlowChat] Completion response: {response.text}")
            if response.status_code == 200:
                data = response.json()
                if data.get("code") == 0:
                    answer = data["data"]["answer"]
                    return answer
                else:
                    logging.error(f"[RAGFlowChat] Failed to get answer: {data.get('message')}")
                    return f"Sorry, unable to get a reply: {data.get('message')}"
            else:
                logging.error(f"[RAGFlowChat] HTTP error when getting answer: {response.status_code}")
                return f"Sorry, unable to connect to RAGFlow API (get reply). HTTP status code: {response.status_code}"
        except Exception as e:
            logging.exception("[RAGFlowChat] Exception when getting answer")
            return f"Sorry, an internal error occurred: {str(e)}"
