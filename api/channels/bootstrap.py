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
"""Chat channel runtime, embedded in the RAGFlow API server.

Continuously reconciles the running channel bots against the ``chat_channel``
table: newly added bots are started, deleted ones are stopped, and edited ones
(credential/type change) are restarted — without restarting the server. Inbound
messages are answered with a RAG completion routed through the conversation
wired to that bot. Replaces the standalone ``server.py`` entrypoint.
"""

from __future__ import annotations

import asyncio
import hashlib
import importlib
import json
import logging
import threading

from api.channels.core.base import IncomingMessage, OutgoingMessage

LOGGER = logging.getLogger(__name__)

# Channel packages bundled under api/channels that self-register on import.
_BUNDLED_CHANNELS = (
    "feishu",
    "discord",
    "telegram",
    "line",
    "wecom",
    "qqbot",
    "dingtalk",
    "whatsapp",
)

# How often (seconds) to reconcile running channels against the database.
_RECONCILE_INTERVAL_SECS = 10


def _remove_reasoning_content(txt: str) -> str:
    """Strip ``<think>...</think>`` reasoning blocks from a reply.

    Mirrors ``LLMBundle._remove_reasoning_content`` (and the shared Go
    ``StripThinkTrailing`` helper): everything through the last ``</think>``
    marker is reasoning, and only what follows is shown to the end user.
    """
    if not txt:
        return txt
    first_think_start = txt.find("<think>")
    if first_think_start == -1:
        return txt
    last_think_end = txt.rfind("</think>")
    if last_think_end == -1 or last_think_end < first_think_start:
        return txt
    return txt[last_think_end + len("</think>") :]


async def _send_thinking_message(ch, msg: IncomingMessage) -> None:
    """Send a lightweight "thinking" placeholder before the real reply (WeCom only)."""
    if ch.channel_id != "wecom":
        return
    try:
        await ch.send(
            OutgoingMessage(
                chat_id=msg.chat_id,
                text="🤔 开始思考...",
                reply_to_message_id=msg.message_id or None,
            )
        )
    except Exception:
        LOGGER.warning(
            "[%s:%s] failed to send thinking placeholder",
            ch.channel_id,
            ch.account_id,
            exc_info=True,
        )


def _register_channels() -> None:
    """Import each bundled channel package so it self-registers a builder.

    Each channel is imported independently: a missing optional dependency only
    disables that one channel instead of taking down the whole channel server.
    """
    for name in _BUNDLED_CHANNELS:
        try:
            importlib.import_module(f"api.channels.{name}")
        except Exception as ex:
            LOGGER.warning("chat channel '%s' unavailable: %s", name, ex)


def _fingerprint(channel: str, credential: dict) -> str:
    """Stable hash of the parts that require a channel restart when changed."""
    payload = json.dumps(
        {"channel": channel, "credential": credential},
        sort_keys=True,
        default=str,
    )
    return hashlib.md5(payload.encode("utf-8")).hexdigest()


def _desired_channels() -> dict:
    """Return {chat_channel.id: (channel_type, credential, fingerprint)} for enabled bots."""
    from api.db.services.chat_channel_service import ChatChannelService

    desired: dict = {}
    for row in ChatChannelService.list_active():
        credential = (row.config or {}).get("credential", {}) or {}
        desired[row.id] = (row.channel, credential, _fingerprint(row.channel, credential))
    return desired


def _build_one(account_id: str, channel: str, credential: dict):
    """Build a single Channel instance, or None if the type has no builder."""
    from api.channels.core.registry import build_channels

    # account_id == chat_channel.id.
    instances = build_channels({"channels": {channel: {"accounts": {account_id: credential}}}})
    return instances[0] if instances else None


def _make_chat_handler(ch):
    """Build the inbound-message handler bound to a single channel.

    Resolves the connected target for each inbound message. Agent targets run
    ``_run_agent_completion``; dialog targets run the RAG completion path.
    The connected target is resolved per message, so connection changes take
    effect immediately without restarting the channel. Channels with no
    connected target ignore inbound messages.
    """
    from api.db.services.chat_channel_service import ChatChannelService
    from api.db.services.conversation_service import ConversationService, structure_answer
    from api.db.services.dialog_service import DialogService, async_chat
    from common.misc_utils import get_uuid

    async def handle(msg: IncomingMessage) -> None:
        if not (msg.text or "").strip():
            return

        # account_id == chat_channel.id; re-read so a re-connected dialog applies live.
        e, cc = ChatChannelService.get_by_id(ch.account_id)
        if not e or (not cc.chat_id and not cc.agent_id):
            LOGGER.info(
                "[%s:%s] no dialog/agent connected; ignoring message",
                ch.channel_id,
                ch.account_id,
            )
            return

        if cc.agent_id:
            await _run_agent_completion(ch, cc, msg)
            return

        e, dia = DialogService.get_by_id(cc.chat_id)
        if not e:
            LOGGER.warning("[%s:%s] connected dialog not found: %s", ch.channel_id, ch.account_id, cc.chat_id)
            return

        conv = ConversationService.get_or_create_for_channel(cc.chat_id, ch.account_id, msg.chat_id)
        if conv is None:
            LOGGER.warning("[%s:%s] failed to get conversation for chat %s", ch.channel_id, ch.account_id, msg.chat_id)
            return

        message_id = get_uuid()
        if not conv.message:
            conv.message = []
        conv.message.append({"role": "user", "content": msg.text, "id": message_id})
        if not conv.reference:
            conv.reference = []
        conv.reference = [r for r in conv.reference if r]
        conv.reference.append({"chunks": [], "doc_aggs": []})

        history = []
        for m in conv.message:
            if m["role"] == "system":
                continue
            if m["role"] == "assistant" and not history:
                continue
            history.append(m)

        await _send_thinking_message(ch, msg)

        answer_text = ""
        try:
            chat_kwargs = {"quote": False}
            if "{knowledge}" in (dia.prompt_config or {}).get("system", ""):
                chat_kwargs["knowledge"] = ""
            async for ans in async_chat(dia, history, False, **chat_kwargs):
                structure_answer(conv, ans, message_id, conv.id)
                answer_text = (ans or {}).get("answer", "") or ""
                ConversationService.update_by_id(conv.id, conv.to_dict())
                break
        except Exception as ex:
            LOGGER.exception("[%s:%s] completion failed: %s", ch.channel_id, ch.account_id, ex)
            answer_text = f"**ERROR**: {ex}"

        if answer_text:
            await ch.send(
                OutgoingMessage(
                    chat_id=msg.chat_id,
                    text=_remove_reasoning_content(answer_text),
                    reply_to_message_id=msg.message_id or None,
                )
            )

    return handle


async def _run_agent_completion(ch, cc, msg: IncomingMessage) -> None:
    """Answer an inbound message with the flow agent connected to the channel.

    Each end-user chat keeps its own agent conversation, identified by a
    deterministic session id derived from (agent, channel, chat) so history
    survives restarts and switching between dialog/agent keeps the two
    histories separate. The answer is produced by the same canvas completion
    path used by the agent API (``canvas_service.completion``).
    """
    from peewee import IntegrityError

    from api.db.services.api_service import API4ConversationService
    from api.db.services.canvas_service import UserCanvasService, completion
    from api.db.services.user_canvas_version import UserCanvasVersionService
    from common.misc_utils import thread_pool_exec

    e, _ = await thread_pool_exec(UserCanvasService.get_by_id, cc.agent_id)
    if not e:
        LOGGER.warning(
            "[%s:%s] connected agent not found: %s",
            ch.channel_id,
            ch.account_id,
            cc.agent_id,
        )
        return

    # Keep agent sessions on the same id space as dialog sessions (32 chars)
    # but distinct from them so rebinding never mixes histories.
    session_id = hashlib.sha256(f"agent:{cc.agent_id}:{ch.account_id}:{msg.chat_id}".encode()).hexdigest()[:32]

    exists, _ = await thread_pool_exec(API4ConversationService.get_by_id, session_id)
    if not exists:
        try:
            _, dsl = await thread_pool_exec(
                UserCanvasService.get_agent_dsl_with_release,
                cc.agent_id,
                release_mode=False,
                tenant_id=cc.tenant_id,
            )
        except Exception as ex:
            LOGGER.warning(
                "[%s:%s] failed to load agent dsl %s: %s",
                ch.channel_id,
                ch.account_id,
                cc.agent_id,
                ex,
            )
            return
        try:
            version_title = await thread_pool_exec(
                UserCanvasVersionService.get_latest_version_title,
                cc.agent_id,
                release_mode=False,
            )
        except Exception:
            LOGGER.warning(
                "[%s:%s] failed to load agent version title; continuing",
                ch.channel_id,
                ch.account_id,
                exc_info=True,
            )
            version_title = None
        try:
            await thread_pool_exec(
                API4ConversationService.save,
                id=session_id,
                dialog_id=cc.agent_id,
                user_id=ch.account_id,
                exp_user_id=msg.sender_id,
                name=f"channel:{ch.account_id}:{msg.chat_id}",
                message=[],
                reference=[],
                source="agent",
                dsl=dsl,
                version_title=version_title,
            )
        except IntegrityError:
            # A concurrent first message from the same end user created the
            # session first; keep processing against the existing session.
            LOGGER.info(
                "[%s:%s] agent conversation created concurrently; reusing it",
                ch.channel_id,
                ch.account_id,
            )
        except Exception:
            LOGGER.exception(
                "[%s:%s] failed to create agent conversation",
                ch.channel_id,
                ch.account_id,
            )
            return

    await _send_thinking_message(ch, msg)

    answer_text = ""
    try:
        async for ans in completion(
            tenant_id=cc.tenant_id,
            agent_id=cc.agent_id,
            session_id=session_id,
            query=msg.text,
            user_id=msg.sender_id,
            files=[],
            inputs={},
        ):
            if isinstance(ans, str):
                try:
                    ans = json.loads(ans[5:])  # strip "data:" prefix
                except (ValueError, TypeError):
                    continue
            if not isinstance(ans, dict):
                continue
            if ans.get("event") != "message":
                continue
            data = ans.get("data") or {}
            answer_text += data.get("content") or ""
            if data.get("start_to_think", False):
                answer_text += "<think>"
            elif data.get("end_to_think", False):
                answer_text += "</think>"
    except Exception as ex:
        LOGGER.exception(
            "[%s:%s] agent completion failed",
            ch.channel_id,
            ch.account_id,
        )
        answer_text = f"**ERROR**: {ex}"

    if answer_text:
        await ch.send(
            OutgoingMessage(
                chat_id=msg.chat_id,
                text=_remove_reasoning_content(answer_text),
                reply_to_message_id=msg.message_id or None,
            )
        )


async def _stop_channel(running: dict, account_id: str) -> None:
    entry = running.pop(account_id, None)
    if not entry:
        return
    ch = entry["ch"]
    try:
        await ch.stop()
        LOGGER.info("stopped chat channel %s:%s", ch.channel_id, account_id)
    except Exception as ex:
        LOGGER.error("failed to stop chat channel %s: %s", account_id, ex)


async def _start_channel(running: dict, account_id: str, channel: str, credential: dict, fp: str) -> bool:
    """Build, wire and start one channel. Returns True on success.

    Any failure (e.g. invalid credentials) is contained here so a single bad bot
    config never aborts the reconcile pass for the other channels.
    """
    try:
        ch = _build_one(account_id, channel, credential)
    except Exception as ex:
        LOGGER.error(
            "failed to build chat channel %s (%s); check its credentials: %s",
            account_id,
            channel,
            ex,
        )
        return False
    if ch is None:
        return False

    ch.set_message_handler(_make_chat_handler(ch))
    try:
        await ch.start()
    except Exception as ex:
        LOGGER.error("failed to start chat channel %s (%s): %s", account_id, channel, ex)
        return False

    running[account_id] = {"ch": ch, "fp": fp}
    LOGGER.info("started chat channel %s:%s", ch.channel_id, account_id)
    return True


async def _reconcile(running: dict, failed: dict, stop_event: threading.Event) -> None:
    """Diff desired (DB) vs running channels and apply start/stop/restart.

    ``failed`` remembers configs that could not be started so they are not
    retried (and re-logged) every tick until their credentials change.
    """
    if stop_event.is_set():
        return
    desired = await asyncio.to_thread(_desired_channels)
    if stop_event.is_set():
        return

    # Stop channels that were removed or whose credentials/type changed.
    for account_id in list(running.keys()):
        changed = account_id in desired and desired[account_id][2] != running[account_id]["fp"]
        if account_id not in desired or changed:
            await _stop_channel(running, account_id)

    # Drop remembered failures that are gone or whose config changed, so an
    # edited (hopefully fixed) bot is retried.
    for account_id in list(failed.keys()):
        if account_id not in desired or desired[account_id][2] != failed[account_id]:
            failed.pop(account_id, None)

    active_whatsapp = any(channel == "whatsapp" for channel, _, _ in desired.values())
    if not active_whatsapp:
        active_whatsapp = any(entry["ch"].channel_id == "whatsapp" for entry in running.values())
    from api.channels.whatsapp.gateway import sync_whatsapp_gateway

    try:
        await sync_whatsapp_gateway(active_whatsapp)
    except Exception:
        LOGGER.exception("failed to sync WhatsApp gateway enabled=%s", active_whatsapp)

    # Start channels that are new (skip ones already known to fail with this config).
    for account_id, (channel, credential, fp) in desired.items():
        if account_id in running or failed.get(account_id) == fp:
            continue
        if not await _start_channel(running, account_id, channel, credential, fp):
            failed[account_id] = fp


async def run_channels(stop_event: threading.Event) -> None:
    """Reconcile and run channels until ``stop_event`` is set."""
    _register_channels()

    running: dict = {}
    failed: dict = {}
    try:
        while not stop_event.is_set():
            try:
                await _reconcile(running, failed, stop_event)
            except RuntimeError as ex:
                if stop_event.is_set():
                    LOGGER.info("chat channel reconcile stopped")
                    break
                LOGGER.error("chat channel reconcile failed: %s", ex)
            except Exception as ex:
                LOGGER.error("chat channel reconcile failed: %s", ex)

            for _ in range(_RECONCILE_INTERVAL_SECS):
                if stop_event.is_set():
                    break
                await asyncio.sleep(1)
    finally:
        LOGGER.info("Stopping chat channels...")
        for account_id in list(running.keys()):
            await _stop_channel(running, account_id)


def start_channel_server(stop_event: threading.Event) -> None:
    """Thread entrypoint: run the channel event loop, isolating any failure."""
    try:
        asyncio.run(run_channels(stop_event))
    except Exception as ex:
        LOGGER.exception("Chat channel server crashed: %s", ex)
