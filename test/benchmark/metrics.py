import math
from dataclasses import dataclass
from typing import Any, List, Optional


@dataclass
class ChatSample:
    t0: float
    t1: Optional[float]
    t2: Optional[float]
    error: Optional[str] = None
    response_text: Optional[str] = None

    @property
    def first_token_latency(self) -> Optional[float]:
        if self.t1 is None:
            return None
        return self.t1 - self.t0

    @property
    def total_latency(self) -> Optional[float]:
        if self.t2 is None:
            return None
        return self.t2 - self.t0


@dataclass
class RetrievalSample:
    t0: float
    t1: Optional[float]
    error: Optional[str] = None
    response: Optional[Any] = None

    @property
    def latency(self) -> Optional[float]:
        if self.t1 is None:
            return None
        return self.t1 - self.t0


def _percentile(sorted_values: List[float], p: float) -> Optional[float]:
    if not sorted_values:
        return None
    n = len(sorted_values)
    k = max(0, math.ceil((p / 100.0) * n) - 1)
    return sorted_values[k]


def summarize(values: List[float]) -> dict:
    if not values:
        return {
            "count": 0,
            "avg": None,
            "min": None,
            "p50": None,
            "p90": None,
            "p95": None,
        }
    sorted_vals = sorted(values)
    return {
        "count": len(values),
        "avg": sum(values) / len(values),
        "min": sorted_vals[0],
        "p50": _percentile(sorted_vals, 50),
        "p90": _percentile(sorted_vals, 90),
        "p95": _percentile(sorted_vals, 95),
    }
