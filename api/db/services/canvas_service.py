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
from api.db.db_models import DB, CanvasTemplate, UserCanvas, API4Conversation
from api.db.services.api_service import API4ConversationService
from api.db.services.common_service import CommonService
from api.db.services.conversation_service import structure_answer
from api.utils import get_uuid


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


def completion(tenant_id, agent_id, question, session_id=None, stream=True, **kwargs):
    e, cvs = UserCanvasService.get_by_id(agent_id)
    assert e, "Agent not found."
    assert cvs.user_id == tenant_id, "You do not own the agent."
    if not isinstance(cvs.dsl,str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)
    canvas = Canvas(cvs.dsl, tenant_id)
    canvas.reset()
    message_id = str(uuid4())
    if not session_id:
        query = canvas.get_preset_param()
        if query:
            for ele in query:
                if not ele["optional"]:
                    if not kwargs.get(ele["key"]):
                        assert False, f"`{ele['key']}` is required"
                    ele["value"] = kwargs[ele["key"]]
                if ele["optional"]:
                    if kwargs.get(ele["key"]):
                        ele["value"] = kwargs[ele['key']]
                    else:
                        if "value" in ele:
                            ele.pop("value")
        cvs.dsl = json.loads(str(canvas))
        session_id=get_uuid()
        conv = {
            "id": session_id,
            "dialog_id": cvs.id,
            "user_id": kwargs.get("user_id", "") if isinstance(kwargs, dict) else "",
            "message": [{"role": "assistant", "content": canvas.get_prologue(), "created_at": time.time()}],
            "source": "agent",
            "dsl": cvs.dsl
        }
        API4ConversationService.save(**conv)
        if query:
            yield "data:" + json.dumps({"code": 0,
                                        "message": "",
                                        "data": {
                                            "session_id": session_id,
                                            "answer": canvas.get_prologue(),
                                            "reference": [],
                                            "param": canvas.get_preset_param()
                                        }
                                        },
                                       ensure_ascii=False) + "\n\n"
            yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"
            return
        else:
            conv = API4Conversation(**conv)
    else:
        e, conv = API4ConversationService.get_by_id(session_id)
        assert e, "Session not found!"
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

    final_ans = {"reference": [], "content": ""}
    if stream:
        try:
            for ans in canvas.run(stream=stream):
                if ans.get("running_status"):
                    yield "data:" + json.dumps({"code": 0, "message": "",
                                                "data": {"answer": ans["content"],
                                                         "running_status": True}},
                                               ensure_ascii=False) + "\n\n"
                    continue
                for k in ans.keys():
                    final_ans[k] = ans[k]
                ans = {"answer": ans["content"], "reference": ans.get("reference", [])}
                ans = structure_answer(conv, ans, message_id, session_id)
                yield "data:" + json.dumps({"code": 0, "message": "", "data": ans},
                                           ensure_ascii=False) + "\n\n"

            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "created_at": time.time(), "id": message_id})
            canvas.history.append(("assistant", final_ans["content"]))
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
        except Exception as e:
            traceback.print_exc()
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield "data:" + json.dumps({"code": 500, "message": str(e),
                                        "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                                       ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

    else:
        for answer in canvas.run(stream=False):
            if answer.get("running_status"):
                continue
            final_ans["content"] = "\n".join(answer["content"]) if "content" in answer else ""
            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "id": message_id})
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            conv.dsl = json.loads(str(canvas))

            result = {"answer": final_ans["content"], "reference": final_ans.get("reference", [])}
            result = structure_answer(conv, result, message_id, session_id)
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield result
            break

def completionOpenAI(tenant_id, agent_id, question, session_id=None, stream=True, **kwargs):
    e, cvs = UserCanvasService.get_by_id(agent_id)
    if not e:
        yield { # Directly yield OpenAI-like structure for non-streaming
                    "id": response_id,
                    "object": "chat.completion", # Or "text_completion"
                    "created": int(time.time()),
                    "model": agent_id,
                    "choices": [
                        {
                            "message": {"role": "assistant", "content":  "**ERROR**: Agent not found."},
                            "index": 0,
                            "finish_reason": "stop"
                        }
                    ],
                    "usage": { # Mock usage - replace with actual if you have
                        "prompt_tokens": 0,
                        "completion_tokens": 0,
                        "total_tokens": 0
                    },
                    # Include reference if needed in OpenAI format - depends on your requirement
                }
        return
    if cvs.user_id != tenant_id:
        yield { # Directly yield OpenAI-like structure for non-streaming
                    "id": response_id,
                    "object": "chat.completion", # Or "text_completion"
                    "created": int(time.time()),
                    "model": agent_id,
                    "choices": [
                        {
                            "message": {"role": "assistant", "content":  "**ERROR**: You do not own the agent."},
                            "index": 0,
                            "finish_reason": "stop"
                        }
                    ],
                    "usage": { # Mock usage - replace with actual if you have
                        "prompt_tokens": 0,
                        "completion_tokens": 0,
                        "total_tokens": 0
                    },
                    # Include reference if needed in OpenAI format - depends on your requirement
                }
        return

    if not isinstance(cvs.dsl,str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)
    canvas = Canvas(cvs.dsl, tenant_id)
    canvas.reset()
    message_id = str(uuid4())
    response_id = str(uuid4()) # OpenAI uses unique IDs for each response

    if not session_id:
        query = canvas.get_preset_param()
        if query:
            for ele in query:
                if not ele["optional"]:
                    if not kwargs.get(ele["key"]):
                        yield { # Directly yield OpenAI-like structure for non-streaming
                                "id": response_id,  
                                "object": "chat.completion", # Or "text_completion"
                                "created": int(time.time()),
                                "model": agent_id,
                                "choices": [
                                    {
                                        "message": {"role": "assistant", "content":  f"`{ele['key']}` is required"},
                                        "index": 0,
                                        "finish_reason": "stop"
                                    }
                                ],
                            }
                        return
                    ele["value"] = kwargs[ele["key"]]
                if ele["optional"]:
                    if kwargs.get(ele["key"]):
                        ele["value"] = kwargs[ele['key']]
                    else:
                        if "value" in ele:
                            ele.pop("value")
        cvs.dsl = json.loads(str(canvas))
        session_id=get_uuid()
        conv = {
            "id": session_id,
            "dialog_id": cvs.id,
            "user_id": kwargs.get("user_id", "") if isinstance(kwargs, dict) else "",
            "message": [{"role": "assistant", "content": canvas.get_prologue(), "created_at": time.time()}],
            "source": "agent",
            "dsl": cvs.dsl
        }
        API4ConversationService.save(**conv)
        if query:
            yield "data: " + json.dumps({
                                        "id": response_id, # Add response ID
                                        "object": "chat.completion.chunk" if stream else "chat.completion", # Adjust object type
                                        "created": int(time.time()),
                                        "model": agent_id, # Or a more descriptive model name
                                        "choices": [
                                            {
                                                "delta" if stream else "message": {"role": "assistant", "content": canvas.get_prologue()}, # delta for stream, message for non-stream
                                                "index": 0,
                                                "finish_reason": None
                                            }
                                        ],
                                        "session_id": session_id, # Add session_id if needed
                                        "param": canvas.get_preset_param() # Add param if needed
                                        },
                                       ensure_ascii=False) + "\n\n"
            if stream: # Only send done signal in streaming mode
                yield "data: [DONE]\n\n"
            return
        else:
            conv = API4Conversation(**conv)
    else:
        e, conv = API4ConversationService.get_by_id(session_id)
        if not e:
            yield   { # Directly yield OpenAI-like structure for non-streaming
                
                    "id": response_id,
                    "object": "chat.completion", # Or "text_completion"
                        "created": int(time.time()),
                        "model": agent_id,
                        "choices": [
                            {
                                "message": {"role": "assistant", "content":  "**ERROR**: Session not found!"},
                                "index": 0,
                                "finish_reason": "stop"
                            }
                        ],
                        "usage": { # Mock usage - replace with actual if you have
                            "prompt_tokens": 0,
                            "completion_tokens": 0,
                            "total_tokens": 0
                        },
                        # Include reference if needed in OpenAI format - depends on your requirement
                }
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

    final_ans = {"reference": [], "content": ""}
    if stream:
        try:
            for ans in canvas.run(stream=stream):
                if ans.get("running_status"):
                    yield "data: " + json.dumps({
                                                "id": response_id,
                                                "object": "chat.completion.chunk",
                                                "created": int(time.time()),
                                                "model": agent_id,
                                                "choices": [
                                                    {
                                                        "delta": {"content": ans["content"]},
                                                        "index": 0,
                                                        "finish_reason": None
                                                    }
                                                ]
                                            },
                                               ensure_ascii=False) + "\n\n"
                    continue
                for k in ans.keys():
                    final_ans[k] = ans[k]
                # ans = {"answer": ans["content"], "reference": ans.get("reference", [])} # No need to restructure like original
                # ans = structure_answer(conv, ans, message_id, session_id) # Structure answer might not be needed for OpenAI format

                yield "data: " + json.dumps({
                                            "id": response_id,
                                            "object": "chat.completion.chunk",
                                            "created": int(time.time()),
                                            "model": agent_id,
                                            "choices": [
                                                {
                                                    "delta": {"content": ans["content"]}, # Directly use ans["content"]
                                                    "index": 0,
                                                    "finish_reason": "stop" if not stream else None # mark finish_reason on last chunk
                                                }
                                            ],
                                            # Include reference if needed in OpenAI format - depends on your requirement
                                            # "usage": { ... } # Usage stats if you track them
                                        },
                                           ensure_ascii=False) + "\n\n"

            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "created_at": time.time(), "id": message_id})
            canvas.history.append(("assistant", final_ans["content"]))
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
        except Exception as e:
            traceback.print_exc()
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield {
                    "id": response_id,
                    "object": "chat.completion.chunk",
                    "created": int(time.time()),
                    "model": agent_id,
                    "choices": [
                        {
                            "delta": {"content": "**ERROR**: " + str(e)},
                            "index": 0,
                            "finish_reason": "stop"
                        }
                    ]
                }
            
        if stream: # Only send done signal in streaming mode
            yield "data: [DONE]\n\n"


    else: # stream=False
        try:
            all_answer_content = ""
            for answer in canvas.run(stream=False):
                if answer.get("running_status"):
                    continue
                final_ans["content"] = "\n".join(answer["content"]) if "content" in answer else ""
                final_ans["reference"] = answer.get("reference", [])
                all_answer_content += final_ans["content"] # Accumulate for non-streamed

            final_ans["content"] = all_answer_content
            canvas.messages.append({"role": "assistant", "content": final_ans["content"], "id": message_id})
            if final_ans.get("reference"):
                canvas.reference.append(final_ans["reference"])
            conv.dsl = json.loads(str(canvas))

            # result = {"answer": final_ans["content"], "reference": final_ans.get("reference", [])} # Original structure
            # result = structure_answer(conv, result, message_id, session_id) # Potentially remove structure_answer

            API4ConversationService.append_message(conv.id, conv.to_dict())

            yield { # Directly yield OpenAI-like structure for non-streaming
                    "id": response_id,
                    "object": "chat.completion", # Or "text_completion"
                    "created": int(time.time()),
                    "model": agent_id,
                    "choices": [
                        {
                            "message": {"role": "assistant", "content": final_ans["content"]},
                            "index": 0,
                            "finish_reason": "stop"
                        }
                    ],
                    "usage": { # Mock usage - replace with actual if you have
                        "prompt_tokens": 0,
                        "completion_tokens": 0,
                        "total_tokens": 0
                    },
                    # Include reference if needed in OpenAI format - depends on your requirement
                }

        except Exception as e:
            traceback.print_exc()
            conv.dsl = json.loads(str(canvas))
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield { # Directly yield OpenAI-like structure for non-streaming
                    "id": response_id,
                    "object": "chat.completion", # Or "text_completion"
                    "created": int(time.time()),
                    "model": agent_id,
                    "choices": [
                        {
                            "message": {"role": "assistant", "content":  "**ERROR**: " + str(e)},
                            "index": 0,
                            "finish_reason": "stop"
                        }
                    ],
                    "usage": { # Mock usage - replace with actual if you have
                        "prompt_tokens": 0,
                        "completion_tokens": 0,
                        "total_tokens": 0
                    },
                    # Include reference if needed in OpenAI format - depends on your requirement
                }
