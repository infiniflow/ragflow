"""Slack connector"""

import itertools
import logging
import re
from collections.abc import Callable, Generator
from datetime import datetime, timezone
from http.client import IncompleteRead, RemoteDisconnected
from typing import Any, cast
from urllib.error import URLError

from slack_sdk import WebClient
from slack_sdk.errors import SlackApiError
from slack_sdk.http_retry import ConnectionErrorRetryHandler
from slack_sdk.http_retry.builtin_interval_calculators import FixedValueRetryIntervalCalculator

from common.data_source.config import (
    INDEX_BATCH_SIZE, SLACK_NUM_THREADS, ENABLE_EXPENSIVE_EXPERT_CALLS,
    _SLACK_LIMIT, FAST_TIMEOUT, MAX_RETRIES, MAX_CHANNELS_TO_LOG
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    UnexpectedValidationError
)
from common.data_source.interfaces import (
    CheckpointedConnectorWithPermSync,
    CredentialsConnector,
    SlimConnectorWithPermSync
)
from common.data_source.models import (
    BasicExpertInfo,
    ConnectorCheckpoint,
    ConnectorFailure,
    Document,
    DocumentFailure,
    SlimDocument,
    TextSection,
    SecondsSinceUnixEpoch,
    GenerateSlimDocumentOutput, MessageType, SlackMessageFilterReason, ChannelType, ThreadType, ProcessedSlackMessage,
    CheckpointOutput
)
from common.data_source.utils import make_paginated_slack_api_call, SlackTextCleaner, expert_info_from_slack_id, \
    get_message_link

# Disallowed message subtypes list
_DISALLOWED_MSG_SUBTYPES = {
    "channel_join", "channel_leave", "channel_archive", "channel_unarchive",
    "pinned_item", "unpinned_item", "ekm_access_denied", "channel_posting_permissions",
    "group_join", "group_leave", "group_archive", "group_unarchive",
    "channel_leave", "channel_name", "channel_join",
}


def default_msg_filter(message: MessageType) -> SlackMessageFilterReason | None:
    """Default message filter"""
    # Filter bot messages
    if message.get("bot_id") or message.get("app_id"):
        bot_profile_name = message.get("bot_profile", {}).get("name")
        if bot_profile_name == "DanswerBot Testing":
            return None
        return SlackMessageFilterReason.BOT

    # Filter non-informative content
    if message.get("subtype", "") in _DISALLOWED_MSG_SUBTYPES:
        return SlackMessageFilterReason.DISALLOWED

    return None


def _collect_paginated_channels(
    client: WebClient,
    exclude_archived: bool,
    channel_types: list[str],
) -> list[ChannelType]:
    """收集分页的频道列表"""
    channels: list[ChannelType] = []
    for result in make_paginated_slack_api_call(
        client.conversations_list,
        exclude_archived=exclude_archived,
        types=channel_types,
    ):
        channels.extend(result["channels"])

    return channels


def get_channels(
    client: WebClient,
    exclude_archived: bool = True,
    get_public: bool = True,
    get_private: bool = True,
) -> list[ChannelType]:
    channel_types = []
    if get_public:
        channel_types.append("public_channel")
    if get_private:
        channel_types.append("private_channel")
    
    # First try to get public and private channels
    try:
        channels = _collect_paginated_channels(
            client=client,
            exclude_archived=exclude_archived,
            channel_types=channel_types,
        )
    except SlackApiError as e:
        msg = f"Unable to fetch private channels due to: {e}."
        if not get_public:
            logging.warning(msg + " Public channels are not enabled.")
            return []

        logging.warning(msg + " Trying again with public channels only.")
        channel_types = ["public_channel"]
        channels = _collect_paginated_channels(
            client=client,
            exclude_archived=exclude_archived,
            channel_types=channel_types,
        )
    return channels


def get_channel_messages(
    client: WebClient,
    channel: ChannelType,
    oldest: str | None = None,
    latest: str | None = None,
    callback: Any = None,
) -> Generator[list[MessageType], None, None]:
    """Get all messages in a channel"""
    # Join channel so bot can access messages
    if not channel["is_member"]:
        client.conversations_join(
            channel=channel["id"],
            is_private=channel["is_private"],
        )
        logging.info(f"Successfully joined '{channel['name']}'")

    for result in make_paginated_slack_api_call(
        client.conversations_history,
        channel=channel["id"],
        oldest=oldest,
        latest=latest,
    ):
        if callback:
            if callback.should_stop():
                raise RuntimeError("get_channel_messages: Stop signal detected")

            callback.progress("get_channel_messages", 0)
        yield cast(list[MessageType], result["messages"])


def get_thread(client: WebClient, channel_id: str, thread_id: str) -> ThreadType:
    threads: list[MessageType] = []
    for result in make_paginated_slack_api_call(
        client.conversations_replies, channel=channel_id, ts=thread_id
    ):
        threads.extend(result["messages"])
    return threads


def get_latest_message_time(thread: ThreadType) -> datetime:
    max_ts = max([float(msg.get("ts", 0)) for msg in thread])
    return datetime.fromtimestamp(max_ts, tz=timezone.utc)


def _build_doc_id(channel_id: str, thread_ts: str) -> str:
    return f"{channel_id}__{thread_ts}"


def thread_to_doc(
    channel: ChannelType,
    thread: ThreadType,
    slack_cleaner: SlackTextCleaner,
    client: WebClient,
    user_cache: dict[str, BasicExpertInfo | None],
    channel_access: Any | None,
) -> Document:
    channel_id = channel["id"]

    initial_sender_expert_info = expert_info_from_slack_id(
        user_id=thread[0].get("user"), client=client, user_cache=user_cache
    )
    initial_sender_name = (
        initial_sender_expert_info.get_semantic_name()
        if initial_sender_expert_info
        else "Unknown"
    )

    valid_experts = None
    if ENABLE_EXPENSIVE_EXPERT_CALLS:
        all_sender_ids = [m.get("user") for m in thread]
        experts = [
            expert_info_from_slack_id(
                user_id=sender_id, client=client, user_cache=user_cache
            )
            for sender_id in all_sender_ids
            if sender_id
        ]
        valid_experts = [expert for expert in experts if expert]

    first_message = slack_cleaner.index_clean(cast(str, thread[0]["text"]))
    snippet = (
        first_message[:50].rstrip() + "..."
        if len(first_message) > 50
        else first_message
    )

    doc_sem_id = f"{initial_sender_name} in #{channel['name']}: {snippet}".replace(
        "\n", " "
    )

    return Document(
        id=_build_doc_id(channel_id=channel_id, thread_ts=thread[0]["ts"]),
        sections=[
            TextSection(
                link=get_message_link(event=m, client=client, channel_id=channel_id),
                text=slack_cleaner.index_clean(cast(str, m["text"])),
            )
            for m in thread
        ],
        source="slack",
        semantic_identifier=doc_sem_id,
        doc_updated_at=get_latest_message_time(thread),
        primary_owners=valid_experts,
        metadata={"Channel": channel["name"]},
        external_access=channel_access,
    )


def filter_channels(
    all_channels: list[ChannelType],
    channels_to_connect: list[str] | None,
    regex_enabled: bool,
) -> list[ChannelType]:
    if not channels_to_connect:
        return all_channels

    if regex_enabled:
        return [
            channel
            for channel in all_channels
            if any(
                re.fullmatch(channel_to_connect, channel["name"])
                for channel_to_connect in channels_to_connect
            )
        ]

    # Validate all specified channels are valid
    all_channel_names = {channel["name"] for channel in all_channels}
    for channel in channels_to_connect:
        if channel not in all_channel_names:
            raise ValueError(
                f"Channel '{channel}' not found in workspace. "
                f"Available channels (Showing {len(all_channel_names)} of "
                f"{min(len(all_channel_names), MAX_CHANNELS_TO_LOG)}): "
                f"{list(itertools.islice(all_channel_names, MAX_CHANNELS_TO_LOG))}"
            )

    return [
        channel for channel in all_channels if channel["name"] in channels_to_connect
    ]


def _get_channel_by_id(client: WebClient, channel_id: str) -> ChannelType:
    response = client.conversations_info(
        channel=channel_id,
    )
    return cast(ChannelType, response["channel"])


def _get_messages(
    channel: ChannelType,
    client: WebClient,
    oldest: str | None = None,
    latest: str | None = None,
    limit: int = _SLACK_LIMIT,
) -> tuple[list[MessageType], bool]:
    """Get messages (Slack returns from newest to oldest)"""

    # Must join channel to read messages
    if not channel["is_member"]:
        try:
            client.conversations_join(
                channel=channel["id"],
                is_private=channel["is_private"],
            )
        except SlackApiError as e:
            if e.response["error"] == "is_archived":
                logging.warning(f"Channel {channel['name']} is archived. Skipping.")
                return [], False

            logging.exception(f"Error joining channel {channel['name']}")
            raise
        logging.info(f"Successfully joined '{channel['name']}'")

    response = client.conversations_history(
        channel=channel["id"],
        oldest=oldest,
        latest=latest,
        limit=limit,
    )
    response.validate()

    messages = cast(list[MessageType], response.get("messages", []))

    cursor = cast(dict[str, Any], response.get("response_metadata", {})).get(
        "next_cursor", ""
    )
    has_more = bool(cursor)
    return messages, has_more


def _message_to_doc(
    message: MessageType,
    client: WebClient,
    channel: ChannelType,
    slack_cleaner: SlackTextCleaner,
    user_cache: dict[str, BasicExpertInfo | None],
    seen_thread_ts: set[str],
    channel_access: Any | None,
    msg_filter_func: Callable[
        [MessageType], SlackMessageFilterReason | None
    ] = default_msg_filter,
) -> tuple[Document | None, SlackMessageFilterReason | None]:
    """Convert message to document"""
    filtered_thread: ThreadType | None = None
    filter_reason: SlackMessageFilterReason | None = None
    thread_ts = message.get("thread_ts")
    if thread_ts:
        # If thread_ts exists, need to process thread
        if thread_ts in seen_thread_ts:
            return None, None

        thread = get_thread(
            client=client, channel_id=channel["id"], thread_id=thread_ts
        )

        filtered_thread = []
        for message in thread:
            filter_reason = msg_filter_func(message)
            if filter_reason:
                continue

            filtered_thread.append(message)
    else:
        filter_reason = msg_filter_func(message)
        if filter_reason:
            return None, filter_reason

        filtered_thread = [message]

    if not filtered_thread:
        return None, filter_reason

    doc = thread_to_doc(
        channel=channel,
        thread=filtered_thread,
        slack_cleaner=slack_cleaner,
        client=client,
        user_cache=user_cache,
        channel_access=channel_access,
    )
    return doc, None


def _process_message(
    message: MessageType,
    client: WebClient,
    channel: ChannelType,
    slack_cleaner: SlackTextCleaner,
    user_cache: dict[str, BasicExpertInfo | None],
    seen_thread_ts: set[str],
    channel_access: Any | None,
    msg_filter_func: Callable[
        [MessageType], SlackMessageFilterReason | None
    ] = default_msg_filter,
) -> ProcessedSlackMessage:
    thread_ts = message.get("thread_ts")
    thread_or_message_ts = thread_ts or message["ts"]
    try:
        doc, filter_reason = _message_to_doc(
            message=message,
            client=client,
            channel=channel,
            slack_cleaner=slack_cleaner,
            user_cache=user_cache,
            seen_thread_ts=seen_thread_ts,
            channel_access=channel_access,
            msg_filter_func=msg_filter_func,
        )
        return ProcessedSlackMessage(
            doc=doc,
            thread_or_message_ts=thread_or_message_ts,
            filter_reason=filter_reason,
            failure=None,
        )
    except Exception as e:
        (logging.exception(f"Error processing message {message['ts']}"))
        return ProcessedSlackMessage(
            doc=None,
            thread_or_message_ts=thread_or_message_ts,
            filter_reason=None,
            failure=ConnectorFailure(
                failed_document=DocumentFailure(
                    document_id=_build_doc_id(
                        channel_id=channel["id"], thread_ts=thread_or_message_ts
                    ),
                    document_link=get_message_link(message, client, channel["id"]),
                ),
                failure_message=str(e),
                exception=e,
            ),
        )


def _get_all_doc_ids(
    client: WebClient,
    channels: list[str] | None = None,
    channel_name_regex_enabled: bool = False,
    msg_filter_func: Callable[
        [MessageType], SlackMessageFilterReason | None
    ] = default_msg_filter,
    callback: Any = None,
) -> GenerateSlimDocumentOutput:
    all_channels = get_channels(client)
    filtered_channels = filter_channels(
        all_channels, channels, channel_name_regex_enabled
    )

    for channel in filtered_channels:
        channel_id = channel["id"]
        external_access = None  # Simplified version, not handling permissions
        channel_message_batches = get_channel_messages(
            client=client,
            channel=channel,
            callback=callback,
        )

        for message_batch in channel_message_batches:
            slim_doc_batch: list[SlimDocument] = []
            for message in message_batch:
                filter_reason = msg_filter_func(message)
                if filter_reason:
                    continue

                slim_doc_batch.append(
                    SlimDocument(
                        id=_build_doc_id(
                            channel_id=channel_id, thread_ts=message["ts"]
                        ),
                        external_access=external_access,
                    )
                )

            yield slim_doc_batch


class SlackConnector(
    SlimConnectorWithPermSync,
    CredentialsConnector,
    CheckpointedConnectorWithPermSync,
):
    """Slack connector"""

    def __init__(
        self,
        channels: list[str] | None = None,
        channel_regex_enabled: bool = False,
        batch_size: int = INDEX_BATCH_SIZE,
        num_threads: int = SLACK_NUM_THREADS,
        use_redis: bool = False,  # Simplified version, not using Redis
    ) -> None:
        self.channels = channels
        self.channel_regex_enabled = channel_regex_enabled
        self.batch_size = batch_size
        self.num_threads = num_threads
        self.client: WebClient | None = None
        self.fast_client: WebClient | None = None
        self.text_cleaner: SlackTextCleaner | None = None
        self.user_cache: dict[str, BasicExpertInfo | None] = {}
        self.credentials_provider: Any = None
        self.use_redis = use_redis

    @property
    def channels(self) -> list[str] | None:
        return self._channels

    @channels.setter
    def channels(self, channels: list[str] | None) -> None:
        self._channels = (
            [channel.removeprefix("#") for channel in channels] if channels else None
        )

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        """Load credentials"""
        raise NotImplementedError("Use set_credentials_provider with this connector.")

    def set_credentials_provider(self, credentials_provider: Any) -> None:
        """Set credentials provider"""
        credentials = credentials_provider.get_credentials()
        bot_token = credentials["slack_bot_token"]

        # Simplified version, not using Redis
        connection_error_retry_handler = ConnectionErrorRetryHandler(
            max_retry_count=MAX_RETRIES,
            interval_calculator=FixedValueRetryIntervalCalculator(),
            error_types=[
                URLError,
                ConnectionResetError,
                RemoteDisconnected,
                IncompleteRead,
            ],
        )

        self.client = WebClient(
            token=bot_token, retry_handlers=[connection_error_retry_handler]
        )

        # For fast response requests
        self.fast_client = WebClient(
            token=bot_token, timeout=FAST_TIMEOUT
        )
        self.text_cleaner = SlackTextCleaner(client=self.client)
        self.credentials_provider = credentials_provider

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: Any = None,
    ) -> GenerateSlimDocumentOutput:
        if self.client is None:
            raise ConnectorMissingCredentialError("Slack")

        return _get_all_doc_ids(
            client=self.client,
            channels=self.channels,
            channel_name_regex_enabled=self.channel_regex_enabled,
            callback=callback,
        )

    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> CheckpointOutput:
        """Load documents from checkpoint"""
        # Simplified version, not implementing full checkpoint functionality
        logging.warning("Checkpoint functionality not implemented in simplified version")
        return []

    def load_from_checkpoint_with_perm_sync(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConnectorCheckpoint,
    ) -> CheckpointOutput:
        """Load documents from checkpoint (with permission sync)"""
        # Simplified version, not implementing full checkpoint functionality
        logging.warning("Checkpoint functionality not implemented in simplified version")
        return []

    def build_dummy_checkpoint(self) -> ConnectorCheckpoint:
        """Build dummy checkpoint"""
        return ConnectorCheckpoint()

    def validate_checkpoint_json(self, checkpoint_json: str) -> ConnectorCheckpoint:
        """Validate checkpoint JSON"""
        return ConnectorCheckpoint()

    def validate_connector_settings(self) -> None:
        """Validate connector settings"""
        if self.fast_client is None:
            raise ConnectorMissingCredentialError("Slack credentials not loaded.")

        try:
            # 1) Validate workspace connection
            auth_response = self.fast_client.auth_test()
            if not auth_response.get("ok", False):
                error_msg = auth_response.get(
                    "error", "Unknown error from Slack auth_test"
                )
                raise ConnectorValidationError(f"Failed Slack auth_test: {error_msg}")

            # 2) Confirm listing channels functionality works
            test_resp = self.fast_client.conversations_list(
                limit=1, types=["public_channel"]
            )
            if not test_resp.get("ok", False):
                error_msg = test_resp.get("error", "Unknown error from Slack")
                if error_msg == "invalid_auth":
                    raise ConnectorValidationError(
                        f"Invalid Slack bot token ({error_msg})."
                    )
                elif error_msg == "not_authed":
                    raise CredentialExpiredError(
                        f"Invalid or expired Slack bot token ({error_msg})."
                    )
                raise UnexpectedValidationError(
                    f"Slack API returned a failure: {error_msg}"
                )

        except SlackApiError as e:
            slack_error = e.response.get("error", "")
            if slack_error == "ratelimited":
                retry_after = int(e.response.headers.get("Retry-After", 1))
                logging.warning(
                    f"Slack API rate limited during validation. Retry suggested after {retry_after} seconds. "
                    "Proceeding with validation, but be aware that connector operations might be throttled."
                )
                return
            elif slack_error == "missing_scope":
                raise InsufficientPermissionsError(
                    "Slack bot token lacks the necessary scope to list/access channels. "
                    "Please ensure your Slack app has 'channels:read' (and/or 'groups:read' for private channels)."
                )
            elif slack_error == "invalid_auth":
                raise CredentialExpiredError(
                    f"Invalid Slack bot token ({slack_error})."
                )
            elif slack_error == "not_authed":
                raise CredentialExpiredError(
                    f"Invalid or expired Slack bot token ({slack_error})."
                )
            raise UnexpectedValidationError(
                f"Unexpected Slack error '{slack_error}' during settings validation."
            )
        except ConnectorValidationError as e:
            raise e
        except Exception as e:
            raise UnexpectedValidationError(
                f"Unexpected error during Slack settings validation: {e}"
            )


if __name__ == "__main__":
    # Example usage
    import os

    slack_channel = os.environ.get("SLACK_CHANNEL")
    connector = SlackConnector(
        channels=[slack_channel] if slack_channel else None,
    )

    # Simplified version, directly using credentials dictionary
    credentials = {
        "slack_bot_token": os.environ.get("SLACK_BOT_TOKEN", "test-token")
    }

    class SimpleCredentialsProvider:
        def get_credentials(self):
            return credentials

    provider = SimpleCredentialsProvider()
    connector.set_credentials_provider(provider)

    try:
        connector.validate_connector_settings()
        print("Slack connector settings validated successfully")
    except Exception as e:
        print(f"Validation failed: {e}")