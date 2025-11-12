"""Discord connector"""

import asyncio
import logging
import os
from datetime import datetime, timezone
from typing import Any, AsyncIterable, Iterable

from discord import Client, MessageType
from discord.channel import TextChannel, Thread
from discord.flags import Intents
from discord.message import Message as DiscordMessage

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource
from common.data_source.exceptions import ConnectorMissingCredentialError
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch
from common.data_source.models import Document, GenerateDocumentsOutput, TextSection

_DISCORD_DOC_ID_PREFIX = "DISCORD_"
_SNIPPET_LENGTH = 30


def _convert_message_to_document(
    message: DiscordMessage,
    sections: list[TextSection],
) -> Document:
    """
    Convert a discord message to a document
    Sections are collected before calling this function because it relies on async
        calls to fetch the thread history if there is one
    """

    metadata: dict[str, str | list[str]] = {}
    semantic_substring = ""

    # Only messages from TextChannels will make it here but we have to check for it anyways
    if isinstance(message.channel, TextChannel) and (channel_name := message.channel.name):
        metadata["Channel"] = channel_name
        semantic_substring += f" in Channel: #{channel_name}"

    # If there is a thread, add more detail to the metadata, title, and semantic identifier
    if isinstance(message.channel, Thread):
        # Threads do have a title
        title = message.channel.name

        # Add more detail to the semantic identifier if available
        semantic_substring += f" in Thread: {title}"

    snippet: str = message.content[:_SNIPPET_LENGTH].rstrip() + "..." if len(message.content) > _SNIPPET_LENGTH else message.content

    semantic_identifier = f"{message.author.name} said{semantic_substring}: {snippet}"

    # fallback to created_at
    doc_updated_at = message.edited_at if message.edited_at else message.created_at
    if doc_updated_at and doc_updated_at.tzinfo is None:
        doc_updated_at = doc_updated_at.replace(tzinfo=timezone.utc)
    elif doc_updated_at:
        doc_updated_at = doc_updated_at.astimezone(timezone.utc)

    return Document(
        id=f"{_DISCORD_DOC_ID_PREFIX}{message.id}",
        source=DocumentSource.DISCORD,
        semantic_identifier=semantic_identifier,
        doc_updated_at=doc_updated_at,
        blob=message.content.encode("utf-8"),
        extension=".txt",
        size_bytes=len(message.content.encode("utf-8")),
    )


async def _fetch_filtered_channels(
    discord_client: Client,
    server_ids: list[int] | None,
    channel_names: list[str] | None,
) -> list[TextChannel]:
    filtered_channels: list[TextChannel] = []

    for channel in discord_client.get_all_channels():
        if not channel.permissions_for(channel.guild.me).read_message_history:
            continue
        if not isinstance(channel, TextChannel):
            continue
        if server_ids and len(server_ids) > 0 and channel.guild.id not in server_ids:
            continue
        if channel_names and channel.name not in channel_names:
            continue
        filtered_channels.append(channel)

    logging.info(f"Found {len(filtered_channels)} channels for the authenticated user")
    return filtered_channels


async def _fetch_documents_from_channel(
    channel: TextChannel,
    start_time: datetime | None,
    end_time: datetime | None,
) -> AsyncIterable[Document]:
    # Discord's epoch starts at 2015-01-01
    discord_epoch = datetime(2015, 1, 1, tzinfo=timezone.utc)
    if start_time and start_time < discord_epoch:
        start_time = discord_epoch

    # NOTE: limit=None is the correct way to fetch all messages and threads with pagination
    # The discord package erroneously uses limit for both pagination AND number of results
    # This causes the history and archived_threads methods to return 100 results even if there are more results within the filters
    # Pagination is handled automatically (100 results at a time) when limit=None

    async for channel_message in channel.history(
        limit=None,
        after=start_time,
        before=end_time,
    ):
        # Skip messages that are not the default type
        if channel_message.type != MessageType.default:
            continue

        sections: list[TextSection] = [
            TextSection(
                text=channel_message.content,
                link=channel_message.jump_url,
            )
        ]

        yield _convert_message_to_document(channel_message, sections)

    for active_thread in channel.threads:
        async for thread_message in active_thread.history(
            limit=None,
            after=start_time,
            before=end_time,
        ):
            # Skip messages that are not the default type
            if thread_message.type != MessageType.default:
                continue

            sections = [
                TextSection(
                    text=thread_message.content,
                    link=thread_message.jump_url,
                )
            ]

            yield _convert_message_to_document(thread_message, sections)

    async for archived_thread in channel.archived_threads(
        limit=None,
    ):
        async for thread_message in archived_thread.history(
            limit=None,
            after=start_time,
            before=end_time,
        ):
            # Skip messages that are not the default type
            if thread_message.type != MessageType.default:
                continue

            sections = [
                TextSection(
                    text=thread_message.content,
                    link=thread_message.jump_url,
                )
            ]

            yield _convert_message_to_document(thread_message, sections)


def _manage_async_retrieval(
    token: str,
    requested_start_date_string: str,
    channel_names: list[str],
    server_ids: list[int],
    start: datetime | None = None,
    end: datetime | None = None,
) -> Iterable[Document]:
    # parse requested_start_date_string to datetime
    pull_date: datetime | None = datetime.strptime(requested_start_date_string, "%Y-%m-%d").replace(tzinfo=timezone.utc) if requested_start_date_string else None

    # Set start_time to the later of start and pull_date, or whichever is provided
    start_time = max(filter(None, [start, pull_date])) if start or pull_date else None

    end_time: datetime | None = end
    proxy_url: str | None = os.environ.get("https_proxy") or os.environ.get("http_proxy")
    if proxy_url:
        logging.info(f"Using proxy for Discord: {proxy_url}")

    async def _async_fetch() -> AsyncIterable[Document]:
        intents = Intents.default()
        intents.message_content = True
        async with Client(intents=intents, proxy=proxy_url) as cli:
            asyncio.create_task(coro=cli.start(token))
            await cli.wait_until_ready()

            filtered_channels: list[TextChannel] = await _fetch_filtered_channels(
                discord_client=cli,
                server_ids=server_ids,
                channel_names=channel_names,
            )

            for channel in filtered_channels:
                async for doc in _fetch_documents_from_channel(
                    channel=channel,
                    start_time=start_time,
                    end_time=end_time,
                ):
                    print(doc)
                    yield doc

    def run_and_yield() -> Iterable[Document]:
        loop = asyncio.new_event_loop()
        try:
            # Get the async generator
            async_gen = _async_fetch()
            # Convert to AsyncIterator
            async_iter = async_gen.__aiter__()
            while True:
                try:
                    # Create a coroutine by calling anext with the async iterator
                    next_coro = anext(async_iter)
                    # Run the coroutine to get the next document
                    doc = loop.run_until_complete(next_coro)
                    yield doc
                except StopAsyncIteration:
                    break
        finally:
            loop.close()

    return run_and_yield()


class DiscordConnector(LoadConnector, PollConnector):
    """Discord connector for accessing Discord messages and channels"""

    def __init__(
        self,
        server_ids: list[str] = [],
        channel_names: list[str] = [],
        # YYYY-MM-DD
        start_date: str | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
    ):
        self.batch_size = batch_size
        self.channel_names: list[str] = channel_names if channel_names else []
        self.server_ids: list[int] = [int(server_id) for server_id in server_ids] if server_ids else []
        self._discord_bot_token: str | None = None
        self.requested_start_date_string: str = start_date or ""

    @property
    def discord_bot_token(self) -> str:
        if self._discord_bot_token is None:
            raise ConnectorMissingCredentialError("Discord")
        return self._discord_bot_token

    def _manage_doc_batching(
        self,
        start: datetime | None = None,
        end: datetime | None = None,
    ) -> GenerateDocumentsOutput:
        doc_batch = []
        def merge_batch():
            nonlocal doc_batch
            id = doc_batch[0].id
            min_updated_at = doc_batch[0].doc_updated_at
            max_updated_at = doc_batch[-1].doc_updated_at
            blob = b''
            size_bytes = 0
            for d in doc_batch:
                min_updated_at = min(min_updated_at, d.doc_updated_at)
                max_updated_at = max(max_updated_at, d.doc_updated_at)
                blob += b'\n\n' + d.blob
                size_bytes += d.size_bytes

            return Document(
                id=id,
                source=DocumentSource.DISCORD,
                semantic_identifier=f"{min_updated_at} -> {max_updated_at}",
                doc_updated_at=max_updated_at,
                blob=blob,
                extension=".txt",
                size_bytes=size_bytes,
            )

        for doc in _manage_async_retrieval(
            token=self.discord_bot_token,
            requested_start_date_string=self.requested_start_date_string,
            channel_names=self.channel_names,
            server_ids=self.server_ids,
            start=start,
            end=end,
        ):
            doc_batch.append(doc)
            if len(doc_batch) >= self.batch_size:
                yield [merge_batch()]
                doc_batch = []

        if doc_batch:
            yield [merge_batch()]

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self._discord_bot_token = credentials["discord_bot_token"]
        return None

    def validate_connector_settings(self) -> None:
        """Validate Discord connector settings"""
        if not self.discord_client:
            raise ConnectorMissingCredentialError("Discord")

    def poll_source(self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch) -> Any:
        """Poll Discord for recent messages"""
        return self._manage_doc_batching(
            datetime.fromtimestamp(start, tz=timezone.utc),
            datetime.fromtimestamp(end, tz=timezone.utc),
        )

    def load_from_state(self) -> Any:
        """Load messages from Discord state"""
        return self._manage_doc_batching(None, None)


if __name__ == "__main__":
    import os
    import time

    end = time.time()
    # 1 day
    start = end - 24 * 60 * 60 * 1
    # "1,2,3"
    server_ids: str | None = os.environ.get("server_ids", None)
    # "channel1,channel2"
    channel_names: str | None = os.environ.get("channel_names", None)

    connector = DiscordConnector(
        server_ids=server_ids.split(",") if server_ids else [],
        channel_names=channel_names.split(",") if channel_names else [],
        start_date=os.environ.get("start_date", None),
    )
    connector.load_credentials({"discord_bot_token": os.environ.get("discord_bot_token")})

    for doc_batch in connector.poll_source(start, end):
        for doc in doc_batch:
            print(doc)
