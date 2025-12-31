import logging
import time
from datetime import datetime
from datetime import timezone
from typing import Any

import requests
from pydantic import BaseModel

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError
from common.data_source.html_utils import parse_html_page_basic
from common.data_source.interfaces import GenerateDocumentsOutput, PollConnector
from common.data_source.models import Document, SecondsSinceUnixEpoch
from common.data_source.utils import process_in_batches, retry_builder, time_str_to_utc, rate_limit_builder

ENTITY_NAME_MAP = {1: "Forum", 3: "Article", 4: "Blog", 9: "Wiki"}


def _get_auth_header(api_key: str) -> dict[str, str]:
    return {"Rest-Api-Key": api_key}


@retry_builder()
@rate_limit_builder(max_calls=5, period=1)
def _rate_limited_request(
    endpoint: str, headers: dict, params: dict | None = None
) -> Any:
    # https://my.axerosolutions.com/spaces/5/communifire-documentation/wiki/view/370/rest-api
    return requests.get(endpoint, headers=headers, params=params)


# https://my.axerosolutions.com/spaces/5/communifire-documentation/wiki/view/595/rest-api-get-content-list
def _get_entities(
    entity_type: int,
    api_key: str,
    axero_base_url: str,
    start: datetime,
    end: datetime,
    space_id: str | None = None,
) -> list[dict]:
    endpoint = axero_base_url + "api/content/list"
    page_num = 1
    pages_fetched = 0
    pages_to_return = []
    break_out = False
    while True:
        params = {
            "EntityType": str(entity_type),
            "SortColumn": "DateUpdated",
            "SortOrder": "1",  # descending
            "StartPage": str(page_num),
        }

        if space_id is not None:
            params["SpaceID"] = space_id

        res = _rate_limited_request(
            endpoint, headers=_get_auth_header(api_key), params=params
        )
        res.raise_for_status()

        # Axero limitations:
        # No next page token, can paginate but things may have changed
        # for example, a doc that hasn't been read in by Onyx is updated and is now front of the list
        # due to this limitation and the fact that Axero has no rate limiting but API calls can cause
        # increased latency for the team, we have to just fetch all the pages quickly to reduce the
        # chance of missing a document due to an update (it will still get updated next pass)
        # Assumes the volume of data isn't too big to store in memory (probably fine)
        data = res.json()
        total_records = data["TotalRecords"]
        contents = data["ResponseData"]
        pages_fetched += len(contents)
        logging.debug(f"Fetched {pages_fetched} {ENTITY_NAME_MAP[entity_type]}")

        for page in contents:
            update_time = time_str_to_utc(page["DateUpdated"])

            if update_time > end:
                continue

            if update_time < start:
                break_out = True
                break

            pages_to_return.append(page)

        if pages_fetched >= total_records:
            break

        page_num += 1

        if break_out:
            break

    return pages_to_return


def _get_obj_by_id(obj_id: int, api_key: str, axero_base_url: str) -> dict:
    endpoint = axero_base_url + f"api/content/{obj_id}"
    res = _rate_limited_request(endpoint, headers=_get_auth_header(api_key))
    res.raise_for_status()

    return res.json()


class AxeroForum(BaseModel):
    doc_id: str
    title: str
    link: str
    initial_content: str
    responses: list[str]
    last_update: datetime


def _map_post_to_parent(
    posts: dict,
    api_key: str,
    axero_base_url: str,
) -> list[AxeroForum]:
    """Cannot handle in batches since the posts aren't ordered or structured in any way
    may need to map any number of them to the initial post"""
    epoch_str = "1970-01-01T00:00:00.000"
    post_map: dict[int, AxeroForum] = {}

    for ind, post in enumerate(posts):
        if (ind + 1) % 25 == 0:
            logging.debug(f"Processed {ind + 1} posts or responses")

        post_time = time_str_to_utc(
            post.get("DateUpdated") or post.get("DateCreated") or epoch_str
        )
        p_id = post.get("ParentContentID")
        if p_id in post_map:
            axero_forum = post_map[p_id]
            axero_forum.responses.insert(0, post.get("ContentSummary"))
            axero_forum.last_update = max(axero_forum.last_update, post_time)
        else:
            initial_post_d = _get_obj_by_id(p_id, api_key, axero_base_url)[
                "ResponseData"
            ]
            initial_post_time = time_str_to_utc(
                initial_post_d.get("DateUpdated")
                or initial_post_d.get("DateCreated")
                or epoch_str
            )
            post_map[p_id] = AxeroForum(
                doc_id="AXERO_" + str(initial_post_d.get("ContentID")),
                title=initial_post_d.get("ContentTitle"),
                link=initial_post_d.get("ContentURL"),
                initial_content=initial_post_d.get("ContentSummary"),
                responses=[post.get("ContentSummary")],
                last_update=max(post_time, initial_post_time),
            )

    return list(post_map.values())


def _get_forums(
    api_key: str,
    axero_base_url: str,
    space_id: str | None = None,
) -> list[dict]:
    endpoint = axero_base_url + "api/content/list"
    page_num = 1
    pages_fetched = 0
    pages_to_return = []
    break_out = False

    while True:
        params = {
            "EntityType": "54",
            "SortColumn": "DateUpdated",
            "SortOrder": "1",  # descending
            "StartPage": str(page_num),
        }

        if space_id is not None:
            params["SpaceID"] = space_id

        res = _rate_limited_request(
            endpoint, headers=_get_auth_header(api_key), params=params
        )
        res.raise_for_status()

        data = res.json()
        total_records = data["TotalRecords"]
        contents = data["ResponseData"]
        pages_fetched += len(contents)
        logging.debug(f"Fetched {pages_fetched} forums")

        for page in contents:
            pages_to_return.append(page)

        if pages_fetched >= total_records:
            break

        page_num += 1

        if break_out:
            break

    return pages_to_return


def _translate_forum_to_doc(af: AxeroForum) -> Document:
    doc = Document(
        id=af.doc_id,
        source=DocumentSource.AXERO,
        semantic_identifier=af.title,
        doc_updated_at=af.last_update,
        metadata={},
    )

    return doc


def _translate_content_to_doc(content: dict) -> Document:
    page_text = ""
    summary = content.get("ContentSummary")
    body = content.get("ContentBody")
    if summary:
        page_text += f"{summary}\n"

    if body:
        content_parsed = parse_html_page_basic(body)
        page_text += content_parsed

    doc = Document(
        id="AXERO_" + str(content["ContentID"]),
        source=DocumentSource.AXERO,
        semantic_identifier=content["ContentTitle"],
        doc_updated_at=time_str_to_utc(content["DateUpdated"]),
        metadata={"space": content["SpaceName"]},
    )

    return doc


class AxeroConnector(PollConnector):
    def __init__(
        self,
        # Strings of the integer ids of the spaces
        spaces: list[str] | None = None,
        include_article: bool = True,
        include_blog: bool = True,
        include_wiki: bool = True,
        include_forum: bool = True,
        batch_size: int = INDEX_BATCH_SIZE,
    ) -> None:
        self.include_article = include_article
        self.include_blog = include_blog
        self.include_wiki = include_wiki
        self.include_forum = include_forum
        self.batch_size = batch_size
        self.space_ids = spaces
        self.axero_key = None
        self.base_url = None

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.axero_key = credentials["axero_api_token"]
        # As the API key specifically applies to a particular deployment, this is
        # included as part of the credential
        base_url = credentials["base_url"]
        if not base_url.endswith("/"):
            base_url += "/"
        self.base_url = base_url
        return None

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> GenerateDocumentsOutput:
        if not self.axero_key or not self.base_url:
            raise ConnectorMissingCredentialError("Axero")

        start_datetime = datetime.fromtimestamp(start, tz=timezone.utc)
        end_datetime = datetime.fromtimestamp(end, tz=timezone.utc)

        entity_types = []
        if self.include_article:
            entity_types.append(3)
        if self.include_blog:
            entity_types.append(4)
        if self.include_wiki:
            entity_types.append(9)

        iterable_space_ids = self.space_ids if self.space_ids else [None]

        for space_id in iterable_space_ids:
            for entity in entity_types:
                axero_obj = _get_entities(
                    entity_type=entity,
                    api_key=self.axero_key,
                    axero_base_url=self.base_url,
                    start=start_datetime,
                    end=end_datetime,
                    space_id=space_id,
                )
                yield from process_in_batches(
                    objects=axero_obj,
                    process_function=_translate_content_to_doc,
                    batch_size=self.batch_size,
                )

            if self.include_forum:
                forums_posts = _get_forums(
                    api_key=self.axero_key,
                    axero_base_url=self.base_url,
                    space_id=space_id,
                )

                all_axero_forums = _map_post_to_parent(
                    posts=forums_posts,
                    api_key=self.axero_key,
                    axero_base_url=self.base_url,
                )

                filtered_forums = [
                    f
                    for f in all_axero_forums
                    if f.last_update >= start_datetime and f.last_update <= end_datetime
                ]

                yield from process_in_batches(
                    objects=filtered_forums,
                    process_function=_translate_forum_to_doc,
                    batch_size=self.batch_size,
                )


if __name__ == "__main__":
    import os

    connector = AxeroConnector()
    connector.load_credentials(
        {
            "axero_api_token": os.environ["AXERO_API_TOKEN"],
            "base_url": os.environ["AXERO_BASE_URL"],
        }
    )
    current = time.time()

    one_year_ago = current - 24 * 60 * 60 * 360
    latest_docs = connector.poll_source(one_year_ago, current)

    print(next(latest_docs))