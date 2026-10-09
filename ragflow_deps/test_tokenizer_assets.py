# Guard: the tokenizer assets production loads must actually be shipped and pinned.
#
# The Go counters in internal/tokenizer read their vocabulary from
# ragflow_deps/huggingface.co/<repo>/<file>. Three places have to agree:
#
#   1. ragflow_deps/download_go_deps.py - fetches the file, and says whether it is
#                                         "runtime" (the counters load it) or "oracle"
#                                         (only scripts/gen_tokenizer_oracle.py does).
#                                         It is the Go-side downloader: everything the
#                                         Go side reads comes from here, and
#                                         ragflow_deps/download_deps.py (upstream) is
#                                         deliberately left alone;
#   2. Dockerfile / Dockerfile_base / Dockerfile_go
#                                       - copy the runtime ones into the image at the
#                                         path the counters search;
#   3. internal/tokenizer/*.go          - pin the SHA-1 of the runtime ones.
#
# When they drift, the layer that can see it fails loudly: the runtime image does not
# build without a runtime asset, and an embedder whose model declares a counter that
# cannot load refuses to ingest rather than substituting the calibrated cl100k count
# (cl100k under-counts XLM-R on some content, and an under-count is what makes a provider
# answer 400). Those failures are still late - a broken image gets built, or the first
# document fails - which is why the drift is checked here, cheaply, instead.
# See internal/tokenizer/embedding_token_limits.md.

import ast
import re
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
RUNTIME_IMAGES = ("Dockerfile", "Dockerfile_base", "Dockerfile_go")
GO_SOURCES = ("spm.go", "wordpiece.go", "bpe.go")
KINDS = ("runtime", "oracle")
DOWNLOADER = "download_go_deps.py"
UPSTREAM_DOWNLOADER = "download_deps.py"


def load_tokenizer_assets():
    """Read TOKENIZER_ASSETS out of the Go-side downloader without importing it (that
    module pulls in huggingface_hub, which a unit test run does not need)."""
    source = (HERE / DOWNLOADER).read_text(encoding="utf-8")
    for node in ast.parse(source).body:
        if isinstance(node, ast.Assign) and any(getattr(target, "id", None) == "TOKENIZER_ASSETS" for target in node.targets):
            return [tuple(ast.literal_eval(element)) for element in node.value.elts]
    raise AssertionError(f"TOKENIZER_ASSETS not found in {DOWNLOADER}")


def image_text(name):
    return (ROOT / name).read_text(encoding="utf-8")


def test_assets_are_well_formed():
    assets = load_tokenizer_assets()
    assert assets, "tokenizer_assets is empty"
    for repo, filename, kind in assets:
        assert kind in KINDS, f"{repo}/{filename}: unknown kind {kind!r}"
        assert "/" in repo and filename, f"{repo}/{filename}: malformed entry"
    runtime = [a for a in assets if a[2] == "runtime"]
    assert runtime, "no runtime asset: at least one counter would always be unavailable"


def test_every_runtime_asset_is_copied_into_every_runtime_image():
    assets = [a for a in load_tokenizer_assets() if a[2] == "runtime"]
    for image in RUNTIME_IMAGES:
        text = image_text(image)
        for repo, filename, _kind in assets:
            assert f"{repo}/{filename}" in text, (
                f"{image} does not copy {repo}/{filename}; the matching counter would be unavailable in that image and ingest would silently fall back to the calibrated count"
            )


def test_every_runtime_asset_is_pinned_by_the_loader():
    sources = {name: (ROOT / "internal" / "tokenizer" / name).read_text(encoding="utf-8") for name in GO_SOURCES}
    for repo, filename, kind in load_tokenizer_assets():
        if kind != "runtime":
            continue
        key = f"{repo}/{filename}"
        pinned = [match.group(1) for text in sources.values() for match in re.finditer(rf'"{re.escape(key)}":\s*"([0-9a-f]{{40}})"', text)]
        assert pinned, f"{key} has no 40-hex SHA-1 pin in internal/tokenizer/*.go"
        for digest in pinned:
            assert len(set(digest)) > 4, f"{key}: pin {digest!r} does not look like a digest"


def test_oracle_only_assets_are_not_shipped():
    assets = [a for a in load_tokenizer_assets() if a[2] == "oracle"]
    for image in RUNTIME_IMAGES:
        text = image_text(image)
        for repo, filename, _kind in assets:
            assert f"{repo}/{filename}" not in text, f"{image} ships {repo}/{filename}, which only the fixture generator needs"


def test_assets_are_fetched_by_the_go_downloader_only():
    """The Go-side assets belong to the Go-side downloader.

    ragflow_deps/download_deps.py is an upstream file (it snapshots the Python side's
    dependencies); adding the Go counters' tokenizer files there split the list across
    two scripts and made the upstream file carry Go concerns. Keeping the fetch in
    download_go_deps.py is what makes a Go checkout self-sufficient.
    """
    upstream = (HERE / UPSTREAM_DOWNLOADER).read_text(encoding="utf-8")
    for repo, filename, _kind in load_tokenizer_assets():
        assert filename not in upstream, f"{UPSTREAM_DOWNLOADER} mentions {repo}/{filename}; the embedding tokenizer assets are fetched by {DOWNLOADER} only"
