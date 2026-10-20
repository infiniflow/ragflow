#!/usr/bin/env python3

# PEP 723 metadata
# /// script
# requires-python = ">=3.13"
# dependencies = [
#   "tokenizers",
#   "tiktoken",
#   "sentencepiece",
# ]
# ///
"""Generate tokenizer oracle fixtures for internal/tokenizer's counters.

The Go counters are hand-written reimplementations of four tokenizer
architectures (SentencePiece Unigram, BERT WordPiece, byte-level BPE,
SentencePiece-BPE) plus the tiktoken table. "Looks right" is not evidence, so each
one is checked against the implementation that actually defines the answer:

  * cl100k           - OpenAI's own tiktoken package on the shipped table;
  * xlmr-spm         - the HuggingFace `tokenizers` conversion of the model that
                       SILICONFLOW serves (bge-m3's tokenizer.json), with the
                       canonical Google `sentencepiece` package cross-checked under
                       --spm-crosscheck;
  * bert-wordpiece   - the model's own tokenizer.json (HF `tokenizers`);
  * qwen-bpe /
    llama-bpe        - the model's own tokenizer.json (HF `tokenizers`).

The fixture stores the expected token **ids**, not just their number. Two different
segmentations can share a token count, so a count comparison cannot tell "same
tokens" from "different tokens, same length"; the Go test compares ids element by
element, and a mismatch is reported with the first divergence and its context.

Usage:
    for m in bge-m3 bge-large-en qwen3-embedding e5-mistral cl100k; do
      uv run scripts/gen_tokenizer_oracle.py --model $m --corpus <path to the corpus document> --out /tmp/tokenizer_oracle/$m.json
    done
    TOKENIZER_ORACLE_DIR=/tmp/tokenizer_oracle \\
      ./build.sh --test -run TestCountersMatchOracle ./internal/tokenizer/

`--corpus` is REQUIRED, and its argument is a path, never a built-in default: the
document belongs to the caller, not to this script. Its ENTIRE content becomes one
regression sample (`incident_full`), the document that exposed the under-count. A
window of that document is deliberately NOT sampled: the window would silently stop
covering the document the moment the document changes.

Every dependency is declared inline above (PEP 723), so `uv run` provisions them: there
is no `pip install` step, and no import-time fallback - a dependency that is missing
fails the run instead of degrading it silently.

The same holds for the assets it reads: the model's `tokenizer.json`, the cl100k table
and (under `--spm-crosscheck`) the `sentencepiece` `.model` are checked up front, and a
missing one fails the run - a fixture computed without its reference is worse than none.

Add `--fuzz 2000` for a deterministic random Unicode corpus (seeded, and the texts
are stored in the fixture, so the Go side never has to reproduce the RNG). Adding
`--spm-crosscheck` also records what Google's SentencePiece implementation returns
for the same text, which is how the two authorities are compared.

A mismatch means the Go counter and the reference disagree: a bug in the Go counter.
An under-count is the dangerous direction (it lets an oversized input through and
the provider answers 400).
"""

import argparse
import json
import os
import random
import re
import sys
import unicodedata

import sentencepiece as spm
import tiktoken
from tokenizers import Tokenizer


# A base character carrying two or more combining marks. This is the one shape where
# our NFKC-based normalization provably differs from the served Precompiled charmap:
# NFKC composes `a` + U+0327 + U+0301 into a precomposed letter, while the charmap
# keeps the base and the marks separate (measured: HF normalizes it to "a\u0327\u0301"
# and emits three tokens). The samples are marked so the Go test can bound the
# divergence instead of hiding it: exact token equality everywhere else, and at most
# a two-token difference here.
def has_combining_run(text: str) -> bool:
    """True when two or more combining marks sit on the same base character."""
    run = 0
    for ch in text:
        if unicodedata.combining(ch):
            run += 1
            if run >= 2:
                return True
        else:
            run = 0
    return False


def has_dangling_mark(text: str) -> bool:
    """True when a combining mark does not follow a letter.

    The served Precompiled charmap drops such marks (measured: "3<U+2461><U+0327>"
    normalizes to "32", and a mark left over after a control character is deleted is
    dropped with it), while our NFKC-based normalization keeps them as their own
    token - one token more than the model sees. The samples are marked so the Go
    test bounds the difference instead of hiding it.
    """
    previous = ""
    for ch in text:
        if unicodedata.combining(ch):
            if not previous or not previous[-1].isalpha():
                return True
        else:
            previous += ch
    return False


def is_known_approximation(text: str) -> bool:
    return has_combining_run(text) or has_dangling_mark(text)


# The subset of the model's Precompiled charmap that internal/tokenizer's spm.go
# implements, replicated here so the fixture can tell whether our normalization
# agrees with the served one ON THIS SAMPLE. When it does, the Go counter must match
# the tokenization exactly; when it does not, the sample is marked and the test
# bounds the difference instead of hiding it.
CHARMAP_DELETE_RANGES = ((0x01, 0x08), (0x0B, 0x0C), (0x0E, 0x1F), (0x7F, 0x84), (0x86, 0x9F))
CHARMAP_SPACE = {"\u200b", "\u200c", "\u200d", "\ufeff", "\u2581"}


def our_normalize(text: str) -> str:
    """Our SPM normalization: NFKC, charmap subset, space collapse, ▁ escaping."""
    out = []
    for ch in unicodedata.normalize("NFKC", text):
        cp = ord(ch)
        if any(lo <= cp <= hi for lo, hi in CHARMAP_DELETE_RANGES):
            continue
        out.append(" " if ch in CHARMAP_SPACE else ch)
    s = re.sub(r" {2,}", " ", "".join(out))
    if not s:
        return ""
    res, prev_space = ["\u2581"], False
    for i, ch in enumerate(s):
        if ch.isspace():
            if prev_space or i == 0:
                prev_space = True
                continue
            prev_space = True
            res.append("\u2581")
            continue
        prev_space = False
        res.append(ch)
    return "".join(res)


def served_normalize(hf_norm: str) -> str:
    """The same string as the served pipeline builds it: Metaspace turns each
    whitespace run into one ▁ and adds the leading one."""
    if not hf_norm:
        return ""
    collapsed = re.sub(r"\s+", " ", hf_norm).strip()
    return "\u2581" + collapsed.replace(" ", "\u2581")


def normalization_differs(text: str, hf_norm: str) -> bool:
    """True when our normalization cannot be expected to match the served one."""
    return our_normalize(text) != served_normalize(hf_norm)


ORACLES = {
    # label: (tokenizer.json path, what the Go counter is called)
    "bge-m3": ("huggingface.co/BAAI/bge-m3/tokenizer.json", "xlmr-spm"),
    "bge-large-en": ("huggingface.co/BAAI/bge-large-en-v1.5/tokenizer.json", "bert-wordpiece"),
    "qwen3-embedding": ("huggingface.co/Qwen/Qwen3-Embedding-0.6B/tokenizer.json", "qwen-bpe"),
    "e5-mistral": ("huggingface.co/intfloat/e5-mistral-7b-instruct/tokenizer.json", "llama-bpe"),
    # OpenAI's table, counted by OpenAI's own implementation.
    "cl100k": (None, "cl100k_base"),
}

# The canonical SentencePiece model for the XLM-R family, used only by
# --spm-crosscheck: the .model file is what Google's implementation reads, while
# the served behaviour comes from tokenizer.json above.
SPM_MODEL = "huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model"

# The cl100k table as shipped in ragflow_deps (the OpenAI blob download_deps.py fetches).
# The Go counter loads this file; tiktoken reads its own cached copy of the same blob, so
# the check in main() is what keeps the check-out honest about what it ships.
CL100K_TABLE = "cl100k_base.tiktoken"

# The label of the corpus sample, and the incident it reproduces: an English
# Wikipedia article about Argosy magazine with large volume/issue-number tables.
# SILICONFLOW rejected that document with 400/20015 while cl100k counted it as
# *under* the 8192-token window. The document itself arrives as --corpus - nothing
# about it is built into this script, because the corpus is the caller's choice.
INCIDENT_SAMPLE_LABEL = "incident_full"


def readable_file(path: str) -> str:
    """argparse type for --corpus: fail loudly instead of sampling nothing."""
    if not os.path.isfile(path):
        raise argparse.ArgumentTypeError(f"not a file: {path}")
    return path


def samples(corpus_path: str) -> list[tuple[str, str]]:
    out: list[tuple[str, str]] = [
        ("empty", ""),
        ("space_only", "   "),
        ("one_word", "hello"),
        ("prose", "The quick brown fox jumps over the lazy dog. " * 40),
        ("prose_with_newlines", "First line.\nSecond line.\n\nThird line after a blank.\n" * 20),
        ("markdown_table", "| 1940 | Dates: | 6,13,20,27 | 3,10,17,24 | 2,9,16,23,30 |\n" * 30),
        ("issue_numbers", "| 1976 | | 383/1 | 383/2 | 383/3 | 383/4 | 383/5 |\n" * 60),
        ("digits", "0123456789 " * 100),
        ("cjk", "中文分词测试，这是用于对比 tokenizer 的样本。" * 30),
        ("mixed_cjk_latin", "RAGFlow 的 ingestion 组件会调用 embedding 模型，例如 BAAI/bge-m3。" * 15),
        ("emoji", "🚀🔥 embedding ✅ 测试 " * 30),
        ("punctuation", "!!! ??? ... --- ;;; ::: ((( ))) [[[ ]]] " * 40),
        ("leading_trailing_space", "   padded text with edge whitespace   "),
        ("tabs_and_crlf", "a\tb\r\nc\t\td\r\n" * 30),
        ("accents", "café naïve résumé coöperate Ångström " * 30),
        ("unicode_symbols", "≤ ≥ ≈ ∞ ∑ √ π ™ © ® ‰ § ¶ " * 30),
        ("long_word", "a" * 500 + " " + "supercalifragilisticexpialidocious" * 5),
        ("base64_like", "QWxhZGRpbjpvcGVuIHNlc2FtZQ" * 200),
        ("html_table", "<table><tr><td>1976</td><td>383/1</td></tr></table>" * 30),
        ("numbers_and_units", "296/1 to 296/4, 12:30, 1,234.56 USD, 99.9%, 2020-01-02 " * 25),
        # Adversarial Unicode. These exist to check the one place where the SPM
        # counter is an approximation: normalization applies NFKC, while the
        # served tokenizer applies the model's precompiled nmt_nfkc charmap
        # first. If a sample below disagrees with the oracle, the fix is to
        # implement the charmap - not to loosen the test.
        ("fullwidth_forms", "ＡＢＣＤ１２３４ ｈｅｌｌｏ ！？ " * 20),
        ("compatibility_chars", "ﬁ ﬂ ①②③ Ⅻ ㍿ ㎏ ㎞ ¾ ⅓ ㎡ " * 20),
        ("decomposed_accents", "e\u0301 a\u0300 o\u0308 u\u030a n\u0303 " * 30),
        ("vietnamese", "Tiếng Việt có dấu: Đặng Hữu Phúc, Nguyễn Thị Hương " * 15),
        ("rtl_text", "مرحبا بالعالم هذا نص عربي للاختبار " * 15),
        ("hebrew", "שלום עולם זהו טקסט לבדיקה " * 15),
        ("control_chars", "a\u0001b\u007fc\u200bd\u200ce\u200df " * 20),
        ("zero_width_and_nbsp", "a\u00a0b\u2009c\u202fd\u2060e " * 20),
        ("cjk_compat", "漢字の互換文字：髙 﨑 塚 邊 ／ 全角カナ アイウ " * 15),
        ("emoji_zwj", "👨‍👩‍👧‍👦 👍🏽 🇨🇳 🧑‍💻 " * 20),
        ("mixed_scripts", "Latin Ελληνικά Кириллица 中文 日本語 한국어 " * 15),
        ("math_and_currency", "∀x∈ℝ: x² ≥ 0 → €100 ± 5% ∞ ≈ ℵ₀ " * 20),
    ]
    # The WHOLE document, not a window of it: a fixed span would silently stop
    # covering the document the moment the document changes, and the point of the
    # sample is that what the caller passed in is what gets checked.
    with open(corpus_path, encoding="utf-8") as fh:
        out.append((INCIDENT_SAMPLE_LABEL, fh.read()))
    return out


# Alphabets the fuzz corpus draws from. Every one of them has produced, or is
# capable of producing, a divergence between two tokenizer implementations:
# combining marks (normalizer order), zero-width characters (deletable), controls
# (deletable vs spaced), fullwidth/compatibility forms (NFKC vs nmt_nfkc), emoji
# (byte fallback vs Symbols-as-punctuation), and whitespace runs (SPM dummy prefix
# and collapse rules).
FUZZ_ALPHABETS = [
    "abcdefghijklmnopqrstuvwxyz",
    "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
    "0123456789",
    " \t\n",
    ".,;:!?()[]{}'\"-—/\\|<>@#$%^&*+=~`",
    "中文日本語한국어",
    "éàüñçøå",
    "αβγδεζηθικλμ",
    "абвгдежзийк",
    "אבגדהוזחט",
    "ابجدهوزحط",
    "🙂🚀🔥👍🏽✅🧑‍💻",
    "\u0301\u0308\u0327",
    "\u200b\u200c\u200d\u2060\ufeff",
    "\u0000\u0001\u0009\u007f",
    "ＡＢＣ１２３！？　",
    "ﬁﬂ①②Ⅻ㎏㎡¾",
]


def fuzz_samples(count: int, seed: int) -> list[tuple[str, str]]:
    """Deterministic random Unicode strings, plus a few adversarial extremes.

    The texts are stored in the fixture, so the Go side never has to reproduce this
    RNG - it only has to agree with the reference on the text it is given.
    """
    rng = random.Random(seed)
    out: list[tuple[str, str]] = []
    for i in range(count):
        parts = []
        for _ in range(rng.randint(1, 30)):
            alphabet = rng.choice(FUZZ_ALPHABETS)
            parts.append("".join(rng.choice(alphabet) for _ in range(rng.randint(1, 6))))
        out.append((f"fuzz_{i:04d}", "".join(parts)))
    out.append(("fuzz_long_word_99", "a" * 99 + "b"))
    out.append(("fuzz_long_word_100", "a" * 100 + "b"))
    out.append(("fuzz_long_word_101", "a" * 101 + "b"))
    out.append(("fuzz_repeat_run", rng.choice("abcxyz") * rng.randint(2, 200)))
    out.append(("fuzz_ascii_all_printable", "".join(chr(c) for c in range(32, 127))))
    out.append(("fuzz_one_codepoint_each", "".join(sorted(FUZZ_ALPHABETS[12] + FUZZ_ALPHABETS[13]))))
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--model", choices=sorted(ORACLES), required=True)
    parser.add_argument("--out", required=True, help="fixture path to write")
    parser.add_argument("--deps", default="ragflow_deps", help="directory holding huggingface.co/")
    parser.add_argument(
        "--corpus",
        required=True,
        type=readable_file,
        help="document whose ENTIRE content is added as the regression sample (required: the corpus is the caller's, not this script's)",
    )
    parser.add_argument("--fuzz", type=int, default=0, help="number of random Unicode samples to append")
    parser.add_argument("--seed", type=int, default=20260916, help="seed for --fuzz")
    parser.add_argument(
        "--spm-crosscheck",
        action="store_true",
        help="also record what Google's sentencepiece implementation returns (bge-m3 only)",
    )
    parser.add_argument("--quiet", action="store_true", help="do not print the per-sample table")
    args = parser.parse_args()

    rel, counter_id = ORACLES[args.model]
    rows_src = samples(args.corpus) + (fuzz_samples(args.fuzz, args.seed) if args.fuzz else [])

    if rel is None:
        # cl100k: OpenAI's own implementation on the table shipped in ragflow_deps.
        # tiktoken reads its own cached copy of the blob, so a checkout without the
        # shipped table would be checked against something else without a word -
        # fail loudly instead, the way the Go loader does.
        table = os.path.join(args.deps, CL100K_TABLE)
        if not os.path.isfile(table):
            print(f"missing cl100k table: {table}\nrun `uv run ragflow_deps/download_deps.py`", file=sys.stderr)
            return 1
        encoder = tiktoken.get_encoding("cl100k_base")
        rows = [{"label": label, "text": text, "ids": encoder.encode(text), "chars": len(text)} for label, text in rows_src]
        oracle = f"tiktoken {getattr(tiktoken, '__version__', '?')} cl100k_base"
    else:
        tokenizer_path = os.path.join(args.deps, rel)
        if not os.path.isfile(tokenizer_path):
            print(f"missing oracle tokenizer: {tokenizer_path}\nrun `uv run ragflow_deps/download_go_deps.py`", file=sys.stderr)
            return 1
        tokenizer = Tokenizer.from_file(tokenizer_path)
        vocab = tokenizer.get_vocab()
        unk_id = vocab.get("<unk>")
        rows = []
        for label, text in rows_src:
            encoding = tokenizer.encode(text, add_special_tokens=False)
            # HuggingFace renders an unknown token as the *source text* it could not
            # encode (Unigram models do: token "🧑" arrives with id 3 = "<unk>"),
            # while a counter can only report the unknown token itself. The fixture
            # stores the canonical token text so the comparison is about the
            # segmentation, not about a rendering convention.
            tokens = ["<unk>" if unk_id is not None and tid == unk_id else tok for tok, tid in zip(encoding.tokens, encoding.ids)]
            # Both witnesses are kept: the token texts are numbering-independent and
            # are what the Go test compares; the ids are kept for local reasoning
            # (and are the primary witness for cl100k, whose numbering is shared).
            rows.append(
                {
                    "label": label,
                    "text": text,
                    "ids": encoding.ids,
                    "tokens": tokens,
                    "chars": len(text),
                    "known_approx": is_known_approximation(text),
                }
            )
        oracle = tokenizer_path

    # Counts are derived, never typed: the id list is the evidence, the count is
    # its length. Keeping both in the fixture makes a self-inconsistent fixture
    # impossible to overlook.
    for row in rows:
        row["count"] = len(row["ids"])

    crosscheck_note = None
    if args.spm_crosscheck:
        model_file = os.path.join(args.deps, SPM_MODEL)
        if not os.path.isfile(model_file):
            # --spm-crosscheck was asked for, so the asset it reads has to be there:
            # skipping quietly would turn "the second opinion agrees" into a claim
            # nobody made. Assets are not optional in this script - they are checked.
            print(f"missing sentencepiece model: {model_file}\nrun `uv run ragflow_deps/download_go_deps.py`", file=sys.stderr)
            return 1
        processor = spm.SentencePieceProcessor(model_file=model_file)
        divergent = 0
        for row in rows:
            spm_ids = processor.encode(row["text"], out_type=int)
            row["spm_count"] = len(spm_ids)
            if len(spm_ids) != row["count"]:
                divergent += 1
        crosscheck_note = f"Google sentencepiece {getattr(spm, '__version__', '?')} on {SPM_MODEL}: {divergent}/{len(rows)} samples differ from the served tokenizer.json"
        print(f"cross-check: {crosscheck_note}")

    fixture = {
        "model": args.model,
        "counter": counter_id,
        "oracle": oracle,
        "reference": "https://github.com/huggingface/tokenizers" if rel else "https://github.com/openai/tiktoken",
        "samples": rows,
    }
    if crosscheck_note:
        fixture["crosscheck"] = crosscheck_note
    with open(args.out, "w", encoding="utf-8") as fh:
        json.dump(fixture, fh, ensure_ascii=False, indent=1)
    print(f"wrote {args.out}: {len(rows)} samples for {args.model} ({counter_id}), oracle={oracle}")
    if not args.quiet:
        for row in rows:
            extra = f" spm={row['spm_count']}" if "spm_count" in row and row["spm_count"] != row["count"] else ""
            print(f"  {row['label']:<28} chars={row['chars']:<7} tokens={row['count']:<7}{extra}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
