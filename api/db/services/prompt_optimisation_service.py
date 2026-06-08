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
"""Automated prompt optimisation pipeline.

Mines evaluation results and user feedback, generates candidate prompt
variants via a meta-LLM call, benchmarks each candidate against a held-out
EvaluationDataset, and promotes the winner atomically.

Usage::

    run_id = PromptOptimisationService.start_run(
        tenant_id=tenant_id,
        source_type="dialog",
        source_id=dialog_id,
        eval_dataset_id=dataset_id,
        triggered_by="manual",
        n_variants=5,
    )
    # poll
    run = PromptOptimisationService.get_run(run_id)
    # promote
    PromptOptimisationService.promote_variant(run_id, variant_id)
"""

import concurrent.futures
import difflib
import json
import logging
import re
import threading
from copy import deepcopy
from typing import Any

from api.db.db_models import DB, Dialog, PromptOptimisationRun, PromptVariant
from api.db.services.common_service import CommonService
from api.db.services.dialog_service import DialogService
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp

logger = logging.getLogger(__name__)

_MAX_CONCURRENT_BENCHMARKS = 3
_MAX_BENCHMARK_CASES = 50


class PromptOptimisationService(CommonService):
    model = PromptOptimisationRun

    # ───────────────────────── quality signals ──────────────────────────

    @classmethod
    def collect_quality_signals(cls, source_id: str, window_days: int = 7) -> dict:
        """Aggregate quality signals for ``source_id`` over the last ``window_days``.

        Returns a ``QualityReport`` dict with keys:
        - ``avg_score``         — rolling average of ``avg_answer_relevancy``
        - ``thumbs_down_rate``  — fraction of downvoted API4Conversation turns
        - ``citation_hit_rate`` — rolling average of ``citation_hit_rate`` metric
        - ``window_days``       — as passed
        """
        from api.db.services.chunk_feedback_service import ChunkFeedbackService
        from api.db.services.evaluation_service import EvaluationService

        avg_score = EvaluationService.get_rolling_score(
            source_id, "avg_answer_relevancy", window_days
        )
        citation_hit_rate = EvaluationService.get_rolling_score(
            source_id, "citation_hit_rate", window_days
        )
        td_rate = ChunkFeedbackService.thumbs_down_rate(source_id, window_days)
        return {
            "avg_score": avg_score,
            "citation_hit_rate": citation_hit_rate,
            "thumbs_down_rate": td_rate,
            "window_days": window_days,
        }

    # ───────────────────────── variant generation ───────────────────────

    @classmethod
    async def generate_variants(cls, prompt_text: str, quality_report: dict,
                                chat_mdl, n: int = 5) -> list[str]:
        """Generate ``n`` improved prompt variants using a meta-LLM call.

        Preserves all ``{placeholder}`` tokens from ``prompt_text``.
        Uses temperature 0.9 to encourage diversity.
        """
        from rag.prompts.template import load_prompt
        from rag.prompts.generator import PROMPT_JINJA_ENV, message_fit_in

        system_tpl = load_prompt("prompt_optimizer_system")
        user_tpl = load_prompt("prompt_optimizer_user")

        system_prompt = PROMPT_JINJA_ENV.from_string(system_tpl).render()
        user_prompt = PROMPT_JINJA_ENV.from_string(user_tpl).render(
            current_prompt=prompt_text,
            quality_report=json.dumps(quality_report, ensure_ascii=False, indent=2),
            n=n,
        )

        _, msg = message_fit_in(
            [{"role": "system", "content": system_prompt},
             {"role": "user", "content": user_prompt}],
            getattr(chat_mdl, "max_length", 8192),
        )

        raw = await chat_mdl.async_chat(
            msg[0]["content"], msg[1:],
            {"temperature": 0.9, "max_tokens": 4096},
        )
        raw = re.sub(r"^.*</think>", "", raw, flags=re.DOTALL).strip()

        variants = cls._parse_variants(raw, n)

        # Preserve placeholders: any variant missing a placeholder from the
        # original is patched by appending the placeholder.
        placeholders = re.findall(r"\{[^}]+\}", prompt_text)
        validated = []
        for v in variants:
            for ph in placeholders:
                if ph not in v:
                    v = v + f"\n{ph}"
            validated.append(v)

        return validated[:n]

    @classmethod
    def _parse_variants(cls, raw: str, n: int) -> list[str]:
        """Extract variant texts from the LLM response.

        Expects a numbered list (1. ... 2. ...) or a JSON array.
        Falls back to returning the raw text as a single variant.
        """
        # Try JSON array
        try:
            stripped = re.sub(r"^```(?:json)?\s*|\s*```$", "", raw.strip(), flags=re.DOTALL)
            parsed = json.loads(stripped)
            if isinstance(parsed, list):
                return [str(v).strip() for v in parsed if v]
        except (json.JSONDecodeError, ValueError):
            pass

        # Try numbered list: "1. ..." or "**1.**"
        parts = re.split(r"\n\s*(?:\*{0,2})(\d+)[.)]\s*(?:\*{0,2})", raw)
        if len(parts) > 2:
            texts = [parts[i].strip() for i in range(2, len(parts), 2) if parts[i].strip()]
            if texts:
                return texts

        return [raw.strip()] if raw.strip() else []

    # ───────────────────────── benchmarking ─────────────────────────────

    @classmethod
    def benchmark_variants(cls, variants: list[str], eval_dataset_id: str,
                           dialog: Any) -> list[dict]:
        """Re-run the evaluation pipeline for each variant against the dataset.

        Caps concurrent runs at ``_MAX_CONCURRENT_BENCHMARKS``.

        Returns a list of ``{prompt_text, score, metrics}`` dicts, sorted
        best-first.
        """
        from api.db.services.evaluation_service import EvaluationService

        # Cap test cases
        cases = EvaluationService.get_test_cases(eval_dataset_id)
        if len(cases) > _MAX_BENCHMARK_CASES:
            cases = cases[:_MAX_BENCHMARK_CASES]

        sem = threading.Semaphore(_MAX_CONCURRENT_BENCHMARKS)
        results = []
        results_lock = threading.Lock()

        def _run_variant(prompt_text: str):
            with sem:
                try:
                    from api.db.db_models import EvaluationRun
                    variant_dialog = deepcopy(dialog)
                    pc = dict(variant_dialog.prompt_config or {})
                    pc["system"] = prompt_text
                    variant_dialog.prompt_config = pc

                    run_id = get_uuid()
                    now = current_timestamp()
                    EvaluationRun.create(
                        id=run_id,
                        dataset_id=eval_dataset_id,
                        dialog_id=getattr(dialog, "id", ""),
                        name=f"variant_benchmark_{run_id[:8]}",
                        config_snapshot={},
                        metrics_summary=None,
                        status="RUNNING",
                        created_by="system",
                        create_time=now,
                        complete_time=None,
                    )
                    EvaluationService._execute_evaluation(run_id, eval_dataset_id, variant_dialog)

                    row = EvaluationRun.get_by_id(run_id)
                    metrics = row.metrics_summary or {}
                    score = metrics.get("avg_answer_relevancy", 0.0)

                    with results_lock:
                        results.append({
                            "prompt_text": prompt_text,
                            "score": score,
                            "metrics": metrics,
                        })
                except Exception as e:
                    logger.error("benchmark_variants: variant run failed: %s", e)
                    with results_lock:
                        results.append({
                            "prompt_text": prompt_text,
                            "score": 0.0,
                            "metrics": {},
                        })

        with concurrent.futures.ThreadPoolExecutor(
            max_workers=_MAX_CONCURRENT_BENCHMARKS
        ) as pool:
            list(pool.map(_run_variant, variants))

        results.sort(key=lambda r: r["score"], reverse=True)
        return results

    # ───────────────────────── run lifecycle ────────────────────────────

    @classmethod
    def start_run(cls, tenant_id: str, source_type: str, source_id: str,
                  eval_dataset_id: str | None = None,
                  triggered_by: str = "manual",
                  n_variants: int = 5) -> str:
        """Create a ``PromptOptimisationRun`` row and kick off the pipeline
        asynchronously in a background thread.

        Returns the new ``run_id``.
        """
        run_id = get_uuid()
        now = current_timestamp()
        PromptOptimisationRun.create(
            id=run_id,
            tenant_id=tenant_id,
            source_type=source_type,
            source_id=source_id,
            eval_dataset_id=eval_dataset_id,
            triggered_by=triggered_by,
            status="pending",
            create_time=now,
            update_time=now,
        )

        t = threading.Thread(
            target=cls._run_pipeline,
            args=(run_id, source_type, source_id, eval_dataset_id, n_variants),
            daemon=True,
        )
        t.start()
        return run_id

    @classmethod
    def _run_pipeline(cls, run_id: str, source_type: str, source_id: str,
                      eval_dataset_id: str | None, n_variants: int) -> None:
        """Background thread: generate variants, optionally benchmark, persist."""
        import asyncio

        def _mark_failed(reason: str) -> None:
            logger.error("PromptOptimisationRun %s failed: %s", run_id, reason)
            try:
                PromptOptimisationRun.update(
                    status="failed",
                    complete_time=current_timestamp(),
                ).where(PromptOptimisationRun.id == run_id).execute()
            except Exception:
                pass

        try:
            PromptOptimisationRun.update(status="running").where(
                PromptOptimisationRun.id == run_id
            ).execute()

            # Resolve dialog / source
            if source_type == "dialog":
                ok, dialog = DialogService.get_by_id(source_id)
                if not ok:
                    _mark_failed(f"Dialog {source_id} not found")
                    return
                current_prompt = (dialog.prompt_config or {}).get("system", "")
            else:
                _mark_failed(f"source_type '{source_type}' not yet supported in pipeline")
                return

            if not current_prompt:
                _mark_failed("source prompt is empty; nothing to optimise")
                return

            quality_report = cls.collect_quality_signals(source_id)

            # Get chat model
            from api.db.services.dialog_service import get_models
            _, _, _, chat_mdl, _ = get_models(dialog)

            # Generate variants
            loop = asyncio.new_event_loop()
            asyncio.set_event_loop(loop)
            try:
                variants_text = loop.run_until_complete(
                    cls.generate_variants(current_prompt, quality_report, chat_mdl, n_variants)
                )
            finally:
                loop.close()

            if not variants_text:
                _mark_failed("generate_variants returned no variants")
                return

            # Persist variant rows
            now = current_timestamp()
            variant_ids = []
            for vt in variants_text:
                vid = get_uuid()
                PromptVariant.create(
                    id=vid,
                    source_type=source_type,
                    source_id=source_id,
                    prompt_text=vt,
                    score=None,
                    status="candidate",
                    create_time=now,
                    update_time=now,
                )
                variant_ids.append(vid)

            winner_variant_id = None

            if eval_dataset_id:
                benchmark_results = cls.benchmark_variants(variants_text, eval_dataset_id, dialog)
                # Update scores on variant rows
                for br, vid in zip(
                    sorted(benchmark_results, key=lambda r: variants_text.index(r["prompt_text"])
                           if r["prompt_text"] in variants_text else 0),
                    variant_ids,
                ):
                    PromptVariant.update(score=br["score"]).where(
                        PromptVariant.id == vid
                    ).execute()

                if benchmark_results:
                    best_text = benchmark_results[0]["prompt_text"]
                    for vid_candidate, vt in zip(variant_ids, variants_text):
                        if vt == best_text:
                            winner_variant_id = vid_candidate
                            break

            PromptOptimisationRun.update(
                status="completed",
                complete_time=current_timestamp(),
                winner_variant_id=winner_variant_id,
            ).where(PromptOptimisationRun.id == run_id).execute()

        except Exception as e:
            _mark_failed(str(e))

    @classmethod
    def get_run(cls, run_id: str) -> dict | None:
        """Return run details including all associated variant rows."""
        try:
            run = PromptOptimisationRun.get_by_id(run_id)
        except PromptOptimisationRun.DoesNotExist:
            return None

        run_dict = run.to_dict()
        variants = list(
            PromptVariant
            .select()
            .where(
                (PromptVariant.source_id == run.source_id) &
                (PromptVariant.status == "candidate")
            )
            .order_by(PromptVariant.score.desc(nulls="last"))
            .dicts()
        )
        run_dict["variants"] = variants
        return run_dict

    # ───────────────────────── promotion ────────────────────────────────

    @classmethod
    def promote_variant(cls, run_id: str, variant_id: str) -> tuple[bool, str]:
        """Atomically promote ``variant_id`` to ``active`` for its source.

        - Archives the previously active variant as ``retired``.
        - Updates ``Dialog.prompt_config.system`` and ``Dialog.prompt_variant_id``.

        Returns ``(success, message)``.
        """
        try:
            run = PromptOptimisationRun.get_by_id(run_id)
            variant = PromptVariant.get_by_id(variant_id)
        except (PromptOptimisationRun.DoesNotExist, PromptVariant.DoesNotExist) as e:
            return False, str(e)

        if variant.source_id != run.source_id:
            return False, "Variant does not belong to this run's source."

        with DB.atomic():
            # Retire existing active variants for this source
            PromptVariant.update(status="retired").where(
                (PromptVariant.source_id == run.source_id) &
                (PromptVariant.status == "active")
            ).execute()

            # Archive the current prompt as a retired variant so it is recoverable
            if run.source_type == "dialog":
                ok, dialog = DialogService.get_by_id(run.source_id)
                if not ok:
                    return False, f"Dialog {run.source_id} not found"

                old_prompt = (dialog.prompt_config or {}).get("system", "")
                if old_prompt:
                    PromptVariant.create(
                        id=get_uuid(),
                        source_type="dialog",
                        source_id=run.source_id,
                        prompt_text=old_prompt,
                        score=None,
                        status="retired",
                        create_time=current_timestamp(),
                        update_time=current_timestamp(),
                    )

                new_pc = dict(dialog.prompt_config or {})
                new_pc["system"] = variant.prompt_text
                Dialog.update(
                    prompt_config=new_pc,
                    prompt_variant_id=variant_id,
                ).where(Dialog.id == run.source_id).execute()

            # Mark the variant as active
            PromptVariant.update(
                status="active",
                promoted_at=current_timestamp(),
                update_time=current_timestamp(),
            ).where(PromptVariant.id == variant_id).execute()

            # Update the run's winner
            PromptOptimisationRun.update(
                winner_variant_id=variant_id,
            ).where(PromptOptimisationRun.id == run_id).execute()

        return True, "Variant promoted successfully."

    # ───────────────────────── variant helpers ──────────────────────────

    @classmethod
    def list_variants(cls, source_type: str, source_id: str) -> list[dict]:
        """Return all ``PromptVariant`` rows for a source, newest first."""
        return list(
            PromptVariant
            .select()
            .where(
                (PromptVariant.source_type == source_type) &
                (PromptVariant.source_id == source_id)
            )
            .order_by(PromptVariant.create_time.desc())
            .dicts()
        )

    @classmethod
    def get_variant_diff(cls, variant_id: str, current_prompt: str) -> str:
        """Return a unified diff between ``current_prompt`` and the variant."""
        try:
            variant = PromptVariant.get_by_id(variant_id)
        except PromptVariant.DoesNotExist:
            return ""

        diff = difflib.unified_diff(
            current_prompt.splitlines(keepends=True),
            variant.prompt_text.splitlines(keepends=True),
            fromfile="current",
            tofile=f"variant/{variant_id[:8]}",
        )
        return "".join(diff)
