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
import json
import logging
import random
import re
from copy import deepcopy

import json_repair
import xxhash

from agent.component.llm import LLMParam, LLM
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.prompts.generator import run_toc_from_text


def _strip_markdown(text: str) -> str:
    """Strip common Markdown syntax from a free-form LLM reply so it reads as
    plain text instead of a formatted document.

    document_level mode (see ExtractorParam.document_level) stores its result
    in a plain-text metadata field (e.g. document_description); a
    whole-document summarization prompt routinely comes back with
    headings/bold/lists/tables even when the destination has no Markdown
    renderer. This only removes formatting syntax, not wording.
    """
    t = text.strip()
    t = re.sub(r"^```[a-zA-Z]*\n?", "", t)
    t = re.sub(r"\n?```$", "", t)
    t = re.sub(r"^#{1,6}\s*", "", t, flags=re.MULTILINE)
    t = re.sub(r"\*\*(.+?)\*\*", r"\1", t)
    t = re.sub(r"(?<!\*)\*([^*\n]+?)\*(?!\*)", r"\1", t)
    t = re.sub(r"^\s*[-*]\s+", "", t, flags=re.MULTILINE)
    t = re.sub(r"^\s*\d+\.\s+", "", t, flags=re.MULTILINE)
    t = re.sub(r"^-{3,}\s*$", "", t, flags=re.MULTILINE)
    # Table syntax -> plain text: drop separator rows entirely (pure
    # formatting, e.g. "|---|---|"), but keep cell content from data rows
    # instead of deleting the whole line. The trailing/leading whitespace
    # here is deliberately [ \t]* rather than \s* -- \s matches newlines
    # too, so a greedy \s*$ right before another "|"-led line would eat the
    # blank line between two table rows and merge their content together.
    t = re.sub(r"^[ \t]*\|?[ \t]*:?-{2,}:?[ \t]*(\|[ \t]*:?-{2,}:?[ \t]*)*\|?[ \t]*$", "", t, flags=re.MULTILINE)
    t = re.sub(r"^[ \t]*\|(.*)\|[ \t]*$", lambda m: m.group(1).replace("|", " ").strip(), t, flags=re.MULTILINE)
    t = re.sub(r"^>\s*", "", t, flags=re.MULTILINE)
    t = re.sub(r"\n{2,}", "\n", t)
    t = re.sub(r"[ \t]{2,}", " ", t)
    return t.strip()


class ExtractorParam(ProcessParamBase, LLMParam):
    def __init__(self):
        super().__init__()
        self.field_name = ""
        # When True, all upstream chunks are concatenated into a single
        # document-level request instead of one LLM call per chunk. Every
        # chunk then carries the same result, so whichever chunk a later
        # "last write wins" doc-metadata merge picks is still the
        # document-level answer rather than one arbitrary chunk's. The
        # concatenated text is still subject to the model's context budget
        # (see Extractor._document_level_extract) -- documents larger than
        # that budget are summarized from a truncated prefix/suffix, not
        # reduced in stages.
        self.document_level = False

    def check(self):
        super().check()
        self.check_empty(self.field_name, "Result Destination")


class Extractor(ProcessBase, LLM):
    component_name = "Extractor"

    async def _build_TOC(self, docs):
        self.callback(0.2, message="Start to generate table of content ...")
        docs = sorted(
            docs,
            key=lambda d: (
                d.get("page_num_int", 0)[0] if isinstance(d.get("page_num_int", 0), list) else d.get("page_num_int", 0),
                d.get("top_int", 0)[0] if isinstance(d.get("top_int", 0), list) else d.get("top_int", 0),
            ),
        )
        toc = await run_toc_from_text([d["text"] for d in docs], self.chat_mdl)
        logging.info("------------ T O C -------------\n" + json.dumps(toc, ensure_ascii=False, indent="  "))
        ii = 0
        while ii < len(toc):
            try:
                idx = int(toc[ii]["chunk_id"])
                del toc[ii]["chunk_id"]
                toc[ii]["ids"] = [docs[idx]["id"]]
                if ii == len(toc) - 1:
                    break
                for jj in range(idx + 1, int(toc[ii + 1]["chunk_id"]) + 1):
                    toc[ii]["ids"].append(docs[jj]["id"])
            except Exception as e:
                logging.exception(e)
            ii += 1

        if toc:
            d = deepcopy(docs[-1])
            d["doc_id"] = self._canvas._doc_id
            d["toc"] = json.dumps(toc, ensure_ascii=False)
            d["content_with_weight"] = json.dumps(toc, ensure_ascii=False)
            d["toc_kwd"] = "toc"
            d["available_int"] = 0
            d["page_num_int"] = [100000000]
            d["id"] = xxhash.xxh64((d["content_with_weight"] + str(d["doc_id"])).encode("utf-8", "surrogatepass")).hexdigest()
            return d
        return None

    async def _document_level_extract(self, chunks, chunks_key, args):
        """Summarize every upstream chunk's text in a single LLM call and
        write the same result onto every chunk (see ExtractorParam.document_level).

        The concatenated text is fit to the chat model's context budget via
        the same fit_messages() every other LLM component call site uses, so
        for a document whose combined chunk text exceeds that budget, the
        model sees a truncated prefix/suffix rather than the literal full
        text (see fit_messages()/message_fit_in() in agent/component/llm.py
        and rag/prompts/generator.py). That is still a single call over the
        concatenated document rather than per-chunk calls, which is what
        fixes the "summary of the last chunk only" bug this mode exists for
        -- it does not turn this into a map-reduce summarizer for documents
        larger than one context window.

        Mirrors the retry-until-parsable-JSON pattern LLM._invoke_async uses
        for its own "structured" output (agent/component/llm.py) instead of a
        bespoke parser: ask for a single JSON object, retry on a
        non-dict/unparsable reply via the same json_repair already used
        elsewhere in the codebase (e.g. common/metadata_utils.py), and give up
        quietly -- leaving the field unset -- rather than store whatever prose
        came back.
        """
        args[chunks_key] = "\n\n".join(ck.get("text", "") for ck in chunks if ck.get("text"))
        msg, sys_prompt = self._sys_prompt_and_msg([], args)
        # The concatenated document text doesn't change between retries, so
        # fitting it to the context budget only needs to happen once.
        msg_fit, fit_error = self.fit_messages(sys_prompt, msg, self.chat_mdl.max_length)
        if fit_error:
            # Every other fit_messages() call site in agent/component/llm.py
            # treats a fit error as terminal rather than sending the
            # not-fitted messages anyway (validate_fitted_messages() already
            # means the request is degenerate, e.g. an empty user turn).
            logging.warning("Extractor document_level: %s; skipping field %r.", fit_error, self._param.field_name)
            return

        parsed = None
        for _ in range(self._param.max_retries + 1):
            result = await self.chat_mdl.async_chat(msg_fit[0]["content"], msg_fit[1:], self._param.gen_conf())
            try:
                candidate = json_repair.loads(result)
            except Exception:
                continue
            if not isinstance(candidate, dict):
                continue
            # This field's whole point is open-ended metadata extraction: the
            # sys_prompt (not field_name -- DataflowService._process_chunks()
            # only ever checks the literal chunk key "metadata") decides what
            # keys come back, so there is no fixed schema to validate key
            # names against here (unlike LLM._invoke_async's "structured"
            # output branch, which has one via self._param.outputs). What IS
            # fixed is the value shape update_metadata_to() (common/
            # metadata_utils.py) accepts downstream: str or list-of-str,
            # everything else is silently dropped there. Filter to that same
            # shape here so a reply that's syntactically a JSON object but
            # semantically useless (e.g. {"document_description": {"nested":
            # 1}}) is treated as a failed attempt and retried, instead of
            # being "accepted" and then silently dropped several calls later.
            usable = {k: v for k, v in candidate.items() if isinstance(v, str) or (isinstance(v, list) and all(isinstance(vv, str) for vv in v))}
            if usable:
                parsed = usable
                break

        if parsed is None:
            logging.warning("Extractor document_level: model never returned a usable JSON object for field %r; skipping.", self._param.field_name)
            return

        parsed = {k: (_strip_markdown(v) if isinstance(v, str) else v) for k, v in parsed.items()}
        stored = json.dumps(parsed, ensure_ascii=False)
        for ck in chunks:
            ck[self._param.field_name] = stored

    async def _invoke(self, **kwargs):
        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Start to generate.")
        inputs = self.get_input_elements()
        chunks = []
        chunks_key = ""
        args = {}
        for k, v in inputs.items():
            args[k] = v["value"]
            if isinstance(args[k], list):
                chunks = deepcopy(args[k])
                chunks_key = k

        if chunks:
            if self._param.field_name == "toc":
                for ck in chunks:
                    ck["doc_id"] = self._canvas._doc_id
                    ck["id"] = xxhash.xxh64((ck["text"] + str(ck["doc_id"])).encode("utf-8")).hexdigest()
                toc = await self._build_TOC(chunks)
                chunks.append(toc)
                self.set_output("chunks", chunks)
                return

            if self._param.document_level:
                self.callback(0.3, "Summarizing the whole document in a single pass.")
                await self._document_level_extract(chunks, chunks_key, args)
                self.set_output("chunks", chunks)
                return

            prog = 0
            for i, ck in enumerate(chunks):
                args[chunks_key] = ck["text"]
                msg, sys_prompt = self._sys_prompt_and_msg([], args)
                msg.insert(0, {"role": "system", "content": sys_prompt})
                ck[self._param.field_name] = await self._generate_async(msg)
                prog += 1.0 / len(chunks)
                if i % (len(chunks) // 100 + 1) == 1:
                    self.callback(prog, f"{i + 1} / {len(chunks)}")
            self.set_output("chunks", chunks)
        else:
            msg, sys_prompt = self._sys_prompt_and_msg([], args)
            msg.insert(0, {"role": "system", "content": sys_prompt})
            self.set_output("chunks", [{self._param.field_name: await self._generate_async(msg)}])
