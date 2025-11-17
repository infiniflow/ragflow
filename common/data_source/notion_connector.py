import logging
from collections.abc import Generator
from typing import Any, Optional
from retry import retry

from common.data_source.config import (
    INDEX_BATCH_SIZE,
    DocumentSource, NOTION_CONNECTOR_DISABLE_RECURSIVE_PAGE_LOOKUP
)
from common.data_source.interfaces import (
    LoadConnector,
    PollConnector,
    SecondsSinceUnixEpoch
)
from common.data_source.models import (
    Document,
    TextSection, GenerateDocumentsOutput
)
from common.data_source.exceptions import (
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError, ConnectorMissingCredentialError
)
from common.data_source.models import (
    NotionPage,
    NotionBlock,
    NotionSearchResponse
)
from common.data_source.utils import (
    rl_requests,
    batch_generator,
    fetch_notion_data,
    properties_to_str,
    filter_pages_by_time, datetime_from_string
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

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_child_blocks(
        self, block_id: str, cursor: Optional[str] = None
    ) -> dict[str, Any] | None:
        """Fetch all child blocks via the Notion API."""
        logging.debug(f"Fetching children of block with ID '{block_id}'")
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
            if hasattr(e, 'response') and e.response.status_code == 404:
                logging.error(
                    f"Unable to access block with ID '{block_id}'. "
                    f"This is likely due to the block not being shared with the integration."
                )
                return None
            else:
                logging.exception(f"Error fetching blocks: {e}")
                raise

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_page(self, page_id: str) -> NotionPage:
        """Fetch a page from its ID via the Notion API."""
        logging.debug(f"Fetching page for ID '{page_id}'")
        page_url = f"https://api.notion.com/v1/pages/{page_id}"

        try:
            data = fetch_notion_data(page_url, self.headers, "GET")
            return NotionPage(**data)
        except Exception as e:
            logging.warning(f"Failed to fetch page, trying database for ID '{page_id}': {e}")
            return self._fetch_database_as_page(page_id)

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_database_as_page(self, database_id: str) -> NotionPage:
        """Attempt to fetch a database as a page."""
        logging.debug(f"Fetching database for ID '{database_id}' as a page")
        database_url = f"https://api.notion.com/v1/databases/{database_id}"

        data = fetch_notion_data(database_url, self.headers, "GET")
        database_name = data.get("title")
        database_name = (
            database_name[0].get("text", {}).get("content") if database_name else None
        )

        return NotionPage(**data, database_name=database_name)

    @retry(tries=3, delay=1, backoff=2)
    def _fetch_database(
        self, database_id: str, cursor: Optional[str] = None
    ) -> dict[str, Any]:
        """Fetch a database from its ID via the Notion API."""
        logging.debug(f"Fetching database for ID '{database_id}'")
        block_url = f"https://api.notion.com/v1/databases/{database_id}/query"
        body = {"start_cursor": cursor} if cursor else None

        try:
            data = fetch_notion_data(block_url, self.headers, "POST", body)
            return data
        except Exception as e:
            if hasattr(e, 'response') and e.response.status_code in [404, 400]:
                logging.error(
                    f"Unable to access database with ID '{database_id}'. "
                    f"This is likely due to the database not being shared with the integration."
                )
                return {"results": [], "next_cursor": None}
            raise

    def _read_pages_from_database(
        self, database_id: str
    ) -> tuple[list[NotionBlock], list[str]]:
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
                        logging.debug(f"Found page with ID '{obj_id}' in database '{database_id}'")
                        result_pages.append(result["id"])
                    elif obj_type == "database":
                        logging.debug(f"Found database with ID '{obj_id}' in database '{database_id}'")
                        _, child_pages = self._read_pages_from_database(obj_id)
                        result_pages.extend(child_pages)

            if data["next_cursor"] is None:
                break

            cursor = data["next_cursor"]

        return result_blocks, result_pages

    def _read_blocks(self, base_block_id: str) -> tuple[list[NotionBlock], list[str]]:
        """Reads all child blocks for the specified block, returns blocks and child page ids."""
        result_blocks: list[NotionBlock] = []
        child_pages: list[str] = []
        cursor = None

        while True:
            data = self._fetch_child_blocks(base_block_id, cursor)

            if data is None:
                return result_blocks, child_pages

            for result in data["results"]:
                logging.debug(f"Found child block for block with ID '{base_block_id}': {result}")
                result_block_id = result["id"]
                result_type = result["type"]
                result_obj = result[result_type]

                if result_type in ["ai_block", "unsupported", "external_object_instance_page"]:
                    logging.warning(f"Skipping unsupported block type '{result_type}'")
                    continue

                cur_result_text_arr = []
                if "rich_text" in result_obj:
                    for rich_text in result_obj["rich_text"]:
                        if "text" in rich_text:
                            text = rich_text["text"]["content"]
                            cur_result_text_arr.append(text)

                if result["has_children"]:
                    if result_type == "child_page":
                        child_pages.append(result_block_id)
                    else:
                        logging.debug(f"Entering sub-block: {result_block_id}")
                        subblocks, subblock_child_pages = self._read_blocks(result_block_id)
                        logging.debug(f"Finished sub-block: {result_block_id}")
                        result_blocks.extend(subblocks)
                        child_pages.extend(subblock_child_pages)

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

        return result_blocks, child_pages

    def _read_page_title(self, page: NotionPage) -> Optional[str]:
        """Extracts the title from a Notion page."""
        if hasattr(page, "database_name") and page.database_name:
            return page.database_name

        for _, prop in page.properties.items():
            if prop["type"] == "title" and len(prop["title"]) > 0:
                page_title = " ".join([t["plain_text"] for t in prop["title"]]).strip()
                return page_title

        return None

    def _read_pages(
        self, pages: list[NotionPage]
    ) -> Generator[Document, None, None]:
        """Reads pages for rich text content and generates Documents."""
        all_child_page_ids: list[str] = []

        for page in pages:
            if isinstance(page, dict):
                page = NotionPage(**page)
            if page.id in self.indexed_pages:
                logging.debug(f"Already indexed page with ID '{page.id}'. Skipping.")
                continue

            logging.info(f"Reading page with ID '{page.id}', with url {page.url}")
            page_blocks, child_page_ids = self._read_blocks(page.id)
            all_child_page_ids.extend(child_page_ids)
            self.indexed_pages.add(page.id)

            raw_page_title = self._read_page_title(page)
            page_title = raw_page_title or f"Untitled Page with ID {page.id}"

            if not page_blocks:
                if not raw_page_title:
                    logging.warning(f"No blocks OR title found for page with ID '{page.id}'. Skipping.")
                    continue

                text = page_title
                if page.properties:
                    text += "\n\n" + "\n".join(
                        [f"{key}: {value}" for key, value in page.properties.items()]
                    )
                sections = [TextSection(link=page.url, text=text)]
            else:
                sections = [
                    TextSection(
                        link=f"{page.url}#{block.id.replace('-', '')}",
                        text=block.prefix + block.text,
                    )
                    for block in page_blocks
                ]

            blob = ("\n".join([sec.text for sec in sections])).encode("utf-8")
            yield Document(
                id=page.id,
                blob=blob,
                source=DocumentSource.NOTION,
                semantic_identifier=page_title,
                extension=".txt",
                size_bytes=len(blob),
                doc_updated_at=datetime_from_string(page.last_edited_time)
            )

        if self.recursive_index_enabled and all_child_page_ids:
            for child_page_batch_ids in batch_generator(all_child_page_ids, INDEX_BATCH_SIZE):
                child_page_batch = [
                    self._fetch_page(page_id)
                    for page_id in child_page_batch_ids
                    if page_id not in self.indexed_pages
                ]
                yield from self._read_pages(child_page_batch)

    @retry(tries=3, delay=1, backoff=2)
    def _search_notion(self, query_dict: dict[str, Any]) -> NotionSearchResponse:
        """Search for pages from a Notion database."""
        logging.debug(f"Searching for pages in Notion with query_dict: {query_dict}")
        data = fetch_notion_data("https://api.notion.com/v1/search", self.headers, "POST", query_dict)
        return NotionSearchResponse(**data)

    def _recursive_load(self) -> Generator[list[Document], None, None]:
        """Recursively load pages starting from root page ID."""
        if self.root_page_id is None or not self.recursive_index_enabled:
            raise RuntimeError("Recursive page lookup is not enabled")

        logging.info(f"Recursively loading pages from Notion based on root page with ID: {self.root_page_id}")
        pages = [self._fetch_page(page_id=self.root_page_id)]
        yield from batch_generator(self._read_pages(pages), self.batch_size)

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Applies integration token to headers."""
        self.headers["Authorization"] = f'Bearer {credentials["notion_integration_token"]}'
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

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        """Poll Notion for updated pages within a time period."""
        if self.recursive_index_enabled and self.root_page_id:
            yield from self._recursive_load()
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
                yield from batch_generator(self._read_pages(pages), self.batch_size)
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
