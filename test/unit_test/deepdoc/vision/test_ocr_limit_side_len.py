"""
Regression test for the DeepDOC ``TextDetector`` detection-downscale limit.

Issue #18509: on large sparse images (e.g. 8000x8000 with text in one corner),
DeepDOC's hard-coded ``limit_side_len: 960`` in
``deepdoc/vision/ocr.py::TextDetector.__init__`` downscales the input to
~0.12x before detection, so any text whose bounding box shrinks below 3x3 px
is silently dropped by ``filter_tag_det_res``. The result is that most of the
readable OCR text on the image never reaches the recognizer.

The fix raises the default to 2048 (4x reduction in downscale ratio, so an
8000x8000 image goes to 2048x2048 at detection time, leaving enough
resolution for text detection) and accepts a ``limit_side_len`` parameter so
callers can tune the value further without re-implementing the wrapper.

This test inspects the source AST to pin the contract: the parameter must
exist, the default must be at least 2048, the pre-process list must pass the
parameter through, and the four call sites inside ``OCR.__init__`` must not
break the new signature with a stray third positional argument.
"""

import ast
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]


def _read_ocr_source() -> str:
    return (REPO_ROOT / "deepdoc" / "vision" / "ocr.py").read_text()


def _find_class_method(source: str, class_name: str, method_name: str) -> ast.FunctionDef:
    tree = ast.parse(source)
    for node in tree.body:
        if isinstance(node, ast.ClassDef) and node.name == class_name:
            for child in node.body:
                if isinstance(child, ast.FunctionDef) and child.name == method_name:
                    return child
    raise AssertionError(f"{class_name}.{method_name} not found in deepdoc/vision/ocr.py")


def test_text_detector_accepts_limit_side_len_parameter():
    """TextDetector.__init__ must accept a ``limit_side_len`` parameter."""
    source = _read_ocr_source()
    func = _find_class_method(source, "TextDetector", "__init__")
    args = func.args

    kwonly = {a.arg for a in args.kwonlyargs}
    positional = {a.arg for a in args.args}
    assert "limit_side_len" in positional or "limit_side_len" in kwonly, (
        f"TextDetector.__init__ must accept a 'limit_side_len' parameter; positional args={sorted(positional)}, kwonly={sorted(kwonly)}"
    )


def test_text_detector_default_limit_side_len_is_2048():
    """The default must be at least 2048 so a 8000x8000 image still leaves the
    text bbox above the 3x3-px filter threshold after downscale."""
    source = _read_ocr_source()
    func = _find_class_method(source, "TextDetector", "__init__")
    args = func.args
    defaults = list(args.defaults)

    arg_names = [a.arg for a in args.args]
    if "limit_side_len" in arg_names:
        idx = arg_names.index("limit_side_len")
        offset = len(args.args) - len(defaults)
        default_idx = idx - offset
        assert 0 <= default_idx < len(defaults), f"limit_side_len at index {idx} has no default; args={args.args}, defaults={defaults}"
        default = defaults[default_idx]
    else:
        kw_defaults = {a.arg: d for a, d in zip(args.kwonlyargs, args.kw_defaults)}
        assert "limit_side_len" in kw_defaults
        default = kw_defaults["limit_side_len"]

    assert isinstance(default, ast.Constant) and isinstance(default.value, int), f"limit_side_len default must be a literal int, got {ast.dump(default)}"
    assert default.value >= 2048, f"limit_side_len default must be >= 2048 to leave text bboxes above the 3x3-px filter threshold on a 8000x8000 image; got {default.value}"


def test_text_detector_passes_limit_side_len_into_preprocess_list():
    """The pre-process ``DetResizeForTest`` operator must receive the parameter,
    not the hard-coded 960 default. We assert the field is referenced inside
    the function body so a regression that adds the parameter but leaves the
    pre-process list with the hard-coded 960 still trips the test."""
    source = _read_ocr_source()
    func = _find_class_method(source, "TextDetector", "__init__")

    name_refs = [n for n in ast.walk(func) if isinstance(n, ast.Name) and n.id == "limit_side_len"]
    assert name_refs, "TextDetector.__init__ must reference the 'limit_side_len' parameter in its body (the parameter exists but is not plumbed into the pre-process list)"


def test_ocr_call_sites_do_not_pass_third_positional_arg():
    """The four ``TextDetector(model_dir[, device_id])`` call sites inside
    ``OCR.__init__`` must keep their existing positional-argument shape.
    No third positional arg may sneak in — a future refactor that adds
    ``TextDetector(model_dir, device_id, 4096)`` would silently bind
    ``4096`` to ``device_id`` and break detection (or to ``limit_side_len``
    if the signature is ever changed back to positional)."""
    source = _read_ocr_source()
    tree = ast.parse(source)

    call_sites: list[ast.Call] = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        if isinstance(node.func, ast.Name) and node.func.id == "TextDetector":
            call_sites.append(node)

    assert len(call_sites) >= 4, f"expected at least 4 TextDetector() call sites (one per branch in OCR.__init__); found {len(call_sites)}"
    for call in call_sites:
        assert len(call.args) <= 2, f"TextDetector call at line {call.lineno} has {len(call.args)} positional args; only model_dir and device_id are allowed (limit_side_len is keyword-only)"


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-v"]))
