#!/usr/bin/env python3
from __future__ import annotations

import ast
import json
import re
import subprocess
import sys
import tokenize
from pathlib import Path

import yaml


MERGE_PATTERNS = ("<<<<<<< ", "=======\n", ">>>>>>> ")

# Printable ASCII (0x20-0x7E) plus newline — matches the regex used by the
# historical check_comment_ascii.py.
_PRINTABLE_ASCII = re.compile(r"^[\n -~]*\Z")


def _read_bytes(path: Path) -> bytes:
    return path.read_bytes()


def _staged_paths() -> list[Path]:
    proc = subprocess.run(
        ["git", "diff", "--cached", "--name-only", "--diff-filter=ACMR"],
        check=True,
        capture_output=True,
        text=True,
    )
    return [Path(line) for line in proc.stdout.splitlines() if line]


def _report(errors: list[str]) -> int:
    if not errors:
        return 0
    for error in errors:
        print(error, file=sys.stderr)
    return 1


def check_json(paths: list[Path], fix: bool = False) -> int:
    errors: list[str] = []
    for path in paths:
        if path.suffix != ".json" or not path.is_file():
            continue
        try:
            json.loads(path.read_text(encoding="utf-8"))
        except Exception as exc:
            errors.append(f"invalid json: {path}: {exc}")
    return _report(errors)


def check_yaml(paths: list[Path], fix: bool = False) -> int:
    errors: list[str] = []
    for path in paths:
        if path.suffix not in {".yaml", ".yml"} or not path.is_file():
            continue
        try:
            yaml.safe_load(path.read_text(encoding="utf-8"))
        except Exception as exc:
            errors.append(f"invalid yaml: {path}: {exc}")
    return _report(errors)


def check_eof(paths: list[Path], fix: bool = False) -> int:
    errors: list[str] = []
    for path in paths:
        if not path.is_file():
            continue
        data = _read_bytes(path)
        if data and not data.endswith(b"\n"):
            if fix:
                with path.open("ab") as f:
                    f.write(b"\n")
                print(f"fixed missing-trailing-newline: {path}", file=sys.stderr)
            else:
                errors.append(f"missing trailing newline: {path}")
    return 0 if fix else _report(errors)


_TRAILING_WS_RE = re.compile(r"[ \t]+(?=\r?\n|$)")


def check_trailing_whitespace(paths: list[Path], fix: bool = False) -> int:
    errors: list[str] = []
    for path in paths:
        if not path.is_file():
            continue
        try:
            text = path.read_text(encoding="utf-8", errors="ignore")
        except Exception:
            continue
        if not text:
            continue
        new_text = _TRAILING_WS_RE.sub("", text)
        if new_text == text:
            continue
        if fix:
            path.write_text(new_text, encoding="utf-8")
            print(f"fixed trailing-whitespace: {path}", file=sys.stderr)
        else:
            old_lines = text.splitlines()
            new_lines = new_text.splitlines()
            for i, (orig, new) in enumerate(zip(old_lines, new_lines), 1):
                if orig != new:
                    errors.append(f"trailing whitespace: {path}:{i}")
    return 0 if fix else _report(errors)


def check_mixed_line_endings(paths: list[Path], fix: bool = False) -> int:
    errors: list[str] = []
    for path in paths:
        if not path.is_file():
            continue
        data = _read_bytes(path)
        has_crlf = b"\r\n" in data
        has_lf = b"\n" in data.replace(b"\r\n", b"")
        if has_crlf and has_lf:
            if fix:
                path.write_bytes(data.replace(b"\r\n", b"\n"))
                print(f"fixed mixed-line-ending: {path}", file=sys.stderr)
            else:
                errors.append(f"mixed line endings: {path}")
    return 0 if fix else _report(errors)


def check_merge_conflicts(paths: list[Path], fix: bool = False) -> int:
    errors: list[str] = []
    for path in paths:
        if not path.is_file():
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        if all(pattern in text for pattern in MERGE_PATTERNS):
            errors.append(f"merge conflict markers: {path}")
    return _report(errors)


def check_symlinks(paths: list[Path], fix: bool = False) -> int:
    errors: list[str] = []
    for path in paths:
        if path.is_symlink() and not path.exists():
            errors.append(f"broken symlink: {path}")
    return _report(errors)


def check_case_conflicts(_: list[Path], fix: bool = False) -> int:
    proc = subprocess.run(
        ["git", "ls-files"],
        check=True,
        capture_output=True,
        text=True,
    )
    seen: dict[str, str] = {}
    errors: list[str] = []
    for path in proc.stdout.splitlines():
        lowered = path.lower()
        other = seen.get(lowered)
        if other and other != path:
            errors.append(f"case conflict: {other} <-> {path}")
        seen[lowered] = path
    return _report(errors)


def check_comment_ascii(paths: list[Path], fix: bool = False) -> int:
    """Ensure Python comments and docstrings contain only ASCII characters.

    Ported from the legacy check_comment_ascii.py. The fix flag is accepted
    for signature consistency but no auto-fix exists — non-ASCII comments
    must be rewritten by hand.
    """
    errors: list[str] = []
    for path in paths:
        if path.suffix != ".py" or not path.is_file():
            continue
        # A common comment begins with `#`
        try:
            with tokenize.open(path) as fp:
                for tk in tokenize.generate_tokens(fp.readline):
                    if tk.type == tokenize.COMMENT and not _PRINTABLE_ASCII.fullmatch(tk.string):
                        errors.append(f"non-ASCII comment: {path}:{tk.start[0]}: {tk.string}")
        except (OSError, SyntaxError, UnicodeDecodeError, tokenize.TokenError):
            # Skip files that can't be tokenised (binary, bad encoding decl,
            # syntax errors). Other tools (e.g. ruff) handle those separately.
            pass

        # A docstring begins and ends with `'''` (or `"""`)
        try:
            source = path.read_text()
        except (OSError, UnicodeDecodeError):
            continue
        try:
            tree = ast.parse(source, filename=str(path))
        except SyntaxError:
            continue
        for node in ast.walk(tree):
            if not isinstance(node, (ast.FunctionDef, ast.ClassDef, ast.Module)):
                continue
            doc = ast.get_docstring(node)
            if not doc or _PRINTABLE_ASCII.fullmatch(doc):
                continue
            first_line = doc.splitlines()[0] if doc.splitlines() else doc
            errors.append(f"non-ASCII docstring: {path}:{node.lineno}: {first_line}")
    return _report(errors)


CHECKS = {
    "json": check_json,
    "yaml": check_yaml,
    "eof": check_eof,
    "trailing-whitespace": check_trailing_whitespace,
    "mixed-line-ending": check_mixed_line_endings,
    "merge-conflict": check_merge_conflicts,
    "symlinks": check_symlinks,
    "case-conflict": check_case_conflicts,
    "comment-ascii": check_comment_ascii,
}


def main() -> int:
    args = sys.argv[1:]
    valid = set(CHECKS)
    if not args or args[0] not in valid or len(args) > 2 or (len(args) == 2 and args[1] != "--fix"):
        print(f"usage: {sys.argv[0]} <{'|'.join(valid)}> [--fix]", file=sys.stderr)
        return 2
    fix = len(args) == 2
    paths = _staged_paths()
    return CHECKS[args[0]](paths, fix=fix)


if __name__ == "__main__":
    raise SystemExit(main())
