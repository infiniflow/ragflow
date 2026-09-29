import pytest

from test.benchmark.metrics import request_rates, summarize
from test.benchmark.report import chat_report, retrieval_report


def test_request_rates_separate_attempts_and_successes():
    assert request_rates(2, 3, 5) == {"qps": 1.0, "success_qps": 0.4, "failure_rate": 0.6}


def test_zero_success_is_zero_throughput_not_unavailable():
    assert request_rates(0, 4, 2) == {"qps": 2.0, "success_qps": 0.0, "failure_rate": 1.0}


@pytest.mark.parametrize("duration", [None, 0])
def test_missing_duration_does_not_invent_throughput(duration):
    assert request_rates(0, 0, duration) == {"qps": None, "success_qps": None, "failure_rate": None}


@pytest.mark.parametrize("command", ["chat", "retrieval"])
def test_text_report_shows_all_failed_run(command):
    kwargs = {"interface": command, "concurrency": 2, "total_duration_s": 2, "iterations": 4, "success": 0, "failure": 4, "errors": ["HTTP 503"], "created": {}}
    stats = summarize([])
    if command == "chat":
        report = chat_report(**kwargs, model="model", total_stats=stats, first_token_stats=stats)
    else:
        report = retrieval_report(**kwargs, stats=stats)
    assert "Successful QPS (success / total duration): 0.00" in report
    assert "Failure Rate: 100.00%" in report
    assert "avg=n/a" in report
