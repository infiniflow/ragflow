from typing import Dict, List, Optional


def _fmt_seconds(value: Optional[float]) -> str:
    if value is None:
        return "n/a"
    return f"{value:.4f}s"


def _fmt_ms(value: Optional[float]) -> str:
    if value is None:
        return "n/a"
    return f"{value * 1000.0:.2f}ms"


def _fmt_qps(qps: Optional[float]) -> str:
    if qps is None or qps <= 0:
        return "n/a"
    return f"{qps:.2f}"


def _calc_qps(total_duration_s: Optional[float], total_requests: int) -> Optional[float]:
    if total_duration_s is None or total_duration_s <= 0:
        return None
    return total_requests / total_duration_s


def render_report(lines: List[str]) -> str:
    return "\n".join(lines).strip() + "\n"


def chat_report(
    *,
    interface: str,
    concurrency: int,
    total_duration_s: Optional[float],
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
        f"Concurrency: {concurrency}",
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
            f"Total Duration: {_fmt_seconds(total_duration_s)}",
            f"QPS (requests / total duration): {_fmt_qps(_calc_qps(total_duration_s, iterations))}",
        ]
    )
    if errors:
        lines.append("Errors: " + "; ".join(errors[:5]))
    return render_report(lines)


def retrieval_report(
    *,
    interface: str,
    concurrency: int,
    total_duration_s: Optional[float],
    iterations: int,
    success: int,
    failure: int,
    stats: Dict[str, Optional[float]],
    errors: List[str],
    created: Dict[str, str],
) -> str:
    lines = [
        f"Interface: {interface}",
        f"Concurrency: {concurrency}",
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
            f"Total Duration: {_fmt_seconds(total_duration_s)}",
            f"QPS (requests / total duration): {_fmt_qps(_calc_qps(total_duration_s, iterations))}",
        ]
    )
    if errors:
        lines.append("Errors: " + "; ".join(errors[:5]))
    return render_report(lines)
