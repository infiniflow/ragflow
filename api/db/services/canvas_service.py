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
import json
import time
import traceback
from uuid import uuid4
from agent.canvas import Canvas
from api.db import TenantPermission
from api.db.db_models import DB, CanvasTemplate, User, UserCanvas, API4Conversation
from api.db.services.api_service import API4ConversationService
from api.db.services.common_service import CommonService
from api.db.services.conversation_service import structure_answer
from api.utils import get_uuid
from api.utils.api_utils import get_data_openai
import tiktoken
from peewee import fn
class CanvasTemplateService(CommonService):
    model = CanvasTemplate


class UserCanvasService(CommonService):
    model = UserCanvas

    @classmethod
    @DB.connection_context()
    def get_list(cls, tenant_id,
                 page_number, items_per_page, orderby, desc, id, title):
        agents = cls.model.select()
        if id:
            agents = agents.where(cls.model.id == id)
        if title:
            agents = agents.where(cls.model.title == title)
        agents = agents.where(cls.model.user_id == tenant_id)
        if desc:
            agents = agents.order_by(cls.model.getter_by(orderby).desc())
        else:
            agents = agents.order_by(cls.model.getter_by(orderby).asc())

        agents = agents.paginate(page_number, items_per_page)

        return list(agents.dicts())
   
    @classmethod
    @DB.connection_context()
    def get_by_tenant_id(cls, pid):
        try:
            
            fields = [
                cls.model.id,
                cls.model.avatar,
                cls.model.title,
                cls.model.dsl,
                cls.model.description,
                cls.model.permission,
                cls.model.update_time,
                cls.model.user_id,
                cls.model.create_time,
                cls.model.create_date,
                cls.model.update_date,
                User.nickname,
                User.avatar.alias('tenant_avatar'),
            ]
            angents = cls.model.select(*fields) \
            .join(User, on=(cls.model.user_id == User.id)) \
            .where(cls.model.id == pid)
            # obj = cls.model.query(id=pid)[0]
            return True, angents.dicts()[0]
        except Exception as e:
            print(e)
            return False, None
     
    @classmethod
    @DB.connection_context()
    def get_by_tenant_ids(cls, joined_tenant_ids, user_id,
                          page_number, items_per_page,
                          orderby, desc, keywords,
                          ):
        fields = [
            cls.model.id,
            cls.model.avatar,
            cls.model.title,
            cls.model.dsl,
            cls.model.description,
            cls.model.permission,
            User.nickname,
            User.avatar.alias('tenant_avatar'),
            cls.model.update_time
        ]
        if keywords:
            angents = cls.model.select(*fields).join(User, on=(cls.model.user_id == User.id)).where(
                ((cls.model.user_id.in_(joined_tenant_ids) & (cls.model.permission == 
                                                                TenantPermission.TEAM.value)) | (
                    cls.model.user_id == user_id)),
                (fn.LOWER(cls.model.title).contains(keywords.lower()))
            )
        else:
            angents = cls.model.select(*fields).join(User, on=(cls.model.user_id == User.id)).where(
                ((cls.model.user_id.in_(joined_tenant_ids) & (cls.model.permission == 
                                                                TenantPermission.TEAM.value)) | (
                    cls.model.user_id == user_id))
            )
        if desc:
            angents = angents.order_by(cls.model.getter_by(orderby).desc())
        else:
            angents = angents.order_by(cls.model.getter_by(orderby).asc())
        count = angents.count()
        angents = angents.paginate(page_number, items_per_page)
        return list(angents.dicts()), count
   

def completion(tenant_id, agent_id, question, session_id=None, stream=True, **kwargs):
    """处理智能体对话补全的核心函数
    Args:
        tenant_id: 租户/用户ID
        agent_id: 智能体唯一标识
        question: 用户输入的问题
        session_id: 会话ID（存在时为续接会话）
        stream: 是否使用流式传输
        **kwargs: 其他请求参数

    流程:
        1. 验证Agent有效性及权限
        2. 处理会话创建/续接逻辑
        3. 执行智能体工作流
        4. 返回流式/非流式响应
    """
    # 验证Agent有效性及用户权限
    e, cvs = UserCanvasService.get_by_id(agent_id)
    assert e, "Agent not found."
    assert cvs.user_id == tenant_id, "You do not own the agent."

    # 序列化DSL配置（Domain Specific Language）
    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

    # 初始化智能体工作流引擎
    canvas = Canvas(cvs.dsl, tenant_id)
    canvas.reset()
    message_id = str(uuid4())

    # 新会话处理逻辑
    if not session_id:
        # 处理预设参数验证
        query = canvas.get_preset_param()
        if query:
            for ele in query:
                # 必填参数检查
                if not ele["optional"]:
                    if not kwargs.get(ele["key"]):
                        assert False, f"`{ele['key']}` is required"
                    ele["value"] = kwargs[ele["key"]]
                # 可选参数处理
                if ele["optional"]:
                    if kwargs.get(ele["key"]):
                        ele["value"] = kwargs[ele['key']]
                    else:
                        if "value" in ele:
                            ele.pop("value")

        # 创建新会话记录
        cvs.dsl = json.loads(str(canvas))
        session_id = get_uuid()
        conv = {
            "id": session_id,
            "dialog_id": cvs.id,
            "user_id": kwargs.get("user_id", "") if isinstance(kwargs, dict) else "",
            "message": [{"role": "assistant", "content": canvas.get_prologue(), "created_at": time.time()}],
            "source": "agent",
            "dsl": cvs.dsl
        }
        API4ConversationService.save(**conv)
        conv = API4Conversation(**conv)

    # 已有会话处理逻辑
    else:
        # 加载现有会话数据
        e, conv = API4ConversationService.get_by_id(session_id)
        assert e, "Session not found!"

        # 初始化工作流并添加用户消息
        canvas = Canvas(json.dumps(conv.dsl), tenant_id)
        canvas.messages.append({"role": "user", "content": question, "id": message_id})
        canvas.add_user_input(question)

        # 更新会话记录
        if not conv.message:
            conv.message = []
        conv.message.append({
            "role": "user",
            "content": question,
            "id": message_id
        })
        if not conv.reference:
            conv.reference = []
        conv.reference.append({"chunks": [], "doc_aggs": []})

    # 初始化响应容器
    final_ans = {"reference": [], "content": ""}

    # 流式响应处理
    if stream:
        try:
            for ans in canvas.run(stream=stream):
                # 处理中间状态响应
                if ans.get("running_status"):
                    yield "data:" + json.dumps({"code": 0, "message": "",
                                                "data": {"answer": ans["content"],
                                                         "running_status": True}},
                                               ensure_ascii=False) + "\n\n"
                    continue

                # 缓存最终响应内容
                for k in ans.keys():
                    final_ans[k] = ans[k]

                # 构造结构化响应
                ans = {"answer": ans["content"], "reference": ans.get("reference", []),
                       "param": canvas.get_preset_param()}
                ans = structure_answer(conv, ans, message_id, session_id)
                yield "data:" + json.dumps({"code": 0, "message": "", "data": ans},
                                           ensure_ascii=False) + "\n\n"

            # 会话最终处理
            canvas.messages.append(
                {"role": "assistant", "content": final_ans["content"], "created_at": time.time(), "id": message_id})
            canvas.history.append(("assistant", final_ans["content"]))
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])

            # 持久化会话状态
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
        except Exception as e:
            # 异常处理及状态回写
            traceback.print_exc()
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield "data:" + json.dumps({"code": 500, "message": str(e),
                                        "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                                       ensure_ascii=False) + "\n\n"
        # 结束标记
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

    # 非流式响应处理
    else:
        for answer in canvas.run(stream=False):
            if answer.get("running_status"):
                continue
            # 拼接最终响应内容
            final_ans["content"] = "\n".join(answer["content"]) if "content" in answer else ""

            # 更新会话记录
            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "id": message_id})
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            # 持久化更新
            conv.dsl = json.loads(str(canvas))

            # 构造返回结果
            result = {"answer": final_ans["content"], "reference": final_ans.get("reference", []),
                      "param": canvas.get_preset_param()}
            result = structure_answer(conv, result, message_id, session_id)
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield result
            break


def completionOpenAI(tenant_id, agent_id, question, session_id=None, stream=True, **kwargs):
    """Main function for OpenAI-compatible completions, structured similarly to the completion function."""
    tiktokenenc = tiktoken.get_encoding("cl100k_base")
    e, cvs = UserCanvasService.get_by_id(agent_id)
    
    if not e:
        yield get_data_openai(
            id=session_id,
            model=agent_id,
            content="**ERROR**: Agent not found."
        )
        return
    
    if cvs.user_id != tenant_id:
        yield get_data_openai(
            id=session_id,
            model=agent_id,
            content="**ERROR**: You do not own the agent"
        )
        return
    
    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)
    
    canvas = Canvas(cvs.dsl, tenant_id)
    canvas.reset()
    message_id = str(uuid4())
    
    # Handle new session creation
    if not session_id:
        query = canvas.get_preset_param()
        if query:
            for ele in query:
                if not ele["optional"]:
                    if not kwargs.get(ele["key"]):
                        yield get_data_openai(
                            id=None,
                            model=agent_id,
                            content=f"`{ele['key']}` is required",
                            completion_tokens=len(tiktokenenc.encode(f"`{ele['key']}` is required")),
                            prompt_tokens=len(tiktokenenc.encode(question if question else ""))
                        )
                        return
                    ele["value"] = kwargs[ele["key"]]
                if ele["optional"]:
                    if kwargs.get(ele["key"]):
                        ele["value"] = kwargs[ele['key']]
                    else:
                        if "value" in ele:
                            ele.pop("value")
        
        cvs.dsl = json.loads(str(canvas))
        session_id = get_uuid()
        conv = {
            "id": session_id,
            "dialog_id": cvs.id,
            "user_id": kwargs.get("user_id", "") if isinstance(kwargs, dict) else "",
            "message": [{"role": "assistant", "content": canvas.get_prologue(), "created_at": time.time()}],
            "source": "agent",
            "dsl": cvs.dsl
        }
        API4ConversationService.save(**conv)
        conv = API4Conversation(**conv)
            
    # Handle existing session
    else:
        e, conv = API4ConversationService.get_by_id(session_id)
        if not e:
            yield get_data_openai(
                id=session_id,
                model=agent_id,
                content="**ERROR**: Session not found!"
            )
            return
        
        canvas = Canvas(json.dumps(conv.dsl), tenant_id)
        canvas.messages.append({"role": "user", "content": question, "id": message_id})
        canvas.add_user_input(question)
        
        if not conv.message:
            conv.message = []
        conv.message.append({
            "role": "user",
            "content": question,
            "id": message_id
        })
        
        if not conv.reference:
            conv.reference = []
        conv.reference.append({"chunks": [], "doc_aggs": []})
    
    # Process request based on stream mode
    final_ans = {"reference": [], "content": ""}
    prompt_tokens = len(tiktokenenc.encode(str(question)))
    
    if stream:
        try:
            completion_tokens = 0
            for ans in canvas.run(stream=True):
                if ans.get("running_status"):
                    completion_tokens += len(tiktokenenc.encode(ans.get("content", "")))
                    yield "data: " + json.dumps(
                        get_data_openai(
                            id=session_id,
                            model=agent_id,
                            content=ans["content"],
                            object="chat.completion.chunk",
                            completion_tokens=completion_tokens,
                            prompt_tokens=prompt_tokens
                        ),
                        ensure_ascii=False
                    ) + "\n\n"
                    continue
                
                for k in ans.keys():
                    final_ans[k] = ans[k]
                
                completion_tokens += len(tiktokenenc.encode(final_ans.get("content", "")))
                yield "data: " + json.dumps(
                    get_data_openai(
                        id=session_id,
                        model=agent_id,
                        content=final_ans["content"],
                        object="chat.completion.chunk",
                        finish_reason="stop",
                        completion_tokens=completion_tokens,
                        prompt_tokens=prompt_tokens
                    ),
                    ensure_ascii=False
                ) + "\n\n"
            
            # Update conversation
            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "created_at": time.time(), "id": message_id})
            canvas.history.append(("assistant", final_ans["content"]))
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            
            yield "data: [DONE]\n\n"
            
        except Exception as e:
            traceback.print_exc()
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield "data: " + json.dumps(
                get_data_openai(
                    id=session_id,
                    model=agent_id,
                    content="**ERROR**: " + str(e),
                    finish_reason="stop",
                    completion_tokens=len(tiktokenenc.encode("**ERROR**: " + str(e))),
                    prompt_tokens=prompt_tokens
                ),
                ensure_ascii=False
            ) + "\n\n"
            yield "data: [DONE]\n\n"
    
    else:  # Non-streaming mode
        try:
            all_answer_content = ""
            for answer in canvas.run(stream=False):
                if answer.get("running_status"):
                    continue
                
                final_ans["content"] = "\n".join(answer["content"]) if "content" in answer else ""
                final_ans["reference"] = answer.get("reference", [])
                all_answer_content += final_ans["content"]
            
            final_ans["content"] = all_answer_content
            
            # Update conversation
            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "created_at": time.time(), "id": message_id})
            canvas.history.append(("assistant", final_ans["content"]))
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            
            # Return the response in OpenAI format
            yield get_data_openai(
                id=session_id,
                model=agent_id,
                content=final_ans["content"],
                finish_reason="stop",
                completion_tokens=len(tiktokenenc.encode(final_ans["content"])),
                prompt_tokens=prompt_tokens,
                param=canvas.get_preset_param()  # Added param info like in completion
            )
            
        except Exception as e:
            traceback.print_exc()
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield get_data_openai(
                id=session_id,
                model=agent_id,
                content="**ERROR**: " + str(e),
                finish_reason="stop",
                completion_tokens=len(tiktokenenc.encode("**ERROR**: " + str(e))),
                prompt_tokens=prompt_tokens
            )

