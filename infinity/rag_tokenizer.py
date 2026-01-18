"""Minimal implementation of the functions/classes expected by RAGFlow tests.
This is a lightweight stub for local unit test runs only.
"""
from typing import List

class RagTokenizer:
    def tokenize(self, s: str) -> str:
        return s

    def fine_grained_tokenize(self, s: str) -> str:
        return s

    @property
    def tag(self) -> List[str]:
        return ["N", "V"]

    @property
    def freq(self) -> dict:
        return {}

    @property
    def _tradi2simp(self) -> callable:
        return lambda x: x

    @property
    def _strQ2B(self) -> callable:
        return lambda x: x


def is_chinese(s: str) -> bool:
    return any('\u4e00' <= c <= '\u9fff' for c in s)


def is_number(s: str) -> bool:
    try:
        float(s)
        return True
    except Exception:
        return False


def is_alphabet(s: str) -> bool:
    return s.isalpha()


def naive_qie(txt: str) -> List[str]:
    # Very naive splitter for tests
    return txt.split()
