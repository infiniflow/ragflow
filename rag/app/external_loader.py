#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""
External Loader parser for RAGFlow.

Routes file extensions to either an external REST API or a built-in RAGFlow
parser, as configured in service_conf.yaml. When an external loader is used,
the returned Markdown is chunked by naive.chunk() (markdown-aware splitting
with child chunk support).

Configuration lives entirely in service_conf.yaml under `external_loaders`:

  external_loaders:
    routes:
      # Each route targets either an external loader (url) or a built-in
      # RAGFlow parser (parser) — the two keys are mutually exclusive.
      # First matching route wins (case-insensitive extension match).
      - extensions: [".docx", ".doc", ".odt"]
        url: "http://doc-loader:8080/process"
        api_key: ""          # optional Bearer token
        method: POST         # optional, default POST
      - extensions: [".xlsx", ".xls"]
        parser: table        # delegate to RAGFlow's built-in table parser
      - extensions: [".pdf"]
        url: "http://pdf-loader:8080/process"
        api_key: "secret"
    fallback:
      # Applied when no route matches the file extension.
      # Option A – send to a generic external loader:
      url: "http://generic-loader:8080/process"
      api_key: ""
      # Option B – delegate to a built-in RAGFlow parser:
      # parser: naive

External loader API contract:
  - Method  : as configured (default POST)
  - Body    : raw file binary
  - Headers : Content-Type (MIME type), X-Filename (URL-encoded basename),
              Authorization: Bearer <api_key>  (omitted when empty)
  - Response: JSON with at least a `page_content` string field
"""

import importlib
import logging
import mimetypes
import urllib.parse
from pathlib import Path

import requests

from common.config_utils import get_base_config
from common.constants import MAXIMUM_PAGE_NUMBER, ParserType
from rag.app import naive

# Parser IDs whose rag.app module name differs from their ParserType value.
_PARSER_MODULE_OVERRIDE = {
    ParserType.KG.value: "naive",  # "knowledge_graph" lives in rag.app.naive
}


def _find_route(ext: str, routes: list) -> dict | None:
    """Return the first route whose extensions list includes *ext* (case-insensitive)."""
    ext_lower = ext.lower()
    for route in routes:
        if ext_lower in [e.lower() for e in (route.get("extensions") or [])]:
            return route
    return None


def _loader_cfg_from_route(route: dict) -> dict:
    """Extract all loader fields from a route or fallback dict."""
    return {
        "url": (route.get("url") or "").strip(),
        "api_key": (route.get("api_key") or "").strip(),
        "method": (route.get("method") or "POST").strip().upper(),
        "_parser": (route.get("parser") or "").strip(),
    }


def _resolve(ext: str, parser_config: dict):
    """
    Resolve the loader for *ext*.

    Returns either:
      dict  – {url, api_key, method} to call an external loader
      str   – name of a built-in RAGFlow parser to delegate to

    Raises ValueError when nothing is configured for *ext*.

    Resolution order:
      1. routes list (first match wins)
      2. fallback section
      3. parser_config["external_loader"] override (can override any field)
    """
    # get_base_config returns None when the YAML key exists but has no value,
    # so `or {}` / `or []` guard against that in addition to the absent-key case.
    yaml_cfg = get_base_config("external_loaders", {}) or {}
    routes = yaml_cfg.get("routes") or []
    fallback = yaml_cfg.get("fallback") or {}

    route = _find_route(ext, routes)
    cfg = _loader_cfg_from_route(route if route is not None else fallback)

    # Per-KB/document override from parser_config
    pc_override = ((parser_config or {}).get("external_loader") or {})
    for k in ("url", "api_key", "method", "_parser"):
        val = (pc_override.get("parser" if k == "_parser" else k) or "").strip()
        if val:
            cfg[k] = val

    if cfg["url"]:
        return {"url": cfg["url"], "api_key": cfg["api_key"], "method": cfg["method"]}

    if cfg["_parser"]:
        return cfg["_parser"]

    raise ValueError(
        f"No loader configured for extension '{ext}'. "
        f"Add a matching entry to external_loaders.routes or configure "
        f"external_loaders.fallback in service_conf.yaml."
    )


def _call_loader(path: Path, binary: bytes, loader_cfg: dict) -> str:
    """
    Send *binary* to the external loader and return the markdown string.

    Request format (foil-serve / MarkGate compatible):
      Content-Type  : MIME type inferred from the file path
      X-Filename    : URL-encoded basename
      Authorization : Bearer <api_key>  (omitted when empty)
    """
    url = loader_cfg["url"]
    api_key = loader_cfg.get("api_key", "")
    method = loader_cfg.get("method", "POST").upper()

    mime_type, _ = mimetypes.guess_type(path)
    if not mime_type:
        mime_type = "application/octet-stream"

    headers = {
        "Content-Type": mime_type,
        "X-Filename": urllib.parse.quote(path.name),
    }
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"

    logging.info("external_loader: %s %s  file=%s", method, url, path.name)
    resp = requests.request(method, url, data=binary, headers=headers, timeout=300)
    resp.raise_for_status()

    try:
        result = resp.json()
    except Exception as e:
        raise ValueError(
            f"External loader returned invalid JSON for '{path.name}'. "
            f"Status: {resp.status_code}, Body preview: {resp.text[:200]!r}"
        ) from e
    page_content = result.get("page_content")
    if page_content is None:
        raise ValueError(
            f"External loader response missing 'page_content' for '{path.name}'. "
            f"Keys present: {list(result.keys())}"
        )
    return page_content


def chunk(filename:str, binary=None, from_page=0, to_page=MAXIMUM_PAGE_NUMBER, lang="Chinese", callback=None, **kwargs):
    """
    Entry point called by RAGFlow's task executor.

    Workflow:
      1. Look up the loader for this file's extension (routes → fallback).
      2a. If a built-in parser is selected: delegate directly (no HTTP call).
      2b. If an external loader URL is configured: POST the binary, receive
          markdown, pass it to naive.chunk() as '<original>.md'.
      3. Restore the original filename in every returned chunk.
    """
    path = Path(filename)
    parser_config = kwargs.get("parser_config", {})

    target = _resolve(path.suffix, parser_config)

    # --- Built-in parser fallback ---
    if isinstance(target, str):
        parser_name = target
        valid = {pt.value for pt in ParserType} - {ParserType.EXTERNAL.value}
        if parser_name not in valid:
            raise ValueError(
                f"Unknown built-in parser '{parser_name}' in external_loaders.fallback.parser. "
                f"Valid values: {sorted(valid)}"
            )
        module_name = _PARSER_MODULE_OVERRIDE.get(parser_name, parser_name)
        module = importlib.import_module(f"rag.app.{module_name}")
        logging.info("external_loader: delegating '%s' to built-in parser '%s'", path.name, parser_name)
        if callback:
            callback(0.1, f"Using built-in parser '{parser_name}'...")
        return module.chunk(str(path), binary=binary, from_page=from_page, to_page=to_page,
                            lang=lang, callback=callback, **kwargs)

    # --- External loader ---
    loader_cfg = target
    if callback:
        callback(0.05, "Calling external loader...")

    if binary is None:
        binary = path.read_bytes()

    markdown_text = _call_loader(path, binary, loader_cfg)
    if callback:
        callback(0.45, "External loader finished, chunking markdown...")

    # Append .md so naive.chunk() takes the markdown-aware code path.
    # e.g. report.pdf -> report.pdf.md
    md_path = path.with_suffix(f"{path.suffix}.md") 

    chunks = naive.chunk(
        filename=str(md_path),
        binary=markdown_text.encode("utf-8"),
        from_page=from_page,
        to_page=to_page,
        lang=lang,
        callback=callback,
        **kwargs,
    )

    # naive.chunk() stamped docnm_kwd with the synthetic name; restore the original.
    for ck in chunks:
        ck["docnm_kwd"] = str(path)

    return chunks
