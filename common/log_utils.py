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

import json
import logging
import os
import os.path
from logging.handlers import RotatingFileHandler

from common.exceptions import UpstreamProviderError
from common.file_utils import get_project_base_directory

initialized_root_logger = False
pkg_levels = {}  # module-level to allow runtime modification

def init_root_logger(logfile_basename: str, log_format: str = "%(asctime)-15s %(levelname)-8s %(process)d %(message)s"):
    global initialized_root_logger, pkg_levels
    if initialized_root_logger:
        return
    initialized_root_logger = True

    logger = logging.getLogger()
    logger.handlers.clear()
    log_path = os.path.abspath(os.path.join(get_project_base_directory(), "logs", f"{logfile_basename}.log"))

    os.makedirs(os.path.dirname(log_path), exist_ok=True)
    formatter = logging.Formatter(log_format)

    handler1 = RotatingFileHandler(log_path, maxBytes=10*1024*1024, backupCount=5)
    handler1.setFormatter(formatter)
    logger.addHandler(handler1)

    handler2 = logging.StreamHandler()
    handler2.setFormatter(formatter)
    logger.addHandler(handler2)

    logging.captureWarnings(True)

    LOG_LEVELS = os.environ.get("LOG_LEVELS", "")
    for pkg_name_level in LOG_LEVELS.split(","):
        terms = pkg_name_level.split("=")
        if len(terms)!= 2:
            continue
        pkg_name, pkg_level = terms[0], terms[1]
        pkg_name = pkg_name.strip()
        pkg_level = logging.getLevelName(pkg_level.strip().upper())
        if not isinstance(pkg_level, int):
            pkg_level = logging.INFO
        pkg_levels[pkg_name] = logging.getLevelName(pkg_level)

    for pkg_name in ['peewee', 'pdfminer']:
        if pkg_name not in pkg_levels:
            pkg_levels[pkg_name] = logging.getLevelName(logging.WARNING)
    if 'root' not in pkg_levels:
        pkg_levels['root'] = logging.getLevelName(logging.INFO)

    for pkg_name, pkg_level in pkg_levels.items():
        pkg_logger = logging.getLogger(pkg_name)
        pkg_logger.setLevel(pkg_level)

    msg = f"{logfile_basename} log path: {log_path}, log levels: {pkg_levels}"
    logger.info(msg)


def set_log_level(pkg_name: str, level: str) -> bool:
    """Set log level for a package at runtime. Returns True if successful."""
    global pkg_levels
    level_value = logging.getLevelName(level.strip().upper())
    if not isinstance(level_value, int):
        return False
    pkg_levels[pkg_name] = logging.getLevelName(level_value)
    pkg_logger = logging.getLogger(pkg_name)
    pkg_logger.setLevel(level_value)
    return True


def get_log_levels() -> dict:
    """Get current log levels for all packages."""
    global pkg_levels
    return dict(pkg_levels)


def _get_response_value(resp, key):
    try:
        if isinstance(resp, dict):
            return resp.get(key)
    except Exception:
        pass

    try:
        getter = getattr(resp, "get", None)
        if callable(getter):
            return getter(key)
    except Exception:
        pass

    try:
        return resp[key]
    except Exception:
        pass

    try:
        return getattr(resp, key)
    except Exception:
        return None


def _append_error_part(parts, seen, label, value):
    if value is None or value == "":
        return
    text = str(value).strip()
    if not text or text.lower() == "none":
        return
    part = f"{label}: {text}"
    if part not in seen:
        seen.add(part)
        parts.append(part)


def _is_error_status(status):
    try:
        return int(status) >= 400
    except Exception:
        return False


def _collect_upstream_error_parts(resp, parts, seen):
    if resp is None:
        return False

    status = _get_response_value(resp, "status_code") or _get_response_value(resp, "status")
    code = _get_response_value(resp, "code")
    request_id = _get_response_value(resp, "request_id")

    error_found = False
    for key in ("message", "error", "reason", "detail", "description"):
        value = _get_response_value(resp, key)
        if isinstance(value, dict):
            error_found = _collect_upstream_error_parts(value, parts, seen) or error_found
        elif value not in (None, ""):
            _append_error_part(parts, seen, key, value)
            error_found = True

    if error_found or _is_error_status(status):
        _append_error_part(parts, seen, "status_code", status)
        _append_error_part(parts, seen, "code", code)
        _append_error_part(parts, seen, "request_id", request_id)
        return True
    return False


def extract_upstream_error_message(*responses):
    """
    Extract useful error fields from provider responses without assuming one SDK shape.

    Providers commonly return dict-like objects or response objects with ``text``, ``message``,
    ``error``, ``reason``, ``code``, ``status_code``, and ``request_id`` fields. This helper
    turns those fields into a compact message suitable for API clients.
    """
    parts = []
    seen = set()
    fallback_texts = []

    for resp in responses:
        if resp is None:
            continue

        text = None
        try:
            text = getattr(resp, "text")
        except Exception:
            text = None

        if text:
            try:
                parsed = json.loads(text)
                if _collect_upstream_error_parts(parsed, parts, seen):
                    continue
            except Exception:
                fallback_texts.append(str(text).strip())

        _collect_upstream_error_parts(resp, parts, seen)

    if parts:
        return ", ".join(parts)

    for text in fallback_texts:
        if text:
            return text
    return ""


def log_exception(e, *args):
    logging.exception(e)
    response_text = None
    for a in args:
        try:
            text = getattr(a, "text")
        except Exception:
            text = None
        if text is not None:
            logging.error(text)
            response_text = text
        else:
            logging.error(str(a))
    upstream_error = extract_upstream_error_message(*args)
    if upstream_error:
        raise UpstreamProviderError(upstream_error) from e
    if response_text is not None:
        raise UpstreamProviderError(response_text) from e
    raise e
