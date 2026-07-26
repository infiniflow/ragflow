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
import hashlib
import time
import logging
from uuid import uuid4
from peewee import IntegrityError
from common.constants import StatusEnum
from api.db.db_models import Conversation, DB
from api.db.services.api_service import API4ConversationService
from api.db.services.common_service import CommonService
from api.db.services.dialog_service import DialogService, async_chat
from common.misc_utils import get_uuid
import json

from rag.prompts.generator import chunks_format


logger = logging.getLogger(__name__)


class ConversationService(CommonService):
    model = Conversation

    @classmethod
    @DB.connection_context()
    def get_list(cls, dialog_id, page_number, items_per_page, orderby, desc, id, name, user_id=None):
        sessions = cls.model.select().where(cls.model.dialog_id == dialog_id)
        if id:
            sessions = sessions.where(cls.model.id == id)
        if name:
            sessions = sessions.where(cls.model.name == name)
        if user_id:
            sessions = sessions.where(cls.model.user_id == user_id)
        if desc:
            sessions = sessions.order_by(cls.model.getter_by(orderby).desc())
        else:
            sessions = sessions.order_by(cls.model.getter_by(orderby).asc())

        if items_per_page > 0:
            sessions = sessions.paginate(page_number, items_per_page)

        return list(sessions.dicts())

    @classmethod
    @DB.connection_context()
    def get_or_create_for_channel(cls, dialog_id, channel_id, chat_id, name=None):
        """Find or create the conversation backing one channel end-user chat.

        A chat_channel is bound to a dialog; each end-user chat on that channel
        keeps its own conversation history. The conversation is identified by a
        deterministic id derived from (dialog_id, channel_id, chat_id) so
        history persists across restarts without a back-reference column on the
        conversation, while still separating histories when the channel is
        re-bound to a different dialog.
        """
        # Use SHA-256 instead of MD5: CodeQL flags MD5 as a weak
        # sensitive-data hashing primitive. The hash here is only
        # used to derive a deterministic conversation id (not for
        # authentication), but switching to SHA-256 keeps the call
        # site consistent with our hashing policy. Truncating to 32
        # hex chars preserves the existing ID length/shape.
        #
        # We also keep the legacy MD5-derived id as a fallback lookup
        # so existing rows created under the previous hashing scheme
        # are still found on the first read after deploy — without
        # that fallback the writer would create a duplicate
        # conversation (splitting the channel's history).
        sha256_id = hashlib.sha256(f"{dialog_id}:{channel_id}:{chat_id}".encode("utf-8")).hexdigest()[:32]
        # codeql[py/weak-sensitive-data-hashing] Intentional: the
        # MD5 here is a backward-compatibility lookup for rows
        # created under the previous hashing scheme. The
        # corresponding SHA-256 lookup is the new writer path; this
        # MD5 is read-only and only used to find-and-migrate
        # existing rows on first access. It is not used for
        # authentication or any other security-sensitive purpose.
        legacy_id = hashlib.md5(f"{dialog_id}:{channel_id}:{chat_id}".encode("utf-8")).hexdigest()[:32]
        conv = cls.model.get_or_none(cls.model.id == sha256_id)
        if conv is not None:
            # SHA row already present. A previous call may have
            # crashed between the SHA insert and the legacy delete,
            # leaving the MD5 row stranded — clean it up here so
            # dialog_id listings don't show the channel chat twice.
            try:
                cls.model.delete_by_id(legacy_id)
            except cls.model.DoesNotExist:
                pass
            return conv
        # Legacy hit: row was written under the old MD5 id. Migrate it
        # forward: write a new row under the SHA-256 id (carrying over
        # message/reference history) and then delete the legacy row so
        # the listing paths (which select by dialog_id) don't show the
        # same channel chat twice during the rollout window.
        #
        # The cls.save and delete happen under @DB.connection_context()
        # at the class level; the migration is not transactional with
        # the cls.save because the new id write needs to be visible to
        # a competing caller before the legacy delete runs, otherwise a
        # racing reader would briefly see no row at all. Concurrent
        # duplicate inserts are caught via IntegrityError and collapsed
        # to a re-read of the SHA-256 row (see below).
        legacy = cls.model.get_or_none(cls.model.id == legacy_id)
        if legacy is not None:
            try:
                cls.save(
                    id=sha256_id,
                    dialog_id=legacy.dialog_id,
                    name=legacy.name,
                    message=list(legacy.message or []),
                    reference=list(legacy.reference or []),
                )
            except IntegrityError:
                # Another caller won the race and wrote the SHA-256
                # row first. Re-read to return it. If the re-read
                # still misses, this is a real constraint failure
                # (e.g. schema mismatch) — re-raise rather than mask
                # the error as a silent None.
                #
                # The race-winner may also have crashed between its
                # SHA insert and its legacy delete; opportunistically
                # clean that up here too (DoesNotExist is a no-op when
                # the legacy row is already gone).
                conv = cls.model.get_or_none(cls.model.id == sha256_id)
                if conv is not None:
                    try:
                        cls.model.delete_by_id(legacy_id)
                    except cls.model.DoesNotExist:
                        pass
                    return conv
                raise
            else:
                # Migration succeeded; remove the legacy row so it no
                # longer appears in dialog_id listings. Skip if it was
                # already deleted (e.g. by a concurrent migrator).
                try:
                    cls.model.delete_by_id(legacy_id)
                except cls.model.DoesNotExist:
                    pass
                return cls.model.get_or_none(cls.model.id == sha256_id)
        try:
            cls.save(
                id=sha256_id,
                dialog_id=dialog_id,
                name=name or f"channel:{channel_id}:{chat_id}",
                message=[],
                reference=[],
            )
        except IntegrityError:
            # Concurrent caller already inserted the row; re-read.
            # Same rule as above: a missing re-read means this is
            # a real constraint failure, not a race — re-raise.
            conv = cls.model.get_or_none(cls.model.id == sha256_id)
            if conv is not None:
                return conv
            raise
        return cls.model.get_or_none(cls.model.id == sha256_id)

    @classmethod
    @DB.connection_context()
    def get_all_conversation_by_dialog_ids(cls, dialog_ids):
        sessions = cls.model.select().where(cls.model.dialog_id.in_(dialog_ids))
        sessions.order_by(cls.model.create_time.asc())
        offset, limit = 0, 100
        res = []
        while True:
            s_batch = sessions.offset(offset).limit(limit)
            _temp = list(s_batch.dicts())
            if not _temp:
                break
            res.extend(_temp)
            offset += limit
        return res


def structure_answer(conv, ans, message_id, session_id):
    reference = ans["reference"]
    if not isinstance(reference, dict):
        reference = {}
        ans["reference"] = {}
    is_final = ans.get("final", True)

    chunk_list = chunks_format(reference)

    reference["chunks"] = chunk_list
    ans["id"] = message_id
    ans["session_id"] = session_id

    if not conv:
        return ans

    if not conv.message:
        conv.message = []
    content = ans["answer"]
    if ans.get("start_to_think"):
        content = "<think>"
    elif ans.get("end_to_think"):
        content = "</think>"

    if not conv.message or conv.message[-1].get("role", "") != "assistant":
        conv.message.append({"role": "assistant", "content": content, "created_at": time.time(), "id": message_id})
    else:
        if is_final:
            if ans.get("answer"):
                conv.message[-1] = {"role": "assistant", "content": ans["answer"], "created_at": time.time(), "id": message_id}
            else:
                conv.message[-1]["created_at"] = time.time()
                conv.message[-1]["id"] = message_id
        else:
            conv.message[-1]["content"] = (conv.message[-1].get("content") or "") + content
            conv.message[-1]["created_at"] = time.time()
            conv.message[-1]["id"] = message_id
    if conv.reference:
        should_update_reference = is_final or bool(reference.get("chunks")) or bool(reference.get("doc_aggs"))
        if should_update_reference:
            conv.reference[-1] = reference
    return ans


async def async_completion(tenant_id, chat_id, question, name="New session", session_id=None, stream=True, **kwargs):
    assert name, "`name` can not be empty."
    dia = DialogService.query(id=chat_id, tenant_id=tenant_id, status=StatusEnum.VALID.value)
    assert dia, "You do not own the chat."

    if not session_id:
        session_id = get_uuid()
        conv = {
            "id": session_id,
            "dialog_id": chat_id,
            "name": name,
            "message": [{"role": "assistant", "content": dia[0].prompt_config.get("prologue"), "created_at": time.time()}],
            "user_id": kwargs.get("user_id", ""),
        }
        ConversationService.save(**conv)
        if stream:
            yield (
                "data:"
                + json.dumps(
                    {"code": 0, "message": "", "data": {"answer": conv["message"][0]["content"], "reference": {}, "audio_binary": None, "id": None, "session_id": session_id}}, ensure_ascii=False
                )
                + "\n\n"
            )
            yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"
            return
        else:
            answer = {"answer": conv["message"][0]["content"], "reference": {}, "audio_binary": None, "id": None, "session_id": session_id}
            yield answer
            return

    conv = ConversationService.query(id=session_id, dialog_id=chat_id)
    if not conv:
        raise LookupError("Session does not exist")

    conv = conv[0]
    msg = []
    question = {"content": question, "role": "user", "id": str(uuid4())}

    # Propagate runtime attachments so downstream chat flow can resolve file content.
    if isinstance(kwargs.get("files"), list) and kwargs["files"]:
        question["files"] = kwargs["files"]

    conv.message.append(question)
    for m in conv.message:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append(m)
    message_id = msg[-1].get("id")
    e, dia = DialogService.get_by_id(conv.dialog_id)

    kb_ids = kwargs.get("kb_ids", [])
    dia.kb_ids = list(set(dia.kb_ids + kb_ids))
    if not conv.reference:
        conv.reference = []
    conv.message.append({"role": "assistant", "content": "", "id": message_id})
    conv.reference.append({"chunks": [], "doc_aggs": []})

    if stream:
        try:
            async for ans in async_chat(dia, msg, True, session_id=session_id, **kwargs):
                ans = structure_answer(conv, ans, message_id, session_id)
                yield "data:" + json.dumps({"code": 0, "data": ans}, ensure_ascii=False) + "\n\n"
            ConversationService.update_by_id(conv.id, conv.to_dict())
        except Exception as e:
            yield "data:" + json.dumps({"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e), "reference": []}}, ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "data": True}, ensure_ascii=False) + "\n\n"

    else:
        answer = None
        async for ans in async_chat(dia, msg, False, session_id=session_id, **kwargs):
            answer = structure_answer(conv, ans, message_id, session_id)
            ConversationService.update_by_id(conv.id, conv.to_dict())
            break
        yield answer


async def async_iframe_completion(dialog_id, question, session_id=None, stream=True, tenant_id=None, **kwargs):
    if tenant_id:
        exists, dia = DialogService.get_by_id(dialog_id)
        if not exists or getattr(dia, "tenant_id", None) != tenant_id or str(getattr(dia, "status", "")) != StatusEnum.VALID.value:
            logger.warning(
                "Dialog lookup failed for tenant-scoped iframe completion: tenant_id=%s dialog_id=%s required_status=%s",
                tenant_id,
                dialog_id,
                StatusEnum.VALID.value,
            )
            raise AssertionError("Dialog not found")
    else:
        e, dia = DialogService.get_by_id(dialog_id)
        assert e, "Dialog not found"
    if not session_id:
        session_id = get_uuid()
        conv = {"id": session_id, "dialog_id": dialog_id, "user_id": kwargs.get("user_id", ""), "message": [{"role": "assistant", "content": dia.prompt_config["prologue"], "created_at": time.time()}]}
        API4ConversationService.save(**conv)
        yield (
            "data:"
            + json.dumps({"code": 0, "message": "", "data": {"answer": conv["message"][0]["content"], "reference": {}, "audio_binary": None, "id": None, "session_id": session_id}}, ensure_ascii=False)
            + "\n\n"
        )
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"
        return
    else:
        session_id = session_id
        e, conv = API4ConversationService.get_by_id(session_id)
        assert e, "Session not found!"
        assert conv.dialog_id == dialog_id, "Session does not belong to this dialog"

    if not conv.message:
        conv.message = []
    messages = conv.message
    question = {"role": "user", "content": question, "id": str(uuid4())}
    messages.append(question)

    msg = []
    for m in messages:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append(m)
    if not msg[-1].get("id"):
        msg[-1]["id"] = get_uuid()
    message_id = msg[-1]["id"]

    if not conv.reference:
        conv.reference = []
    conv.reference.append({"chunks": [], "doc_aggs": []})

    if stream:
        try:
            async for ans in async_chat(dia, msg, True, session_id=session_id, **kwargs):
                ans = structure_answer(conv, ans, message_id, session_id)
                yield "data:" + json.dumps({"code": 0, "message": "", "data": ans}, ensure_ascii=False) + "\n\n"
            API4ConversationService.append_message(conv.id, conv.to_dict())
        except Exception as e:
            yield "data:" + json.dumps({"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e), "reference": []}}, ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

    else:
        answer = None
        async for ans in async_chat(dia, msg, False, session_id=session_id, **kwargs):
            answer = structure_answer(conv, ans, message_id, session_id)
            API4ConversationService.append_message(conv.id, conv.to_dict())
            break
        yield answer
