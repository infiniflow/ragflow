from typing import Dict, List, Optional


def _fmt_seconds(value: Optional[float]) -> str:
    if value is None:
        return "n/a"
    return f"{value:.4f}s"


def _fmt_ms(value: Optional[float]) -> str:
    if value is None:
        return "n/a"
    return f"{value * 1000.0:.2f}ms"


def _fmt_qps(avg_latency_s: Optional[float]) -> str:
    if avg_latency_s is None or avg_latency_s <= 0:
        return "n/a"
    return f"{1.0 / avg_latency_s:.2f}"


def render_report(lines: List[str]) -> str:
    return "\n".join(lines).strip() + "\n"


def chat_report(
    *,
    interface: str,
    concurrency: int,
    concurrency_note: Optional[str],
    iterations: int,
    success: int,
    failure: int,
    model: str,
    total_stats: Dict[str, Optional[float]],
    first_token_stats: Dict[str, Optional[float]],
    errors: List[str],
    created: Dict[str, str],
) -> str:
    lines = [
        f"Interface: {interface}",
        f"Concurrency: {concurrency}{(' ' + concurrency_note) if concurrency_note else ''}",
        f"Iterations: {iterations}",
        f"Success: {success}",
        f"Failure: {failure}",
        f"Model: {model}",
    ]
    for key, value in created.items():
        lines.append(f"{key}: {value}")
    lines.extend(
        [
            "Latency (total): "
            f"avg={_fmt_ms(total_stats['avg'])}, min={_fmt_ms(total_stats['min'])}, "
            f"p50={_fmt_ms(total_stats['p50'])}, p90={_fmt_ms(total_stats['p90'])}, p95={_fmt_ms(total_stats['p95'])}",
            "Latency (first token): "
            f"avg={_fmt_ms(first_token_stats['avg'])}, min={_fmt_ms(first_token_stats['min'])}, "
            f"p50={_fmt_ms(first_token_stats['p50'])}, p90={_fmt_ms(first_token_stats['p90'])}, p95={_fmt_ms(first_token_stats['p95'])}",
            f"QPS (1 / avg total latency): {_fmt_qps(total_stats['avg'])}",
        ]
    )
    if errors:
        lines.append("Errors: " + "; ".join(errors[:5]))
    return render_report(lines)


def retrieval_report(
    *,
    interface: str,
    concurrency: int,
    concurrency_note: Optional[str],
    iterations: int,
    success: int,
    failure: int,
    stats: Dict[str, Optional[float]],
    errors: List[str],
    created: Dict[str, str],
) -> str:
    lines = [
        f"Interface: {interface}",
        f"Concurrency: {concurrency}{(' ' + concurrency_note) if concurrency_note else ''}",
        f"Iterations: {iterations}",
        f"Success: {success}",
        f"Failure: {failure}",
    ]
    for key, value in created.items():
        lines.append(f"{key}: {value}")
    lines.extend(
        [
            "Latency: "
            f"avg={_fmt_ms(stats['avg'])}, min={_fmt_ms(stats['min'])}, "
            f"p50={_fmt_ms(stats['p50'])}, p90={_fmt_ms(stats['p90'])}, p95={_fmt_ms(stats['p95'])}",
            f"QPS (1 / avg latency): {_fmt_qps(stats['avg'])}",
        ]
    )
    if errors:
        lines.append("Errors: " + "; ".join(errors[:5]))
    return render_report(lines)

