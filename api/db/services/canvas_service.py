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
                ans = {"answer": ans["content"], "reference": ans.get("reference", []), "param": canvas.get_preset_param()}
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

            result = {"answer": final_ans["content"], "reference": final_ans.get("reference", []) , "param": canvas.get_preset_param()}
            result = structure_answer(conv, result, message_id, session_id)
            API4ConversationService.append_message(conv.id, conv.to_dict())
            yield result
            break
