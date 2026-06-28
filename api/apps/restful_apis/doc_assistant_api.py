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
import re
from pathlib import Path

from quart import Response

from api.apps import login_required, current_user
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
from api.db.services.llm_service import LLMBundle
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from common.constants import LLMType

logger = logging.getLogger(__name__)

DOCS_DIR = Path(__file__).resolve().parents[3] / "docs"

DOC_ASSISTANT_SYSTEM_PROMPT = (
    "You are RAGFlow Documentation Assistant, an AI helper that answers questions "
    "about the RAGFlow project using official documentation.\n\n"
    "Instructions:\n"
    "- Answer based ONLY on the provided documentation context.\n"
    "- If the context does not contain enough information, say so honestly.\n"
    "- Provide concise, actionable answers.\n"
    "- When applicable, include relevant configuration examples or steps.\n"
    "- Reference the source document path so the user can find more details.\n"
    "- Format your response in Markdown for readability.\n"
)

_doc_cache: dict[str, list[dict]] = {}


def _load_docs() -> list[dict]:
    if "chunks" in _doc_cache:
        return _doc_cache["chunks"]

    chunks: list[dict] = []
    if not DOCS_DIR.is_dir():
        logger.warning("Documentation directory not found: %s", DOCS_DIR)
        return chunks

    for doc_path in sorted(DOCS_DIR.rglob("*")):
        if doc_path.suffix not in (".md", ".mdx"):
            continue
        try:
            text = doc_path.read_text(encoding="utf-8")
        except Exception:
            logger.warning("Failed to read doc file: %s", doc_path, exc_info=True)
            continue

        rel_path = str(doc_path.relative_to(DOCS_DIR))
        sections = _split_into_sections(text, rel_path)
        chunks.extend(sections)

    _doc_cache["chunks"] = chunks
    logger.info("Loaded %d documentation chunks from %s", len(chunks), DOCS_DIR)
    return chunks


def _split_into_sections(text: str, rel_path: str) -> list[dict]:
    sections: list[dict] = []
    heading_pattern = re.compile(r"^(#{1,3})\s+(.+)", re.MULTILINE)
    matches = list(heading_pattern.finditer(text))

    if not matches:
        content = text.strip()
        if content:
            sections.append(
                {
                    "content": content[:2000],
                    "source": rel_path,
                    "heading": _title_from_path(rel_path),
                }
            )
        return sections

    for i, match in enumerate(matches):
        start = match.start()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(text)
        content = text[start:end].strip()
        if content:
            sections.append(
                {
                    "content": content[:2000],
                    "source": rel_path,
                    "heading": match.group(2).strip(),
                }
            )

    return sections


def _title_from_path(rel_path: str) -> str:
    stem = Path(rel_path).stem
    return stem.replace("_", " ").replace("-", " ").title()


def _search_docs(query: str, top_n: int = 5) -> list[dict]:
    chunks = _load_docs()
    if not chunks:
        return []

    query_lower = query.lower()
    terms = [t for t in re.split(r"\W+", query_lower) if len(t) > 2]
    if not terms:
        terms = [query_lower]

    scored: list[tuple[float, dict]] = []
    for chunk in chunks:
        searchable = (chunk["content"] + " " + chunk["heading"]).lower()
        score = 0.0
        for term in terms:
            count = searchable.count(term)
            if count > 0:
                score += count
                if term in chunk["heading"].lower():
                    score += 3.0

        if score > 0:
            scored.append((score, chunk))

    scored.sort(key=lambda x: x[0], reverse=True)
    return [item[1] for item in scored[:top_n]]


def _build_context(relevant_chunks: list[dict]) -> str:
    if not relevant_chunks:
        return "No relevant documentation found."

    parts: list[str] = []
    for i, chunk in enumerate(relevant_chunks, 1):
        parts.append(f"--- Document {i}: {chunk['source']} ---\nSection: {chunk['heading']}\n\n{chunk['content']}\n")
    return "\n".join(parts)


def _build_references(relevant_chunks: list[dict]) -> list[dict]:
    seen: set[str] = set()
    refs: list[dict] = []
    for chunk in relevant_chunks:
        source = chunk["source"]
        if source not in seen:
            seen.add(source)
            refs.append(
                {
                    "source": source,
                    "heading": chunk["heading"],
                    "url": f"https://ragflow.io/docs/dev/{source.removesuffix('.md').removesuffix('.mdx')}",
                }
            )
    return refs


@manager.route("/doc-assistant/ask", methods=["POST"])  # noqa: F821
@login_required
@validate_request("question")
async def doc_assistant_ask():
    try:
        req = await get_request_json()
        question = req["question"].strip()
        if not question:
            return get_data_error_result(message="Question cannot be empty.")

        session_history = req.get("history", [])

        relevant_chunks = _search_docs(question, top_n=5)
        context = _build_context(relevant_chunks)
        references = _build_references(relevant_chunks)

        try:
            chat_model_config = get_tenant_default_model_by_type(current_user.id, LLMType.CHAT)
        except Exception as e:
            return get_data_error_result(message=f"No default chat model configured. Please set one in Settings > Model. ({e})")

        chat_mdl = LLMBundle(current_user.id, chat_model_config)

        system_prompt = DOC_ASSISTANT_SYSTEM_PROMPT + "\n\n--- Documentation Context ---\n" + context

        messages = []
        for msg in session_history[-10:]:
            if msg.get("role") in ("user", "assistant") and msg.get("content"):
                messages.append({"role": msg["role"], "content": msg["content"]})
        messages.append({"role": "user", "content": question})

        gen_conf = {"temperature": 0.3, "max_tokens": 2048}

        stream = req.get("stream", False)
        if stream:

            async def generate():
                try:
                    async for chunk in chat_mdl.async_chat_streamly(system_prompt, messages, gen_conf):
                        if isinstance(chunk, tuple):
                            text = chunk[0] if chunk[0] else ""
                        else:
                            text = str(chunk)
                        payload = json.dumps(
                            {
                                "code": 0,
                                "message": "",
                                "data": {
                                    "answer": text,
                                    "references": references,
                                    "done": False,
                                },
                            },
                            ensure_ascii=False,
                        )
                        yield f"data:{payload}\n\n"
                except Exception as ex:
                    logger.exception("Doc assistant stream error")
                    error_payload = json.dumps(
                        {
                            "code": 500,
                            "message": str(ex),
                            "data": {
                                "answer": f"**ERROR**: {ex}",
                                "references": [],
                                "done": True,
                            },
                        },
                        ensure_ascii=False,
                    )
                    yield f"data:{error_payload}\n\n"

                done_payload = json.dumps(
                    {"code": 0, "message": "", "data": True},
                    ensure_ascii=False,
                )
                yield f"data:{done_payload}\n\n"

            resp = Response(generate(), mimetype="text/event-stream")
            resp.headers.add_header("Cache-control", "no-cache")
            resp.headers.add_header("Connection", "keep-alive")
            resp.headers.add_header("X-Accel-Buffering", "no")
            resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
            return resp

        answer = await chat_mdl.async_chat(system_prompt, messages, gen_conf)

        return get_json_result(
            data={
                "answer": answer,
                "references": references,
            }
        )
    except Exception as ex:
        return server_error_response(ex)


@manager.route("/doc-assistant/status", methods=["GET"])  # noqa: F821
@login_required
async def doc_assistant_status():
    try:
        chunks = _load_docs()
        doc_count = len(chunks)
        return get_json_result(
            data={
                "enabled": doc_count > 0,
                "doc_count": doc_count,
                "docs_dir": str(DOCS_DIR),
            }
        )
    except Exception as ex:
        return server_error_response(ex)
