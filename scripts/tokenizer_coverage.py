#!/usr/bin/env python3
"""Report how the embedding models in the catalog are covered by tokenizer counters.

The catalog (`conf/all_models.json`) declares a `tokenizer` per model when the model
has been matched to a counter in `internal/tokenizer`. Everything else falls back to
the calibrated path at runtime, which is safe but less precise - so the number that
matters operationally is "how many models are tagged, and which untagged ones could be
tagged cheaply because their architecture is already implemented".

This script answers that from the catalog, with no network and no guesses about
individual models: it groups the tagged models by counter, then groups the untagged
ones by a *name heuristic* into the architecture they most likely use, and prints the
result as markdown so it can be pasted into
`internal/tokenizer/embedding_token_limits.md`.

Usage:
    python3 scripts/tokenizer_coverage.py                  # human-readable
    python3 scripts/tokenizer_coverage.py --markdown       # table for the doc
    python3 scripts/tokenizer_coverage.py --untagged       # list every untagged model
"""

import argparse
import collections
import json
import sys

CATALOG = "conf/all_models.json"

# Counters implemented in internal/tokenizer (see embedding_token_limits.md). A model
# tagged with one of these is counted exactly; anything else is calibrated at runtime.
IMPLEMENTED = {
    "cl100k_base": "tiktoken BPE (OpenAI table)",
    "xlmr-spm": "XLM-R Unigram SentencePiece",
    "bert-wordpiece": "BERT WordPiece",
    "qwen-bpe": "byte-level BPE (Qwen pre-tokenizer regex)",
    "llama-bpe": "SentencePiece-BPE (▁ prepend/replace, byte fallback)",
}

# Name heuristics for the untagged models. They are deliberately conservative: a wrong
# guess here means tagging a model with a counter that does not match its tokenizer,
# which is worse than leaving it calibrated. Anything not matched lands in
# "arch-mismatch" (a family we do not implement) or "hosted" (no downloadable artifact).
FAMILY_HINTS = [
    ("xlmr-spm", ("bge-m3", "multilingual-e5", "m3e", "gte-multilingual", "jina-embeddings-v3", "paraphrase-multilingual", "labse", "infgrad", "bge-multilingual")),
    ("bert-wordpiece", ("bge-", "e5-", "gte-", "jina-embeddings-v2", "bce-", "text2vec-", "mxbai", "gte-base", "gte-large", "gte-small", "arabic-", "multilingual-e5-small")),
    ("qwen-bpe", ("qwen3-embedding", "gte-qwen", "qwen3-reranker")),
    ("llama-bpe", ("e5-mistral", "mistral-embed", "nomic-embed", "nemoretriever", "sfr-embedding", "linq-embed", "llama-embed", "bge-en-icl", "stella")),
    ("cl100k_base", ("text-embedding-3", "text-embedding-ada")),
]

HOSTED_HINTS = (
    "gemini",
    "text-embedding-004",
    "cohere",
    "embed-multilingual",
    "embed-english",
    "voyage",
    "titan",
    "amazon.",
    "baichuan",
    "jina-clip",
    "cloudsway",
    "upstage",
    "nvidia",
    "gme-",
    "ibm",
    "watsonx",
    "zhipu",
    "zai-org",
    "yi-",
    "ernie",
    "doubao",
    "minimax",
    "tencent",
    "siliconflow",
    "text-similarity",
    "embedding-v1",
)

# Task and packaging variants of the same model. The catalog lists one entry per
# variant (a retrieval/classification/clustering head, a GGUF or MLX conversion, a
# quantisation), which inflates the model count without adding a tokenizer to verify:
# they all share the base model's tokenizer file. Counting base models is what tells
# us how much tokenizer work is actually left.
VARIANT_SUFFIXES = (
    "-classification",
    "-clustering",
    "-retrieval",
    "-text-matching",
    "-reranking",
    "-GGUF",
    "-gguf",
    "-mlx",
    "-qat-q4_0-unquantized",
    "-qat-q8_0-unquantized",
    "-unquantized",
    "-int8",
    "-fp16",
    "-onnx",
    "-v0",
    "-v1",
)


def base_name(name: str) -> str:
    """The model a catalog entry is a variant of (`.../x-retrieval-mlx` -> `.../x`)."""
    base = name.split(":")[0]
    changed = True
    while changed:
        changed = False
        for suffix in VARIANT_SUFFIXES:
            if base.endswith(suffix):
                base = base[: -len(suffix)]
                changed = True
    return base


def embedding_models(catalog: dict) -> list[dict]:
    out = []
    for model in catalog["models"]:
        types = model.get("model_types") or []
        if not isinstance(types, list):
            types = [types]
        if any("embed" in str(t).lower() for t in types):
            out.append(model)
    return out


def guess_family(name: str) -> str:
    lowered = name.lower()
    for family, hints in FAMILY_HINTS:
        if any(hint in lowered for hint in hints):
            return family
    if any(hint in lowered for hint in HOSTED_HINTS):
        return "hosted"
    return "other-architecture"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--catalog", default=CATALOG)
    parser.add_argument("--markdown", action="store_true", help="print the tables as markdown")
    parser.add_argument("--untagged", action="store_true", help="list every untagged model by guess")
    args = parser.parse_args()

    try:
        with open(args.catalog, encoding="utf-8") as handle:
            catalog = json.load(handle)
    except (OSError, json.JSONDecodeError) as err:
        print(f"cannot read {args.catalog}: {err}", file=sys.stderr)
        return 1

    models = embedding_models(catalog)
    tagged = collections.defaultdict(list)
    untagged = collections.defaultdict(list)
    bases: dict[str, str] = {}
    for model in models:
        name = model.get("name", "?")
        bases.setdefault(base_name(name), name)
        counter = model.get("tokenizer")
        if counter:
            tagged[counter].append(name)
        else:
            untagged[guess_family(name)].append(base_name(name))

    total = len(models)
    tagged_n = sum(len(v) for v in tagged.values())
    print(f"# Tokenizer coverage of the model catalog ({args.catalog})")
    print()
    print(f"embedding entries: **{total}** | distinct base models: **{len(bases)}** | tagged entries: **{tagged_n}** | calibrated entries: **{total - tagged_n}**")
    print()
    print("## Tagged (counted exactly)")
    print()
    if args.markdown:
        print("| counter | entries | distinct base models | scheme |")
        print("|---|---|---|---|")
        for counter, names in sorted(tagged.items(), key=lambda kv: -len(kv[1])):
            bases_here = len({base_name(n) for n in names})
            print(f"| `{counter}` | {len(names)} | {bases_here} | {IMPLEMENTED.get(counter, '**not implemented**')} |")
    else:
        for counter, names in sorted(tagged.items(), key=lambda kv: -len(kv[1])):
            print(f"  {counter:<18} {len(names):>3}   {IMPLEMENTED.get(counter, 'NOT IMPLEMENTED')}")
    print()
    print("## Untagged (calibrated at runtime; grouped by name heuristic)")
    print()
    if args.markdown:
        print("| likely architecture | entries | distinct base models | counter exists? |")
        print("|---|---|---|---|")
        for family, names in sorted(untagged.items(), key=lambda kv: -len(kv[1])):
            exists = "yes - only the tag is missing" if family in IMPLEMENTED else ("no - new implementation, plus an oracle" if family != "hosted" else "no artifact to verify against")
            print(f"| {family} | {len(names)} | {len(set(names))} | {exists} |")
    else:
        for family, names in sorted(untagged.items(), key=lambda kv: -len(kv[1])):
            print(f"  {family:<20} {len(names):>3} entries, {len(set(names)):>3} base models")
    if args.untagged:
        print()
        print("## Every untagged model")
        print()
        for family, names in sorted(untagged.items(), key=lambda kv: -len(kv[1])):
            print(f"### {family} ({len(names)})")
            for name in sorted(names):
                print(f"- {name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
