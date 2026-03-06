"""
WixQA A/B Benchmark: Precise KB Routing vs Global Search

Uses the Wix/WixQA dataset from HuggingFace:
  - wix_kb_corpus: 6221 Wix support articles
  - wixqa_expertwritten: 200 expert-written Q&A pairs
  - wixqa_simulated: 200 simulated Q&A pairs

Two KBs are created:
  - wix-expert-articles: articles referenced only by expertwritten questions
  - wix-sim-articles:    articles referenced by simulated questions (may overlap)

A/B comparison:
  - Group A (Baseline): search both KBs for every question
  - Group B (Routing):  route each question to the KB that owns its article(s)

Usage:
    OPENAI_API_KEY=sk-xxx uv run python test/benchmark/wixqa_eval.py \\
        --base-url http://127.0.0.1:9380 \\
        --api-key ragflow-xxx \\
        [--n-expert N] [--n-sim N] [--teardown] [--skip-ragas] [--output report.md]
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional, Set

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from test.benchmark.dataset import (
    create_dataset,
    delete_dataset,
    extract_document_ids,
    list_datasets,
    parse_documents,
    upload_documents,
    wait_for_parse_done,
)
from test.benchmark.http_client import HttpClient
from test.benchmark.metrics import summarize
from test.benchmark.retrieval import build_payload, run_retrieval

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

KB_EXPERT = "wix-expert-articles"
KB_SIM = "wix-sim-articles"

# How many questions to sample from each split (None = all)
DEFAULT_N_EXPERT = 50
DEFAULT_N_SIM = 50

# RAGFlow KB chunk size is limited; cap article content to avoid parse issues
MAX_ARTICLE_CHARS = 8000


# ---------------------------------------------------------------------------
# Data classes
# ---------------------------------------------------------------------------

@dataclass
class WixQuestion:
    question: str
    answer: str                    # ground truth
    article_ids: List[str]         # which corpus articles answer this
    source: str                    # "expert" or "simulated"
    kb_name: str                   # which KB to route to


@dataclass
class RetrievalResult:
    question: str
    contexts: List[str]
    latency_ms: float
    error: Optional[str] = None


# ---------------------------------------------------------------------------
# Dataset loading
# ---------------------------------------------------------------------------

def _load_wixqa(n_expert: int, n_sim: int):
    from datasets import load_dataset

    print("[data] Loading WixQA from HuggingFace...")
    corpus_ds = load_dataset("Wix/WixQA", "wix_kb_corpus", split="train")
    expert_ds = load_dataset("Wix/WixQA", "wixqa_expertwritten", split="train")
    sim_ds = load_dataset("Wix/WixQA", "wixqa_simulated", split="train")

    corpus: Dict[str, Dict] = {row["id"]: row for row in corpus_ds}

    # Collect article_id sets per split
    expert_ids: Set[str] = set(aid for ex in expert_ds for aid in ex["article_ids"])
    sim_ids: Set[str] = set(aid for ex in sim_ds for aid in ex["article_ids"])

    # KB assignment: articles exclusively in expert go to KB_EXPERT,
    # articles in simulated (including overlap) go to KB_SIM.
    # Questions are routed based on where the majority of their articles live.
    expert_only_ids = expert_ids - sim_ids
    sim_kb_ids = sim_ids  # KB_SIM covers all simulated articles

    def _assign_kb(article_ids: List[str]) -> str:
        """Route question to the KB that owns more of its articles."""
        in_expert = sum(1 for a in article_ids if a in expert_only_ids)
        in_sim = sum(1 for a in article_ids if a in sim_kb_ids)
        return KB_EXPERT if in_expert >= in_sim else KB_SIM

    def _to_questions(rows, source: str, limit: int) -> List[WixQuestion]:
        questions = []
        for row in list(rows)[:limit]:
            # Skip questions whose articles are not in corpus
            if not all(a in corpus for a in row["article_ids"]):
                continue
            kb = _assign_kb(row["article_ids"])
            questions.append(WixQuestion(
                question=row["question"],
                answer=row["answer"],
                article_ids=row["article_ids"],
                source=source,
                kb_name=kb,
            ))
        return questions

    expert_qs = _to_questions(expert_ds, "expert", n_expert)
    sim_qs = _to_questions(sim_ds, "simulated", n_sim)

    print(f"[data] Expert questions: {len(expert_qs)}, Simulated questions: {len(sim_qs)}")
    print(f"[data] Expert-only article IDs: {len(expert_only_ids)}, Sim article IDs: {len(sim_kb_ids)}")

    return corpus, expert_qs, sim_qs, expert_only_ids, sim_kb_ids


# ---------------------------------------------------------------------------
# Evaluator
# ---------------------------------------------------------------------------

class WixQAEvaluator:
    def __init__(self, client: HttpClient, teardown: bool = False):
        self.client = client
        self.teardown = teardown
        self._dataset_ids: Dict[str, str] = {}   # kb_name -> ragflow dataset id
        self._corpus: Dict[str, Dict] = {}

    # ------------------------------------------------------------------
    # Setup
    # ------------------------------------------------------------------

    def setup(
        self,
        corpus: Dict[str, Dict],
        expert_article_ids: Set[str],
        sim_article_ids: Set[str],
    ) -> None:
        self._corpus = corpus
        kb_articles = {
            KB_EXPERT: expert_article_ids - sim_article_ids,  # exclusive to expert
            KB_SIM: sim_article_ids,
        }

        with tempfile.TemporaryDirectory(prefix="wixqa_eval_") as tmp_dir:
            tmp = Path(tmp_dir)
            for kb_name, article_ids in kb_articles.items():
                self._setup_kb(kb_name, article_ids, tmp)

        print(f"[setup] Done. Dataset IDs: {self._dataset_ids}")

    def _setup_kb(self, kb_name: str, article_ids: Set[str], tmp_dir: Path) -> None:
        # Reuse existing KB if available
        try:
            existing = list_datasets(self.client, name=kb_name)
        except Exception:
            existing = []
        if existing:
            self._dataset_ids[kb_name] = existing[0]["id"]
            print(f"  [skip] '{kb_name}' already exists (id={existing[0]['id']}, "
                  f"{len(article_ids)} articles)")
            return

        ds = create_dataset(self.client, kb_name)
        ds_id = ds["id"]
        self._dataset_ids[kb_name] = ds_id
        print(f"  [created] '{kb_name}' (id={ds_id}, {len(article_ids)} articles)")

        # Write each article as a separate .txt file
        file_paths = []
        for aid in article_ids:
            article = self._corpus.get(aid)
            if not article:
                continue
            content = article.get("contents", "") or ""
            content = content[:MAX_ARTICLE_CHARS]
            title = article.get("title", aid)[:120]
            safe_name = aid[:16]
            fpath = tmp_dir / f"{safe_name}.txt"
            fpath.write_text(f"# {title}\n\n{content}", encoding="utf-8")
            file_paths.append(str(fpath))

        if not file_paths:
            print(f"  [warn] No articles found for '{kb_name}'")
            return

        # Upload in batches of 50 to avoid timeouts
        all_doc_ids = []
        batch_size = 50
        for i in range(0, len(file_paths), batch_size):
            batch = file_paths[i:i + batch_size]
            docs_meta = upload_documents(self.client, ds_id, batch)
            all_doc_ids.extend(extract_document_ids(docs_meta))
            print(f"  [upload] '{kb_name}' batch {i // batch_size + 1}: "
                  f"{len(batch)} files uploaded")

        parse_documents(self.client, ds_id, all_doc_ids)
        print(f"  [parsing] '{kb_name}' — waiting for {len(all_doc_ids)} docs...")
        wait_for_parse_done(self.client, ds_id, all_doc_ids, timeout=300, interval=5)
        print(f"  [ready] '{kb_name}'")

    # ------------------------------------------------------------------
    # Retrieval
    # ------------------------------------------------------------------

    def _retrieve(self, question: str, dataset_ids: List[str]) -> RetrievalResult:
        payload = build_payload(
            question=question,
            dataset_ids=dataset_ids,
            payload={"similarity_threshold": 0.2, "vector_similarity_weight": 0.3, "page_size": 3},
        )
        sample = run_retrieval(self.client, payload)
        latency_ms = (sample.latency or 0.0) * 1000

        if sample.error:
            return RetrievalResult(question=question, contexts=[], latency_ms=latency_ms, error=sample.error)

        chunks = sample.response.get("data", {}).get("chunks", []) if sample.response else []
        contexts = [c.get("content_with_weight") or c.get("content", "") for c in chunks]
        return RetrievalResult(question=question, contexts=contexts, latency_ms=latency_ms)

    def run_baseline(self, questions: List[WixQuestion]) -> List[RetrievalResult]:
        """Group A: search ALL KBs for every question."""
        print(f"\n[A] Baseline — global search across both KBs ({len(questions)} questions)...")
        all_ids = list(self._dataset_ids.values())
        results = []
        for i, q in enumerate(questions, 1):
            print(f"  [{i}/{len(questions)}] {q.question[:70]}...")
            results.append(self._retrieve(q.question, all_ids))
        return results

    def run_with_routing(self, questions: List[WixQuestion]) -> List[RetrievalResult]:
        """Group B: route each question to its correct KB."""
        print(f"\n[B] Routing — precise KB per question ({len(questions)} questions)...")
        results = []
        for i, q in enumerate(questions, 1):
            print(f"  [{i}/{len(questions)}] [{q.kb_name}] {q.question[:60]}...")
            target_id = self._dataset_ids.get(q.kb_name)
            ids = [target_id] if target_id else list(self._dataset_ids.values())
            results.append(self._retrieve(q.question, ids))
        return results

    # ------------------------------------------------------------------
    # ragas evaluation
    # ------------------------------------------------------------------

    def evaluate_with_ragas(
        self,
        questions: List[WixQuestion],
        results_a: List[RetrievalResult],
        results_b: List[RetrievalResult],
        openai_api_key: str,
    ) -> Dict[str, Any]:
        print("\n[ragas] Evaluating with OpenAI gpt-4o as judge...")
        try:
            from datasets import Dataset
            from openai import OpenAI
            from ragas import evaluate
            from ragas.llms import llm_factory
            from ragas.metrics import ContextPrecision, ContextRecall
        except ImportError as exc:
            print(f"[ragas] Import error: {exc}")
            return {}

        try:
            client = OpenAI(api_key=openai_api_key)
            judge_llm = llm_factory("gpt-4o", client=client)
            metrics = [ContextPrecision(llm=judge_llm), ContextRecall(llm=judge_llm)]
        except Exception as exc:
            print(f"[ragas] Failed to init LLM: {exc}")
            return {}

        # Truncate each chunk to avoid exceeding LLM max_tokens during ragas eval
        _MAX_CTX = 300

        _MAX_REF = 400  # Truncate ground truth to limit ragas JSON output size

        def _build_ds(results: List[RetrievalResult]) -> Dataset:
            return Dataset.from_dict({
                "user_input": [q.question for q in questions],
                "retrieved_contexts": [
                    [c[:_MAX_CTX] for c in r.contexts] if r.contexts else [""]
                    for r in results
                ],
                "reference": [q.answer[:_MAX_REF] for q in questions],
            })

        def _avg(result: Any, key: str) -> Optional[float]:
            val = result[key]
            if isinstance(val, list):
                valid = [v for v in val if isinstance(v, (int, float))]
                return float(sum(valid) / len(valid)) if valid else None
            try:
                return float(val)
            except (TypeError, ValueError):
                return None

        scores: Dict[str, Any] = {}
        for group, group_results in [("baseline", results_a), ("routing", results_b)]:
            print(f"  [ragas] Evaluating '{group}'...")
            try:
                result = evaluate(_build_ds(group_results), metrics=metrics, raise_exceptions=False)
                scores[group] = {
                    "context_precision": _avg(result, "context_precision"),
                    "context_recall": _avg(result, "context_recall"),
                }
                cp = scores[group]["context_precision"]
                cr = scores[group]["context_recall"]
                cp_str = f"{cp:.3f}" if cp is not None else "N/A"
                cr_str = f"{cr:.3f}" if cr is not None else "N/A"
                print(f"    precision={cp_str}, recall={cr_str}")
            except Exception as exc:
                print(f"  [ragas] Failed for '{group}': {exc}")
                scores[group] = {"context_precision": None, "context_recall": None}

        return scores

    # ------------------------------------------------------------------
    # Report
    # ------------------------------------------------------------------

    def generate_report(
        self,
        questions: List[WixQuestion],
        results_a: List[RetrievalResult],
        results_b: List[RetrievalResult],
        ragas_scores: Dict[str, Any],
    ) -> str:
        stats_a = summarize([r.latency_ms for r in results_a if not r.error])
        stats_b = summarize([r.latency_ms for r in results_b if not r.error])
        errors_a = sum(1 for r in results_a if r.error)
        errors_b = sum(1 for r in results_b if r.error)

        n_expert = sum(1 for q in questions if q.source == "expert")
        n_sim = sum(1 for q in questions if q.source == "simulated")

        def _f(v, d=3):
            return f"{v:.{d}f}" if v is not None else "N/A"

        def _delta(a, b):
            if a is None or b is None or a == 0:
                return "N/A"
            d = (b - a) / a * 100
            return f"{'+' if d >= 0 else ''}{d:.1f}%"

        cp_a = ragas_scores.get("baseline", {}).get("context_precision")
        cp_b = ragas_scores.get("routing", {}).get("context_precision")
        cr_a = ragas_scores.get("baseline", {}).get("context_recall")
        cr_b = ragas_scores.get("routing", {}).get("context_recall")

        routing_dist = {}
        for q in questions:
            routing_dist[q.kb_name] = routing_dist.get(q.kb_name, 0) + 1

        return f"""\
# WixQA A/B Benchmark Report

## Dataset
- Source: [Wix/WixQA](https://huggingface.co/datasets/Wix/WixQA)
- Questions: {len(questions)} total ({n_expert} expert-written, {n_sim} simulated)
- Knowledge Bases: 2
  - `{KB_EXPERT}`: Wix support articles referenced by expert-written Q&A
  - `{KB_SIM}`: Wix support articles referenced by simulated Q&A

## Routing Distribution
{json.dumps(routing_dist, indent=2)}

## Summary

| Metric            | Baseline (Both KBs) | With Routing | Delta       |
|-------------------|--------------------|--------------|-------------|
| Context Precision | {_f(cp_a)}         | {_f(cp_b)}   | {_delta(cp_a, cp_b)} |
| Context Recall    | {_f(cr_a)}         | {_f(cr_b)}   | {_delta(cr_a, cr_b)} |
| Avg Latency (ms)  | {_f(stats_a.get('avg'), 1)} | {_f(stats_b.get('avg'), 1)} | {_delta(stats_a.get('avg'), stats_b.get('avg'))} |
| Errors            | {errors_a}         | {errors_b}   | —           |

## Latency Details

| Stat  | Baseline | Routing |
|-------|----------|---------|
| avg   | {_f(stats_a.get('avg'), 1)} ms | {_f(stats_b.get('avg'), 1)} ms |
| p50   | {_f(stats_a.get('p50'), 1)} ms | {_f(stats_b.get('p50'), 1)} ms |
| p90   | {_f(stats_a.get('p90'), 1)} ms | {_f(stats_b.get('p90'), 1)} ms |
| p95   | {_f(stats_a.get('p95'), 1)} ms | {_f(stats_b.get('p95'), 1)} ms |

## Dataset IDs
{json.dumps(self._dataset_ids, indent=2)}

## Interpretation
- **Context Precision**: fraction of retrieved chunks that are relevant.
- **Context Recall**: fraction of relevant information that was retrieved.
- Higher precision with routing means MCP Skill eliminates irrelevant KB noise.
- Lower latency reflects fewer documents being searched.
"""

    # ------------------------------------------------------------------
    # Teardown
    # ------------------------------------------------------------------

    def teardown_datasets(self) -> None:
        if not self.teardown:
            return
        print("\n[teardown] Deleting test KBs...")
        for name, ds_id in self._dataset_ids.items():
            try:
                delete_dataset(self.client, ds_id)
                print(f"  [deleted] '{name}' (id={ds_id})")
            except Exception as exc:
                print(f"  [warn] Failed to delete '{name}': {exc}")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main() -> None:
    parser = argparse.ArgumentParser(description="WixQA A/B Benchmark: KB Routing vs Global Search")
    parser.add_argument("--base-url", default="http://127.0.0.1:9380")
    parser.add_argument("--api-key", required=True)
    parser.add_argument("--n-expert", type=int, default=DEFAULT_N_EXPERT,
                        help=f"Number of expert-written questions to use (default: {DEFAULT_N_EXPERT})")
    parser.add_argument("--n-sim", type=int, default=DEFAULT_N_SIM,
                        help=f"Number of simulated questions to use (default: {DEFAULT_N_SIM})")
    parser.add_argument("--teardown", action="store_true", help="Delete KBs after evaluation")
    parser.add_argument("--skip-ragas", action="store_true", help="Skip ragas evaluation")
    parser.add_argument("--output", default=None, help="Write Markdown report to file")
    args = parser.parse_args()

    openai_api_key = os.environ.get("OPENAI_API_KEY", "")
    if not args.skip_ragas and not openai_api_key:
        print("[warn] OPENAI_API_KEY not set — skipping ragas. Use --skip-ragas to suppress.")
        args.skip_ragas = True

    corpus, expert_qs, sim_qs, expert_ids, sim_ids = _load_wixqa(args.n_expert, args.n_sim)
    questions = expert_qs + sim_qs

    client = HttpClient(base_url=args.base_url, api_key=args.api_key)
    evaluator = WixQAEvaluator(client=client, teardown=args.teardown)

    try:
        print(f"\n[setup] Setting up KBs...")
        evaluator.setup(corpus, expert_ids, sim_ids)

        results_a = evaluator.run_baseline(questions)
        results_b = evaluator.run_with_routing(questions)

        ragas_scores: Dict[str, Any] = {}
        if not args.skip_ragas:
            ragas_scores = evaluator.evaluate_with_ragas(
                questions, results_a, results_b, openai_api_key
            )

        report = evaluator.generate_report(questions, results_a, results_b, ragas_scores)
        print("\n" + "=" * 60)
        print(report)

        if args.output:
            Path(args.output).write_text(report, encoding="utf-8")
            print(f"[report] Written to {args.output}")

    finally:
        evaluator.teardown_datasets()


if __name__ == "__main__":
    main()
