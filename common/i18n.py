#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from __future__ import annotations

import json
import logging
import math
import os
import re
from contextvars import ContextVar
from pathlib import Path

DEFAULT_LOCALE = "en"
SUPPORTED_LOCALES = (
    "en",
    "zh-Hans",
    "zh-Hant",
    "id",
    "ja",
    "es",
    "vi",
    "ru",
    "pt-BR",
    "de",
    "fr",
    "it",
    "bg",
    "ar",
    "tr",
    "ko",
)

# User.language display names → BCP47, aligned with web LanguageTranslationMap.
_DISPLAY_NAME_MAP = {
    "english": "en",
    "chinese": "zh-Hans",
    "简体中文": "zh-Hans",
    "traditional chinese": "zh-Hant",
    "繁體中文": "zh-Hant",
    "russian": "ru",
    "indonesian": "id",
    "indonesia": "id",
    "spanish": "es",
    "vietnamese": "vi",
    "japanese": "ja",
    "korean": "ko",
    "portuguese br": "pt-BR",
    "pt-br": "pt-BR",
    "german": "de",
    "french": "fr",
    "italian": "it",
    "bulgarian": "bg",
    "arabic": "ar",
    "turkish": "tr",
    "dutch": "nl",
}

_ALIAS = {
    "zh": "zh-Hans",
    "zh-cn": "zh-Hans",
    "zh-sg": "zh-Hans",
    "zh-hans": "zh-Hans",
    "zh-tw": "zh-Hant",
    "zh-hk": "zh-Hant",
    "zh-mo": "zh-Hant",
    "zh-hant": "zh-Hant",
    "pt": "pt-BR",
    "pt-br": "pt-BR",
}

_SUPPORTED = frozenset(SUPPORTED_LOCALES)
_locale: ContextVar[str | None] = ContextVar("api_locale", default=None)
_catalogs: dict[str, dict[str, str]] | None = None
_PLACEHOLDER = re.compile(r"\{\{(\w+)\}\}")
_ID_KEY = re.compile(r"^error(?:\.[a-z][a-z0-9_]*)+$")
_template_index: list[tuple[re.Pattern[str], str]] | None = None


class _MsgNode:
    __slots__ = ("_path", "_key", "_children")

    def __init__(self, path: str, key: str | None):
        self._path = path
        self._key = key
        self._children: dict[str, _MsgNode] = {}

    def __getattr__(self, name: str) -> _MsgNode:
        child = self._children.get(name)
        if child is None:
            prefix = f"{self._path}." if self._path else "error."
            raise AttributeError(f"unknown i18n message {prefix}{name}")
        return child

    def __dir__(self) -> list[str]:
        return sorted(self._children)

    def __str__(self) -> str:
        if self._key is None:
            raise TypeError(f"{self._path} is a message group, not a key")
        return self._key

    def __repr__(self) -> str:
        if self._key is None:
            return f"<msg {self._path}>"
        return self._key

    def __hash__(self) -> int:
        return hash(str(self))

    def __eq__(self, other: object) -> bool:
        return str(self) == other if self._key is not None else False


class _MsgProxy:
    def __getattr__(self, name: str) -> _MsgNode:
        _load_catalogs()
        assert _msg_root is not None
        return getattr(_msg_root, name)

    def __dir__(self) -> list[str]:
        _load_catalogs()
        assert _msg_root is not None
        return dir(_msg_root)


msg = _MsgProxy()
_msg_root: _MsgNode | None = None


def _is_id_key(key: str) -> bool:
    return bool(_ID_KEY.fullmatch(key))


def _build_msg_tree(catalog: dict[str, str]) -> _MsgNode:
    root = _MsgNode("error", None)
    for key in catalog:
        if not _is_id_key(key):
            continue
        node = root
        path = "error"
        for part in key.split(".")[1:]:
            path = f"{path}.{part}"
            child = node._children.get(part)
            if child is None:
                child = _MsgNode(path, None)
                node._children[part] = child
            node = child
        node._key = key
    return root


def _key_str(key: object) -> str:
    if key is None:
        return ""
    if isinstance(key, str):
        return key
    return str(key)


def _locales_dir() -> Path:
    return Path(__file__).resolve().parent.parent / "locales" / "api"


def _load_catalogs() -> dict[str, dict[str, str]]:
    global _catalogs, _msg_root
    if _catalogs is not None:
        return _catalogs
    catalogs: dict[str, dict[str, str]] = {}
    root = _locales_dir()
    for loc in SUPPORTED_LOCALES:
        path = root / f"{loc}.json"
        if not path.is_file():
            catalogs[loc] = {}
            continue
        with path.open(encoding="utf-8") as f:
            data = json.load(f)
        catalogs[loc] = {str(k): str(v) for k, v in data.items()}
    _catalogs = catalogs
    _msg_root = _build_msg_tree(catalogs.get(DEFAULT_LOCALE, {}))
    return catalogs


def normalize_locale(raw: str | None) -> str | None:
    if not raw:
        return None
    text = raw.strip()
    if not text:
        return None
    lowered = text.lower()
    if lowered in {"c", "posix"}:
        return None
    mapped = _DISPLAY_NAME_MAP.get(lowered)
    if mapped:
        return mapped if mapped in _SUPPORTED else None
    tag = text.split(",", 1)[0].split(";", 1)[0].strip()
    tag = tag.split(".", 1)[0].strip().replace("_", "-")
    if not tag:
        return None
    parts = tag.split("-")
    primary = parts[0].lower()
    rest = "-".join(parts[1:])
    candidate = f"{primary}-{rest}" if rest else primary
    aliased = _ALIAS.get(candidate.lower(), _ALIAS.get(primary, candidate if rest else primary))
    if aliased in _SUPPORTED:
        return aliased
    if primary in _SUPPORTED:
        return primary
    return None


def parse_accept_language(header: str | None) -> str | None:
    if not header:
        return None
    ranges: list[tuple[float, int, str]] = []
    for idx, part in enumerate(header.split(",")):
        piece = part.strip()
        if not piece:
            continue
        tag, *params = [p.strip() for p in piece.split(";")]
        q = 1.0
        for param in params:
            if param.startswith("q="):
                try:
                    q = float(param[2:])
                except ValueError:
                    q = 0.0
        if math.isnan(q) or q <= 0 or q > 1:
            continue
        ranges.append((-q, idx, tag))
    ranges.sort()
    for _, _, tag in ranges:
        loc = normalize_locale(tag)
        if loc:
            return loc
    return None


def _is_explicit_app_locale(header: str | None) -> bool:
    """True when the client sent a single app locale (e.g. frontend i18n.language).

    Browser / Postman defaults like ``en-US,en;q=0.9`` or ``en-US`` are not explicit.
    """
    if not header:
        return False
    tag = header.strip()
    if not tag or "," in tag or ";" in tag:
        return False
    if tag in _SUPPORTED:
        return True
    lowered = tag.lower().replace("_", "-")
    if lowered in _ALIAS:
        return True
    if tag.lower() in _DISPLAY_NAME_MAP:
        return True
    return False


def resolve_locale(accept_language: str | None = None, user_language: str | None = None, lang_env: str | None = None) -> str:
    header_loc = parse_accept_language(accept_language)
    user_loc = normalize_locale(user_language)
    env_loc = normalize_locale(lang_env)
    if header_loc and _is_explicit_app_locale(accept_language):
        return header_loc
    if user_loc:
        return user_loc
    if header_loc:
        return header_loc
    if env_loc:
        return env_loc
    return DEFAULT_LOCALE


def set_locale(locale: str | None):
    return _locale.set(locale)


def get_locale() -> str:
    header = None
    user_lang = None
    try:
        from quart import g, request

        header = request.headers.get("Accept-Language")
        user = getattr(g, "user", None)
        if user is not None:
            user_lang = getattr(user, "language", None)
        if user_lang or header:
            return resolve_locale(header, user_lang, os.environ.get("LANG"))
    except (RuntimeError, ImportError):
        pass
    except Exception:
        logging.exception("failed to resolve locale from request")
    loc = _locale.get()
    if loc:
        return loc
    return resolve_locale(header, user_lang, os.environ.get("LANG"))


def interpolate(template: str, kwargs: dict[str, object]) -> str:
    if not kwargs:
        return template

    def repl(match: re.Match[str]) -> str:
        name = match.group(1)
        if name in kwargs:
            return str(kwargs[name])
        return match.group(0)

    return _PLACEHOLDER.sub(repl, template)


def _compile_template(template: str) -> re.Pattern[str]:
    parts = _PLACEHOLDER.split(template)
    pattern = "^"
    for i, part in enumerate(parts):
        if i % 2 == 0:
            pattern += re.escape(part)
        else:
            pattern += f"(?P<{part}>.*)"
    pattern += "$"
    return re.compile(pattern)


def _en_template_index() -> list[tuple[re.Pattern[str], str]]:
    global _template_index
    if _template_index is not None:
        return _template_index
    en = _load_catalogs().get(DEFAULT_LOCALE, {})
    items: list[tuple[int, re.Pattern[str], str]] = []
    seen: set[str] = set()
    for key, value in en.items():
        source = key if "{{" in key else value if "{{" in value else None
        if not source or source in seen:
            continue
        seen.add(source)
        items.append((len(source), _compile_template(source), key))
    items.sort(key=lambda item: item[0], reverse=True)
    _template_index = [(pattern, key) for _, pattern, key in items]
    return _template_index


def _lookup(catalogs: dict[str, dict[str, str]], locale: str, key: str) -> str | None:
    text = catalogs.get(locale, {}).get(key)
    if text is None and locale != DEFAULT_LOCALE:
        text = catalogs.get(DEFAULT_LOCALE, {}).get(key)
    return text


def t(key: object, **kwargs: object) -> str:
    key = _key_str(key)
    if not key:
        return key
    catalogs = _load_catalogs()
    locale = get_locale()
    text = _lookup(catalogs, locale, key)
    if text is None:
        for pattern, catalog_key in _en_template_index():
            matched = pattern.fullmatch(key)
            if not matched:
                continue
            text = _lookup(catalogs, locale, catalog_key)
            if text is None:
                text = key
            kwargs = {**matched.groupdict(), **kwargs}
            break
        else:
            text = key
    return interpolate(text, kwargs)


def message_ids() -> list[str]:
    return sorted(k for k in _load_catalogs().get(DEFAULT_LOCALE, {}) if _is_id_key(k))


def _go_const_name(key: str) -> str:
    rest = key.split(".", 1)[1]
    return "".join(part.title() for part in rest.replace(".", "_").split("_"))


def render_go_msg() -> str:
    ids = message_ids()
    names: dict[str, str] = {}
    for key in ids:
        name = _go_const_name(key)
        if name in names:
            raise ValueError(f"Go const collision: {names[name]} and {key} -> {name}")
        names[name] = key
    width = max(len(name) for name in names)
    lines = [
        "//",
        "//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.",
        "//",
        "// Code generated from locales/api/en.json. DO NOT EDIT.",
        "//",
        "",
        "package i18n",
        "",
        "const (",
    ]
    for name in sorted(names):
        lines.append(f'\t{name.ljust(width)} = "{names[name]}"')
    lines.append(")")
    lines.append("")
    lines.append("var Keys = []string{")
    for name in sorted(names):
        lines.append(f"\t{name},")
    lines.append("}")
    lines.append("")
    return "\n".join(lines)


def write_go_msg() -> Path:
    path = Path(__file__).resolve().parent.parent / "internal" / "i18n" / "msg.go"
    path.write_text(render_go_msg(), encoding="utf-8", newline="\n")
    return path

