import requests
import json
from bridge.context import Context, ContextType  # Import Context, ContextType
from bridge.reply import Reply, ReplyType  # Import Reply, ReplyType
from bridge import *
from common.log import logger
from config import conf
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
        logger.info("[RAGFlowChat] Plugin initialized")

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
            logger.error("[RAGFlowChat] Missing configuration")
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
                logger.debug(f"[RAGFlowChat] New conversation response: {response.text}")
                if response.status_code == 200:
                    data = response.json()
                    if data.get("retcode") == 0:
                        conversation_id = data["data"]["id"]
                        self.conversations[user_id] = conversation_id
                    else:
                        logger.error(f"[RAGFlowChat] Failed to create conversation: {data.get('retmsg')}")
                        return f"Sorry, unable to create a conversation: {data.get('retmsg')}"
                else:
                    logger.error(f"[RAGFlowChat] HTTP error when creating conversation: {response.status_code}")
                    return f"Sorry, unable to connect to RAGFlow API (create conversation). HTTP status code: {response.status_code}"
            except Exception as e:
                logger.exception(f"[RAGFlowChat] Exception when creating conversation: {e}")
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
            logger.debug(f"[RAGFlowChat] Completion response: {response.text}")
            if response.status_code == 200:
                data = response.json()
                if data.get("retcode") == 0:
                    answer = data["data"]["answer"]
                    return answer
                else:
                    logger.error(f"[RAGFlowChat] Failed to get answer: {data.get('retmsg')}")
                    return f"Sorry, unable to get a reply: {data.get('retmsg')}"
            else:
                logger.error(f"[RAGFlowChat] HTTP error when getting answer: {response.status_code}")
                return f"Sorry, unable to connect to RAGFlow API (get reply). HTTP status code: {response.status_code}"
        except Exception as e:
            logger.exception(f"[RAGFlowChat] Exception when getting answer: {e}")
            return f"Sorry, an internal error occurred: {str(e)}"
