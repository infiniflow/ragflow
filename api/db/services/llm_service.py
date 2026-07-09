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
import asyncio
import inspect
import logging
import queue
import re
import threading
from functools import partial
from typing import Generator

from langfuse import propagate_attributes

from api.db.db_models import LLM
from api.db.services.common_service import CommonService
from api.db.services.tenant_llm_service import LLM4Tenant
from common.token_utils import num_tokens_from_string, record_run_token_usage, langfuse_run_attrs


class LLMService(CommonService):
    model = LLM


class LLMBundle(LLM4Tenant):
    def __init__(self, tenant_id: str, model_config: dict, lang="Chinese", **kwargs):
        super().__init__(tenant_id, model_config, lang, **kwargs)

    def _start_langfuse_observation(self, **kwargs):
        # Correlating attributes (session_id/user_id) let Langfuse group all of a
        # turn's generations. They may come from this bundle (chat/dialog path) or,
        # for agent runs whose bundles are created without them, from the per-run
        # context installed by Canvas.run.
        attrs = {}
        if self.langfuse_session_id:
            attrs["session_id"] = self.langfuse_session_id
        run_attrs = langfuse_run_attrs.get()
        if run_attrs:
            for k in ("session_id", "user_id"):
                if run_attrs.get(k) and k not in attrs:
                    attrs[k] = run_attrs[k]
        if attrs:
            with propagate_attributes(**attrs):
                return self.langfuse.start_observation(**kwargs)
        return self.langfuse.start_observation(**kwargs)

    def _reset_last_usage(self) -> None:
        """Clear the model's per-call usage so a failed call that returns before
        updating it cannot leak the previous call's usage into this run."""
        if hasattr(self.mdl, "last_usage"):
            self.mdl.last_usage = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}

    def _report_usage(self, total_tokens: int) -> dict:
        """Record a chat call's usage to the active agent run and return the
        prompt/completion/total split for Langfuse.

        ``total_tokens`` is the authoritative total from the call. The prompt/completion
        split is taken from the provider response (``mdl.last_usage``) only when it is
        consistent with ``total_tokens`` (i.e. produced by this same call); otherwise the
        split is reported as 0 while the total still aggregates correctly.
        """
        split = getattr(self.mdl, "last_usage", None) or {}
        prompt = int(split.get("prompt_tokens", 0) or 0)
        completion = int(split.get("completion_tokens", 0) or 0)
        if not total_tokens:
            total_tokens = int(split.get("total_tokens", 0) or 0)
        if (prompt + completion) != total_tokens:
            # Stale or inconsistent split — keep the total, drop the unreliable split.
            prompt, completion = 0, 0
        record_run_token_usage(prompt, completion, total_tokens)
        return {"input": prompt, "output": completion, "total": total_tokens}

    def close(self):
        """Release resources held by this LLMBundle instance."""
        super().close()

    def __enter__(self):
        """Enter context manager."""
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Exit context manager and release resources."""
        self.close()
        return False

    def bind_tools(self, toolcall_session, tools):
        if not self.is_tools:
            logging.warning("Model does not support tool call, but you have assigned one or more tools to it!")
            return
        self.mdl.bind_tools(toolcall_session, tools)

    def encode(self, texts: list):
        if self.langfuse:
            generation = self._start_langfuse_observation(trace_context=self.trace_context, as_type="generation", name="encode", model=self.model_config["llm_name"], input={"texts": texts})

        safe_texts = []
        for idx, text in enumerate(texts):
            # Embedding APIs (OpenAI-compatible, Zhipu, etc.) reject empty or
            # whitespace-only inputs with errors like "Input at index N cannot
            # be empty or whitespace only". Upstream parsers can produce such
            # chunks — e.g. when OCR/vision on an embedded DOCX image returns
            # nothing, or a table has only empty cells — so coerce to a safe
            # placeholder here, at the single boundary every embedding path
            # funnels through.
            if text is None or not str(text).strip():
                marker = "None" if text is None else "whitespace-only"
                logging.warning(
                    # codeql[py/clear-text-logging-sensitive-data] False positive:
                    # model_config["llm_name"] is the model identifier (e.g.
                    # "gpt-4"), not an API key or credential. CodeQL flags
                    # it as a sensitive data source only because it lives
                    # in the same dict as api_key.
                    "LLMBundle.encode: empty input at index %d (%s) coerced to placeholder 'None' for model %s",
                    idx,
                    marker,
                    self.model_config["llm_name"],
                )
                safe_texts.append("None")
                continue
            token_size = num_tokens_from_string(text)
            if token_size > self.max_length * 0.95:
                target_len = int(self.max_length * 0.95)
                safe_texts.append(text[:target_len])
            else:
                safe_texts.append(text)

        embeddings, used_tokens = self.mdl.encode(safe_texts)
        if self.model_config["llm_factory"] == "Builtin":
            logging.debug("LLMBundle.encode query: {}, emd len: {}, used_tokens: {}. Builtin model don't need to update token usage".format(texts, len(embeddings), used_tokens))
        else:
            logging.info("LLMBundle.encode used_tokens: %d", used_tokens)

        if self.langfuse:
            generation.update(usage_details={"total_tokens": used_tokens})
            generation.end()

        return embeddings, used_tokens

    def encode_queries(self, query: str):
        if self.langfuse:
            generation = self._start_langfuse_observation(trace_context=self.trace_context, as_type="generation", name="encode_queries", model=self.model_config["llm_name"], input={"query": query})

        if query is None or not str(query).strip():
            marker = "None" if query is None else "whitespace-only"
            logging.warning(
                # codeql[py/clear-text-logging-sensitive-data] False positive:
                # llm_name is a model identifier, not a credential. See the
                # matching suppression on the encode() warning above.
                "LLMBundle.encode_queries: empty query (%s) coerced to placeholder 'None' for model %s",
                marker,
                self.model_config["llm_name"],
            )
            query = "None"
        emd, used_tokens = self.mdl.encode_queries(query)
        if self.model_config["llm_factory"] == "Builtin":
            logging.info("LLMBundle.encode_queries query: {}, emd len: {}, used_tokens: {}. Builtin model don't need to update token usage".format(query, len(emd), used_tokens))
        else:
            logging.info("LLMBundle.encode_queries used_tokens: %d", used_tokens)

        if self.langfuse:
            generation.update(usage_details={"total_tokens": used_tokens})
            generation.end()

        return emd, used_tokens

    def similarity(self, query: str, texts: list):
        if self.langfuse:
            generation = self._start_langfuse_observation(
                trace_context=self.trace_context, as_type="generation", name="similarity", model=self.model_config["llm_name"], input={"query": query, "texts": texts}
            )

        sim, used_tokens = self.mdl.similarity(query, texts)
        logging.info("LLMBundle.similarity used_tokens: %d", used_tokens)

        if self.langfuse:
            generation.update(usage_details={"total_tokens": used_tokens})
            generation.end()

        return sim, used_tokens

    def describe(self, image, max_tokens=300):
        if self.langfuse:
            generation = self._start_langfuse_observation(trace_context=self.trace_context, as_type="generation", name="describe", metadata={"model": self.model_config["llm_name"]})

        txt, used_tokens = self.mdl.describe(image)
        logging.info("LLMBundle.describe used_tokens: %d", used_tokens)

        if self.langfuse:
            generation.update(output={"output": txt}, usage_details={"total_tokens": used_tokens})
            generation.end()

        return txt

    def describe_with_prompt(self, image, prompt):
        if self.langfuse:
            generation = self._start_langfuse_observation(
                trace_context=self.trace_context, as_type="generation", name="describe_with_prompt", metadata={"model": self.model_config["llm_name"], "prompt": prompt}
            )

        txt, used_tokens = self.mdl.describe_with_prompt(image, prompt)
        logging.info("LLMBundle.describe_with_prompt used_tokens: %d", used_tokens)

        if self.langfuse:
            generation.update(output={"output": txt}, usage_details={"total_tokens": used_tokens})
            generation.end()

        return txt

    def transcription(self, audio):
        if self.langfuse:
            generation = self._start_langfuse_observation(trace_context=self.trace_context, as_type="generation", name="transcription", metadata={"model": self.model_config["llm_name"]})

        txt, used_tokens = self.mdl.transcription(audio)
        logging.info("LLMBundle.transcription used_tokens: %d", used_tokens)

        if self.langfuse:
            generation.update(output={"output": txt}, usage_details={"total_tokens": used_tokens})
            generation.end()

        return txt

    def stream_transcription(self, audio):
        mdl = self.mdl
        supports_stream = hasattr(mdl, "stream_transcription") and callable(getattr(mdl, "stream_transcription"))
        if supports_stream:
            if self.langfuse:
                generation = self._start_langfuse_observation(
                    as_type="generation",
                    trace_context=self.trace_context,
                    name="stream_transcription",
                    metadata={"model": self.model_config["llm_name"]},
                )
            final_text = ""
            used_tokens = 0

            try:
                for evt in mdl.stream_transcription(audio):
                    if evt.get("event") == "final":
                        final_text = evt.get("text", "")

                    yield evt

            except Exception as e:
                err = {"event": "error", "text": str(e)}
                yield err
                final_text = final_text or ""
            finally:
                if final_text:
                    used_tokens = num_tokens_from_string(final_text)
                    logging.info("LLMBundle.stream_transcription used_tokens: %d", used_tokens)

                if self.langfuse:
                    generation.update(
                        output={"output": final_text},
                        usage_details={"total_tokens": used_tokens},
                    )
                    generation.end()

            return

        if self.langfuse:
            generation = self._start_langfuse_observation(
                as_type="generation",
                trace_context=self.trace_context,
                name="stream_transcription",
                metadata={"model": self.model_config["llm_name"]},
            )

        full_text, used_tokens = mdl.transcription(audio)
        logging.info("LLMBundle.stream_transcription used_tokens: %d", used_tokens)

        if self.langfuse:
            generation.update(
                output={"output": full_text},
                usage_details={"total_tokens": used_tokens},
            )
            generation.end()

        yield {
            "event": "final",
            "text": full_text,
            "streaming": False,
        }

    def tts(self, text: str) -> Generator[bytes, None, None]:
        if self.langfuse:
            generation = self._start_langfuse_observation(trace_context=self.trace_context, as_type="generation", name="tts", input={"text": text})

        for chunk in self.mdl.tts(text):
            if isinstance(chunk, int):
                # codeql[py/clear-text-logging-sensitive-data] False positive:
                # llm_name is a model identifier (e.g. "tts-1"), not a
                # credential. The token count is non-sensitive.
                logging.info("LLMBundle.tts used_tokens: {}, model_name: {}".format(chunk, self.model_config["llm_name"]))
                return
            yield chunk

        if self.langfuse:
            generation.end()

    def _remove_reasoning_content(self, txt: str) -> str:
        if txt is None:
            return None
        first_think_start = txt.find("<think>")
        if first_think_start == -1:
            return txt

        last_think_end = txt.rfind("</think>")
        if last_think_end == -1:
            return txt

        if last_think_end < first_think_start:
            return txt

        return txt[last_think_end + len("</think>") :]

    @staticmethod
    def _clean_param(chat_partial, **kwargs):
        func = chat_partial.func
        sig = inspect.signature(func)
        support_var_args = False
        allowed_params = set()

        for param in sig.parameters.values():
            if param.kind == inspect.Parameter.VAR_KEYWORD:
                support_var_args = True
            elif param.kind in (inspect.Parameter.POSITIONAL_OR_KEYWORD, inspect.Parameter.KEYWORD_ONLY):
                allowed_params.add(param.name)
        if support_var_args:
            return kwargs
        else:
            return {k: v for k, v in kwargs.items() if k in allowed_params}

    def _run_coroutine_sync(self, coro):
        try:
            asyncio.get_running_loop()
        except RuntimeError:
            return asyncio.run(coro)

        result_queue: queue.Queue = queue.Queue()

        def runner():
            try:
                result_queue.put((True, asyncio.run(coro)))
            except Exception as e:
                result_queue.put((False, e))

        thread = threading.Thread(target=runner, daemon=True)
        thread.start()
        thread.join()

        success, value = result_queue.get_nowait()
        if success:
            return value
        raise value

    def _sync_from_async_stream(self, async_gen_fn, *args, **kwargs):
        result_queue: queue.Queue = queue.Queue()

        def runner():
            loop = asyncio.new_event_loop()
            asyncio.set_event_loop(loop)

            async def consume():
                try:
                    async for item in async_gen_fn(*args, **kwargs):
                        result_queue.put(item)
                except Exception as e:
                    result_queue.put(e)
                finally:
                    result_queue.put(StopIteration)

            loop.run_until_complete(consume())
            loop.close()

        threading.Thread(target=runner, daemon=True).start()

        while True:
            item = result_queue.get()
            if item is StopIteration:
                break
            if isinstance(item, Exception):
                raise item
            yield item

    def _bridge_sync_stream(self, gen):
        loop = asyncio.get_running_loop()
        queue: asyncio.Queue = asyncio.Queue()

        def worker():
            try:
                for item in gen:
                    loop.call_soon_threadsafe(queue.put_nowait, item)
            except Exception as e:
                loop.call_soon_threadsafe(queue.put_nowait, e)
            finally:
                loop.call_soon_threadsafe(queue.put_nowait, StopAsyncIteration)

        threading.Thread(target=worker, daemon=True).start()
        return queue

    async def async_chat(self, system: str, history: list, gen_conf: dict = {}, **kwargs):
        if self.is_tools and getattr(self.mdl, "is_tools", False) and hasattr(self.mdl, "async_chat_with_tools"):
            base_fn = self.mdl.async_chat_with_tools
        elif hasattr(self.mdl, "async_chat"):
            base_fn = self.mdl.async_chat
        else:
            raise RuntimeError(f"Model {self.mdl} does not implement async_chat or async_chat_with_tools")

        generation = None
        if self.langfuse:
            generation = self._start_langfuse_observation(
                trace_context=self.trace_context, as_type="generation", name="chat", model=self.model_config["llm_name"], input={"system": system, "history": history}
            )

        chat_partial = partial(base_fn, system, history, gen_conf)
        use_kwargs = self._clean_param(chat_partial, **kwargs)

        self._reset_last_usage()
        try:
            txt, used_tokens = await chat_partial(**use_kwargs)
        except Exception as e:
            if generation:
                generation.update(output={"error": str(e)})
                generation.end()
            raise

        txt = self._remove_reasoning_content(txt)
        if not self.verbose_tool_use:
            txt = re.sub(r"<tool_call>.*?</tool_call>", "", txt, flags=re.DOTALL)

        if used_tokens:
            logging.info("LLMBundle.async_chat used_tokens: %d", used_tokens)

        usage_details = self._report_usage(used_tokens)

        if generation:
            generation.update(output={"output": txt}, usage_details=usage_details)
            generation.end()

        return txt

    async def async_chat_streamly(self, system: str, history: list, gen_conf: dict = {}, **kwargs):
        total_tokens = 0
        ans = ""
        _bundle_is_tools = self.is_tools
        _mdl_is_tools = getattr(self.mdl, "is_tools", False)
        _has_with_tools = hasattr(self.mdl, "async_chat_streamly_with_tools")
        if _bundle_is_tools and _mdl_is_tools and _has_with_tools:
            stream_fn = getattr(self.mdl, "async_chat_streamly_with_tools", None)
        elif hasattr(self.mdl, "async_chat_streamly"):
            stream_fn = getattr(self.mdl, "async_chat_streamly", None)
        else:
            raise RuntimeError(f"Model {self.mdl} does not implement async_chat or async_chat_with_tools")

        generation = None
        if self.langfuse:
            generation = self._start_langfuse_observation(
                trace_context=self.trace_context, as_type="generation", name="chat_streamly", model=self.model_config["llm_name"], input={"system": system, "history": history}
            )

        if stream_fn:
            chat_partial = partial(stream_fn, system, history, gen_conf)
            use_kwargs = self._clean_param(chat_partial, **kwargs)
            self._reset_last_usage()
            try:
                async for txt in chat_partial(**use_kwargs):
                    if isinstance(txt, int):
                        total_tokens = txt
                        break

                    if txt.endswith("</think>") and ans.endswith("</think>"):
                        ans = ans[: -len("</think>")]

                    if not self.verbose_tool_use:
                        txt = re.sub(r"<tool_call>.*?</tool_call>", "", txt, flags=re.DOTALL)

                    ans += txt
                    yield ans
            except Exception as e:
                if generation:
                    generation.update(output={"error": str(e)})
                    generation.end()
                raise
            if total_tokens:
                logging.info("LLMBundle.async_chat_streamly used_tokens: %d", total_tokens)
            usage_details = self._report_usage(total_tokens)
            if generation:
                generation.update(output={"output": ans}, usage_details=usage_details)
                generation.end()
            return

    async def async_chat_streamly_delta(self, system: str, history: list, gen_conf: dict = {}, **kwargs):
        total_tokens = 0
        ans = ""
        if self.is_tools and getattr(self.mdl, "is_tools", False) and hasattr(self.mdl, "async_chat_streamly_with_tools"):
            stream_fn = getattr(self.mdl, "async_chat_streamly_with_tools", None)
        elif hasattr(self.mdl, "async_chat_streamly"):
            stream_fn = getattr(self.mdl, "async_chat_streamly", None)
        else:
            raise RuntimeError(f"Model {self.mdl} does not implement async_chat or async_chat_with_tools")

        generation = None
        if self.langfuse:
            generation = self._start_langfuse_observation(
                trace_context=self.trace_context, as_type="generation", name="chat_streamly", model=self.model_config["llm_name"], input={"system": system, "history": history}
            )

        if stream_fn:
            chat_partial = partial(stream_fn, system, history, gen_conf)
            use_kwargs = self._clean_param(chat_partial, **kwargs)
            self._reset_last_usage()
            try:
                async for txt in chat_partial(**use_kwargs):
                    if isinstance(txt, int):
                        total_tokens = txt
                        break

                    if txt.endswith("</think>") and ans.endswith("</think>"):
                        ans = ans[: -len("</think>")]

                    if not self.verbose_tool_use:
                        txt = re.sub(r"<tool_call>.*?</tool_call>", "", txt, flags=re.DOTALL)

                    ans += txt
                    yield txt
            except Exception as e:
                if generation:
                    generation.update(output={"error": str(e)})
                    generation.end()
                raise
            if total_tokens:
                logging.info("LLMBundle.async_chat_streamly_delta used_tokens: %d", total_tokens)
            usage_details = self._report_usage(total_tokens)
            if generation:
                generation.update(output={"output": ans}, usage_details=usage_details)
                generation.end()
            return
