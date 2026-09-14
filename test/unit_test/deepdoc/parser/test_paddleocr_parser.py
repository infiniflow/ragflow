import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import Mock

import pytest


def _load_paddleocr_parser(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    deepdoc_mod = ModuleType("deepdoc")
    deepdoc_mod.__path__ = [str(repo_root / "deepdoc")]
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_mod)

    parser_mod = ModuleType("deepdoc.parser")
    parser_mod.__path__ = [str(repo_root / "deepdoc" / "parser")]
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_mod)

    pdf_parser_mod = ModuleType("deepdoc.parser.pdf_parser")

    class _RAGFlowPdfParser:
        pass

    pdf_parser_mod.RAGFlowPdfParser = _RAGFlowPdfParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_mod)

    utils_mod = ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = lambda *_args, **_kwargs: []
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)

    module_name = "test_paddleocr_parser_unit_module"
    module_path = repo_root / "deepdoc" / "parser" / "paddleocr_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _clear_env(monkeypatch):
    for key in ("PADDLEOCR_BASE_URL", "PADDLEOCR_ACCESS_TOKEN", "PADDLEOCR_ALGORITHM"):
        monkeypatch.delenv(key, raising=False)


def _local_response(markdown="hello", pruned=None):
    resp = Mock()
    resp.status_code = 200
    resp.json.return_value = {
        "logId": "test-log-id",
        "errorCode": 0,
        "errorMsg": "Success",
        "result": {
            "layoutParsingResults": [
                {
                    "prunedResult": pruned or {"parsing_res_list": []},
                    "markdown": {"text": markdown},
                }
            ]
        },
    }
    return resp


@pytest.mark.p1
@pytest.mark.parametrize(
    # Not named "base_url": pytest-base-url ships a session-scoped fixture of
    # that name, and shadowing it with a function-scoped parametrize argument
    # makes its session-scoped consumer fail with ScopeMismatch.
    ("address", "expect_local"),
    [
        # Only the hosted address serves the asynchronous job API.
        ("https://paddleocr.aistudio-app.com", False),
        ("https://paddleocr.aistudio-app.com/", False),
        ("https://paddleocr.aistudio-app.com/api", False),
        ("http://PaddleOCR.AIStudio-App.com", False),
        (None, False),
        ("http://localhost:8080", True),
        ("http://127.0.0.1:8080/layout-parsing", True),
        ("https://paddleocr.internal.example.com", True),
        # An AI Studio deployment on a sibling subdomain serves the synchronous
        # pipeline, so matching the parent domain rather than the exact host
        # would route a working deployment to the wrong protocol.
        ("https://o5r7debeac17pbt3.aistudio-app.com/layout-parsing", True),
        # A lookalike host must not be mistaken for the hosted service.
        ("https://paddleocr.aistudio-app.com.evil.example", True),
        ("localhost:8080", True),
    ],
)
def test_protocol_is_selected_by_address(monkeypatch, address, expect_local):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    parser = module.PaddleOCRParser(base_url=address)
    assert parser.local is expect_local
    assert module.PaddleOCRConfig(base_url=parser.base_url).local is expect_local


@pytest.mark.p1
def test_self_hosted_needs_base_url_not_token(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    # A self-hosted deployment usually runs unauthenticated, so the token must
    # not be required; its address must be.
    ok, reason = module.PaddleOCRParser(base_url="http://127.0.0.1:8080").check_installation()
    assert ok is True
    assert reason == ""

    monkeypatch.setenv("PADDLEOCR_BASE_URL", "")
    ok, reason = module.PaddleOCRParser().check_installation()
    assert ok is False
    assert "Base URL" in reason


@pytest.mark.p1
def test_hosted_still_requires_token(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    ok, reason = module.PaddleOCRParser().check_installation()
    assert ok is False
    assert "Access token" in reason

    ok, _ = module.PaddleOCRParser(access_token="tok").check_installation()
    assert ok is True


@pytest.mark.p1
def test_self_hosted_posts_to_layout_parsing_once(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    post = Mock(return_value=_local_response())
    monkeypatch.setattr(module.requests, "post", post)
    # The synchronous endpoint must not be polled like the hosted job API.
    monkeypatch.setattr(module.requests, "get", Mock(side_effect=AssertionError("must not poll")))

    parser = module.PaddleOCRParser(base_url="http://127.0.0.1:8080/")
    result = parser._send_request(b"\x89PNG binary", module.PaddleOCRConfig(base_url="http://127.0.0.1:8080"), None)

    assert post.call_count == 1
    assert post.call_args.args[0] == "http://127.0.0.1:8080/layout-parsing"
    assert result["layoutParsingResults"][0]["markdown"]["text"] == "hello"
    assert result["ocrResults"] == []


@pytest.mark.p1
@pytest.mark.parametrize(
    ("configured", "expected"),
    [
        ("http://127.0.0.1:8080", "http://127.0.0.1:8080/layout-parsing"),
        ("http://127.0.0.1:8080/", "http://127.0.0.1:8080/layout-parsing"),
        # PaddleX documents the endpoint rather than the root, so the whole URL
        # is the likelier thing to be pasted into the address field.
        ("http://127.0.0.1:8080/layout-parsing", "http://127.0.0.1:8080/layout-parsing"),
        ("http://127.0.0.1:8080/layout-parsing/", "http://127.0.0.1:8080/layout-parsing"),
        ("http://127.0.0.1:8080/paddle/layout-parsing", "http://127.0.0.1:8080/paddle/layout-parsing"),
    ],
)
def test_self_hosted_endpoint_is_not_duplicated(monkeypatch, configured, expected):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    post = Mock(return_value=_local_response())
    monkeypatch.setattr(module.requests, "post", post)

    parser = module.PaddleOCRParser(base_url=configured)
    parser._send_request(b"%PDF-1.7 rest", module.PaddleOCRConfig(base_url=configured), None)

    assert post.call_args.args[0] == expected


@pytest.mark.p1
@pytest.mark.parametrize(
    ("data", "expected_type"),
    [(b"%PDF-1.7 rest", 0), (b"\x89PNG\r\n\x1a\n", 1), (b"\xff\xd8\xff\xe0 jpeg", 1), (b"", 1)],
)
def test_file_type_detection(monkeypatch, data, expected_type):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    post = Mock(return_value=_local_response())
    monkeypatch.setattr(module.requests, "post", post)

    parser = module.PaddleOCRParser(base_url="http://svc")
    parser._send_request(data, module.PaddleOCRConfig(base_url="http://svc"), None)

    assert post.call_args.kwargs["json"]["fileType"] == expected_type


@pytest.mark.p1
def test_self_hosted_sends_base64_and_skips_markdown_images(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    post = Mock(return_value=_local_response())
    monkeypatch.setattr(module.requests, "post", post)

    parser = module.PaddleOCRParser(base_url="https://svc", access_token="tok")
    parser._send_request(b"%PDF-1.7 body", module.PaddleOCRConfig(base_url="https://svc", access_token="tok"), None)

    payload = post.call_args.kwargs["json"]
    assert payload["file"] == "JVBERi0xLjcgYm9keQ=="
    # Section text has images stripped anyway, so inlining them would only
    # inflate the response.
    assert payload["returnMarkdownImages"] is False
    assert post.call_args.kwargs["headers"]["Authorization"] == "Bearer tok"


@pytest.mark.p1
def test_self_hosted_omits_authorization_over_plain_http(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    post = Mock(return_value=_local_response())
    monkeypatch.setattr(module.requests, "post", post)

    parser = module.PaddleOCRParser(base_url="http://svc", access_token="tok")
    parser._send_request(b"%PDF-1.7", module.PaddleOCRConfig(base_url="http://svc", access_token="tok"), None)

    # A bearer token on a plaintext connection is readable by anyone on the path.
    assert "Authorization" not in post.call_args.kwargs["headers"]


@pytest.mark.p1
def test_self_hosted_omits_authorization_without_token(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    post = Mock(return_value=_local_response())
    monkeypatch.setattr(module.requests, "post", post)

    parser = module.PaddleOCRParser(base_url="http://svc")
    parser._send_request(b"%PDF-1.7", module.PaddleOCRConfig(base_url="http://svc"), None)

    assert "Authorization" not in post.call_args.kwargs["headers"]


@pytest.mark.p1
@pytest.mark.parametrize(
    ("response_attrs", "expected"),
    [
        ({"status_code": 404, "text": "Not Found"}, "HTTP 404"),
        ({"status_code": 200, "json.return_value": {"errorCode": 1001, "errorMsg": "bad request"}}, "bad request"),
        ({"status_code": 200, "json.side_effect": ValueError("no json")}, "not JSON"),
    ],
)
def test_self_hosted_failures_reach_the_callback(monkeypatch, response_attrs, expected):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    resp = Mock()
    resp.configure_mock(**response_attrs)
    monkeypatch.setattr(module.requests, "post", Mock(return_value=resp))

    reported: list[tuple[float, str]] = []
    parser = module.PaddleOCRParser(base_url="http://svc")
    with pytest.raises(RuntimeError, match=expected):
        parser._send_request(b"%PDF", module.PaddleOCRConfig(base_url="http://svc"), lambda p, m: reported.append((p, m)))

    # by_paddleocr() only logs the raised error, so the callback is the only way
    # the cause becomes visible instead of the document silently ending empty.
    assert [msg for prog, msg in reported if prog == -1], reported
    assert expected in [msg for prog, msg in reported if prog == -1][0]


@pytest.mark.p1
def test_self_hosted_connection_error_reaches_the_callback(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    monkeypatch.setattr(module.requests, "post", Mock(side_effect=OSError("connection refused")))

    reported: list[tuple[float, str]] = []
    parser = module.PaddleOCRParser(base_url="http://svc")
    with pytest.raises(RuntimeError, match="connection refused"):
        parser._send_request(b"%PDF", module.PaddleOCRConfig(base_url="http://svc"), lambda p, m: reported.append((p, m)))

    assert any(prog == -1 and "connection refused" in msg for prog, msg in reported), reported


@pytest.mark.p1
def test_parse_image_uses_self_hosted_protocol(monkeypatch, tmp_path):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    pruned = {"parsing_res_list": [{"block_content": "invoice total 42", "block_label": "text", "block_bbox": [1, 2, 3, 4]}]}
    monkeypatch.setattr(module.requests, "post", Mock(return_value=_local_response(pruned=pruned)))

    image = tmp_path / "sample.png"
    image.write_bytes(b"\x89PNG\r\n\x1a\n fake")

    parser = module.PaddleOCRParser(base_url="http://svc")
    assert parser.parse_image(str(image)) == "invoice total 42"


@pytest.mark.p1
def test_parse_pdf_sections_carry_position_tags(monkeypatch, tmp_path):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    pruned = {"parsing_res_list": [{"block_content": "chapter one", "block_label": "text", "block_bbox": [10, 20, 30, 40]}]}
    monkeypatch.setattr(module.requests, "post", Mock(return_value=_local_response(pruned=pruned)))

    pdf = tmp_path / "sample.pdf"
    pdf.write_bytes(b"%PDF-1.7 fake")

    parser = module.PaddleOCRParser(base_url="http://svc")
    sections, tables = parser.parse_pdf(str(pdf))

    # bbox survives the self-hosted response, which is what crop() relies on.
    assert sections == [("chapter one", "@@1\t5.0\t15.0\t10.0\t20.0##")]
    assert tables == []


@pytest.mark.p1
def test_pure_image_blocks_do_not_become_empty_sections(monkeypatch, tmp_path):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    # A scanned page the layout model classified as one image block: its content
    # is nothing but markup, so it must drop out instead of yielding an empty
    # section that still carries a position tag.
    pruned = {
        "parsing_res_list": [
            {
                "block_content": '<div style="text-align: center;"><img src="imgs/img.jpg" alt="Image" width="99%" /></div>\n',
                "block_label": "image",
                "block_bbox": [3, 235, 1190, 1431],
            },
            {"block_content": "real text", "block_label": "text", "block_bbox": [10, 20, 30, 40]},
        ]
    }
    monkeypatch.setattr(module.requests, "post", Mock(return_value=_local_response(pruned=pruned)))

    pdf = tmp_path / "scan.pdf"
    pdf.write_bytes(b"%PDF-1.7 fake")

    sections, _ = module.PaddleOCRParser(base_url="http://svc").parse_pdf(str(pdf))
    assert sections == [("real text", "@@1\t5.0\t15.0\t10.0\t20.0##")]


@pytest.mark.p1
def test_hosted_payload_includes_vl_params_for_every_supported_algorithm(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    parser = module.PaddleOCRParser(access_token="tok")
    for algorithm in module.SUPPORTED_PADDLEOCR_ALGORITHMS:
        config = module.PaddleOCRConfig.from_dict({"algorithm": algorithm, "algorithm_config": {"max_new_tokens": 128}})
        payload = parser._build_payload(config)
        assert payload["maxNewTokens"] == 128, algorithm
        assert payload["prettifyMarkdown"] is True, algorithm


@pytest.mark.p1
def test_hosted_protocol_still_submits_and_polls(monkeypatch):
    _clear_env(monkeypatch)
    module = _load_paddleocr_parser(monkeypatch)

    submit = Mock()
    submit.status_code = 200
    submit.json.return_value = {"data": {"jobId": "job-1"}}
    monkeypatch.setattr(module.requests, "post", Mock(return_value=submit))

    poll = Mock()
    poll.status_code = 200
    poll.json.return_value = {"data": {"state": "done", "resultJsonUrl": "http://results/1.jsonl"}}
    fetch = Mock()
    fetch.status_code = 200
    fetch.text = json.dumps({"result": {"layoutParsingResults": [{"markdown": {"text": "hosted"}}]}})
    fetch.raise_for_status = Mock()
    monkeypatch.setattr(module.requests, "get", Mock(side_effect=[poll, fetch]))

    parser = module.PaddleOCRParser(access_token="tok")
    result = parser._send_request(b"%PDF", module.PaddleOCRConfig(access_token="tok"), None)

    assert result["layoutParsingResults"][0]["markdown"]["text"] == "hosted"
