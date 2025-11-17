

"""Confluence connector"""
import copy
import json
import logging
import time
from datetime import datetime, timezone, timedelta
from pathlib import Path
from typing import Any, cast, Iterator, Callable, Generator

import requests
from typing_extensions import override
from urllib.parse import quote

import bs4
from atlassian.errors import ApiError
from atlassian import Confluence
from requests.exceptions import HTTPError

from common.data_source.config import INDEX_BATCH_SIZE, DocumentSource, CONTINUE_ON_CONNECTOR_FAILURE, \
    CONFLUENCE_CONNECTOR_LABELS_TO_SKIP, CONFLUENCE_TIMEZONE_OFFSET, CONFLUENCE_CONNECTOR_USER_PROFILES_OVERRIDE, \
    CONFLUENCE_SYNC_TIME_BUFFER_SECONDS, \
    OAUTH_CONFLUENCE_CLOUD_CLIENT_ID, OAUTH_CONFLUENCE_CLOUD_CLIENT_SECRET, _DEFAULT_PAGINATION_LIMIT, \
    _PROBLEMATIC_EXPANSIONS, _REPLACEMENT_EXPANSIONS, _USER_NOT_FOUND, _COMMENT_EXPANSION_FIELDS, \
    _ATTACHMENT_EXPANSION_FIELDS, _PAGE_EXPANSION_FIELDS, ONE_DAY, ONE_HOUR, _RESTRICTIONS_EXPANSION_FIELDS, \
    _SLIM_DOC_BATCH_SIZE, CONFLUENCE_CONNECTOR_ATTACHMENT_SIZE_THRESHOLD
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    InsufficientPermissionsError,
    UnexpectedValidationError, CredentialExpiredError
)
from common.data_source.html_utils import format_document_soup
from common.data_source.interfaces import (
    ConnectorCheckpoint,
    CredentialsConnector,
    SecondsSinceUnixEpoch,
    SlimConnectorWithPermSync, StaticCredentialsProvider, CheckpointedConnector, SlimConnector,
    CredentialsProviderInterface, ConfluenceUser, IndexingHeartbeatInterface, AttachmentProcessingResult,
    CheckpointOutput
)
from common.data_source.models import ConnectorFailure, Document, TextSection, ImageSection, BasicExpertInfo, \
    DocumentFailure, GenerateSlimDocumentOutput, SlimDocument, ExternalAccess
from common.data_source.utils import load_all_docs_from_checkpoint_connector, scoped_url, \
    process_confluence_user_profiles_override, confluence_refresh_tokens, run_with_timeout, _handle_http_error, \
    update_param_in_path, get_start_param_from_url, build_confluence_document_id, datetime_from_string, \
    is_atlassian_date_error, validate_attachment_filetype
from rag.utils.redis_conn import RedisDB, REDIS_CONN

_USER_ID_TO_DISPLAY_NAME_CACHE: dict[str, str | None] = {}
_USER_EMAIL_CACHE: dict[str, str | None] = {}

class ConfluenceCheckpoint(ConnectorCheckpoint):

    next_page_url: str | None


class ConfluenceRateLimitError(Exception):
    pass


class OnyxConfluence:
    """
    This is a custom Confluence class that:

    A. overrides the default Confluence class to add a custom CQL method.
    B.
    This is necessary because the default Confluence class does not properly support cql expansions.
    All methods are automatically wrapped with handle_confluence_rate_limit.
    """

    CREDENTIAL_PREFIX = "connector:confluence:credential"
    CREDENTIAL_TTL = 300  # 5 min
    PROBE_TIMEOUT = 5  # 5 seconds

    def __init__(
        self,
        is_cloud: bool,
        url: str,
        credentials_provider: CredentialsProviderInterface,
        timeout: int | None = None,
        scoped_token: bool = False,
        # should generally not be passed in, but making it overridable for
        # easier testing
        confluence_user_profiles_override: list[dict[str, str]] | None = (
            CONFLUENCE_CONNECTOR_USER_PROFILES_OVERRIDE
        ),
    ) -> None:
        self.base_url = url  #'/'.join(url.rstrip("/").split("/")[:-1])
        url = scoped_url(url, "confluence") if scoped_token else url

        self._is_cloud = is_cloud
        self._url = url.rstrip("/")
        self._credentials_provider = credentials_provider
        self.scoped_token = scoped_token
        self.redis_client: RedisDB | None = None
        self.static_credentials: dict[str, Any] | None = None
        if self._credentials_provider.is_dynamic():
            self.redis_client = REDIS_CONN
        else:
            self.static_credentials = self._credentials_provider.get_credentials()

        self._confluence = Confluence(url)
        self.credential_key: str = (
            self.CREDENTIAL_PREFIX
            + f":credential_{self._credentials_provider.get_provider_key()}"
        )

        self._kwargs: Any = None

        self.shared_base_kwargs: dict[str, str | int | bool] = {
            "api_version": "cloud" if is_cloud else "latest",
            "backoff_and_retry": True,
            "cloud": is_cloud,
        }
        if timeout:
            self.shared_base_kwargs["timeout"] = timeout

        self._confluence_user_profiles_override = (
            process_confluence_user_profiles_override(confluence_user_profiles_override)
            if confluence_user_profiles_override
            else None
        )

    def _renew_credentials(self) -> tuple[dict[str, Any], bool]:
        """credential_json - the current json credentials
        Returns a tuple
        1. The up to date credentials
        2. True if the credentials were updated

        This method is intended to be used within a distributed lock.
        Lock, call this, update credentials if the tokens were refreshed, then release
        """
        # static credentials are preloaded, so no locking/redis required
        if self.static_credentials:
            return self.static_credentials, False

        if not self.redis_client:
            raise RuntimeError("self.redis_client is None")

        # dynamic credentials need locking
        # check redis first, then fallback to the DB
        credential_raw = self.redis_client.get(self.credential_key)
        if credential_raw is not None:
            credential_bytes = cast(bytes, credential_raw)
            credential_str = credential_bytes.decode("utf-8")
            credential_json: dict[str, Any] = json.loads(credential_str)
        else:
            credential_json = self._credentials_provider.get_credentials()

        if "confluence_refresh_token" not in credential_json:
            # static credentials ... cache them permanently and return
            self.static_credentials = credential_json
            return credential_json, False

        if not OAUTH_CONFLUENCE_CLOUD_CLIENT_ID:
            raise RuntimeError("OAUTH_CONFLUENCE_CLOUD_CLIENT_ID must be set!")

        if not OAUTH_CONFLUENCE_CLOUD_CLIENT_SECRET:
            raise RuntimeError("OAUTH_CONFLUENCE_CLOUD_CLIENT_SECRET must be set!")

        # check if we should refresh tokens. we're deciding to refresh halfway
        # to expiration
        now = datetime.now(timezone.utc)
        created_at = datetime.fromisoformat(credential_json["created_at"])
        expires_in: int = credential_json["expires_in"]
        renew_at = created_at + timedelta(seconds=expires_in // 2)
        if now <= renew_at:
            # cached/current credentials are reasonably up to date
            return credential_json, False

        # we need to refresh
        logging.info("Renewing Confluence Cloud credentials...")
        new_credentials = confluence_refresh_tokens(
            OAUTH_CONFLUENCE_CLOUD_CLIENT_ID,
            OAUTH_CONFLUENCE_CLOUD_CLIENT_SECRET,
            credential_json["cloud_id"],
            credential_json["confluence_refresh_token"],
        )

        # store the new credentials to redis and to the db thru the provider
        # redis: we use a 5 min TTL because we are given a 10 minute grace period
        # when keys are rotated. it's easier to expire the cached credentials
        # reasonably frequently rather than trying to handle strong synchronization
        # between the db and redis everywhere the credentials might be updated
        new_credential_str = json.dumps(new_credentials)
        self.redis_client.set(
            self.credential_key, new_credential_str, nx=True, ex=self.CREDENTIAL_TTL
        )
        self._credentials_provider.set_credentials(new_credentials)

        return new_credentials, True

    @staticmethod
    def _make_oauth2_dict(credentials: dict[str, Any]) -> dict[str, Any]:
        oauth2_dict: dict[str, Any] = {}
        if "confluence_refresh_token" in credentials:
            oauth2_dict["client_id"] = OAUTH_CONFLUENCE_CLOUD_CLIENT_ID
            oauth2_dict["token"] = {}
            oauth2_dict["token"]["access_token"] = credentials[
                "confluence_access_token"
            ]
        return oauth2_dict

    def _probe_connection(
        self,
        **kwargs: Any,
    ) -> None:
        merged_kwargs = {**self.shared_base_kwargs, **kwargs}
        # add special timeout to make sure that we don't hang indefinitely
        merged_kwargs["timeout"] = self.PROBE_TIMEOUT

        with self._credentials_provider:
            credentials, _ = self._renew_credentials()
            if self.scoped_token:
                # v2 endpoint doesn't always work with scoped tokens, use v1
                token = credentials["confluence_access_token"]
                probe_url = f"{self.base_url}/rest/api/space?limit=1"
                import requests

                logging.info(f"First and Last 5 of token: {token[:5]}...{token[-5:]}")

                try:
                    r = requests.get(
                        probe_url,
                        headers={"Authorization": f"Bearer {token}"},
                        timeout=10,
                    )
                    r.raise_for_status()
                except HTTPError as e:
                    if e.response.status_code == 403:
                        logging.warning(
                            "scoped token authenticated but not valid for probe endpoint (spaces)"
                        )
                    else:
                        if "WWW-Authenticate" in e.response.headers:
                            logging.warning(
                                f"WWW-Authenticate: {e.response.headers['WWW-Authenticate']}"
                            )
                            logging.warning(f"Full error: {e.response.text}")
                        raise e
                return

            # probe connection with direct client, no retries
            if "confluence_refresh_token" in credentials:
                logging.info("Probing Confluence with OAuth Access Token.")

                oauth2_dict: dict[str, Any] = OnyxConfluence._make_oauth2_dict(
                    credentials
                )
                url = (
                    f"https://api.atlassian.com/ex/confluence/{credentials['cloud_id']}"
                )
                confluence_client_with_minimal_retries = Confluence(
                    url=url, oauth2=oauth2_dict, **merged_kwargs
                )
            else:
                logging.info("Probing Confluence with Personal Access Token.")
                url = self._url
                if self._is_cloud:
                    logging.info("running with cloud client")
                    confluence_client_with_minimal_retries = Confluence(
                        url=url,
                        username=credentials["confluence_username"],
                        password=credentials["confluence_access_token"],
                        **merged_kwargs,
                    )
                else:
                    confluence_client_with_minimal_retries = Confluence(
                        url=url,
                        token=credentials["confluence_access_token"],
                        **merged_kwargs,
                    )

            # This call sometimes hangs indefinitely, so we run it in a timeout
            spaces = run_with_timeout(
                timeout=10,
                func=confluence_client_with_minimal_retries.get_all_spaces,
                limit=1,
            )

            # uncomment the following for testing
            # the following is an attempt to retrieve the user's timezone
            # Unfornately, all data is returned in UTC regardless of the user's time zone
            # even tho CQL parses incoming times based on the user's time zone
            # space_key = spaces["results"][0]["key"]
            # space_details = confluence_client_with_minimal_retries.cql(f"space.key={space_key}+AND+type=space")

            if not spaces:
                raise RuntimeError(
                    f"No spaces found at {url}! "
                    "Check your credentials and wiki_base and make sure "
                    "is_cloud is set correctly."
                )

            logging.info("Confluence probe succeeded.")

    def _initialize_connection(
        self,
        **kwargs: Any,
    ) -> None:
        """Called externally to init the connection in a thread safe manner."""
        merged_kwargs = {**self.shared_base_kwargs, **kwargs}
        with self._credentials_provider:
            credentials, _ = self._renew_credentials()
            self._confluence = self._initialize_connection_helper(
                credentials, **merged_kwargs
            )
            self._kwargs = merged_kwargs

    def _initialize_connection_helper(
        self,
        credentials: dict[str, Any],
        **kwargs: Any,
    ) -> Confluence:
        """Called internally to init the connection. Distributed locking
        to prevent multiple threads from modifying the credentials
        must be handled around this function."""

        confluence = None

        # probe connection with direct client, no retries
        if "confluence_refresh_token" in credentials:
            logging.info("Connecting to Confluence Cloud with OAuth Access Token.")

            oauth2_dict: dict[str, Any] = OnyxConfluence._make_oauth2_dict(credentials)
            url = f"https://api.atlassian.com/ex/confluence/{credentials['cloud_id']}"
            confluence = Confluence(url=url, oauth2=oauth2_dict, **kwargs)
        else:
            logging.info(
                f"Connecting to Confluence with Personal Access Token as user: {credentials['confluence_username']}"
            )
            if self._is_cloud:
                confluence = Confluence(
                    url=self._url,
                    username=credentials["confluence_username"],
                    password=credentials["confluence_access_token"],
                    **kwargs,
                )
            else:
                confluence = Confluence(
                    url=self._url,
                    token=credentials["confluence_access_token"],
                    **kwargs,
                )

        return confluence

    # https://developer.atlassian.com/cloud/confluence/rate-limiting/
    # This uses the native rate limiting option provided by the
    # confluence client and otherwise applies a simpler set of error handling.
    def _make_rate_limited_confluence_method(
        self, name: str, credential_provider: CredentialsProviderInterface | None
    ) -> Callable[..., Any]:
        def wrapped_call(*args: list[Any], **kwargs: Any) -> Any:
            MAX_RETRIES = 5

            TIMEOUT = 600
            timeout_at = time.monotonic() + TIMEOUT

            for attempt in range(MAX_RETRIES):
                if time.monotonic() > timeout_at:
                    raise TimeoutError(
                        f"Confluence call attempts took longer than {TIMEOUT} seconds."
                    )

                # we're relying more on the client to rate limit itself
                # and applying our own retries in a more specific set of circumstances
                try:
                    if credential_provider:
                        with credential_provider:
                            credentials, renewed = self._renew_credentials()
                            if renewed:
                                self._confluence = self._initialize_connection_helper(
                                    credentials, **self._kwargs
                                )
                            attr = getattr(self._confluence, name, None)
                            if attr is None:
                                # The underlying Confluence client doesn't have this attribute
                                raise AttributeError(
                                    f"'{type(self).__name__}' object has no attribute '{name}'"
                                )

                            return attr(*args, **kwargs)
                    else:
                        attr = getattr(self._confluence, name, None)
                        if attr is None:
                            # The underlying Confluence client doesn't have this attribute
                            raise AttributeError(
                                f"'{type(self).__name__}' object has no attribute '{name}'"
                            )

                        return attr(*args, **kwargs)

                except HTTPError as e:
                    delay_until = _handle_http_error(e, attempt)
                    logging.warning(
                        f"HTTPError in confluence call. "
                        f"Retrying in {delay_until} seconds..."
                    )
                    while time.monotonic() < delay_until:
                        # in the future, check a signal here to exit
                        time.sleep(1)
                except AttributeError as e:
                    # Some error within the Confluence library, unclear why it fails.
                    # Users reported it to be intermittent, so just retry
                    if attempt == MAX_RETRIES - 1:
                        raise e

                    logging.exception(
                        "Confluence Client raised an AttributeError. Retrying..."
                    )
                    time.sleep(5)

        return wrapped_call

    def __getattr__(self, name: str) -> Any:
        """Dynamically intercept attribute/method access."""
        attr = getattr(self._confluence, name, None)
        if attr is None:
            # The underlying Confluence client doesn't have this attribute
            raise AttributeError(
                f"'{type(self).__name__}' object has no attribute '{name}'"
            )

        # If it's not a method, just return it after ensuring token validity
        if not callable(attr):
            return attr

        # skip methods that start with "_"
        if name.startswith("_"):
            return attr

        # wrap the method with our retry handler
        rate_limited_method: Callable[..., Any] = (
            self._make_rate_limited_confluence_method(name, self._credentials_provider)
        )

        return rate_limited_method

    def _try_one_by_one_for_paginated_url(
        self,
        url_suffix: str,
        initial_start: int,
        limit: int,
    ) -> Generator[dict[str, Any], None, str | None]:
        """
        Go through `limit` items, starting at `initial_start` one by one (e.g. using
        `limit=1` for each call).

        If we encounter an error, we skip the item and try the next one. We will return
        the items we were able to retrieve successfully.

        Returns the expected next url_suffix. Returns None if it thinks we've hit the end.

        TODO (chris): make this yield failures as well as successes.
        TODO (chris): make this work for confluence cloud somehow.
        """
        if self._is_cloud:
            raise RuntimeError("This method is not implemented for Confluence Cloud.")

        found_empty_page = False
        temp_url_suffix = url_suffix

        for ind in range(limit):
            try:
                temp_url_suffix = update_param_in_path(
                    url_suffix, "start", str(initial_start + ind)
                )
                temp_url_suffix = update_param_in_path(temp_url_suffix, "limit", "1")
                logging.info(f"Making recovery confluence call to {temp_url_suffix}")
                raw_response = self.get(path=temp_url_suffix, advanced_mode=True)
                raw_response.raise_for_status()

                latest_results = raw_response.json().get("results", [])
                yield from latest_results

                if not latest_results:
                    # no more results, break out of the loop
                    logging.info(
                        f"No results found for call '{temp_url_suffix}'"
                        "Stopping pagination."
                    )
                    found_empty_page = True
                    break
            except Exception:
                logging.exception(
                    f"Error in confluence call to {temp_url_suffix}. Continuing."
                )

        if found_empty_page:
            return None

        # if we got here, we successfully tried `limit` items
        return update_param_in_path(url_suffix, "start", str(initial_start + limit))

    def _paginate_url(
        self,
        url_suffix: str,
        limit: int | None = None,
        # Called with the next url to use to get the next page
        next_page_callback: Callable[[str], None] | None = None,
        force_offset_pagination: bool = False,
    ) -> Iterator[dict[str, Any]]:
        """
        This will paginate through the top level query.
        """
        if not limit:
            limit = _DEFAULT_PAGINATION_LIMIT

        url_suffix = update_param_in_path(url_suffix, "limit", str(limit))

        while url_suffix:
            logging.debug(f"Making confluence call to {url_suffix}")
            try:
                raw_response = self.get(
                    path=url_suffix,
                    advanced_mode=True,
                    params={
                        "body-format": "atlas_doc_format",
                        "expand": "body.atlas_doc_format",
                    },
                )
            except Exception as e:
                logging.exception(f"Error in confluence call to {url_suffix}")
                raise e

            try:
                raw_response.raise_for_status()
            except Exception as e:
                logging.warning(f"Error in confluence call to {url_suffix}")

                # If the problematic expansion is in the url, replace it
                # with the replacement expansion and try again
                # If that fails, raise the error
                if _PROBLEMATIC_EXPANSIONS in url_suffix:
                    logging.warning(
                        f"Replacing {_PROBLEMATIC_EXPANSIONS} with {_REPLACEMENT_EXPANSIONS}"
                        " and trying again."
                    )
                    url_suffix = url_suffix.replace(
                        _PROBLEMATIC_EXPANSIONS,
                        _REPLACEMENT_EXPANSIONS,
                    )
                    continue

                # If we fail due to a 500, try one by one.
                # NOTE: this iterative approach only works for server, since cloud uses cursor-based
                # pagination
                if raw_response.status_code == 500 and not self._is_cloud:
                    initial_start = get_start_param_from_url(url_suffix)
                    if initial_start is None:
                        # can't handle this if we don't have offset-based pagination
                        raise

                    # this will just yield the successful items from the batch
                    new_url_suffix = yield from self._try_one_by_one_for_paginated_url(
                        url_suffix,
                        initial_start=initial_start,
                        limit=limit,
                    )

                    # this means we ran into an empty page
                    if new_url_suffix is None:
                        if next_page_callback:
                            next_page_callback("")
                        break

                    url_suffix = new_url_suffix
                    continue

                else:
                    logging.exception(
                        f"Error in confluence call to {url_suffix} \n"
                        f"Raw Response Text: {raw_response.text} \n"
                        f"Full Response: {raw_response.__dict__} \n"
                        f"Error: {e} \n"
                    )
                    raise

            try:
                next_response = raw_response.json()
            except Exception as e:
                logging.exception(
                    f"Failed to parse response as JSON. Response: {raw_response.__dict__}"
                )
                raise e

            # Yield the results individually.
            results = cast(list[dict[str, Any]], next_response.get("results", []))

            # Note 1:
            # Make sure we don't update the start by more than the amount
            # of results we were able to retrieve. The Confluence API has a
            # weird behavior where if you pass in a limit that is too large for
            # the configured server, it will artificially limit the amount of
            # results returned BUT will not apply this to the start parameter.
            # This will cause us to miss results.
            #
            # Note 2:
            # We specifically perform manual yielding (i.e., `for x in xs: yield x`) as opposed to using a `yield from xs`
            # because we *have to call the `next_page_callback`* prior to yielding the last element!
            #
            # If we did:
            #
            # ```py
            # yield from results
            # if next_page_callback:
            #   next_page_callback(url_suffix)
            # ```
            #
            # then the logic would fail since the iterator would finish (and the calling scope would exit out of its driving
            # loop) prior to the callback being called.

            old_url_suffix = url_suffix
            updated_start = get_start_param_from_url(old_url_suffix)
            url_suffix = cast(str, next_response.get("_links", {}).get("next", ""))
            for i, result in enumerate(results):
                updated_start += 1
                if url_suffix and next_page_callback and i == len(results) - 1:
                    # update the url if we're on the last result in the page
                    if not self._is_cloud:
                        # If confluence claims there are more results, we update the start param
                        # based on how many results were returned and try again.
                        url_suffix = update_param_in_path(
                            url_suffix, "start", str(updated_start)
                        )
                    # notify the caller of the new url
                    next_page_callback(url_suffix)

                elif force_offset_pagination and i == len(results) - 1:
                    url_suffix = update_param_in_path(
                        old_url_suffix, "start", str(updated_start)
                    )

                yield result

            # we've observed that Confluence sometimes returns a next link despite giving
            # 0 results. This is a bug with Confluence, so we need to check for it and
            # stop paginating.
            if url_suffix and not results:
                logging.info(
                    f"No results found for call '{old_url_suffix}' despite next link "
                    "being present. Stopping pagination."
                )
                break

    def build_cql_url(self, cql: str, expand: str | None = None) -> str:
        expand_string = f"&expand={expand}" if expand else ""
        return f"rest/api/content/search?cql={cql}{expand_string}"

    def paginated_cql_retrieval(
        self,
        cql: str,
        expand: str | None = None,
        limit: int | None = None,
    ) -> Iterator[dict[str, Any]]:
        """
        The content/search endpoint can be used to fetch pages, attachments, and comments.
        """
        cql_url = self.build_cql_url(cql, expand)
        yield from self._paginate_url(cql_url, limit)

    def paginated_page_retrieval(
        self,
        cql_url: str,
        limit: int,
        # Called with the next url to use to get the next page
        next_page_callback: Callable[[str], None] | None = None,
    ) -> Iterator[dict[str, Any]]:
        """
        Error handling (and testing) wrapper for _paginate_url,
        because the current approach to page retrieval involves handling the
        next page links manually.
        """
        try:
            yield from self._paginate_url(
                cql_url, limit=limit, next_page_callback=next_page_callback
            )
        except Exception as e:
            logging.exception(f"Error in paginated_page_retrieval: {e}")
            raise e

    def cql_paginate_all_expansions(
        self,
        cql: str,
        expand: str | None = None,
        limit: int | None = None,
    ) -> Iterator[dict[str, Any]]:
        """
        This function will paginate through the top level query first, then
        paginate through all of the expansions.
        """

        def _traverse_and_update(data: dict | list) -> None:
            if isinstance(data, dict):
                next_url = data.get("_links", {}).get("next")
                if next_url and "results" in data:
                    data["results"].extend(self._paginate_url(next_url, limit=limit))

                for value in data.values():
                    _traverse_and_update(value)
            elif isinstance(data, list):
                for item in data:
                    _traverse_and_update(item)

        for confluence_object in self.paginated_cql_retrieval(cql, expand, limit):
            _traverse_and_update(confluence_object)
            yield confluence_object

    def paginated_cql_user_retrieval(
        self,
        expand: str | None = None,
        limit: int | None = None,
    ) -> Iterator[ConfluenceUser]:
        """
        The search/user endpoint can be used to fetch users.
        It's a separate endpoint from the content/search endpoint used only for users.
        Otherwise it's very similar to the content/search endpoint.
        """

        # this is needed since there is a live bug with Confluence Server/Data Center
        # where not all users are returned by the APIs. This is a workaround needed until
        # that is patched.
        if self._confluence_user_profiles_override:
            yield from self._confluence_user_profiles_override

        elif self._is_cloud:
            cql = "type=user"
            url = "rest/api/search/user"
            expand_string = f"&expand={expand}" if expand else ""
            url += f"?cql={cql}{expand_string}"
            for user_result in self._paginate_url(
                url, limit, force_offset_pagination=True
            ):
                user = user_result["user"]
                yield ConfluenceUser(
                    user_id=user["accountId"],
                    username=None,
                    display_name=user["displayName"],
                    email=user.get("email"),
                    type=user["accountType"],
                )
        else:
            for user in self._paginate_url("rest/api/user/list", limit):
                yield ConfluenceUser(
                    user_id=user["userKey"],
                    username=user["username"],
                    display_name=user["displayName"],
                    email=None,
                    type=user.get("type", "user"),
                )

    def paginated_groups_by_user_retrieval(
        self,
        user_id: str,  # accountId in Cloud, userKey in Server
        limit: int | None = None,
    ) -> Iterator[dict[str, Any]]:
        """
        This is not an SQL like query.
        It's a confluence specific endpoint that can be used to fetch groups.
        """
        user_field = "accountId" if self._is_cloud else "key"
        user_value = user_id
        # Server uses userKey (but calls it key during the API call), Cloud uses accountId
        user_query = f"{user_field}={quote(user_value)}"

        url = f"rest/api/user/memberof?{user_query}"
        yield from self._paginate_url(url, limit, force_offset_pagination=True)

    def paginated_groups_retrieval(
        self,
        limit: int | None = None,
    ) -> Iterator[dict[str, Any]]:
        """
        This is not an SQL like query.
        It's a confluence specific endpoint that can be used to fetch groups.
        """
        yield from self._paginate_url("rest/api/group", limit)

    def paginated_group_members_retrieval(
        self,
        group_name: str,
        limit: int | None = None,
    ) -> Iterator[dict[str, Any]]:
        """
        This is not an SQL like query.
        It's a confluence specific endpoint that can be used to fetch the members of a group.
        THIS DOESN'T WORK FOR SERVER because it breaks when there is a slash in the group name.
        E.g. neither "test/group" nor "test%2Fgroup" works for confluence.
        """
        group_name = quote(group_name)
        yield from self._paginate_url(f"rest/api/group/{group_name}/member", limit)

    def get_all_space_permissions_server(
        self,
        space_key: str,
    ) -> list[dict[str, Any]]:
        """
        This is a confluence server specific method that can be used to
        fetch the permissions of a space.
        This is better logging than calling the get_space_permissions method
        because it returns a jsonrpc response.
        TODO: Make this call these endpoints for newer confluence versions:
        - /rest/api/space/{spaceKey}/permissions
        - /rest/api/space/{spaceKey}/permissions/anonymous
        """
        url = "rpc/json-rpc/confluenceservice-v2"
        data = {
            "jsonrpc": "2.0",
            "method": "getSpacePermissionSets",
            "id": 7,
            "params": [space_key],
        }
        response = self.post(url, data=data)
        logging.debug(f"jsonrpc response: {response}")
        if not response.get("result"):
            logging.warning(
                f"No jsonrpc response for space permissions for space {space_key}"
                f"\nResponse: {response}"
            )

        return response.get("result", [])

    def get_current_user(self, expand: str | None = None) -> Any:
        """
        Implements a method that isn't in the third party client.

        Get information about the current user
        :param expand: OPTIONAL expand for get status of user.
                Possible param is "status". Results are "Active, Deactivated"
        :return: Returns the user details
        """

        from atlassian.errors import ApiPermissionError  # type:ignore

        url = "rest/api/user/current"
        params = {}
        if expand:
            params["expand"] = expand
        try:
            response = self.get(url, params=params)
        except HTTPError as e:
            if e.response.status_code == 403:
                raise ApiPermissionError(
                    "The calling user does not have permission", reason=e
                )
            raise
        return response


def get_user_email_from_username__server(
    confluence_client: OnyxConfluence, user_name: str
) -> str | None:
    global _USER_EMAIL_CACHE
    if _USER_EMAIL_CACHE.get(user_name) is None:
        try:
            response = confluence_client.get_mobile_parameters(user_name)
            email = response.get("email")
        except Exception:
            logging.warning(f"failed to get confluence email for {user_name}")
            # For now, we'll just return None and log a warning. This means
            # we will keep retrying to get the email every group sync.
            email = None
            # We may want to just return a string that indicates failure so we dont
            # keep retrying
            # email = f"FAILED TO GET CONFLUENCE EMAIL FOR {user_name}"
        _USER_EMAIL_CACHE[user_name] = email
    return _USER_EMAIL_CACHE[user_name]


def _get_user(confluence_client: OnyxConfluence, user_id: str) -> str:
    """Get Confluence Display Name based on the account-id or userkey value

    Args:
        user_id (str): The user id (i.e: the account-id or userkey)
        confluence_client (Confluence): The Confluence Client

    Returns:
        str: The User Display Name. 'Unknown User' if the user is deactivated or not found
    """
    global _USER_ID_TO_DISPLAY_NAME_CACHE
    if _USER_ID_TO_DISPLAY_NAME_CACHE.get(user_id) is None:
        try:
            result = confluence_client.get_user_details_by_userkey(user_id)
            found_display_name = result.get("displayName")
        except Exception:
            found_display_name = None

        if not found_display_name:
            try:
                result = confluence_client.get_user_details_by_accountid(user_id)
                found_display_name = result.get("displayName")
            except Exception:
                found_display_name = None

        _USER_ID_TO_DISPLAY_NAME_CACHE[user_id] = found_display_name

    return _USER_ID_TO_DISPLAY_NAME_CACHE.get(user_id) or _USER_NOT_FOUND


def sanitize_attachment_title(title: str) -> str:
    """
    Sanitize the attachment title to be a valid HTML attribute.
    """
    return title.replace("<", "_").replace(">", "_").replace(" ", "_").replace(":", "_")


def extract_text_from_confluence_html(
    confluence_client: OnyxConfluence,
    confluence_object: dict[str, Any],
    fetched_titles: set[str],
) -> str:
    """Parse a Confluence html page and replace the 'user Id' by the real
        User Display Name

    Args:
        confluence_object (dict): The confluence object as a dict
        confluence_client (Confluence): Confluence client
        fetched_titles (set[str]): The titles of the pages that have already been fetched
    Returns:
        str: loaded and formated Confluence page
    """
    body = confluence_object["body"]
    object_html = body.get("storage", body.get("view", {})).get("value")

    soup = bs4.BeautifulSoup(object_html, "html.parser")

    _remove_macro_stylings(soup=soup)

    for user in soup.findAll("ri:user"):
        user_id = (
            user.attrs["ri:account-id"]
            if "ri:account-id" in user.attrs
            else user.get("ri:userkey")
        )
        if not user_id:
            logging.warning(
                "ri:userkey not found in ri:user element. " f"Found attrs: {user.attrs}"
            )
            continue
        # Include @ sign for tagging, more clear for LLM
        user.replaceWith("@" + _get_user(confluence_client, user_id))

    for html_page_reference in soup.findAll("ac:structured-macro"):
        # Here, we only want to process page within page macros
        if html_page_reference.attrs.get("ac:name") != "include":
            continue

        page_data = html_page_reference.find("ri:page")
        if not page_data:
            logging.warning(
                f"Skipping retrieval of {html_page_reference} because because page data is missing"
            )
            continue

        page_title = page_data.attrs.get("ri:content-title")
        if not page_title:
            # only fetch pages that have a title
            logging.warning(
                f"Skipping retrieval of {html_page_reference} because it has no title"
            )
            continue

        if page_title in fetched_titles:
            # prevent recursive fetching of pages
            logging.debug(f"Skipping {page_title} because it has already been fetched")
            continue

        fetched_titles.add(page_title)

        # Wrap this in a try-except because there are some pages that might not exist
        try:
            page_query = f"type=page and title='{quote(page_title)}'"

            page_contents: dict[str, Any] | None = None
            # Confluence enforces title uniqueness, so we should only get one result here
            for page in confluence_client.paginated_cql_retrieval(
                cql=page_query,
                expand="body.storage.value",
                limit=1,
            ):
                page_contents = page
                break
        except Exception as e:
            logging.warning(
                f"Error getting page contents for object {confluence_object}: {e}"
            )
            continue

        if not page_contents:
            continue

        text_from_page = extract_text_from_confluence_html(
            confluence_client=confluence_client,
            confluence_object=page_contents,
            fetched_titles=fetched_titles,
        )

        html_page_reference.replaceWith(text_from_page)

    for html_link_body in soup.findAll("ac:link-body"):
        # This extracts the text from inline links in the page so they can be
        # represented in the document text as plain text
        try:
            text_from_link = html_link_body.text
            html_link_body.replaceWith(f"(LINK TEXT: {text_from_link})")
        except Exception as e:
            logging.warning(f"Error processing ac:link-body: {e}")

    for html_attachment in soup.findAll("ri:attachment"):
        # This extracts the text from inline attachments in the page so they can be
        # represented in the document text as plain text
        try:
            html_attachment.replaceWith(
                f"<attachment>{sanitize_attachment_title(html_attachment.attrs['ri:filename'])}</attachment>"
            )  # to be replaced later
        except Exception as e:
            logging.warning(f"Error processing ac:attachment: {e}")

    return format_document_soup(soup)


def _remove_macro_stylings(soup: bs4.BeautifulSoup) -> None:
    for macro_root in soup.findAll("ac:structured-macro"):
        if not isinstance(macro_root, bs4.Tag):
            continue

        macro_styling = macro_root.find(name="ac:parameter", attrs={"ac:name": "page"})
        if not macro_styling or not isinstance(macro_styling, bs4.Tag):
            continue

        macro_styling.extract()


def get_page_restrictions(
    confluence_client: OnyxConfluence,
    page_id: str,
    page_restrictions: dict[str, Any],
    ancestors: list[dict[str, Any]],
) -> ExternalAccess | None:
    """
    Get page access restrictions for a Confluence page.
    This functionality requires Enterprise Edition.

    Args:
        confluence_client: OnyxConfluence client instance
        page_id: The ID of the page
        page_restrictions: Dictionary containing page restriction data
        ancestors: List of ancestor pages with their restriction data

    Returns:
        ExternalAccess object for the page. None if EE is not enabled or no restrictions found.
    """
    # Fetch the EE implementation
    """
    ee_get_all_page_restrictions = cast(
        Callable[
            [OnyxConfluence, str, dict[str, Any], list[dict[str, Any]]],
            ExternalAccess | None,
        ],
        fetch_versioned_implementation(
            "onyx.external_permissions.confluence.page_access", "get_page_restrictions"
        ),
    )

    return ee_get_all_page_restrictions(
        confluence_client, page_id, page_restrictions, ancestors
    )"""
    return {}


def get_all_space_permissions(
    confluence_client: OnyxConfluence,
    is_cloud: bool,
) -> dict[str, ExternalAccess]:
    """
    Get access permissions for all spaces in Confluence.
    This functionality requires Enterprise Edition.

    Args:
        confluence_client: OnyxConfluence client instance
        is_cloud: Whether this is a Confluence Cloud instance

    Returns:
        Dictionary mapping space keys to ExternalAccess objects. Empty dict if EE is not enabled.
    """
    """
    # Fetch the EE implementation
    ee_get_all_space_permissions = cast(
        Callable[
            [OnyxConfluence, bool],
            dict[str, ExternalAccess],
        ],
        fetch_versioned_implementation(
            "onyx.external_permissions.confluence.space_access",
            "get_all_space_permissions",
        ),
    )

    return ee_get_all_space_permissions(confluence_client, is_cloud)"""
    return {}


def _make_attachment_link(
    confluence_client: "OnyxConfluence",
    attachment: dict[str, Any],
    parent_content_id: str | None = None,
) -> str | None:
    download_link = ""

    if "api.atlassian.com" in confluence_client.url:
        # https://developer.atlassian.com/cloud/confluence/rest/v1/api-group-content---attachments/#api-wiki-rest-api-content-id-child-attachment-attachmentid-download-get
        if not parent_content_id:
            logging.warning(
                "parent_content_id is required to download attachments from Confluence Cloud!"
            )
            return None

        download_link = (
            confluence_client.url
            + f"/rest/api/content/{parent_content_id}/child/attachment/{attachment['id']}/download"
        )
    else:
        download_link = confluence_client.url + attachment["_links"]["download"]

    return download_link


def _process_image_attachment(
    confluence_client: "OnyxConfluence",
    attachment: dict[str, Any],
    raw_bytes: bytes,
    media_type: str,
) -> AttachmentProcessingResult:
    """Process an image attachment by saving it without generating a summary."""
    return AttachmentProcessingResult(text="", file_blob=raw_bytes, file_name=attachment.get("title", "unknown_title"), error=None)


def process_attachment(
    confluence_client: "OnyxConfluence",
    attachment: dict[str, Any],
    parent_content_id: str | None,
    allow_images: bool,
) -> AttachmentProcessingResult:
    """
    Processes a Confluence attachment. If it's a document, extracts text,
    or if it's an image, stores it for later analysis. Returns a structured result.
    """
    try:
        # Get the media type from the attachment metadata
        media_type: str = attachment.get("metadata", {}).get("mediaType", "")
        # Validate the attachment type
        if not validate_attachment_filetype(attachment):
            return AttachmentProcessingResult(
                text=None,
                file_blob=None,
                file_name=None,
                error=f"Unsupported file type: {media_type}",
            )

        attachment_link = _make_attachment_link(
            confluence_client, attachment, parent_content_id
        )
        if not attachment_link:
            return AttachmentProcessingResult(
                text=None, file_blob=None, file_name=None, error="Failed to make attachment link"
            )

        attachment_size = attachment["extensions"]["fileSize"]

        if media_type.startswith("image/"):
            if not allow_images:
                return AttachmentProcessingResult(
                    text=None,
                    file_blob=None,
                    file_name=None,
                    error="Image downloading is not enabled",
                )
        else:
            if attachment_size > CONFLUENCE_CONNECTOR_ATTACHMENT_SIZE_THRESHOLD:
                logging.warning(
                    f"Skipping {attachment_link} due to size. "
                    f"size={attachment_size} "
                    f"threshold={CONFLUENCE_CONNECTOR_ATTACHMENT_SIZE_THRESHOLD}"
                )
                return AttachmentProcessingResult(
                    text=None,
                    file_blob=None,
                    file_name=None,
                    error=f"Attachment text too long: {attachment_size} chars",
                )

        logging.info(
            f"Downloading attachment: "
            f"title={attachment['title']} "
            f"length={attachment_size} "
            f"link={attachment_link}"
        )

        # Download the attachment
        resp: requests.Response = confluence_client._session.get(attachment_link)
        if resp.status_code != 200:
            logging.warning(
                f"Failed to fetch {attachment_link} with status code {resp.status_code}"
            )
            return AttachmentProcessingResult(
                text=None,
                file_blob=None,
                file_name=None,
                error=f"Attachment download status code is {resp.status_code}",
            )

        raw_bytes = resp.content
        if not raw_bytes:
            return AttachmentProcessingResult(
                text=None, file_blob=None, file_name=None, error="attachment.content is None"
            )

        # Process image attachments
        if media_type.startswith("image/"):
            return _process_image_attachment(
                confluence_client, attachment, raw_bytes, media_type
            )

        # Process document attachments
        try:
            return AttachmentProcessingResult(text="",file_blob=raw_bytes, file_name=attachment.get("title", "unknown_title"), error=None)
        except Exception as e:
            logging.exception(e)
            return AttachmentProcessingResult(
                text=None, file_blob=None, file_name=None, error=f"Failed to extract text: {e}"
            )

    except Exception as e:
        return AttachmentProcessingResult(
            text=None, file_blob=None, file_name=None, error=f"Failed to process attachment: {e}"
        )


def convert_attachment_to_content(
    confluence_client: "OnyxConfluence",
    attachment: dict[str, Any],
    page_id: str,
    allow_images: bool,
) -> tuple[str | None, bytes | bytearray | None] | None:
    """
    Facade function which:
      1. Validates attachment type
      2. Extracts content or stores image for later processing
      3. Returns (content_text, stored_file_name) or None if we should skip it
    """
    media_type = attachment.get("metadata", {}).get("mediaType", "")
    # Quick check for unsupported types:
    if media_type.startswith("video/") or media_type == "application/gliffy+json":
        logging.warning(
            f"Skipping unsupported attachment type: '{media_type}' for {attachment['title']}"
        )
        return None

    result = process_attachment(confluence_client, attachment, page_id, allow_images)
    if result.error is not None:
        logging.warning(
            f"Attachment {attachment['title']} encountered error: {result.error}"
        )
        return None

    return result.file_name, result.file_blob


class ConfluenceConnector(
    CheckpointedConnector[ConfluenceCheckpoint],
    SlimConnector,
    SlimConnectorWithPermSync,
    CredentialsConnector,
):
    def __init__(
        self,
        wiki_base: str,
        is_cloud: bool,
        space: str = "",
        page_id: str = "",
        index_recursively: bool = False,
        cql_query: str | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
        continue_on_failure: bool = CONTINUE_ON_CONNECTOR_FAILURE,
        # if a page has one of the labels specified in this list, we will just
        # skip it. This is generally used to avoid indexing extra sensitive
        # pages.
        labels_to_skip: list[str] = CONFLUENCE_CONNECTOR_LABELS_TO_SKIP,
        timezone_offset: float = CONFLUENCE_TIMEZONE_OFFSET,
        time_buffer_seconds: int = CONFLUENCE_SYNC_TIME_BUFFER_SECONDS,
        scoped_token: bool = False,
    ) -> None:
        self.wiki_base = wiki_base
        self.is_cloud = is_cloud
        self.space = space
        self.page_id = page_id
        self.index_recursively = index_recursively
        self.cql_query = cql_query
        self.batch_size = batch_size
        self.labels_to_skip = labels_to_skip
        self.timezone_offset = timezone_offset
        self.time_buffer_seconds = max(0, time_buffer_seconds)
        self.scoped_token = scoped_token
        self._confluence_client: OnyxConfluence | None = None
        self._low_timeout_confluence_client: OnyxConfluence | None = None
        self._fetched_titles: set[str] = set()
        self.allow_images = False

        # Remove trailing slash from wiki_base if present
        self.wiki_base = wiki_base.rstrip("/")
        """
        If nothing is provided, we default to fetching all pages
        Only one or none of the following options should be specified so
            the order shouldn't matter
        However, we use elif to ensure that only of the following is enforced
        """
        base_cql_page_query = "type=page"
        if cql_query:
            base_cql_page_query = cql_query
        elif page_id:
            if index_recursively:
                base_cql_page_query += f" and (ancestor='{page_id}' or id='{page_id}')"
            else:
                base_cql_page_query += f" and id='{page_id}'"
        elif space:
            uri_safe_space = quote(space)
            base_cql_page_query += f" and space='{uri_safe_space}'"

        self.base_cql_page_query = base_cql_page_query

        self.cql_label_filter = ""
        if labels_to_skip:
            labels_to_skip = list(set(labels_to_skip))
            comma_separated_labels = ",".join(
                f"'{quote(label)}'" for label in labels_to_skip
            )
            self.cql_label_filter = f" and label not in ({comma_separated_labels})"

        self.timezone: timezone = timezone(offset=timedelta(hours=timezone_offset))
        self.credentials_provider: CredentialsProviderInterface | None = None

        self.probe_kwargs = {
            "max_backoff_retries": 6,
            "max_backoff_seconds": 10,
        }

        self.final_kwargs = {
            "max_backoff_retries": 10,
            "max_backoff_seconds": 60,
        }

        # deprecated
        self.continue_on_failure = continue_on_failure

    def set_allow_images(self, value: bool) -> None:
        logging.info(f"Setting allow_images to {value}.")
        self.allow_images = value

    def _adjust_start_for_query(
        self, start: SecondsSinceUnixEpoch | None
    ) -> SecondsSinceUnixEpoch | None:
        if not start or start <= 0:
            return start
        if self.time_buffer_seconds <= 0:
            return start
        return max(0.0, start - self.time_buffer_seconds)

    def _is_newer_than_start(
        self, doc_time: datetime | None, start: SecondsSinceUnixEpoch | None
    ) -> bool:
        if not start or start <= 0:
            return True
        if doc_time is None:
            return True
        return doc_time.timestamp() > start

    @property
    def confluence_client(self) -> OnyxConfluence:
        if self._confluence_client is None:
            raise ConnectorMissingCredentialError("Confluence")
        return self._confluence_client

    @property
    def low_timeout_confluence_client(self) -> OnyxConfluence:
        if self._low_timeout_confluence_client is None:
            raise ConnectorMissingCredentialError("Confluence")
        return self._low_timeout_confluence_client

    def set_credentials_provider(
        self, credentials_provider: CredentialsProviderInterface
    ) -> None:
        self.credentials_provider = credentials_provider

        # raises exception if there's a problem
        confluence_client = OnyxConfluence(
            is_cloud=self.is_cloud,
            url=self.wiki_base,
            credentials_provider=credentials_provider,
            scoped_token=self.scoped_token,
        )
        confluence_client._probe_connection(**self.probe_kwargs)
        confluence_client._initialize_connection(**self.final_kwargs)

        self._confluence_client = confluence_client

        # create a low timeout confluence client for sync flows
        low_timeout_confluence_client = OnyxConfluence(
            is_cloud=self.is_cloud,
            url=self.wiki_base,
            credentials_provider=credentials_provider,
            timeout=3,
            scoped_token=self.scoped_token,
        )
        low_timeout_confluence_client._probe_connection(**self.probe_kwargs)
        low_timeout_confluence_client._initialize_connection(**self.final_kwargs)

        self._low_timeout_confluence_client = low_timeout_confluence_client

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        raise NotImplementedError("Use set_credentials_provider with this connector.")

    def _construct_page_cql_query(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> str:
        """
        Constructs a CQL query for use in the confluence API. See
        https://developer.atlassian.com/server/confluence/advanced-searching-using-cql/
        for more information. This is JUST the CQL, not the full URL used to hit the API.
        Use _build_page_retrieval_url to get the full URL.
        """
        page_query = self.base_cql_page_query + self.cql_label_filter
        # Add time filters
        query_start = self._adjust_start_for_query(start)
        if query_start:
            formatted_start_time = datetime.fromtimestamp(
                query_start, tz=self.timezone
            ).strftime("%Y-%m-%d %H:%M")
            page_query += f" and lastmodified >= '{formatted_start_time}'"
        if end:
            formatted_end_time = datetime.fromtimestamp(end, tz=self.timezone).strftime(
                "%Y-%m-%d %H:%M"
            )
            page_query += f" and lastmodified <= '{formatted_end_time}'"

        page_query += " order by lastmodified asc"
        return page_query

    def _construct_attachment_query(
        self,
        confluence_page_id: str,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> str:
        attachment_query = f"type=attachment and container='{confluence_page_id}'"
        attachment_query += self.cql_label_filter

        # Add time filters to avoid reprocessing unchanged attachments during refresh
        query_start = self._adjust_start_for_query(start)
        if query_start:
            formatted_start_time = datetime.fromtimestamp(
                query_start, tz=self.timezone
            ).strftime("%Y-%m-%d %H:%M")
            attachment_query += f" and lastmodified >= '{formatted_start_time}'"
        if end:
            formatted_end_time = datetime.fromtimestamp(end, tz=self.timezone).strftime(
                "%Y-%m-%d %H:%M"
            )
            attachment_query += f" and lastmodified <= '{formatted_end_time}'"

        attachment_query += " order by lastmodified asc"
        return attachment_query

    def _get_comment_string_for_page_id(self, page_id: str) -> str:
        comment_string = ""
        comment_cql = f"type=comment and container='{page_id}'"
        comment_cql += self.cql_label_filter
        expand = ",".join(_COMMENT_EXPANSION_FIELDS)

        for comment in self.confluence_client.paginated_cql_retrieval(
            cql=comment_cql,
            expand=expand,
        ):
            comment_string += "\nComment:\n"
            comment_string += extract_text_from_confluence_html(
                confluence_client=self.confluence_client,
                confluence_object=comment,
                fetched_titles=set(),
            )
        return comment_string

    def _convert_page_to_document(
        self, page: dict[str, Any]
    ) -> Document | ConnectorFailure:
        """
        Converts a Confluence page to a Document object.
        Includes the page content, comments, and attachments.
        """
        page_id = page_url = ""
        try:
            # Extract basic page information
            page_id = page["id"]
            page_title = page["title"]
            logging.info(f"Converting page {page_title} to document")
            page_url = build_confluence_document_id(
                self.wiki_base, page["_links"]["webui"], self.is_cloud
            )

            # Get the page content
            page_content = extract_text_from_confluence_html(
                self.confluence_client, page, self._fetched_titles
            )

            # Create the main section for the page content
            sections: list[TextSection | ImageSection] = [
                TextSection(text=page_content, link=page_url)
            ]

            # Process comments if available
            comment_text = self._get_comment_string_for_page_id(page_id)
            if comment_text:
                sections.append(
                    TextSection(text=comment_text, link=f"{page_url}#comments")
                )
            # Note: attachments are no longer merged into the page document.
            # They are indexed as separate documents downstream.

            # Extract metadata
            metadata = {}
            if "space" in page:
                metadata["space"] = page["space"].get("name", "")

            # Extract labels
            labels = []
            if "metadata" in page and "labels" in page["metadata"]:
                for label in page["metadata"]["labels"].get("results", []):
                    labels.append(label.get("name", ""))
            if labels:
                metadata["labels"] = labels

            # Extract owners
            primary_owners = []
            if "version" in page and "by" in page["version"]:
                author = page["version"]["by"]
                display_name = author.get("displayName", "Unknown")
                email = author.get("email", "unknown@domain.invalid")
                primary_owners.append(
                    BasicExpertInfo(display_name=display_name, email=email)
                )

            # Create the document
            return Document(
                id=page_url,
                source=DocumentSource.CONFLUENCE,
                semantic_identifier=page_title,
                extension=".html",  # Confluence pages are HTML
                blob=page_content.encode("utf-8"),  # Encode page content as bytes
                size_bytes=len(page_content.encode("utf-8")),  # Calculate size in bytes
                doc_updated_at=datetime_from_string(page["version"]["when"]),
                primary_owners=primary_owners if primary_owners else None,
            )
        except Exception as e:
            logging.error(f"Error converting page {page.get('id', 'unknown')}: {e}")
            if is_atlassian_date_error(e):  # propagate error to be caught and retried
                raise
            return ConnectorFailure(
                failed_document=DocumentFailure(
                    document_id=page_id,
                    document_link=page_url,
                ),
                failure_message=f"Error converting page {page.get('id', 'unknown')}: {e}",
                exception=e,
            )

    def _fetch_page_attachments(
        self,
        page: dict[str, Any],
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> tuple[list[Document], list[ConnectorFailure]]:
        """
        Inline attachments are added directly to the document as text or image sections by
        this function. The returned documents/connectorfailures are for non-inline attachments
        and those at the end of the page.
        """
        attachment_query = self._construct_attachment_query(page["id"], start, end)
        attachment_failures: list[ConnectorFailure] = []
        attachment_docs: list[Document] = []
        page_url = ""

        for attachment in self.confluence_client.paginated_cql_retrieval(
            cql=attachment_query,
            expand=",".join(_ATTACHMENT_EXPANSION_FIELDS),
        ):
            media_type: str = attachment.get("metadata", {}).get("mediaType", "")

            # TODO(rkuo): this check is partially redundant with validate_attachment_filetype
            # and checks in convert_attachment_to_content/process_attachment
            # but doing the check here avoids an unnecessary download. Due for refactoring.
            if not self.allow_images:
                if media_type.startswith("image/"):
                    logging.info(
                        f"Skipping attachment because allow images is False: {attachment['title']}"
                    )
                    continue

            if not validate_attachment_filetype(
                attachment,
            ):
                logging.info(
                    f"Skipping attachment because it is not an accepted file type: {attachment['title']}"
                )
                continue


            logging.info(
                f"Processing attachment: {attachment['title']} attached to page {page['title']}"
            )
            # Attachment document id: use the download URL for stable identity
            try:
                object_url = build_confluence_document_id(
                    self.wiki_base, attachment["_links"]["download"], self.is_cloud
                )
            except Exception as e:
                logging.warning(
                    f"Invalid attachment url for id {attachment['id']}, skipping"
                )
                logging.debug(f"Error building attachment url: {e}")
                continue
            try:
                response = convert_attachment_to_content(
                    confluence_client=self.confluence_client,
                    attachment=attachment,
                    page_id=page["id"],
                    allow_images=self.allow_images,
                )
                if response is None:
                    continue

                file_storage_name, file_blob = response

                if not file_blob:
                    logging.info("Skipping attachment because it is no blob fetched")
                    continue

                # Build attachment-specific metadata
                attachment_metadata: dict[str, str | list[str]] = {}
                if "space" in attachment:
                    attachment_metadata["space"] = attachment["space"].get("name", "")
                labels: list[str] = []
                if "metadata" in attachment and "labels" in attachment["metadata"]:
                    for label in attachment["metadata"]["labels"].get("results", []):
                        labels.append(label.get("name", ""))
                if labels:
                    attachment_metadata["labels"] = labels
                page_url = page_url or build_confluence_document_id(
                    self.wiki_base, page["_links"]["webui"], self.is_cloud
                )
                attachment_metadata["parent_page_id"] = page_url
                attachment_id = build_confluence_document_id(
                    self.wiki_base, attachment["_links"]["webui"], self.is_cloud
                )

                primary_owners: list[BasicExpertInfo] | None = None
                if "version" in attachment and "by" in attachment["version"]:
                    author = attachment["version"]["by"]
                    display_name = author.get("displayName", "Unknown")
                    email = author.get("email", "unknown@domain.invalid")
                    primary_owners = [
                        BasicExpertInfo(display_name=display_name, email=email)
                    ]

                extension = Path(attachment.get("title", "")).suffix or ".unknown"

                attachment_doc = Document(
                    id=attachment_id,
                    # sections=sections,
                    source=DocumentSource.CONFLUENCE,
                    semantic_identifier=attachment.get("title", object_url),
                    extension=extension,
                    blob=file_blob,
                    size_bytes=len(file_blob),
                    metadata=attachment_metadata,
                    doc_updated_at=(
                        datetime_from_string(attachment["version"]["when"])
                        if attachment.get("version")
                        and attachment["version"].get("when")
                        else None
                    ),
                    primary_owners=primary_owners,
                )
                if self._is_newer_than_start(attachment_doc.doc_updated_at, start):
                    attachment_docs.append(attachment_doc)
            except Exception as e:
                logging.error(
                    f"Failed to extract/summarize attachment {attachment['title']}",
                    exc_info=e,
                )
                if is_atlassian_date_error(e):
                    # propagate error to be caught and retried
                    raise
                attachment_failures.append(
                    ConnectorFailure(
                        failed_document=DocumentFailure(
                            document_id=object_url,
                            document_link=object_url,
                        ),
                        failure_message=f"Failed to extract/summarize attachment {attachment['title']} for doc {object_url}",
                        exception=e,
                    )
                )

        return attachment_docs, attachment_failures

    def _fetch_document_batches(
        self,
        checkpoint: ConfluenceCheckpoint,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
    ) -> CheckpointOutput[ConfluenceCheckpoint]:
        """
        Yields batches of Documents. For each page:
         - Create a Document with 1 Section for the page text/comments
         - Then fetch attachments. For each attachment:
             - Attempt to convert it with convert_attachment_to_content(...)
             - If successful, create a new Section with the extracted text or summary.
        """
        checkpoint = copy.deepcopy(checkpoint)

        # use "start" when last_updated is 0 or for confluence server
        start_ts = start
        page_query_url = checkpoint.next_page_url or self._build_page_retrieval_url(
            start_ts, end, self.batch_size
        )
        logging.debug(f"page_query_url: {page_query_url}")

        # store the next page start for confluence server, cursor for confluence cloud
        def store_next_page_url(next_page_url: str) -> None:
            checkpoint.next_page_url = next_page_url

        for page in self.confluence_client.paginated_page_retrieval(
            cql_url=page_query_url,
            limit=self.batch_size,
            next_page_callback=store_next_page_url,
        ):
            # Build doc from page
            doc_or_failure = self._convert_page_to_document(page)

            if isinstance(doc_or_failure, ConnectorFailure):
                yield doc_or_failure
                continue

            # yield completed document (or failure)
            if self._is_newer_than_start(doc_or_failure.doc_updated_at, start):
                yield doc_or_failure

            # Now get attachments for that page:
            attachment_docs, attachment_failures = self._fetch_page_attachments(
                page, start, end
            )
            # yield attached docs and failures
            yield from attachment_docs
            # yield from attachment_failures

            # Create checkpoint once a full page of results is returned
            if checkpoint.next_page_url and checkpoint.next_page_url != page_query_url:
                return checkpoint

        checkpoint.has_more = False
        return checkpoint

    def _build_page_retrieval_url(
        self,
        start: SecondsSinceUnixEpoch | None,
        end: SecondsSinceUnixEpoch | None,
        limit: int,
    ) -> str:
        """
        Builds the full URL used to retrieve pages from the confluence API.
        This can be used as input to the confluence client's _paginate_url
        or paginated_page_retrieval methods.
        """
        page_query = self._construct_page_cql_query(start, end)
        cql_url = self.confluence_client.build_cql_url(
            page_query, expand=",".join(_PAGE_EXPANSION_FIELDS)
        )
        logging.info(f"[Confluence Connector] Building CQL URL {cql_url}")
        return update_param_in_path(cql_url, "limit", str(limit))

    @override
    def load_from_checkpoint(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
        checkpoint: ConfluenceCheckpoint,
    ) -> CheckpointOutput[ConfluenceCheckpoint]:
        end += ONE_DAY  # handle time zone weirdness
        try:
            return self._fetch_document_batches(checkpoint, start, end)
        except Exception as e:
            if is_atlassian_date_error(e) and start is not None:
                logging.warning(
                    "Confluence says we provided an invalid 'updated' field. This may indicate"
                    "a real issue, but can also appear during edge cases like daylight"
                    f"savings time changes. Retrying with a 1 hour offset. Error: {e}"
                )
                return self._fetch_document_batches(checkpoint, start - ONE_HOUR, end)
            raise

    @override
    def build_dummy_checkpoint(self) -> ConfluenceCheckpoint:
        return ConfluenceCheckpoint(has_more=True, next_page_url=None)

    @override
    def validate_checkpoint_json(self, checkpoint_json: str) -> ConfluenceCheckpoint:
        return ConfluenceCheckpoint.model_validate_json(checkpoint_json)

    @override
    def retrieve_all_slim_docs(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: IndexingHeartbeatInterface | None = None,
    ) -> GenerateSlimDocumentOutput:
        return self._retrieve_all_slim_docs(
            start=start,
            end=end,
            callback=callback,
            include_permissions=False,
        )

    def retrieve_all_slim_docs_perm_sync(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: IndexingHeartbeatInterface | None = None,
    ) -> GenerateSlimDocumentOutput:
        """
        Return 'slim' docs (IDs + minimal permission data).
        Does not fetch actual text. Used primarily for incremental permission sync.
        """
        return self._retrieve_all_slim_docs(
            start=start,
            end=end,
            callback=callback,
            include_permissions=True,
        )

    def _retrieve_all_slim_docs(
        self,
        start: SecondsSinceUnixEpoch | None = None,
        end: SecondsSinceUnixEpoch | None = None,
        callback: IndexingHeartbeatInterface | None = None,
        include_permissions: bool = True,
    ) -> GenerateSlimDocumentOutput:
        doc_metadata_list: list[SlimDocument] = []
        restrictions_expand = ",".join(_RESTRICTIONS_EXPANSION_FIELDS)

        space_level_access_info: dict[str, ExternalAccess] = {}
        if include_permissions:
            space_level_access_info = get_all_space_permissions(
                self.confluence_client, self.is_cloud
            )

        def get_external_access(
            doc_id: str, restrictions: dict[str, Any], ancestors: list[dict[str, Any]]
        ) -> ExternalAccess | None:
            return get_page_restrictions(
                self.confluence_client, doc_id, restrictions, ancestors
            ) or space_level_access_info.get(page_space_key)

        # Query pages
        page_query = self.base_cql_page_query + self.cql_label_filter
        for page in self.confluence_client.cql_paginate_all_expansions(
            cql=page_query,
            expand=restrictions_expand,
            limit=_SLIM_DOC_BATCH_SIZE,
        ):
            page_id = page["id"]
            page_restrictions = page.get("restrictions") or {}
            page_space_key = page.get("space", {}).get("key")
            page_ancestors = page.get("ancestors", [])

            page_id = build_confluence_document_id(
                self.wiki_base, page["_links"]["webui"], self.is_cloud
            )
            doc_metadata_list.append(
                SlimDocument(
                    id=page_id,
                    external_access=(
                        get_external_access(page_id, page_restrictions, page_ancestors)
                        if include_permissions
                        else None
                    ),
                )
            )

            # Query attachments for each page
            attachment_query = self._construct_attachment_query(page["id"])
            for attachment in self.confluence_client.cql_paginate_all_expansions(
                cql=attachment_query,
                expand=restrictions_expand,
                limit=_SLIM_DOC_BATCH_SIZE,
            ):
                # If you skip images, you'll skip them in the permission sync
                attachment["metadata"].get("mediaType", "")
                if not validate_attachment_filetype(
                    attachment,
                ):
                    continue

                attachment_restrictions = attachment.get("restrictions", {})
                if not attachment_restrictions:
                    attachment_restrictions = page_restrictions or {}

                attachment_space_key = attachment.get("space", {}).get("key")
                if not attachment_space_key:
                    attachment_space_key = page_space_key

                attachment_id = build_confluence_document_id(
                    self.wiki_base,
                    attachment["_links"]["webui"],
                    self.is_cloud,
                )
                doc_metadata_list.append(
                    SlimDocument(
                        id=attachment_id,
                        external_access=(
                            get_external_access(
                                attachment_id, attachment_restrictions, []
                            )
                            if include_permissions
                            else None
                        ),
                    )
                )

            if len(doc_metadata_list) > _SLIM_DOC_BATCH_SIZE:
                yield doc_metadata_list[:_SLIM_DOC_BATCH_SIZE]
                doc_metadata_list = doc_metadata_list[_SLIM_DOC_BATCH_SIZE:]

                if callback and callback.should_stop():
                    raise RuntimeError(
                        "retrieve_all_slim_docs_perm_sync: Stop signal detected"
                    )
                if callback:
                    callback.progress("retrieve_all_slim_docs_perm_sync", 1)

        yield doc_metadata_list

    def validate_connector_settings(self) -> None:
        try:
            spaces = self.low_timeout_confluence_client.get_all_spaces(limit=1)
        except HTTPError as e:
            status_code = e.response.status_code if e.response else None
            if status_code == 401:
                raise CredentialExpiredError(
                    "Invalid or expired Confluence credentials (HTTP 401)."
                )
            elif status_code == 403:
                raise InsufficientPermissionsError(
                    "Insufficient permissions to access Confluence resources (HTTP 403)."
                )
            raise UnexpectedValidationError(
                f"Unexpected Confluence error (status={status_code}): {e}"
            )
        except Exception as e:
            raise UnexpectedValidationError(
                f"Unexpected error while validating Confluence settings: {e}"
            )

        if self.space:
            try:
                self.low_timeout_confluence_client.get_space(self.space)
            except ApiError as e:
                raise ConnectorValidationError(
                    "Invalid Confluence space key provided"
                ) from e

        if not spaces or not spaces.get("results"):
            raise ConnectorValidationError(
                "No Confluence spaces found. Either your credentials lack permissions, or "
                "there truly are no spaces in this Confluence instance."
            )



if __name__ == "__main__":
    import os

    # base url
    wiki_base = os.environ["CONFLUENCE_URL"]

    # auth stuff
    username = os.environ["CONFLUENCE_USERNAME"]
    access_token = os.environ["CONFLUENCE_ACCESS_TOKEN"]
    is_cloud = os.environ["CONFLUENCE_IS_CLOUD"].lower() == "true"

    # space + page
    space = os.environ["CONFLUENCE_SPACE_KEY"]
    # page_id = os.environ["CONFLUENCE_PAGE_ID"]

    confluence_connector = ConfluenceConnector(
        wiki_base=wiki_base,
        space=space,
        is_cloud=is_cloud,
        # page_id=page_id,
    )

    credentials_provider = StaticCredentialsProvider(
        None,
        DocumentSource.CONFLUENCE,
        {
            "confluence_username": username,
            "confluence_access_token": access_token,
        },
    )
    confluence_connector.set_credentials_provider(credentials_provider)

    start = 0.0
    end = datetime.now().timestamp()

    # Fetch all `SlimDocuments`.
    for slim_doc in confluence_connector.retrieve_all_slim_docs_perm_sync():
        print(slim_doc)

    # Fetch all `Documents`.
    for doc in load_all_docs_from_checkpoint_connector(
        connector=confluence_connector,
        start=start,
        end=end,
    ):
        print(doc)
