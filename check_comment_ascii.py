#!/usr/bin/env python3
import sys
import tokenize
import ast
import pathlib
import re

ASCII = re.compile(r"^[ -~]*\Z")  # Only printable ASCII


def check(src: str, name: str) -> int:
    """
    I'm a docstring
    """
    ok = 1
    # A common comment begins with `#`
    with tokenize.open(src) as fp:
        for tk in tokenize.generate_tokens(fp.readline):
            if tk.type == tokenize.COMMENT and not ASCII.fullmatch(tk.string):
                print(f"{name}:{tk.start[0]}: non-ASCII comment: {tk.string}")
                ok = 0
    # A docstring begins and ends with `'''`
    for node in ast.walk(ast.parse(pathlib.Path(src).read_text(), filename=name)):
        if isinstance(node, (ast.FunctionDef, ast.ClassDef, ast.Module)):
            if (doc := ast.get_docstring(node)) and not ASCII.fullmatch(doc):
                print(f"{name}:{node.lineno}: non-ASCII docstring: {doc}")
                ok = 0
    return ok


if __name__ == "__main__":
    status = 0
    for file in sys.argv[1:]:
        if not check(file, file):
            status = 1
    sys.exit(status)
