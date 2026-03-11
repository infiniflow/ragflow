"""
MCP Skill A/B Benchmark: Precise Routing vs Global Search

Evaluates the impact of RAGFlow MCP Skill (ragflow_retrieval_skill prompt) by comparing:
  - Group A (Baseline): Global search across ALL knowledge bases
  - Group B (Routing):  Precise routing to the correct knowledge base per question

Uses ragas (context_precision, context_recall) with Kimi API as the judge LLM.

Usage:
    MOONSHOT_API_KEY=xxx uv run python test/benchmark/mcp_skill_eval.py \\
        --base-url http://127.0.0.1:9380 \\
        --api-key ragflow-xxx \\
        [--teardown]
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import textwrap
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional

# ---------------------------------------------------------------------------
# Reuse existing benchmark infrastructure
# ---------------------------------------------------------------------------
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
# Inline test documents (written to temp files during setup)
# ---------------------------------------------------------------------------

_DOC_PYTHON = textwrap.dedent("""\
    # Python Programming Guide

    ## Variables and Types
    Python supports dynamic typing. Variables are created by assignment:
        x = 10
        name = "Alice"
        pi = 3.14159

    ## Functions
    Define functions with the `def` keyword:
        def greet(name):
            return f"Hello, {name}!"

    ## List Comprehensions
    A concise way to create lists:
        squares = [x**2 for x in range(10)]

    ## Classes
    Python is object-oriented:
        class Animal:
            def __init__(self, name):
                self.name = name
            def speak(self):
                return f"{self.name} makes a sound"

    ## Error Handling
    Use try/except blocks:
        try:
            result = 10 / 0
        except ZeroDivisionError:
            print("Cannot divide by zero")

    ## Standard Library
    Python has batteries included: os, sys, json, datetime, collections, itertools.
""")

_DOC_MACHINE_LEARNING = textwrap.dedent("""\
    # Machine Learning Fundamentals

    ## Supervised Learning
    Supervised learning uses labeled training data to learn a mapping from inputs to outputs.
    Common algorithms: Linear Regression, Decision Trees, Random Forests, SVM, Neural Networks.

    ## Unsupervised Learning
    Learns patterns from unlabeled data.
    Common algorithms: K-Means Clustering, PCA, Autoencoders, DBSCAN.

    ## Model Evaluation
    Key metrics:
    - Classification: Accuracy, Precision, Recall, F1-Score, ROC-AUC
    - Regression: MAE, MSE, RMSE, R-squared

    ## Overfitting and Regularization
    Overfitting occurs when a model learns noise in the training data.
    Regularization techniques: L1 (Lasso), L2 (Ridge), Dropout, Early Stopping.

    ## Neural Networks
    A neural network consists of layers of interconnected neurons.
    - Input layer: receives raw features
    - Hidden layers: learn intermediate representations
    - Output layer: produces predictions
    Activation functions: ReLU, Sigmoid, Softmax.

    ## Training Process
    1. Initialize weights randomly
    2. Forward pass: compute predictions
    3. Compute loss (cross-entropy, MSE, etc.)
    4. Backpropagation: compute gradients
    5. Optimizer step: update weights (SGD, Adam, RMSprop)
""")

_DOC_CLOUD_COMPUTING = textwrap.dedent("""\
    # Cloud Computing Concepts

    ## Service Models
    - IaaS (Infrastructure as a Service): Virtual machines, storage, networking. Examples: AWS EC2, Azure VMs.
    - PaaS (Platform as a Service): Managed runtime environments. Examples: Heroku, Google App Engine.
    - SaaS (Software as a Service): Full applications delivered via browser. Examples: Gmail, Salesforce.

    ## Deployment Models
    - Public Cloud: Resources owned and operated by a third-party provider.
    - Private Cloud: Dedicated infrastructure operated for a single organization.
    - Hybrid Cloud: Combination of public and private cloud.

    ## Key Concepts
    - Scalability: Ability to increase/decrease resources on demand.
    - Elasticity: Automatic scaling based on workload.
    - High Availability: Redundant systems to minimize downtime.
    - Fault Tolerance: Continue operating despite component failures.

    ## Containers and Orchestration
    Docker packages applications into portable containers.
    Kubernetes orchestrates containers across clusters:
    - Pods: Smallest deployable unit
    - Services: Stable network endpoint for pods
    - Deployments: Declarative management of pod replicas
    - Namespaces: Logical cluster partitioning

    ## Serverless Computing
    Run code without managing servers. Examples: AWS Lambda, Google Cloud Functions.
    Billing based on actual invocations and execution time.
""")


# ---------------------------------------------------------------------------
# Test question bank
# ---------------------------------------------------------------------------

@dataclass
class Question:
    question: str
    expected_dataset_name: str  # matches KB name created during setup
    ground_truth: str


QUESTION_BANK: List[Question] = [
    # Python questions
    Question("How do you define a function in Python?", "eval-python", "Use the `def` keyword followed by the function name and parameters."),
    Question("What is a list comprehension in Python?", "eval-python", "A concise syntax to create lists: [expr for item in iterable]."),
    Question("How does Python handle errors and exceptions?", "eval-python", "Using try/except blocks to catch and handle exceptions like ZeroDivisionError."),
    Question("What are Python classes and how do you create one?", "eval-python", "Classes are defined with the `class` keyword and use `__init__` for initialization."),
    Question("What does Python's standard library include?", "eval-python", "Modules like os, sys, json, datetime, collections, and itertools."),
    Question("How are variables typed in Python?", "eval-python", "Python uses dynamic typing; variables are created by assignment without declaring a type."),
    Question("What is f-string formatting in Python?", "eval-python", "A way to embed expressions in string literals using f\"...\"."),
    # Machine Learning questions
    Question("What is supervised learning?", "eval-ml", "Learning from labeled data to map inputs to outputs."),
    Question("What metrics are used to evaluate classification models?", "eval-ml", "Accuracy, Precision, Recall, F1-Score, and ROC-AUC."),
    Question("What is overfitting and how can it be prevented?", "eval-ml", "Overfitting is when a model learns noise; prevented by L1/L2 regularization, dropout, early stopping."),
    Question("Explain the neural network training process.", "eval-ml", "Initialize weights, forward pass, compute loss, backpropagation, optimizer update."),
    Question("What is the difference between L1 and L2 regularization?", "eval-ml", "L1 (Lasso) promotes sparsity; L2 (Ridge) penalizes large weights."),
    Question("What activation functions are used in neural networks?", "eval-ml", "ReLU, Sigmoid, and Softmax are common activation functions."),
    Question("What are unsupervised learning algorithms?", "eval-ml", "K-Means Clustering, PCA, Autoencoders, and DBSCAN."),
    # Cloud Computing questions
    Question("What is the difference between IaaS, PaaS, and SaaS?", "eval-cloud", "IaaS provides VMs/storage, PaaS provides managed runtimes, SaaS delivers full applications."),
    Question("What is Kubernetes and what are its key components?", "eval-cloud", "Kubernetes orchestrates containers; key components are Pods, Services, Deployments, and Namespaces."),
    Question("What is the difference between public and private cloud?", "eval-cloud", "Public cloud is shared third-party infrastructure; private cloud is dedicated to one organization."),
    Question("What is serverless computing?", "eval-cloud", "Running code without managing servers, billed by invocations. Examples: AWS Lambda, Google Cloud Functions."),
    Question("What does elasticity mean in cloud computing?", "eval-cloud", "Automatic scaling of resources based on workload demand."),
    Question("What is Docker used for in cloud deployments?", "eval-cloud", "Docker packages applications into portable containers for consistent deployment across environments."),
    Question("What is high availability in cloud computing?", "eval-cloud", "Using redundant systems to minimize downtime and ensure continuous service."),
]


# ---------------------------------------------------------------------------
# Data classes for results
# ---------------------------------------------------------------------------

@dataclass
class RetrievalResult:
    question: str
    expected_dataset: str
    contexts: List[str]
    latency_ms: float
    error: Optional[str] = None


@dataclass
class EvalSample:
    question: str
    expected_dataset: str
    ground_truth: str
    contexts_a: List[str]  # baseline (all KBs)
    contexts_b: List[str]  # routing (correct KB)
    latency_a_ms: float
    latency_b_ms: float


# ---------------------------------------------------------------------------
# Core evaluator
# ---------------------------------------------------------------------------

class MCPSkillEvaluator:
    def __init__(self, client: HttpClient, teardown: bool = False):
        self.client = client
        self.teardown = teardown
        self._dataset_ids: Dict[str, str] = {}  # name -> id

    # ------------------------------------------------------------------
    # Setup: create 3 KBs, upload inline docs, parse, wait
    # ------------------------------------------------------------------

    def setup_test_datasets(self) -> None:
        print("[setup] Creating test knowledge bases...")
        docs = {
            "eval-python": _DOC_PYTHON,
            "eval-ml": _DOC_MACHINE_LEARNING,
            "eval-cloud": _DOC_CLOUD_COMPUTING,
        }
        tmp_dir = Path("/tmp/mcp_skill_eval")
        tmp_dir.mkdir(exist_ok=True)

        for kb_name, content in docs.items():
            # Re-use existing KB if it already exists
            existing = list_datasets(self.client, name=kb_name)
            if existing:
                ds_id = existing[0]["id"]
                print(f"  [skip] '{kb_name}' already exists (id={ds_id})")
                self._dataset_ids[kb_name] = ds_id
                continue

            ds = create_dataset(self.client, kb_name)
            ds_id = ds["id"]
            self._dataset_ids[kb_name] = ds_id
            print(f"  [created] '{kb_name}' (id={ds_id})")

            # Write inline doc to temp file
            tmp_file = tmp_dir / f"{kb_name}.txt"
            tmp_file.write_text(content, encoding="utf-8")

            docs_meta = upload_documents(self.client, ds_id, [str(tmp_file)])
            doc_ids = extract_document_ids(docs_meta)
            parse_documents(self.client, ds_id, doc_ids)
            print(f"  [parsing] '{kb_name}' — waiting for parse to complete...")
            wait_for_parse_done(self.client, ds_id, doc_ids, timeout=120, interval=3)
            print(f"  [ready] '{kb_name}'")

        print(f"[setup] Done. Dataset IDs: {self._dataset_ids}")

    # ------------------------------------------------------------------
    # Retrieval helpers
    # ------------------------------------------------------------------

    def _retrieve(self, question: str, dataset_ids: List[str]) -> RetrievalResult:
        payload = build_payload(
            question=question,
            dataset_ids=dataset_ids,
            payload={"similarity_threshold": 0.2, "vector_similarity_weight": 0.3, "page_size": 10},
        )
        sample = run_retrieval(self.client, payload)
        latency_ms = (sample.latency or 0.0) * 1000

        if sample.error:
            return RetrievalResult(
                question=question,
                expected_dataset="",
                contexts=[],
                latency_ms=latency_ms,
                error=sample.error,
            )

        chunks = sample.response.get("data", {}).get("chunks", []) if sample.response else []
        contexts = [c.get("content_with_weight") or c.get("content", "") for c in chunks]
        return RetrievalResult(
            question=question,
            expected_dataset="",
            contexts=contexts,
            latency_ms=latency_ms,
        )

    def run_baseline(self, questions: List[Question]) -> List[RetrievalResult]:
        """Group A: search ALL knowledge bases (no routing)."""
        print("\n[A] Running baseline (global search across all KBs)...")
        all_ids = list(self._dataset_ids.values())
        results = []
        for i, q in enumerate(questions, 1):
            print(f"  [{i}/{len(questions)}] {q.question[:60]}...")
            result = self._retrieve(q.question, all_ids)
            result.expected_dataset = q.expected_dataset_name
            results.append(result)
        return results

    def run_with_routing(self, questions: List[Question]) -> List[RetrievalResult]:
        """Group B: route to the correct knowledge base."""
        print("\n[B] Running with precise routing (correct KB per question)...")
        results = []
        for i, q in enumerate(questions, 1):
            print(f"  [{i}/{len(questions)}] {q.question[:60]}...")
            target_id = self._dataset_ids.get(q.expected_dataset_name)
            dataset_ids = [target_id] if target_id else list(self._dataset_ids.values())
            result = self._retrieve(q.question, dataset_ids)
            result.expected_dataset = q.expected_dataset_name
            results.append(result)
        return results

    # ------------------------------------------------------------------
    # ragas evaluation
    # ------------------------------------------------------------------

    def evaluate_with_ragas(
        self,
        questions: List[Question],
        results_a: List[RetrievalResult],
        results_b: List[RetrievalResult],
        moonshot_api_key: str,
    ) -> Dict[str, Any]:
        print("\n[ragas] Running evaluation with Kimi as judge LLM...")
        try:
            from datasets import Dataset
            from openai import OpenAI
            from ragas import evaluate
            from ragas.llms import LangchainLLMWrapper
            from ragas.metrics import context_precision, context_recall
        except ImportError as exc:
            print(f"[ragas] Import error: {exc}")
            print("[ragas] Install with: uv sync --group test")
            return {}

        try:
            from langchain_openai import ChatOpenAI

            kimi_llm = LangchainLLMWrapper(
                ChatOpenAI(
                    model="moonshot-v1-8k",
                    openai_api_key=moonshot_api_key,
                    openai_api_base="https://api.moonshot.cn/v1",
                )
            )
        except Exception as exc:
            print(f"[ragas] Failed to create Kimi LLM wrapper: {exc}")
            return {}

        def _build_dataset(results: List[RetrievalResult]) -> Dataset:
            return Dataset.from_dict({
                "user_input": [q.question for q in questions],
                "retrieved_contexts": [r.contexts if r.contexts else [""] for r in results],
                "reference": [q.ground_truth for q in questions],
            })

        scores: Dict[str, Any] = {}
        for group_name, group_results in [("baseline", results_a), ("routing", results_b)]:
            print(f"  [ragas] Evaluating group '{group_name}'...")
            try:
                ds = _build_dataset(group_results)
                result = evaluate(
                    ds,
                    metrics=[context_precision, context_recall],
                    llm=kimi_llm,
                    raise_exceptions=False,
                )
                scores[group_name] = {
                    "context_precision": float(result["context_precision"]),
                    "context_recall": float(result["context_recall"]),
                }
                print(f"    context_precision={scores[group_name]['context_precision']:.3f}, "
                      f"context_recall={scores[group_name]['context_recall']:.3f}")
            except Exception as exc:
                print(f"  [ragas] Evaluation failed for '{group_name}': {exc}")
                scores[group_name] = {"context_precision": None, "context_recall": None}

        return scores

    # ------------------------------------------------------------------
    # Latency stats
    # ------------------------------------------------------------------

    @staticmethod
    def _latency_stats(results: List[RetrievalResult]) -> Dict[str, Any]:
        latencies = [r.latency_ms for r in results if r.error is None]
        return summarize(latencies)

    # ------------------------------------------------------------------
    # Report generation
    # ------------------------------------------------------------------

    def generate_report(
        self,
        results_a: List[RetrievalResult],
        results_b: List[RetrievalResult],
        ragas_scores: Dict[str, Any],
    ) -> str:
        stats_a = self._latency_stats(results_a)
        stats_b = self._latency_stats(results_b)

        errors_a = sum(1 for r in results_a if r.error)
        errors_b = sum(1 for r in results_b if r.error)

        def _fmt(v: Optional[float], decimals: int = 3) -> str:
            return f"{v:.{decimals}f}" if v is not None else "N/A"

        def _delta(a: Optional[float], b: Optional[float], pct: bool = True) -> str:
            if a is None or b is None or a == 0:
                return "N/A"
            d = (b - a) / a * 100
            sign = "+" if d >= 0 else ""
            return f"{sign}{d:.1f}%" if pct else f"{sign}{b - a:.3f}"

        cp_a = ragas_scores.get("baseline", {}).get("context_precision")
        cp_b = ragas_scores.get("routing", {}).get("context_precision")
        cr_a = ragas_scores.get("baseline", {}).get("context_recall")
        cr_b = ragas_scores.get("routing", {}).get("context_recall")

        lat_a = stats_a.get("avg")
        lat_b = stats_b.get("avg")

        report = f"""\
# MCP Skill A/B Benchmark Report

## Summary

| Metric            | Baseline (All KBs) | With Routing | Delta       |
|-------------------|--------------------|--------------|-------------|
| Context Precision | {_fmt(cp_a)}               | {_fmt(cp_b)}         | {_delta(cp_a, cp_b)} |
| Context Recall    | {_fmt(cr_a)}               | {_fmt(cr_b)}         | {_delta(cr_a, cr_b)} |
| Avg Latency (ms)  | {_fmt(lat_a, 1)}             | {_fmt(lat_b, 1)}       | {_delta(lat_a, lat_b)} |
| Errors            | {errors_a}                  | {errors_b}           | —           |

## Latency Details

| Stat  | Baseline | Routing |
|-------|----------|---------|
| avg   | {_fmt(lat_a, 1)} ms | {_fmt(lat_b, 1)} ms |
| p50   | {_fmt(stats_a.get('p50'), 1)} ms | {_fmt(stats_b.get('p50'), 1)} ms |
| p90   | {_fmt(stats_a.get('p90'), 1)} ms | {_fmt(stats_b.get('p90'), 1)} ms |
| p95   | {_fmt(stats_a.get('p95'), 1)} ms | {_fmt(stats_b.get('p95'), 1)} ms |

## Dataset IDs Used

{json.dumps(self._dataset_ids, indent=2)}

## Interpretation

- **Context Precision** measures what fraction of retrieved chunks are relevant to the question.
- **Context Recall** measures what fraction of relevant information was retrieved.
- Higher precision with routing indicates the MCP Skill correctly eliminates irrelevant KBs.
- Lower latency with routing reflects fewer datasets being searched.
"""
        return report

    # ------------------------------------------------------------------
    # Teardown
    # ------------------------------------------------------------------

    def teardown_datasets(self) -> None:
        if not self.teardown:
            return
        print("\n[teardown] Deleting test knowledge bases...")
        for name, ds_id in self._dataset_ids.items():
            try:
                delete_dataset(self.client, ds_id)
                print(f"  [deleted] '{name}' (id={ds_id})")
            except Exception as exc:
                print(f"  [warn] Failed to delete '{name}': {exc}")


# ---------------------------------------------------------------------------
# CLI entry point
# ---------------------------------------------------------------------------

def main() -> None:
    parser = argparse.ArgumentParser(
        description="MCP Skill A/B Benchmark: Precise Routing vs Global Search"
    )
    parser.add_argument("--base-url", default="http://127.0.0.1:9380", help="RAGFlow base URL")
    parser.add_argument("--api-key", required=True, help="RAGFlow API key")
    parser.add_argument("--teardown", action="store_true", help="Delete created KBs after evaluation")
    parser.add_argument("--skip-ragas", action="store_true", help="Skip ragas evaluation (latency only)")
    parser.add_argument("--output", default=None, help="Write Markdown report to this file")
    args = parser.parse_args()

    moonshot_api_key = os.environ.get("MOONSHOT_API_KEY", "")
    if not args.skip_ragas and not moonshot_api_key:
        print("[warn] MOONSHOT_API_KEY not set. ragas evaluation will be skipped. Use --skip-ragas to suppress this warning.")
        args.skip_ragas = True

    client = HttpClient(base_url=args.base_url, api_key=args.api_key)
    evaluator = MCPSkillEvaluator(client=client, teardown=args.teardown)

    try:
        evaluator.setup_test_datasets()

        questions = QUESTION_BANK
        results_a = evaluator.run_baseline(questions)
        results_b = evaluator.run_with_routing(questions)

        ragas_scores: Dict[str, Any] = {}
        if not args.skip_ragas:
            ragas_scores = evaluator.evaluate_with_ragas(questions, results_a, results_b, moonshot_api_key)

        report = evaluator.generate_report(results_a, results_b, ragas_scores)
        print("\n" + "=" * 60)
        print(report)

        if args.output:
            Path(args.output).write_text(report, encoding="utf-8")
            print(f"[report] Written to {args.output}")

    finally:
        evaluator.teardown_datasets()


if __name__ == "__main__":
    main()
