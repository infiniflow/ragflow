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
import logging
import time
from uuid import uuid4
from agent.canvas import Canvas
from api.db import CanvasCategory, TenantPermission
from api.db.db_models import DB, CanvasTemplate, User, UserCanvas, API4Conversation
from api.db.services.api_service import API4ConversationService
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid
from api.utils.api_utils import get_data_openai
import tiktoken
from peewee import fn


class CanvasTemplateService(CommonService):
    model = CanvasTemplate

class DataFlowTemplateService(CommonService):
    """
    Alias of CanvasTemplateService
    """
    model = CanvasTemplate


class UserCanvasService(CommonService):
    model = UserCanvas

    @classmethod
    @DB.connection_context()
    def get_list(cls, tenant_id,
                 page_number, items_per_page, orderby, desc, id, title, canvas_category=CanvasCategory.Agent):
        agents = cls.model.select()
        if id:
            agents = agents.where(cls.model.id == id)
        if title:
            agents = agents.where(cls.model.title == title)
        agents = agents.where(cls.model.user_id == tenant_id)
        agents = agents.where(cls.model.canvas_category == canvas_category)
        if desc:
            agents = agents.order_by(cls.model.getter_by(orderby).desc())
        else:
            agents = agents.order_by(cls.model.getter_by(orderby).asc())

        agents = agents.paginate(page_number, items_per_page)

        return list(agents.dicts())

    @classmethod
    @DB.connection_context()
    def get_all_agents_by_tenant_ids(cls, tenant_ids, user_id):
        # will get all permitted agents, be cautious
        fields = [
            cls.model.id,
            cls.model.avatar,
            cls.model.title,
            cls.model.permission,
            cls.model.canvas_type,
            cls.model.canvas_category
        ]
        # find team agents and owned agents
        agents = cls.model.select(*fields).where(
            (cls.model.user_id.in_(tenant_ids) & (cls.model.permission == TenantPermission.TEAM.value)) | (
                cls.model.user_id == user_id
            )
        )
        # sort by create_time, asc
        agents.order_by(cls.model.create_time.asc())
        # maybe cause slow query by deep paginate, optimize later
        offset, limit = 0, 50
        res = []
        while True:
            ag_batch = agents.offset(offset).limit(limit)
            _temp = list(ag_batch.dicts())
            if not _temp:
                break
            res.extend(_temp)
            offset += limit
        return res

    @classmethod
    @DB.connection_context()
    def get_by_canvas_id(cls, pid):
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
                cls.model.canvas_category,
                User.nickname,
                User.avatar.alias('tenant_avatar'),
            ]
            agents = cls.model.select(*fields) \
            .join(User, on=(cls.model.user_id == User.id)) \
            .where(cls.model.id == pid)
            # obj = cls.model.query(id=pid)[0]
            return True, agents.dicts()[0]
        except Exception as e:
            logging.exception(e)
            return False, None

    @classmethod
    @DB.connection_context()
    def get_basic_info_by_canvas_ids(cls, canvas_id):
        fields = [
            cls.model.id,
            cls.model.avatar,
            cls.model.user_id,
            cls.model.title,
            cls.model.permission,
            cls.model.canvas_category
        ]
        return cls.model.select(*fields).where(cls.model.id.in_(canvas_id)).dicts()

    @classmethod
    @DB.connection_context()
    def get_by_tenant_ids(cls, joined_tenant_ids, user_id,
                          page_number, items_per_page,
                          orderby, desc, keywords, canvas_category=None
                          ):
        fields = [
            cls.model.id,
            cls.model.avatar,
            cls.model.title,
            cls.model.description,
            cls.model.permission,
            cls.model.user_id.alias("tenant_id"),
            User.nickname,
            User.avatar.alias('tenant_avatar'),
            cls.model.update_time,
            cls.model.canvas_category,
        ]
        if keywords:
            agents = cls.model.select(*fields).join(User, on=(cls.model.user_id == User.id)).where(
                (((cls.model.user_id.in_(joined_tenant_ids)) & (cls.model.permission == TenantPermission.TEAM.value)) | (cls.model.user_id == user_id)),
                (fn.LOWER(cls.model.title).contains(keywords.lower()))
            )
        else:
            agents = cls.model.select(*fields).join(User, on=(cls.model.user_id == User.id)).where(
                (((cls.model.user_id.in_(joined_tenant_ids)) & (cls.model.permission == TenantPermission.TEAM.value)) | (cls.model.user_id == user_id))
            )
        if canvas_category:
            agents = agents.where(cls.model.canvas_category == canvas_category)
        if desc:
            agents = agents.order_by(cls.model.getter_by(orderby).desc())
        else:
            agents = agents.order_by(cls.model.getter_by(orderby).asc())

        count = agents.count()
        if page_number and items_per_page:
            agents = agents.paginate(page_number, items_per_page)
        return list(agents.dicts()), count

    @classmethod
    @DB.connection_context()
    def accessible(cls, canvas_id, tenant_id):
        from api.db.services.user_service import UserTenantService
        e, c = UserCanvasService.get_by_canvas_id(canvas_id)
        if not e:
            return False

        tids = [t.tenant_id for t in UserTenantService.query(user_id=tenant_id)]
        if c["user_id"] != canvas_id and c["user_id"]  not in tids:
            return False
        return True


async def completion(tenant_id, agent_id, session_id=None, **kwargs):
    query = kwargs.get("query", "") or kwargs.get("question", "")
    files = kwargs.get("files", [])
    inputs = kwargs.get("inputs", {})
    user_id = kwargs.get("user_id", "")
    custom_header = kwargs.get("custom_header", "")

    if session_id:
        e, conv = API4ConversationService.get_by_id(session_id)
        assert e, "Session not found!"
        if not conv.message:
            conv.message = []
        if not isinstance(conv.dsl, str):
            conv.dsl = json.dumps(conv.dsl, ensure_ascii=False)
        canvas = Canvas(conv.dsl, tenant_id, agent_id, custom_header=custom_header)
    else:
        e, cvs = UserCanvasService.get_by_id(agent_id)
        assert e, "Agent not found."
        assert cvs.user_id == tenant_id, "You do not own the agent."
        if not isinstance(cvs.dsl, str):
            cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)
        session_id=get_uuid()
        canvas = Canvas(cvs.dsl, tenant_id, agent_id, canvas_id=cvs.id, custom_header=custom_header)
        canvas.reset()
        conv = {
            "id": session_id,
            "dialog_id": cvs.id,
            "user_id": user_id,
            "message": [],
            "source": "agent",
            "dsl": cvs.dsl,
            "reference": []
        }
        API4ConversationService.save(**conv)
        conv = API4Conversation(**conv)

    message_id = str(uuid4())
    conv.message.append({
        "role": "user",
        "content": query,
        "id": message_id
    })
    txt = ""
    async for ans in canvas.run(query=query, files=files, user_id=user_id, inputs=inputs):
        ans["session_id"] = session_id
        if ans["event"] == "message":
            txt += ans["data"]["content"]
            if ans["data"].get("start_to_think", False):
                txt += "<think>"
            elif ans["data"].get("end_to_think", False):
                txt += "</think>"
        yield "data:" + json.dumps(ans, ensure_ascii=False) + "\n\n"

    conv.message.append({"role": "assistant", "content": txt, "created_at": time.time(), "id": message_id})
    conv.reference = canvas.get_reference()
    conv.errors = canvas.error
    conv.dsl = str(canvas)
    conv = conv.to_dict()
    API4ConversationService.append_message(conv["id"], conv)


async def completion_openai(tenant_id, agent_id, question, session_id=None, stream=True, **kwargs):
    tiktoken_encoder = tiktoken.get_encoding("cl100k_base")
    prompt_tokens = len(tiktoken_encoder.encode(str(question)))
    user_id = kwargs.get("user_id", "")

    if stream:
        completion_tokens = 0
        try:
            async for ans in completion(
                tenant_id=tenant_id,
                agent_id=agent_id,
                session_id=session_id,
                query=question,
                user_id=user_id,
                **kwargs
            ):
                if isinstance(ans, str):
                    try:
                        ans = json.loads(ans[5:])  # remove "data:"
                    except Exception as e:
                        logging.exception(f"Agent OpenAI-Compatible completion_openai parse answer failed: {e}")
                        continue
                if ans.get("event") not in ["message", "message_end"]:
                    continue

                content_piece = ""
                if ans["event"] == "message":
                    content_piece = ans["data"]["content"]

                completion_tokens += len(tiktoken_encoder.encode(content_piece))

                openai_data = get_data_openai(
                        id=session_id or str(uuid4()),
                        model=agent_id,
                        content=content_piece,
                        prompt_tokens=prompt_tokens,
                        completion_tokens=completion_tokens,
                        stream=True
                    )

                if ans.get("data", {}).get("reference", None):
                    openai_data["choices"][0]["delta"]["reference"] = ans["data"]["reference"]

                yield "data: " + json.dumps(openai_data, ensure_ascii=False) + "\n\n"

            yield "data: [DONE]\n\n"

        except Exception as e:
            logging.exception(e)
            yield "data: " + json.dumps(
                get_data_openai(
                    id=session_id or str(uuid4()),
                    model=agent_id,
                    content=f"**ERROR**: {str(e)}",
                    finish_reason="stop",
                    prompt_tokens=prompt_tokens,
                    completion_tokens=len(tiktoken_encoder.encode(f"**ERROR**: {str(e)}")),
                    stream=True
                ),
                ensure_ascii=False
            ) + "\n\n"
            yield "data: [DONE]\n\n"

    else:
        try:
            all_content = ""
            reference = {}
            async for ans in completion(
                tenant_id=tenant_id,
                agent_id=agent_id,
                session_id=session_id,
                query=question,
                user_id=user_id,
                **kwargs
            ):
                if isinstance(ans, str):
                    ans = json.loads(ans[5:])
                if ans.get("event") not in ["message", "message_end"]:
                    continue

                if ans["event"] == "message":
                    all_content += ans["data"]["content"]

                if ans.get("data", {}).get("reference", None):
                    reference.update(ans["data"]["reference"])

            completion_tokens = len(tiktoken_encoder.encode(all_content))

            openai_data = get_data_openai(
                id=session_id or str(uuid4()),
                model=agent_id,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                content=all_content,
                finish_reason="stop",
                param=None
            )

            if reference:
                openai_data["choices"][0]["message"]["reference"] = reference

            yield openai_data
        except Exception as e:
            logging.exception(e)
            yield get_data_openai(
                id=session_id or str(uuid4()),
                model=agent_id,
                prompt_tokens=prompt_tokens,
                completion_tokens=len(tiktoken_encoder.encode(f"**ERROR**: {str(e)}")),
                content=f"**ERROR**: {str(e)}",
                finish_reason="stop",
                param=None
            )
