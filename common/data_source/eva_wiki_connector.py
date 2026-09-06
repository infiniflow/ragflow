import hashlib
import json
import logging
import mimetypes
import uuid
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable
from urllib.parse import quote, urljoin, urlparse, urlsplit, urlunsplit

import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

from common.data_source.config import INDEX_BATCH_SIZE, REQUEST_TIMEOUT_SECONDS, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError, ConnectorValidationError, InsufficientPermissionsError
from common.data_source.html_utils import parse_html_page_basic
from common.data_source.interfaces import LoadConnector, PollConnector, SlimConnectorWithPermSync
from common.data_source.models import BasicExpertInfo, Document, GenerateDocumentsOutput, GenerateSlimDocumentOutput, SecondsSinceUnixEpoch, SlimDocument
from business_documents.domain.names import normalize_title


class EvaWikiConnector(LoadConnector, PollConnector, SlimConnectorWithPermSync):
    """Load one EVA Wiki project through EVA's authenticated JSON-RPC API."""

    DEFAULT_ATTACHMENT_SIZE_LIMIT = 10 * 1024 * 1024
    DEFAULT_PAGE_SIZE_LIMIT = 25 * 1024 * 1024
    DEFAULT_RETRY_COUNT = 3
    MAX_BATCH_SIZE = 1000
    MAX_CONTENT_SIZE_LIMIT = 100 * 1024 * 1024
    MAX_SEMANTIC_IDENTIFIER_LENGTH = 240

    _RETRYABLE_STATUS_CODES = frozenset({429, 500, 502, 503, 504})
    _SUPPORTED_ATTACHMENT_EXTENSIONS = frozenset(
        {
            ".aac",
            ".ai",
            ".alac",
            ".ape",
            ".apng",
            ".asf",
            ".asx",
            ".avi",
            ".avif",
            ".c",
            ".cdr",
            ".cpp",
            ".cs",
            ".csv",
            ".dat",
            ".doc",
            ".docx",
            ".dxf",
            ".eml",
            ".epub",
            ".eps",
            ".exif",
            ".flac",
            ".fpx",
            ".gif",
            ".go",
            ".h",
            ".hlp",
            ".htm",
            ".html",
            ".ico",
            ".icon",
            ".ini",
            ".java",
            ".jpg",
            ".jpeg",
            ".js",
            ".json",
            ".jsonl",
            ".key",
            ".kt",
            ".ldjson",
            ".md",
            ".mdx",
            ".mkv",
            ".mov",
            ".mp3",
            ".mp4",
            ".mpa",
            ".mpe",
            ".mpeg",
            ".mpg",
            ".msg",
            ".numbers",
            ".ogg",
            ".opus",
            ".pages",
            ".pcd",
            ".pcx",
            ".pdf",
            ".php",
            ".png",
            ".ppt",
            ".pptx",
            ".psd",
            ".py",
            ".raw",
            ".rm",
            ".rmvb",
            ".rtf",
            ".sh",
            ".sql",
            ".svg",
            ".tga",
            ".tif",
            ".ts",
            ".txt",
            ".ufo",
            ".vorbis",
            ".wav",
            ".webp",
            ".wmf",
            ".wps",
            ".wv",
            ".wvx",
            ".wavpack",
            ".xls",
            ".xlsx",
            ".xml",
            ".yml",
        }
    )

    _PAGE_INDEX_FIELDS = [
        "id",
        "name",
        "code",
        "cmf_modified_at",
        "cmf_deleted",
        "cmf_archived",
        "parent_id",
        "project_id",
        "project.name",
    ]
    _DOCUMENT_FIELDS = [
        *_PAGE_INDEX_FIELDS,
        "text",
        "text_render",
        "doc_version",
        "cmf_author_id",
        "cmf_owner_id",
        "cmf_author.email",
        "cmf_author.name",
        "cmf_owner.email",
        "cmf_owner.name",
        "perm_public",
        "perm_has_acl",
        "perm_inherit",
        "perm_acl_id",
        "perm_inherit_acl_id",
        "perm_effective_acl_id",
        "perm_encrypt",
        "is_public",
        "is_web_public",
        "tags.name",
        "workflow_id",
        "cur_published_version_id",
        "cur_workflow_version_id",
    ]
    _EDITABLE_DOCUMENT_FIELDS = [*_DOCUMENT_FIELDS, "text_draft"]
    _COMMENT_FIELDS = [
        "id",
        "parent_id",
        "project_id",
        "text",
        "cmf_created_at",
        "cmf_modified_at",
        "cmf_deleted",
        "cmf_archived",
        "private",
        "cmf_author_id",
        "cmf_author.email",
        "cmf_author.name",
    ]
    _ATTACHMENT_FIELDS = [
        "id",
        "name",
        "code",
        "file_name",
        "file_type",
        "st_size",
        "cmf_modified_at",
        "cmf_deleted",
        "cmf_archived",
        "parent_id",
        "project_id",
        "project.name",
        "url",
        "private",
        "cmf_author_id",
        "cmf_owner_id",
        "cmf_author.email",
        "cmf_author.name",
        "cmf_owner.email",
        "cmf_owner.name",
        "perm_public",
        "perm_has_acl",
        "perm_inherit",
        "perm_acl_id",
        "perm_inherit_acl_id",
        "perm_effective_acl_id",
        "perm_encrypt",
    ]

    def __init__(
        self,
        api_base_url: str,
        web_base_url: str | None = None,
        project_id: str | None = None,
        include_attachments: bool = True,
        include_archived: bool = False,
        verify_ssl: bool = True,
        batch_size: int = INDEX_BATCH_SIZE,
        attachment_size_limit: int = DEFAULT_ATTACHMENT_SIZE_LIMIT,
        page_size_limit: int = DEFAULT_PAGE_SIZE_LIMIT,
        retry_count: int = DEFAULT_RETRY_COUNT,
    ) -> None:
        self.api_base_url = str(api_base_url or "").strip().rstrip("/")
        self.web_base_url = str(web_base_url or api_base_url or "").strip().rstrip("/")
        self.project_id = str(project_id or "").strip()
        self.include_attachments = self._as_bool(include_attachments)
        self.include_archived = self._as_bool(include_archived)
        self.verify_ssl = self._as_bool(verify_ssl)
        self.batch_size = self._coerce_int(batch_size)
        self.attachment_size_limit = self._coerce_int(attachment_size_limit)
        self.page_size_limit = self._coerce_int(page_size_limit)
        self.retry_count = self._coerce_int(retry_count)
        self.credentials: dict[str, Any] = {}
        self._page_index: dict[str, dict[str, Any]] = {}
        self._page_paths: dict[str, str] = {}
        self._session = self._build_session()

    def _build_session(self) -> requests.Session:
        retry = Retry(
            total=max(self.retry_count, 0),
            connect=max(self.retry_count, 0),
            read=max(self.retry_count, 0),
            status=max(self.retry_count, 0),
            allowed_methods=frozenset({"GET", "POST"}),
            status_forcelist=self._RETRYABLE_STATUS_CODES,
            backoff_factor=0.5,
            respect_retry_after_header=True,
            raise_on_status=False,
        )
        adapter = HTTPAdapter(max_retries=retry)
        session = requests.Session()
        session.mount("http://", adapter)
        session.mount("https://", adapter)
        return session

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.credentials = credentials or {}
        token = str(self.credentials.get("eva_api_token") or "").strip()
        self._session.headers.pop("X-Eva-Token", None)
        self._session.headers.update({"Accept": "application/json"})
        if token:
            self._session.headers["X-Eva-Token"] = token
        return None

    def validate_connector_settings(self) -> None:
        self.get_project()

    def get_project(self) -> dict[str, str]:
        """Return the configured EVA project with its display metadata."""
        self._validate_config()
        filters: list[list[Any]] = [["id", "==", self.project_id], ["cmf_deleted", "==", False]]
        if not self.include_archived:
            filters.append(["cmf_archived", "==", False])
        projects = self._rpc(
            "CmfProject.list",
            {
                "filter": self._encode_filter(filters),
                "fields": ["id", "name", "code", "cmf_deleted", "cmf_archived"],
                "slice": [0, 1],
            },
        )
        if not projects:
            raise ConnectorValidationError(f"EVA Wiki project was not found or is not accessible: {self.project_id}")
        project = projects[0]
        if not isinstance(project, dict):
            raise ConnectorValidationError("Unexpected EVA Wiki project record")
        project_id = str(project.get("id") or "").strip()
        if project_id != self.project_id:
            raise ConnectorValidationError("EVA Wiki returned an unexpected project")
        return {
            "id": project_id,
            "name": str(project.get("name") or project.get("code") or "").strip(),
            "code": str(project.get("code") or "").strip(),
        }

    def list_projects(self) -> list[dict[str, str]]:
        """Return EVA projects accessible to the token and allowed by the archive setting."""
        self._validate_config(require_project_id=False)
        projects: list[dict[str, str]] = []
        last_id: str | None = None
        while True:
            filters: list[list[Any]] = [["cmf_deleted", "==", False]]
            if not self.include_archived:
                filters.append(["cmf_archived", "==", False])
            if last_id is not None:
                filters.append(["id", ">", last_id])
            rows = self._rpc(
                "CmfProject.list",
                {
                    "filter": self._encode_filter(filters),
                    "fields": ["id", "name", "code"],
                    "order_by": ["id"],
                    "slice": [0, self.batch_size],
                },
            )
            if not isinstance(rows, list):
                raise ConnectorValidationError("Unexpected EVA Wiki response for CmfProject.list")
            if not rows:
                break
            for row in rows:
                if not isinstance(row, dict):
                    raise ConnectorValidationError("Unexpected EVA Wiki project record")
                project_id = str(row.get("id") or "").strip()
                if not project_id or (last_id is not None and project_id <= last_id):
                    raise ConnectorValidationError("EVA Wiki returned a non-monotonic project list")
                last_id = project_id
                projects.append(
                    {
                        "id": project_id,
                        "name": str(row.get("name") or row.get("code") or project_id),
                        "code": str(row.get("code") or ""),
                    }
                )
            if len(rows) < self.batch_size:
                break
        return sorted(projects, key=lambda project: (project["name"].casefold(), project["code"].casefold(), project["id"]))

    def search_documents(self, query: str = "", limit: int = 20) -> list[dict[str, Any]]:
        """Return published pages that may be selected for an edit workflow."""

        self._validate_config()
        normalized_query = " ".join(str(query or "").casefold().split())
        safe_limit = min(max(self._coerce_int(limit), 1), 100)
        matches: list[dict[str, Any]] = []
        for page in self._iter_entities("CmfDocument", self._DOCUMENT_FIELDS):
            name = str(page.get("name") or page.get("code") or page.get("id") or "")
            code = str(page.get("code") or "")
            raw_content = str(page.get("text") or page.get("text_render") or "")
            searchable = " ".join((name, code, parse_html_page_basic(raw_content))).casefold()
            if normalized_query and normalized_query not in searchable:
                continue
            matches.append(self._editable_document(page, include_content=False))
            if len(matches) >= safe_limit:
                break
        return matches

    def find_documents_by_exact_title(self, title: str) -> list[dict[str, Any]]:
        """Return every accessible page whose normalized title matches exactly."""

        self._validate_config()
        expected = normalize_title(str(title or ""))
        if not expected:
            return []
        pages = list(self._iter_entities("CmfDocument", self._PAGE_INDEX_FIELDS))
        page_index = {str(page.get("id") or ""): page for page in pages if str(page.get("id") or "")}
        matches: list[dict[str, Any]] = []
        for page in pages:
            name = str(page.get("name") or page.get("code") or page.get("id") or "")
            if normalize_title(name) != expected:
                continue
            editable = self._editable_document(page, include_content=False)
            editable["parent_id"] = str(page.get("parent_id") or "") or None
            editable["breadcrumbs"] = self._page_breadcrumbs(str(page["id"]), page_index)
            editable["hierarchy"] = " › ".join(item["name"] for item in editable["breadcrumbs"])
            matches.append(editable)
        return sorted(matches, key=lambda item: (normalize_title(str(item.get("hierarchy") or "")), str(item.get("id") or "")))

    def get_document_for_edit(self, document_id: str) -> dict[str, Any]:
        """Load one published page together with its current EVA draft."""

        self._validate_config()
        normalized_id = str(document_id or "").strip()
        if not normalized_id:
            raise ConnectorValidationError("EVA Wiki document_id is required")
        page = self._get_entity_by_id("CmfDocument", self._EDITABLE_DOCUMENT_FIELDS, normalized_id)
        if page is None:
            raise ConnectorValidationError(f"EVA Wiki document was not found or is not accessible: {normalized_id}")
        return self._editable_document(page, include_content=True)

    def _editable_document(self, page: dict[str, Any], *, include_content: bool) -> dict[str, Any]:
        document_id = str(page.get("id") or "")
        code = str(page.get("code") or "").strip()
        raw_content = str(page.get("text") or page.get("text_render") or "")
        result: dict[str, Any] = {
            "id": document_id,
            "name": str(page.get("name") or code or document_id),
            "code": code,
            "project_id": str(page.get("project_id") or self.project_id),
            "version": self._document_version(page),
            "modified_at": str(page.get("cmf_modified_at") or ""),
            "web_url": f"{self.web_base_url}/project/Document/{quote(code, safe='')}" if code else self.web_base_url,
            "excerpt": parse_html_page_basic(raw_content)[:240],
        }
        if include_content:
            result["html"] = raw_content
            result["draft_html"] = str(page.get("text_draft") or "")
        return result

    def _page_breadcrumbs(self, page_id: str, page_index: dict[str, dict[str, Any]]) -> list[dict[str, str]]:
        breadcrumbs: list[dict[str, str]] = []
        current_id = page_id
        visited: set[str] = set()
        while current_id in page_index and current_id not in visited:
            visited.add(current_id)
            page = page_index[current_id]
            code = str(page.get("code") or "").strip()
            name = str(page.get("name") or code or current_id)
            breadcrumbs.append(
                {
                    "id": current_id,
                    "name": name,
                    "web_url": f"{self.web_base_url}/project/Document/{quote(code, safe='')}" if code else self.web_base_url,
                }
            )
            current_id = str(page.get("parent_id") or "")
        breadcrumbs.reverse()
        return breadcrumbs

    @staticmethod
    def _document_version(page: dict[str, Any]) -> str:
        values = (
            page.get("doc_version"),
            page.get("cur_published_version_id"),
            page.get("cmf_modified_at"),
        )
        return "|".join(str(value or "") for value in values)

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._load_documents(end=datetime.now(timezone.utc).timestamp())

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        if start > end:
            raise ConnectorValidationError("EVA Wiki poll start must not be later than poll end")
        yield from self._load_documents(start=start, end=end)

    def retrieve_all_slim_docs_perm_sync(self, callback: Any = None) -> GenerateSlimDocumentOutput:
        del callback
        active_page_ids: set[str] = set()
        yield from self._load_slim_entities(
            "CmfDocument",
            prefix="eva-wiki:document:",
            seen_entity_ids=active_page_ids,
        )
        if self.include_attachments:
            yield from self._load_slim_entities(
                "CmfAttachment",
                prefix="eva-wiki:attachment:",
                include_size=True,
                require_supported_extension=True,
                require_url=True,
                active_parent_ids=active_page_ids,
            )

    def _load_documents(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> GenerateDocumentsOutput:
        self._validate_config()
        snapshot_end = end if end is not None else datetime.now(timezone.utc).timestamp()
        self._build_page_index(snapshot_end)
        comments_by_parent = self._load_active_comments(snapshot_end)
        comment_change_times = self._load_comment_change_times(start, snapshot_end) if start is not None else {}

        batch: list[Document] = []
        emitted_page_ids: set[str] = set()
        changed_page_ids: set[str] = set()

        for page in self._iter_entities("CmfDocument", self._DOCUMENT_FIELDS, start=start, end=snapshot_end):
            page_id = str(page["id"])
            emitted_page_ids.add(page_id)
            changed_page_ids.add(page_id)
            batch.append(self._build_page_document(page, comments_by_parent.get(page_id, []), comment_change_times.get(page_id)))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        for page_id, comment_updated_at in comment_change_times.items():
            if page_id in emitted_page_ids or page_id not in self._page_index:
                continue
            page = self._get_entity_by_id("CmfDocument", self._DOCUMENT_FIELDS, page_id, end=snapshot_end)
            if page is None:
                continue
            emitted_page_ids.add(page_id)
            changed_page_ids.add(page_id)
            batch.append(self._build_page_document(page, comments_by_parent.get(page_id, []), comment_updated_at))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []

        if self.include_attachments:
            emitted_attachment_ids: set[str] = set()
            for attachment in self._iter_entities("CmfAttachment", self._ATTACHMENT_FIELDS, start=start, end=snapshot_end):
                document = self._attachment_to_document(attachment)
                if document is None:
                    continue
                emitted_attachment_ids.add(str(attachment["id"]))
                batch.append(document)
                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

            if start is not None:
                for page_id in changed_page_ids:
                    for attachment in self._iter_entities(
                        "CmfAttachment",
                        self._ATTACHMENT_FIELDS,
                        end=snapshot_end,
                        extra_filters=[["parent_id", "==", page_id]],
                    ):
                        attachment_id = str(attachment["id"])
                        if attachment_id in emitted_attachment_ids:
                            continue
                        document = self._attachment_to_document(attachment)
                        if document is None:
                            continue
                        emitted_attachment_ids.add(attachment_id)
                        batch.append(document)
                        if len(batch) >= self.batch_size:
                            yield batch
                            batch = []

        if batch:
            yield batch

    def _load_slim_entities(
        self,
        model: str,
        prefix: str,
        include_size: bool = False,
        require_supported_extension: bool = False,
        require_url: bool = False,
        active_parent_ids: set[str] | None = None,
        seen_entity_ids: set[str] | None = None,
    ) -> GenerateSlimDocumentOutput:
        batch: list[SlimDocument] = []
        fields = ["id", "cmf_deleted", "cmf_archived"]
        if include_size:
            fields.extend(["st_size", "file_name", "file_type"])
        if require_url:
            fields.append("url")
        if active_parent_ids is not None:
            fields.append("parent_id")
        for entity in self._iter_entities(model, fields):
            if include_size and self._as_int(entity.get("st_size")) > self.attachment_size_limit:
                continue
            if require_supported_extension and self._attachment_extension(str(entity.get("file_name") or ""), entity.get("file_type")) is None:
                continue
            if require_url and not str(entity.get("url") or "").strip():
                continue
            parent_id = str(entity.get("parent_id") or "")
            if active_parent_ids is not None and parent_id and parent_id not in active_parent_ids:
                continue
            entity_id = str(entity["id"])
            if seen_entity_ids is not None:
                seen_entity_ids.add(entity_id)
            batch.append(SlimDocument(id=f"{prefix}{entity_id}"))
            if len(batch) >= self.batch_size:
                yield batch
                batch = []
        if batch:
            yield batch

    def _iter_entities(
        self,
        model: str,
        fields: list[str],
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        *,
        include_deleted: bool = False,
        extra_filters: Iterable[list[Any]] = (),
    ):
        base_filters = [*self._project_filter(), *extra_filters]
        if not include_deleted:
            base_filters.append(["cmf_deleted", "==", False])
        if not self.include_archived:
            base_filters.append(["cmf_archived", "==", False])
        if start is not None:
            base_filters.append(["cmf_modified_at", ">", self._timestamp_to_iso(start)])
        if end is not None:
            base_filters.append(["cmf_modified_at", "<=", self._timestamp_to_iso(end)])

        last_id: str | None = None
        while True:
            filters = [*base_filters]
            if last_id is not None:
                filters.append(["id", ">", last_id])
            entities = self._rpc(
                f"{model}.list",
                {
                    "filter": self._encode_filter(filters),
                    "fields": fields,
                    "order_by": ["id"],
                    "slice": [0, self.batch_size],
                },
            )
            if not isinstance(entities, list):
                raise ConnectorValidationError(f"Unexpected EVA Wiki response for {model}.list")
            if not entities:
                break
            for entity in entities:
                entity_id = str(entity.get("id") or "")
                if not entity_id or (last_id is not None and entity_id <= last_id):
                    raise ConnectorValidationError(f"EVA Wiki returned a non-monotonic page for {model}.list")
                last_id = entity_id
                yield entity
            if len(entities) < self.batch_size:
                break

    def _get_entity_by_id(self, model: str, fields: list[str], entity_id: str, *, end: SecondsSinceUnixEpoch | None = None) -> dict[str, Any] | None:
        entities = list(self._iter_entities(model, fields, end=end, extra_filters=[["id", "==", entity_id]]))
        if len(entities) > 1:
            raise ConnectorValidationError(f"EVA Wiki returned duplicate IDs for {model}: {entity_id}")
        return entities[0] if entities else None

    def _project_filter(self) -> list[list[Any]]:
        return [["project_id", "==", self.project_id]]

    def _build_page_index(self, end: SecondsSinceUnixEpoch) -> None:
        self._page_index = {str(page["id"]): page for page in self._iter_entities("CmfDocument", self._PAGE_INDEX_FIELDS, end=end)}
        raw_paths = {page_id: self._build_page_path(page_id, set()) for page_id in self._page_index}
        path_counts = Counter(raw_paths.values())
        self._page_paths = {}
        for page_id, raw_path in raw_paths.items():
            if path_counts[raw_path] > 1:
                page = self._page_index[page_id]
                discriminator = str(page.get("code") or page_id.rsplit(":", 1)[-1][:8])
                raw_path = f"{raw_path} [{discriminator}]"
            self._page_paths[page_id] = self._bounded_semantic_identifier(raw_path, page_id)

    def _build_page_path(self, page_id: str, visiting: set[str]) -> str:
        page = self._page_index[page_id]
        title = self._clean_path_component(page.get("name") or page.get("code") or page_id)
        if page_id in visiting:
            return title
        parent_id = str(page.get("parent_id") or "")
        if parent_id in self._page_index:
            return f"{self._build_page_path(parent_id, visiting | {page_id})} > {title}"
        project = page.get("project") if isinstance(page.get("project"), dict) else {}
        project_name = self._clean_path_component(project.get("name") or self.project_id)
        return f"{project_name} > {title}" if project_name and project_name != title else title

    def _load_active_comments(self, end: SecondsSinceUnixEpoch) -> dict[str, list[dict[str, Any]]]:
        comments_by_parent: defaultdict[str, list[dict[str, Any]]] = defaultdict(list)
        for comment in self._iter_entities("CmfComment", self._COMMENT_FIELDS, end=end):
            parent_id = str(comment.get("parent_id") or "")
            if parent_id in self._page_index:
                comments_by_parent[parent_id].append(comment)
        for comments in comments_by_parent.values():
            comments.sort(key=lambda comment: (str(comment.get("cmf_created_at") or ""), str(comment.get("id") or "")))
        return dict(comments_by_parent)

    def _load_comment_change_times(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> dict[str, datetime]:
        change_times: dict[str, datetime] = {}
        for comment in self._iter_entities("CmfComment", self._COMMENT_FIELDS, start=start, end=end, include_deleted=True):
            parent_id = str(comment.get("parent_id") or "")
            if not parent_id.startswith("CmfDocument:"):
                continue
            changed_at = self._parse_datetime(comment.get("cmf_modified_at"))
            change_times[parent_id] = max(change_times.get(parent_id, datetime.fromtimestamp(0, tz=timezone.utc)), changed_at)
        return change_times

    def _build_page_document(
        self,
        page: dict[str, Any],
        comments: list[dict[str, Any]],
        comment_updated_at: datetime | None = None,
    ) -> Document:
        page_id = str(page["id"])
        semantic_identifier = self._page_paths.get(page_id) or self._bounded_semantic_identifier(str(page.get("name") or page.get("code") or page_id), page_id)
        title = str(page.get("name") or page.get("code") or page_id)
        raw_content = str(page.get("text") or page.get("text_render") or "")
        raw_bytes = raw_content.encode("utf-8")
        content_truncated = len(raw_bytes) > self.page_size_limit
        if content_truncated:
            raw_content = raw_bytes[: self.page_size_limit].decode("utf-8", errors="ignore")
        content = parse_html_page_basic(raw_content) if raw_content else ""

        comment_sections: list[str] = []
        latest_comment = comment_updated_at
        for comment in comments:
            comment_text = parse_html_page_basic(str(comment.get("text") or ""))
            if not comment_text:
                continue
            author = self._person_label(comment.get("cmf_author"), comment.get("cmf_author_id")) or "Unknown"
            created_at = self._parse_datetime(comment.get("cmf_created_at"))
            modified_at = self._parse_datetime(comment.get("cmf_modified_at"))
            latest_comment = max(latest_comment or modified_at, modified_at)
            comment_sections.append(f"[{created_at.isoformat()}] {author}: {comment_text}")

        body_parts = [title]
        if content:
            body_parts.append(content)
        if comment_sections:
            body_parts.append("Comments\n" + "\n".join(comment_sections))
        if content_truncated:
            body_parts.append("[EVA page content truncated by connector size limit]")
        blob = "\n\n".join(body_parts).encode("utf-8")
        if len(blob) > self.page_size_limit:
            blob = blob[: self.page_size_limit].decode("utf-8", errors="ignore").encode("utf-8")
            content_truncated = True

        code = str(page.get("code") or "").strip()
        link = f"{self.web_base_url}/project/Document/{quote(code, safe='')}" if code else self.web_base_url
        page_updated_at = self._parse_datetime(page.get("cmf_modified_at"))
        doc_updated_at = max(page_updated_at, latest_comment) if latest_comment else page_updated_at
        metadata = self._entity_metadata(page, link=link)
        metadata.update(
            {
                "eva_code": code,
                "parent_id": str(page.get("parent_id") or ""),
                "comment_count": len(comment_sections),
                "content_truncated": content_truncated,
                "source_size_bytes": len(raw_bytes),
            }
        )
        tags = self._relation_names(page.get("tags"))
        if tags:
            metadata["tags"] = tags

        return Document(
            id=f"eva-wiki:document:{page_id}",
            source=DocumentSource.EVA_WIKI,
            semantic_identifier=semantic_identifier,
            extension=".txt",
            blob=blob,
            doc_updated_at=doc_updated_at,
            size_bytes=len(blob),
            primary_owners=self._primary_owners(page),
            metadata=metadata,
            fingerprint=hashlib.md5(blob, usedforsecurity=False).hexdigest(),
        )

    def _attachment_to_document(self, attachment: dict[str, Any]) -> Document | None:
        attachment_id = str(attachment.get("id") or "")
        if not attachment_id:
            raise ConnectorValidationError("EVA Wiki attachment has no ID")
        parent_id = str(attachment.get("parent_id") or "")
        if parent_id and parent_id not in self._page_index:
            logging.warning("Skipping orphaned EVA Wiki attachment %s", attachment_id)
            return None
        size = self._as_int(attachment.get("st_size"))
        if size > self.attachment_size_limit:
            logging.warning("Skipping EVA Wiki attachment %s because it is %s bytes (limit: %s)", attachment_id, size, self.attachment_size_limit)
            return None
        file_name = str(attachment.get("file_name") or attachment.get("name") or attachment_id)
        if self._attachment_extension(file_name, attachment.get("file_type")) is None:
            logging.warning("Skipping unsupported EVA Wiki attachment %s (%s)", attachment_id, file_name)
            return None
        if not str(attachment.get("url") or "").strip():
            logging.warning("Skipping EVA Wiki attachment %s because it has no download URL", attachment_id)
            return None
        return self._build_attachment_document(attachment)

    def _build_attachment_document(self, attachment: dict[str, Any]) -> Document:
        attachment_id = str(attachment["id"])
        file_name = str(attachment.get("file_name") or attachment.get("name") or attachment_id)
        file_url = str(attachment.get("url") or "").strip()
        download_url = self._resolve_same_origin_api_url(file_url)
        try:
            response = self._session.get(
                download_url,
                timeout=REQUEST_TIMEOUT_SECONDS,
                verify=self.verify_ssl,
                stream=True,
                allow_redirects=False,
            )
        except requests.RequestException as exc:
            raise ConnectorValidationError(f"EVA Wiki request failed while downloading attachment {attachment_id}") from exc
        try:
            self._raise_for_status(response, operation="download attachment")
            content_length = self._as_int(response.headers.get("Content-Length"))
            if content_length > self.attachment_size_limit:
                raise ConnectorValidationError(f"EVA Wiki attachment {attachment_id} exceeds the configured size limit of {self.attachment_size_limit} bytes")
            content = bytearray()
            for chunk in response.iter_content(chunk_size=64 * 1024):
                if not chunk:
                    continue
                content.extend(chunk)
                if len(content) > self.attachment_size_limit:
                    raise ConnectorValidationError(f"EVA Wiki attachment {attachment_id} exceeds the configured size limit of {self.attachment_size_limit} bytes")
            blob = bytes(content)
        finally:
            response.close()

        extension = self._attachment_extension(file_name, attachment.get("file_type"))
        if extension is None:
            raise ConnectorValidationError(f"Unsupported EVA Wiki attachment type: {file_name}")
        parent_id = str(attachment.get("parent_id") or "")
        parent_path = self._page_paths.get(parent_id, "")
        semantic_identifier = self._bounded_semantic_identifier(f"{parent_path} > {file_name}" if parent_path else file_name, attachment_id)
        split_download_url = urlsplit(download_url)
        browser_path = urlunsplit(("", "", split_download_url.path, split_download_url.query, "")).lstrip("/")
        browser_link = urljoin(f"{self.web_base_url}/", browser_path)
        metadata = self._entity_metadata(attachment, link=browser_link)
        metadata.update({"eva_code": str(attachment.get("code") or ""), "parent_id": parent_id})

        return Document(
            id=f"eva-wiki:attachment:{attachment_id}",
            source=DocumentSource.EVA_WIKI,
            semantic_identifier=semantic_identifier,
            extension=extension,
            blob=blob,
            doc_updated_at=self._parse_datetime(attachment.get("cmf_modified_at")),
            size_bytes=len(blob),
            primary_owners=self._primary_owners(attachment),
            metadata=metadata,
            fingerprint=hashlib.md5(blob, usedforsecurity=False).hexdigest(),
        )

    def _entity_metadata(self, entity: dict[str, Any], *, link: str) -> dict[str, Any]:
        metadata: dict[str, Any] = {
            "link": link,
            "eva_id": str(entity.get("id") or ""),
            "project_id": str(entity.get("project_id") or ""),
            "eva_permission_scope": "token_principal",
            "eva_perm_public": self._as_bool(entity.get("perm_public")),
            "eva_perm_has_acl": self._as_bool(entity.get("perm_has_acl")),
            "eva_perm_inherit": self._as_bool(entity.get("perm_inherit")),
            "eva_perm_encrypted": self._as_bool(entity.get("perm_encrypt")),
        }
        project = entity.get("project") if isinstance(entity.get("project"), dict) else {}
        if project.get("name"):
            metadata["project_name"] = str(project["name"])
        for source_key, metadata_key in (
            ("perm_acl_id", "eva_perm_acl_id"),
            ("perm_inherit_acl_id", "eva_perm_inherit_acl_id"),
            ("perm_effective_acl_id", "eva_perm_effective_acl_id"),
            ("workflow_id", "eva_workflow_id"),
            ("cur_published_version_id", "eva_published_version_id"),
            ("cur_workflow_version_id", "eva_workflow_version_id"),
        ):
            if entity.get(source_key):
                metadata[metadata_key] = str(entity[source_key])
        author = self._person_label(entity.get("cmf_author"), entity.get("cmf_author_id"))
        owner = self._person_label(entity.get("cmf_owner"), entity.get("cmf_owner_id"))
        if author:
            metadata["eva_author"] = author
        if owner:
            metadata["eva_owner"] = owner
        return metadata

    def _primary_owners(self, entity: dict[str, Any]) -> list[BasicExpertInfo] | None:
        owners: list[BasicExpertInfo] = []
        seen: set[tuple[str, str]] = set()
        for relation_key in ("cmf_owner", "cmf_author"):
            person = entity.get(relation_key)
            if not isinstance(person, dict):
                continue
            display_name = str(person.get("name") or person.get("login") or "").strip()
            email = str(person.get("email") or "").strip()
            identity = (display_name, email)
            if identity == ("", "") or identity in seen:
                continue
            seen.add(identity)
            owners.append(BasicExpertInfo(display_name=display_name or None, email=email or None))
        return owners or None

    @staticmethod
    def _person_label(person: Any, fallback_id: Any = None) -> str:
        if isinstance(person, dict):
            name = str(person.get("name") or person.get("login") or "").strip()
            email = str(person.get("email") or "").strip()
            if name and email:
                return f"{name} <{email}>"
            if name or email:
                return name or email
        return str(fallback_id or "").strip()

    @staticmethod
    def _relation_names(value: Any) -> list[str]:
        if not isinstance(value, list):
            return []
        names = []
        for item in value:
            if isinstance(item, dict):
                name = str(item.get("name") or item.get("code") or "").strip()
                if name:
                    names.append(name)
        return names

    def _resolve_same_origin_api_url(self, candidate: str) -> str:
        resolved = urljoin(f"{self.api_base_url}/", candidate)
        parsed = urlparse(resolved)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc or parsed.username or parsed.password:
            raise ConnectorValidationError("EVA Wiki attachment URL is invalid")
        if self._url_origin(resolved) != self._url_origin(self.api_base_url):
            raise ConnectorValidationError("EVA Wiki attachment URL must use the configured API origin")
        return resolved

    @staticmethod
    def _url_origin(url: str) -> tuple[str, str, int | None]:
        parsed = urlparse(url)
        port = parsed.port
        if port is None:
            port = 443 if parsed.scheme.lower() == "https" else 80 if parsed.scheme.lower() == "http" else None
        return parsed.scheme.lower(), (parsed.hostname or "").lower(), port

    def _rpc(
        self,
        method: str,
        kwargs: dict[str, Any],
        *,
        args: list[Any] | None = None,
        call_kwargs: dict[str, Any] | None = None,
    ) -> Any:
        endpoint = self._resolve_same_origin_api_url(f"api/?m={quote(method, safe='')}")
        request_payload: dict[str, Any] = {"method": method, "kwargs": kwargs, "callid": str(uuid.uuid4()), "jsonrpc": "2.2"}
        if args is not None:
            request_payload["args"] = args
        if call_kwargs is not None:
            request_payload["kwargs"] = call_kwargs
        try:
            response = self._session.post(
                endpoint,
                json=request_payload,
                timeout=REQUEST_TIMEOUT_SECONDS,
                verify=self.verify_ssl,
                allow_redirects=False,
            )
        except requests.RequestException as exc:
            raise ConnectorValidationError(f"EVA Wiki request failed while trying to {method}") from exc
        self._raise_for_status(response, operation=method)
        try:
            payload = response.json()
        except requests.JSONDecodeError as exc:
            raise ConnectorValidationError(f"EVA Wiki returned invalid JSON for {method}") from exc

        error = payload.get("error") if isinstance(payload, dict) else None
        abort = payload.get("abort") if isinstance(payload, dict) else None
        if abort:
            error = abort
        if error:
            if isinstance(error, dict):
                message = error.get("message") or error.get("error") or json.dumps(error, ensure_ascii=False)
            else:
                message = str(error)
            raise ConnectorValidationError(f"EVA Wiki API error in {method}: {message}")
        if not isinstance(payload, dict) or "result" not in payload:
            raise ConnectorValidationError(f"EVA Wiki returned an unexpected response for {method}")
        return payload["result"]

    def _validate_config(self, *, require_project_id: bool = True) -> None:
        if not str(self.credentials.get("eva_api_token") or "").strip():
            raise ConnectorMissingCredentialError("EVA Wiki")
        for field_name, value in (("api_base_url", self.api_base_url), ("web_base_url", self.web_base_url)):
            parsed = urlparse(value)
            if parsed.scheme not in {"http", "https"} or not parsed.netloc or parsed.username or parsed.password:
                raise ConnectorValidationError(f"{field_name} must be a valid HTTP or HTTPS URL without embedded credentials")
        if require_project_id and not self.project_id:
            raise ConnectorValidationError("project_id is required so EVA content is scoped to one project")
        if not 1 <= self.batch_size <= self.MAX_BATCH_SIZE:
            raise ConnectorValidationError(f"batch_size must be between 1 and {self.MAX_BATCH_SIZE}")
        for field_name, value in (("attachment_size_limit", self.attachment_size_limit), ("page_size_limit", self.page_size_limit)):
            if not 1 <= value <= self.MAX_CONTENT_SIZE_LIMIT:
                raise ConnectorValidationError(f"{field_name} must be between 1 and {self.MAX_CONTENT_SIZE_LIMIT} bytes")
        if not 0 <= self.retry_count <= 10:
            raise ConnectorValidationError("retry_count must be between 0 and 10")

    @staticmethod
    def _raise_for_status(response: requests.Response, operation: str) -> None:
        if response.status_code in {401, 403}:
            raise InsufficientPermissionsError(f"EVA Wiki rejected credentials while trying to {operation}")
        if 300 <= response.status_code < 400:
            raise ConnectorValidationError(f"EVA Wiki redirect was refused while trying to {operation}")
        try:
            response.raise_for_status()
        except requests.RequestException as exc:
            raise ConnectorValidationError(f"EVA Wiki request failed while trying to {operation}: HTTP {response.status_code}") from exc

    @staticmethod
    def _encode_filter(filters: list[list[Any]]) -> str:
        return json.dumps(filters, ensure_ascii=False, separators=(",", ":"))

    @staticmethod
    def _timestamp_to_iso(timestamp: SecondsSinceUnixEpoch) -> str:
        return datetime.fromtimestamp(timestamp, tz=timezone.utc).isoformat()

    @staticmethod
    def _parse_datetime(value: Any) -> datetime:
        if not value:
            return datetime.fromtimestamp(0, tz=timezone.utc)
        try:
            parsed = datetime.fromisoformat(str(value).replace("Z", "+00:00"))
        except (TypeError, ValueError) as exc:
            raise ConnectorValidationError(f"EVA Wiki returned an invalid datetime: {value!s}") from exc
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone(timezone.utc)

    @staticmethod
    def _coerce_int(value: Any) -> int:
        try:
            return int(value)
        except (TypeError, ValueError):
            return 0

    @staticmethod
    def _as_int(value: Any) -> int:
        try:
            return int(value or 0)
        except (TypeError, ValueError):
            return 0

    @staticmethod
    def _as_bool(value: Any) -> bool:
        if isinstance(value, str):
            return value.strip().lower() in {"1", "true", "yes", "on"}
        return bool(value)

    @classmethod
    def _attachment_extension(cls, file_name: str, file_type: Any) -> str | None:
        suffix = Path(file_name).suffix.lower()
        if not suffix:
            raw_file_type = str(file_type or "").strip().lower()
            if "/" in raw_file_type:
                suffix = mimetypes.guess_extension(raw_file_type, strict=False) or ""
            elif raw_file_type:
                suffix = raw_file_type if raw_file_type.startswith(".") else f".{raw_file_type}"
        if suffix == ".jpe":
            suffix = ".jpg"
        return suffix if suffix in cls._SUPPORTED_ATTACHMENT_EXTENSIONS else None

    @classmethod
    def _bounded_semantic_identifier(cls, value: str, entity_id: str) -> str:
        cleaned = cls._clean_path_component(value) or entity_id
        if len(cleaned) <= cls.MAX_SEMANTIC_IDENTIFIER_LENGTH:
            return cleaned
        suffix = f" [{hashlib.sha256(entity_id.encode('utf-8')).hexdigest()[:8]}]"
        return cleaned[: cls.MAX_SEMANTIC_IDENTIFIER_LENGTH - len(suffix)].rstrip() + suffix

    @staticmethod
    def _clean_path_component(value: Any) -> str:
        return " ".join(str(value or "").replace("\x00", "").replace("/", "-").replace("\\", "-").split())


class EvaWikiMutationClient:
    """Narrow EVA client that exposes only document mutation operations."""

    def __init__(
        self,
        api_base_url: str,
        project_id: str,
        *,
        verify_ssl: bool = True,
        retry_count: int = EvaWikiConnector.DEFAULT_RETRY_COUNT,
    ) -> None:
        self.__transport = EvaWikiConnector(
            api_base_url=api_base_url,
            project_id=project_id,
            include_attachments=False,
            verify_ssl=verify_ssl,
            retry_count=retry_count,
        )

    def load_credentials(self, credentials: dict[str, Any]) -> None:
        self.__transport.load_credentials(credentials)

    def update_document_draft(self, document_id: str, html: str, name: str | None = None) -> Any:
        """Replace EVA's unpublished draft and, when supplied, its page name."""

        self.__transport._validate_config()
        normalized_id = str(document_id or "").strip()
        if not normalized_id:
            raise ConnectorValidationError("EVA Wiki document_id is required")
        call_kwargs = {"text_draft": str(html)}
        if name is not None:
            normalized_name = str(name).strip()
            if not normalized_name:
                raise ConnectorValidationError("EVA Wiki document name is required")
            call_kwargs["name"] = normalized_name
        return self.__transport._rpc("CmfDocument.update", {}, args=[normalized_id], call_kwargs=call_kwargs)

    def publish_document(self, document_id: str) -> Any:
        """Publish the current EVA draft for a page."""

        self.__transport._validate_config()
        normalized_id = str(document_id or "").strip()
        if not normalized_id:
            raise ConnectorValidationError("EVA Wiki document_id is required")
        return self.__transport._rpc("CmfDocument.do_publish", {}, args=[normalized_id], call_kwargs={})
