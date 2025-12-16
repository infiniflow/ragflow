import html
import logging
from collections.abc import Generator
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional
from urllib.parse import urlparse

from retry import retry

from common.data_source.config import (
    INDEX_BATCH_SIZE,
    NOTION_CONNECTOR_DISABLE_RECURSIVE_PAGE_LOOKUP,
    DocumentSource,
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError,
)
from common.data_source.interfaces import (
    LoadConnector,
    PollConnector,
    SecondsSinceUnixEpoch,
)
from common.data_source.models import (
    Document,
    GenerateDocumentsOutput,
    NotionBlock,
    NotionPage,
    NotionSearchResponse,
    TextSection,
)
from common.data_source.utils import (
    batch_generator,
    datetime_from_string,
    fetch_notion_data,
    filter_pages_by_time,
    properties_to_str,
    rl_requests,
)


class NotionConnector(LoadConnector, PollConnector):
    """Notion Page connector that reads all Notion pages this integration has access to.

    Arguments:
        batch_size (int): Number of objects to index in a batch
        recursive_index_enabled (bool): Whether to recursively index child pages
        root_page_id (str | None): Specific root page ID to start indexing from
    """

    def __init__(
        self,
        batch_size: int = INDEX_BATCH_SIZE,
        recursive_index_enabled: bool = not NOTION_CONNECTOR_DISABLE_RECURSIVE_PAGE_LOOKUP,
        root_page_id: Optional[str] = None,
    ) -> None:
        self.batch_size = batch_size
        self.headers = {
            "Content-Type": "application/json",
            "Notion-Version": "2022-06-28",
        }
        self.indexed_pages: set[str] = set()
        self.root_page_id = root_page_id
        self.recursive_index_enabled = recursive_index_enabled or bool(root_page_id)
        self.page_path_cache: dict[str, str] = {}

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_child_blocks(self, block_id: str, cursor: Optional[str] = None) -> dict[str, Any] | None:
        """Fetch all child blocks via the Notion API."""
        logging.debug(f"[Notion]: Fetching children of block with ID {block_id}")
        block_url = f"https://api.notion.com/v1/blocks/{block_id}/children"
        query_params = {"start_cursor": cursor} if cursor else None

        try:
            response = rl_requests.get(
                block_url,
                headers=self.headers,
                params=query_params,
                timeout=30,
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            if hasattr(e, "response") and e.response.status_code == 404:
                logging.error(f"[Notion]: Unable to access block with ID {block_id}. This is likely due to the block not being shared with the integration.")
                return None
            else:
                logging.exception(f"[Notion]: Error fetching blocks: {e}")
                raise

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_page(self, page_id: str) -> NotionPage:
        """Fetch a page from its ID via the Notion API."""
        logging.debug(f"[Notion]: Fetching page for ID {page_id}")
        page_url = f"https://api.notion.com/v1/pages/{page_id}"

        try:
            data = fetch_notion_data(page_url, self.headers, "GET")
            return NotionPage(**data)
        except Exception as e:
            logging.warning(f"[Notion]: Failed to fetch page, trying database for ID {page_id}: {e}")
            return self._fetch_database_as_page(page_id)

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_database_as_page(self, database_id: str) -> NotionPage:
        """Attempt to fetch a database as a page."""
        logging.debug(f"[Notion]: Fetching database for ID {database_id} as a page")
        database_url = f"https://api.notion.com/v1/databases/{database_id}"

        data = fetch_notion_data(database_url, self.headers, "GET")
        database_name = data.get("title")
        database_name = database_name[0].get("text", {}).get("content") if database_name else None

        return NotionPage(**data, database_name=database_name)

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_database(self, database_id: str, cursor: Optional[str] = None) -> dict[str, Any]:
        """Fetch a database from its ID via the Notion API."""
        logging.debug(f"[Notion]: Fetching database for ID {database_id}")
        block_url = f"https://api.notion.com/v1/databases/{database_id}/query"
        body = {"start_cursor": cursor} if cursor else None

        try:
            data = fetch_notion_data(block_url, self.headers, "POST", body)
            return data
        except Exception as e:
            if hasattr(e, "response") and e.response.status_code in [404, 400]:
                logging.error(f"[Notion]: Unable to access database with ID {database_id}. This is likely due to the database not being shared with the integration.")
                return {"results": [], "next_cursor": None}
            raise

    def _read_pages_from_database(self, database_id: str) -> tuple[list[NotionBlock], list[str]]:
        """Returns a list of top level blocks and all page IDs in the database."""
        result_blocks: list[NotionBlock] = []
        result_pages: list[str] = []
        cursor = None

        while True:
            data = self._fetch_database(database_id, cursor)

            for result in data["results"]:
                obj_id = result["id"]
                obj_type = result["object"]
                text = properties_to_str(result.get("properties", {}))

                if text:
                    result_blocks.append(NotionBlock(id=obj_id, text=text, prefix="\n"))

                if self.recursive_index_enabled:
                    if obj_type == "page":
                        logging.debug(f"[Notion]: Found page with ID {obj_id} in database {database_id}")
                        result_pages.append(result["id"])
                    elif obj_type == "database":
                        logging.debug(f"[Notion]: Found database with ID {obj_id} in database {database_id}")
                        _, child_pages = self._read_pages_from_database(obj_id)
                        result_pages.extend(child_pages)

            if data["next_cursor"] is None:
                break

            cursor = data["next_cursor"]

        return result_blocks, result_pages

    def _extract_rich_text(self, rich_text_array: list[dict[str, Any]]) -> str:
        collected_text: list[str] = []
        for rich_text in rich_text_array:
            content = ""
            r_type = rich_text.get("type")

            if r_type == "equation":
                expr = rich_text.get("equation", {}).get("expression")
                if expr:
                    content = expr
            elif r_type == "mention":
                mention = rich_text.get("mention", {}) or {}
                mention_type = mention.get("type")
                mention_value = mention.get(mention_type, {}) if mention_type else {}
                if mention_type == "date":
                    start = mention_value.get("start")
                    end = mention_value.get("end")
                    if start and end:
                        content = f"{start} - {end}"
                    elif start:
                        content = start
                elif mention_type in {"page", "database"}:
                    content = mention_value.get("id", rich_text.get("plain_text", ""))
                elif mention_type == "link_preview":
                    content = mention_value.get("url", rich_text.get("plain_text", ""))
                else:
                    content = rich_text.get("plain_text", "") or str(mention_value)
            else:
                if rich_text.get("plain_text"):
                    content = rich_text["plain_text"]
                elif "text" in rich_text and rich_text["text"].get("content"):
                    content = rich_text["text"]["content"]

            href = rich_text.get("href")
            if content and href:
                content = f"{content} ({href})"

            if content:
                collected_text.append(content)

        return "".join(collected_text).strip()

    def _build_table_html(self, table_block_id: str) -> str | None:
        rows: list[str] = []
        cursor = None
        while True:
            data = self._fetch_child_blocks(table_block_id, cursor)
            if data is None:
                break

            for result in data["results"]:
                if result.get("type") != "table_row":
                    continue
                cells_html: list[str] = []
                for cell in result["table_row"].get("cells", []):
                    cell_text = self._extract_rich_text(cell)
                    cell_html = html.escape(cell_text) if cell_text else ""
                    cells_html.append(f"<td>{cell_html}</td>")
                rows.append(f"<tr>{''.join(cells_html)}</tr>")

            if data.get("next_cursor") is None:
                break
            cursor = data["next_cursor"]

        if not rows:
            return None
        return "<table>\n" + "\n".join(rows) + "\n</table>"

    def _download_file(self, url: str) -> bytes | None:
        try:
            response = rl_requests.get(url, timeout=60)
            response.raise_for_status()
            return response.content
        except Exception as exc:
            logging.warning(f"[Notion]: Failed to download Notion file from {url}: {exc}")
            return None

    def _append_block_id_to_name(self, name: str, block_id: Optional[str]) -> str:
        """Append the Notion block ID to the filename while keeping the extension."""
        if not block_id:
            return name

        path = Path(name)
        stem = path.stem or name
        suffix = path.suffix

        if not stem:
            return name

        return f"{stem}_{block_id}{suffix}" if suffix else f"{stem}_{block_id}"

    def _extract_file_metadata(self, result_obj: dict[str, Any], block_id: str) -> tuple[str | None, str, str | None]:
        file_source_type = result_obj.get("type")
        file_source = result_obj.get(file_source_type, {}) if file_source_type else {}
        url = file_source.get("url")

        name = result_obj.get("name") or file_source.get("name")
        if url and not name:
            parsed_name = Path(urlparse(url).path).name
            name = parsed_name or f"notion_file_{block_id}"
        elif not name:
            name = f"notion_file_{block_id}"

        name = self._append_block_id_to_name(name, block_id)

        caption = self._extract_rich_text(result_obj.get("caption", [])) if "caption" in result_obj else None

        return url, name, caption

    def _build_attachment_document(
        self,
        block_id: str,
        url: str,
        name: str,
        caption: Optional[str],
        page_last_edited_time: Optional[str],
        page_path: Optional[str],
    ) -> Document | None:
        file_bytes = self._download_file(url)
        if file_bytes is None:
            return None

        extension = Path(name).suffix or Path(urlparse(url).path).suffix or ".bin"
        if extension and not extension.startswith("."):
            extension = f".{extension}"
        if not extension:
            extension = ".bin"

        updated_at = datetime_from_string(page_last_edited_time) if page_last_edited_time else datetime.now(timezone.utc)
        base_identifier = name or caption or (f"Notion file {block_id}" if block_id else "Notion file")
        semantic_identifier = f"{page_path} / {base_identifier}" if page_path else base_identifier

        return Document(
            id=block_id,
            blob=file_bytes,
            source=DocumentSource.NOTION,
            semantic_identifier=semantic_identifier,
            extension=extension,
            size_bytes=len(file_bytes),
            doc_updated_at=updated_at,
        )

    def _read_blocks(self, base_block_id: str, page_last_edited_time: Optional[str] = None, page_path: Optional[str] = None) -> tuple[list[NotionBlock], list[str], list[Document]]:
        result_blocks: list[NotionBlock] = []
        child_pages: list[str] = []
        attachments: list[Document] = []
        cursor = None

        while True:
            data = self._fetch_child_blocks(base_block_id, cursor)

            if data is None:
                return result_blocks, child_pages, attachments

            for result in data["results"]:
                logging.debug(f"[Notion]: Found child block for block with ID {base_block_id}: {result}")
                result_block_id = result["id"]
                result_type = result["type"]
                result_obj = result[result_type]

                if result_type in ["ai_block", "unsupported", "external_object_instance_page"]:
                    logging.warning(f"[Notion]: Skipping unsupported block type {result_type}")
                    continue

                if result_type == "table":
                    table_html = self._build_table_html(result_block_id)
                    if table_html:
                        result_blocks.append(
                            NotionBlock(
                                id=result_block_id,
                                text=table_html,
                                prefix="\n\n",
                            )
                        )
                    continue

                if result_type == "equation":
                    expr = result_obj.get("expression")
                    if expr:
                        result_blocks.append(
                            NotionBlock(
                                id=result_block_id,
                                text=expr,
                                prefix="\n",
                            )
                        )
                    continue

                cur_result_text_arr = []
                if "rich_text" in result_obj:
                    text = self._extract_rich_text(result_obj["rich_text"])
                    if text:
                        cur_result_text_arr.append(text)

                if result_type == "bulleted_list_item":
                    if cur_result_text_arr:
                        cur_result_text_arr[0] = f"- {cur_result_text_arr[0]}"
                    else:
                        cur_result_text_arr = ["- "]

                if result_type == "numbered_list_item":
                    if cur_result_text_arr:
                        cur_result_text_arr[0] = f"1. {cur_result_text_arr[0]}"
                    else:
                        cur_result_text_arr = ["1. "]

                if result_type == "to_do":
                    checked = result_obj.get("checked")
                    checkbox_prefix = "[x]" if checked else "[ ]"
                    if cur_result_text_arr:
                        cur_result_text_arr = [f"{checkbox_prefix} {cur_result_text_arr[0]}"] + cur_result_text_arr[1:]
                    else:
                        cur_result_text_arr = [checkbox_prefix]

                if result_type in {"file", "image", "pdf", "video", "audio"}:
                    file_url, file_name, caption = self._extract_file_metadata(result_obj, result_block_id)
                    if file_url:
                        attachment_doc = self._build_attachment_document(
                            block_id=result_block_id,
                            url=file_url,
                            name=file_name,
                            caption=caption,
                            page_last_edited_time=page_last_edited_time,
                            page_path=page_path,
                        )
                        if attachment_doc:
                            attachments.append(attachment_doc)

                        attachment_label = file_name
                        if caption:
                            attachment_label = f"{file_name} ({caption})"
                        if attachment_label:
                            cur_result_text_arr.append(f"{result_type.capitalize()}: {attachment_label}")

                if result["has_children"]:
                    if result_type == "child_page":
                        child_pages.append(result_block_id)
                    else:
                        logging.debug(f"[Notion]: Entering sub-block: {result_block_id}")
                        subblocks, subblock_child_pages, subblock_attachments = self._read_blocks(result_block_id, page_last_edited_time, page_path)
                        logging.debug(f"[Notion]: Finished sub-block: {result_block_id}")
                        result_blocks.extend(subblocks)
                        child_pages.extend(subblock_child_pages)
                        attachments.extend(subblock_attachments)

                if result_type == "child_database":
                    inner_blocks, inner_child_pages = self._read_pages_from_database(result_block_id)
                    result_blocks.extend(inner_blocks)

                    if self.recursive_index_enabled:
                        child_pages.extend(inner_child_pages)

                if cur_result_text_arr:
                    new_block = NotionBlock(
                        id=result_block_id,
                        text="\n".join(cur_result_text_arr),
                        prefix="\n",
                    )
                    result_blocks.append(new_block)

            if data["next_cursor"] is None:
                break

            cursor = data["next_cursor"]

        return result_blocks, child_pages, attachments

    def _read_page_title(self, page: NotionPage) -> Optional[str]:
        """Extracts the title from a Notion page."""
        if hasattr(page, "database_name") and page.database_name:
            return page.database_name

        for _, prop in page.properties.items():
            if prop["type"] == "title" and len(prop["title"]) > 0:
                page_title = " ".join([t["plain_text"] for t in prop["title"]]).strip()
                return page_title

        return None

    def _build_page_path(self, page: NotionPage, visited: Optional[set[str]] = None) -> Optional[str]:
        """Construct a hierarchical path for a page based on its parent chain."""
        if page.id in self.page_path_cache:
            return self.page_path_cache[page.id]

        visited = visited or set()
        if page.id in visited:
            logging.warning(f"[Notion]: Detected cycle while building path for page {page.id}")
            return self._read_page_title(page)
        visited.add(page.id)

        current_title = self._read_page_title(page) or f"Untitled Page {page.id}"

        parent_info = getattr(page, "parent", None) or {}
        parent_type = parent_info.get("type")
        parent_id = parent_info.get(parent_type) if parent_type else None

        parent_path = None
        if parent_type in {"page_id", "database_id"} and isinstance(parent_id, str):
            try:
                parent_page = self._fetch_page(parent_id)
                parent_path = self._build_page_path(parent_page, visited)
            except Exception as exc:
                logging.warning(f"[Notion]: Failed to resolve parent {parent_id} for page {page.id}: {exc}")

        full_path = f"{parent_path} / {current_title}" if parent_path else current_title
        self.page_path_cache[page.id] = full_path
        return full_path

    def _read_pages(self, pages: list[NotionPage], start: SecondsSinceUnixEpoch | None = None, end: SecondsSinceUnixEpoch | None = None) -> Generator[Document, None, None]:
        """Reads pages for rich text content and generates Documents."""
        all_child_page_ids: list[str] = []

        for page in pages:
            if isinstance(page, dict):
                page = NotionPage(**page)
            if page.id in self.indexed_pages:
                logging.debug(f"[Notion]: Already indexed page with ID {page.id}. Skipping.")
                continue

            if start is not None and end is not None:
                page_ts = datetime_from_string(page.last_edited_time).timestamp()
                if not (page_ts > start and page_ts <= end):
                    logging.debug(f"[Notion]: Skipping page {page.id} outside polling window.")
                    continue

            logging.info(f"[Notion]: Reading page with ID {page.id}, with url {page.url}")
            page_path = self._build_page_path(page)
            page_blocks, child_page_ids, attachment_docs = self._read_blocks(page.id, page.last_edited_time, page_path)
            all_child_page_ids.extend(child_page_ids)
            self.indexed_pages.add(page.id)

            raw_page_title = self._read_page_title(page)
            page_title = raw_page_title or f"Untitled Page with ID {page.id}"

            # Append the page id to help disambiguate duplicate names
            base_identifier = page_path or page_title
            semantic_identifier = f"{base_identifier}_{page.id}" if base_identifier else page.id

            if not page_blocks:
                if not raw_page_title:
                    logging.warning(f"[Notion]: No blocks OR title found for page with ID {page.id}. Skipping.")
                    continue

                text = page_title
                if page.properties:
                    text += "\n\n" + "\n".join([f"{key}: {value}" for key, value in page.properties.items()])
                sections = [TextSection(link=page.url, text=text)]
            else:
                sections = [
                    TextSection(
                        link=f"{page.url}#{block.id.replace('-', '')}",
                        text=block.prefix + block.text,
                    )
                    for block in page_blocks
                ]

            joined_text = "\n".join(sec.text for sec in sections)
            blob = joined_text.encode("utf-8")
            yield Document(
                id=page.id, blob=blob, source=DocumentSource.NOTION, semantic_identifier=semantic_identifier, extension=".txt", size_bytes=len(blob), doc_updated_at=datetime_from_string(page.last_edited_time)
            )

            for attachment_doc in attachment_docs:
                yield attachment_doc

        if self.recursive_index_enabled and all_child_page_ids:
            for child_page_batch_ids in batch_generator(all_child_page_ids, INDEX_BATCH_SIZE):
                child_page_batch = [self._fetch_page(page_id) for page_id in child_page_batch_ids if page_id not in self.indexed_pages]
                yield from self._read_pages(child_page_batch, start, end)

    @retry(tries=3, delay=1, backoff=2)
    def _search_notion(self, query_dict: dict[str, Any]) -> NotionSearchResponse:
        """Search for pages from a Notion database."""
        logging.debug(f"[Notion]: Searching for pages in Notion with query_dict: {query_dict}")
        data = fetch_notion_data("https://api.notion.com/v1/search", self.headers, "POST", query_dict)
        return NotionSearchResponse(**data)

    def _recursive_load(self, start: SecondsSinceUnixEpoch | None = None, end: SecondsSinceUnixEpoch | None = None) -> Generator[list[Document], None, None]:
        """Recursively load pages starting from root page ID."""
        if self.root_page_id is None or not self.recursive_index_enabled:
            raise RuntimeError("Recursive page lookup is not enabled")

        logging.info(f"[Notion]: Recursively loading pages from Notion based on root page with ID: {self.root_page_id}")
        pages = [self._fetch_page(page_id=self.root_page_id)]
        yield from batch_generator(self._read_pages(pages, start, end), self.batch_size)

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Applies integration token to headers."""
        self.headers["Authorization"] = f"Bearer {credentials['notion_integration_token']}"
        return None

    def load_from_state(self) -> GenerateDocumentsOutput:
        """Loads all page data from a Notion workspace."""
        if self.recursive_index_enabled and self.root_page_id:
            yield from self._recursive_load()
            return

        query_dict = {
            "filter": {"property": "object", "value": "page"},
            "page_size": 100,
        }

        while True:
            db_res = self._search_notion(query_dict)
            pages = [NotionPage(**page) for page in db_res.results]
            yield from batch_generator(self._read_pages(pages), self.batch_size)

            if db_res.has_more:
                query_dict["start_cursor"] = db_res.next_cursor
            else:
                break

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> GenerateDocumentsOutput:
        """Poll Notion for updated pages within a time period."""
        if self.recursive_index_enabled and self.root_page_id:
            yield from self._recursive_load(start, end)
            return

        query_dict = {
            "page_size": 100,
            "sort": {"timestamp": "last_edited_time", "direction": "descending"},
            "filter": {"property": "object", "value": "page"},
        }

        while True:
            db_res = self._search_notion(query_dict)
            pages = filter_pages_by_time(db_res.results, start, end, "last_edited_time")

            if pages:
                yield from batch_generator(self._read_pages(pages, start, end), self.batch_size)
                if db_res.has_more:
                    query_dict["start_cursor"] = db_res.next_cursor
                else:
                    break
            else:
                break

    def validate_connector_settings(self) -> None:
        """Validate Notion connector settings and credentials."""
        if not self.headers.get("Authorization"):
            raise ConnectorMissingCredentialError("Notion credentials not loaded.")

        try:
            if self.root_page_id:
                response = rl_requests.get(
                    f"https://api.notion.com/v1/pages/{self.root_page_id}",
                    headers=self.headers,
                    timeout=30,
                )
            else:
                test_query = {"filter": {"property": "object", "value": "page"}, "page_size": 1}
                response = rl_requests.post(
                    "https://api.notion.com/v1/search",
                    headers=self.headers,
                    json=test_query,
                    timeout=30,
                )

            response.raise_for_status()

        except rl_requests.exceptions.HTTPError as http_err:
            status_code = http_err.response.status_code if http_err.response else None

            if status_code == 401:
                raise CredentialExpiredError("Notion credential appears to be invalid or expired (HTTP 401).")
            elif status_code == 403:
                raise InsufficientPermissionsError("Your Notion token does not have sufficient permissions (HTTP 403).")
            elif status_code == 404:
                raise ConnectorValidationError("Notion resource not found or not shared with the integration (HTTP 404).")
            elif status_code == 429:
                raise ConnectorValidationError("Validation failed due to Notion rate-limits being exceeded (HTTP 429).")
            else:
                raise UnexpectedValidationError(f"Unexpected Notion HTTP error (status={status_code}): {http_err}")

        except Exception as exc:
            raise UnexpectedValidationError(f"Unexpected error during Notion settings validation: {exc}")


if __name__ == "__main__":
    import os

    root_page_id = os.environ.get("NOTION_ROOT_PAGE_ID")
    connector = NotionConnector(root_page_id=root_page_id)
    connector.load_credentials({"notion_integration_token": os.environ.get("NOTION_INTEGRATION_TOKEN")})
    document_batches = connector.load_from_state()
    for doc_batch in document_batches:
        for doc in doc_batch:
            print(doc)