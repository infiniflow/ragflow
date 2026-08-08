#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#

import re
from urllib.parse import quote, urlencode

CONTENT_TYPE_MAP = {
    "docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    "doc": "application/msword",
    "pdf": "application/pdf",
    "csv": "text/csv",
    "xls": "application/vnd.ms-excel",
    "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    "txt": "text/plain",
    "py": "text/plain",
    "js": "text/plain",
    "java": "text/plain",
    "c": "text/plain",
    "cpp": "text/plain",
    "h": "text/plain",
    "php": "text/plain",
    "go": "text/plain",
    "ts": "text/plain",
    "sh": "text/plain",
    "cs": "text/plain",
    "kt": "text/plain",
    "sql": "text/plain",
    "md": "text/markdown",
    "markdown": "text/markdown",
    "mdx": "text/markdown",
    "htm": "text/html",
    "html": "text/html",
    "json": "application/json",
    "png": "image/png",
    "jpg": "image/jpeg",
    "jpeg": "image/jpeg",
    "gif": "image/gif",
    "bmp": "image/bmp",
    "tiff": "image/tiff",
    "tif": "image/tiff",
    "webp": "image/webp",
    "svg": "image/svg+xml",
    "ico": "image/x-icon",
    "avif": "image/avif",
    "heic": "image/heic",
    "ppt": "application/vnd.ms-powerpoint",
    "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
}

FORCE_ATTACHMENT_EXTENSIONS = {
    "htm",
    "html",
    "shtml",
    "xht",
    "xhtml",
    "xml",
    "mhtml",
    "svg",
}

FORCE_ATTACHMENT_CONTENT_TYPES = {
    "text/html",
    "image/svg+xml",
    "application/xhtml+xml",
    "text/xml",
    "application/xml",
    "multipart/related",
}


def should_force_attachment(ext: str | None, content_type: str | None = None) -> bool:
    normalized_ext = (ext or "").lower().strip(".")
    if normalized_ext in FORCE_ATTACHMENT_EXTENSIONS:
        return True
    normalized_type = (content_type or "").lower().split(";")[0].strip()
    return normalized_type in FORCE_ATTACHMENT_CONTENT_TYPES


def sanitize_content_disposition_filename(filename: str | None) -> str | None:
    if not filename:
        return None
    base = re.sub(r"[^\w.\-]", "_", str(filename).split("/")[-1].split("\\")[-1])
    return base or None


def ascii_content_disposition_filename(filename: str | None) -> str | None:
    if not filename:
        return None
    base = str(filename).split("/")[-1].split("\\")[-1]
    if not base:
        return None
    ascii_only = base.encode("ascii", "ignore").decode("ascii")
    safe = re.sub(r"[^\w.\-]", "_", ascii_only).strip("._")
    return safe or None


def format_content_disposition(disposition: str, filename: str | None) -> str:
    if not filename:
        return disposition
    base = str(filename).split("/")[-1].split("\\")[-1]
    if not base:
        return disposition
    ascii_fallback = ascii_content_disposition_filename(base) or "file"
    encoded = quote(base, safe="")
    return f'{disposition}; filename="{ascii_fallback}"; filename*=UTF-8\'\'{encoded}'


def resolve_attachment_content_type(ext: str | None = None, mime_type: str | None = None) -> tuple[str | None, str | None]:
    if mime_type:
        normalized_type = mime_type.lower().split(";")[0].strip()
        for known_ext, known_type in CONTENT_TYPE_MAP.items():
            if known_type == normalized_type:
                return normalized_type, known_ext
        return normalized_type, (ext or "").lower().strip(".") or None
    if ext:
        normalized_ext = ext.lower().strip(".")
        return CONTENT_TYPE_MAP.get(normalized_ext, f"application/{normalized_ext}"), normalized_ext
    return None, None


def apply_preview_file_response_headers(
    response,
    content_type: str | None,
    ext: str | None = None,
    filename: str | None = None,
):
    if content_type:
        response.headers.set("Content-Type", content_type)
    if should_force_attachment(ext, content_type):
        response.headers.set("X-Content-Type-Options", "nosniff")
        response.headers.set("Content-Disposition", format_content_disposition("attachment", filename) if filename else "attachment")
        return response
    if filename:
        response.headers.set("Content-Disposition", format_content_disposition("inline", filename))
    else:
        response.headers.set("Content-Disposition", "inline")
    return response


def apply_download_file_response_headers(
    response,
    content_type: str | None,
    ext: str | None = None,
    filename: str | None = None,
):
    if content_type:
        response.headers.set("Content-Type", content_type)
    if should_force_attachment(ext, content_type):
        response.headers.set("X-Content-Type-Options", "nosniff")
        response.headers.set("Content-Disposition", format_content_disposition("attachment", filename) if filename else "attachment")
        return response
    if filename:
        response.headers.set("Content-Disposition", format_content_disposition("attachment", filename))
    else:
        response.headers.set("Content-Disposition", "attachment")
    return response


def agent_attachment_preview_path(attachment_id: str, *, ext: str | None = None, mime_type: str | None = None) -> str:
    query: dict[str, str] = {}
    if ext:
        query["ext"] = ext
    if mime_type:
        query["mime_type"] = mime_type
    suffix = f"?{urlencode(query)}" if query else ""
    return f"/api/v1/agents/attachments/{attachment_id}/preview{suffix}"
