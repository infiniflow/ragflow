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

import pytest
from ragflow_sdk import Session

class TestSessionMethods:
    """
    专门用于测试 sdk/python/ragflow_sdk/modules/session.py 中的方法，
    """

    @pytest.mark.p1
    def test_ask_sync(self, add_sessions_with_chat_assistant_func):
        """测试同步问答 (stream=False)"""
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        session = sessions[0]
        
        # 调用 ask 方法
        messages = list(session.ask(question="What is RAGFlow?", stream=False))
        
        assert len(messages) > 0
        message = messages[0]
        assert message.role == "assistant"
        assert hasattr(message, "content")
        assert message.content is not None

    @pytest.mark.p1
    def test_ask_stream(self, add_sessions_with_chat_assistant_func):
        """测试流式问答 (stream=True)"""
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        session = sessions[0]
        
        # 调用 ask 方法并迭代生成器
        stream_responses = []
        for message in session.ask(question="Tell me a joke.", stream=True):
            assert message.role == "assistant"
            stream_responses.append(message)
        
        assert len(stream_responses) > 0

    @pytest.mark.p1
    def test_session_update_method(self, add_sessions_with_chat_assistant_func):
        """测试 Session 对象的 update 方法"""
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        session = sessions[0]
        
        new_name = "Updated Session Name"
        # 直接调用 session 对象的 update 方法，这会触发 session.py 中的 update
        session.update({"name": new_name})
        
        # 验证更新结果
        updated_sessions = chat_assistant.list_sessions(id=session.id)
        assert updated_sessions[0].name == new_name

    @pytest.mark.p2
    def test_message_structure(self, add_sessions_with_chat_assistant_func):
        """验证 Message 对象的结构"""
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        session = sessions[0]
        
        # 获取一个回答
        messages = list(session.ask(question="Hi", stream=False))
        message = messages[0]
        
        # 验证 Message 属性
        assert message.role == "assistant"
        assert isinstance(message.content, str)
        # 即使没有参考资料，reference 属性也应该存在（默认为 None 或 list）
        assert hasattr(message, "reference")
