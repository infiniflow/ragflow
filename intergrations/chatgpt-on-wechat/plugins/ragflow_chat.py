# ragflow_chat.py

import requests
import json
from bridge.context import Context, ContextType  # 导入 Context, ContextType
from bridge.reply import Reply, ReplyType  # 导入 Reply, ReplyType
from bridge import *
from common.log import logger
from config import conf
from plugins import Plugin, register  # 导入 Plugin 和 register
from plugins.event import Event, EventContext, EventAction  # 导入事件相关的类

@register(name="RAGFlowChat", desc="Use RAGFlow API to chat", version="1.0", author="Your Name")
class RAGFlowChat(Plugin):
    def __init__(self):
        super().__init__()
        # 加载插件配置
        self.cfg = self.load_config()
        # 绑定事件处理函数
        self.handlers[Event.ON_HANDLE_CONTEXT] = self.on_handle_context
        # 用于存储每个用户的 conversation_id
        self.conversations = {}
        logger.info("[RAGFlowChat] Plugin initialized")

    def on_handle_context(self, e_context: EventContext):
        context = e_context['context']
        if context.type != ContextType.TEXT:
            return  # 只处理文本消息

        user_input = context.content.strip()
        session_id = context['session_id']

        # 调用 RAGFlow API 获取回复
        reply_text = self.get_ragflow_reply(user_input, session_id)
        if reply_text:
            reply = Reply()
            reply.type = ReplyType.TEXT
            reply.content = reply_text
            e_context['reply'] = reply
            e_context.action = EventAction.BREAK_PASS  # 跳过默认的处理逻辑
        else:
            # 如果未能获取回复，继续交给下一个插件或默认逻辑处理
            e_context.action = EventAction.CONTINUE

    def get_ragflow_reply(self, user_input, session_id):
        # 从配置中获取 API_KEY 和主机地址
        api_key = self.cfg.get("api_key")
        host_address = self.cfg.get("host_address")
        user_id = session_id  # 使用 session_id 作为 user_id

        if not api_key or not host_address:
            logger.error("[RAGFlowChat] Missing configuration")
            return "插件配置不完整，请检查配置。"

        headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json"
        }

        # Step 1: 获取或创建 conversation_id
        conversation_id = self.conversations.get(user_id)
        if not conversation_id:
            # 创建新的会话
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
                        return f"抱歉，无法创建会话：{data.get('retmsg')}"
                else:
                    logger.error(f"[RAGFlowChat] HTTP error when creating conversation: {response.status_code}")
                    return f"抱歉，无法连接到 RAGFlow API（创建会话）。HTTP 状态码：{response.status_code}"
            except Exception as e:
                logger.exception(f"[RAGFlowChat] Exception when creating conversation: {e}")
                return f"抱歉，发生了内部错误：{str(e)}"

        # Step 2: 发送消息并获取回复
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
                    return f"抱歉，无法获取回复：{data.get('retmsg')}"
            else:
                logger.error(f"[RAGFlowChat] HTTP error when getting answer: {response.status_code}")
                return f"抱歉，无法连接到 RAGFlow API（获取回复）。HTTP 状态码：{response.status_code}"
        except Exception as e:
            logger.exception(f"[RAGFlowChat] Exception when getting answer: {e}")
            return f"抱歉，发生了内部错误：{str(e)}"
