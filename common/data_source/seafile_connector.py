"""SeaFile connector with granular sync support"""
import logging
from datetime import datetime, timezone
from typing import Any, Optional

from retry import retry

from common.data_source.utils import (
    get_file_ext,
    rl_requests,
)
from common.data_source.config import (
    DocumentSource,
    INDEX_BATCH_SIZE,
    BLOB_STORAGE_SIZE_THRESHOLD,
)
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
    CredentialExpiredError,
    InsufficientPermissionsError,
)
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import (
    Document,
    SecondsSinceUnixEpoch,
    GenerateDocumentsOutput,
    SeafileSyncScope,
)

logger = logging.getLogger(__name__)

class SeaFileConnector(LoadConnector, PollConnector):
    """SeaFile connector supporting account-, library- and directory-level sync.

    API endpoints used:
        Account token (api2):
            GET  /api2/account/info/
            GET  /api2/repos/
            GET  /api2/repos/{repo_id}/
            GET  /api2/repos/{repo_id}/dir/?p=...
            GET  /api2/repos/{repo_id}/file/?p=...&reuse=1

        Repo token (api/v2.1/via-repo-token):
            GET  /api/v2.1/via-repo-token/repo-info/
            GET  /api/v2.1/via-repo-token/dir/?path=...
            GET  /api/v2.1/via-repo-token/download-link/?path=...
    """

    def __init__(
        self,
        seafile_url: str,
        batch_size: int = INDEX_BATCH_SIZE,
        include_shared: bool = True,
        sync_scope: str = SeafileSyncScope.ACCOUNT,
        repo_id: Optional[str] = None,
        sync_path: Optional[str] = None,
    ) -> None:
        self.seafile_url = seafile_url.rstrip("/")
        self.batch_size = batch_size
        self.include_shared = include_shared
        self.sync_scope = SeafileSyncScope(sync_scope)
        self.repo_id = repo_id
        self.sync_path = self._normalise_path(sync_path)

        self.token: Optional[str] = None          # account-level
        self.repo_token: Optional[str] = None     # library-scoped
        self.current_user_email: Optional[str] = None
        self.size_threshold: int = BLOB_STORAGE_SIZE_THRESHOLD

        self._validate_scope_params()

  
    @staticmethod
    def _normalise_path(path: Optional[str]) -> str:
        if not path:
            return "/"
        path = path.strip()
        if not path.startswith("/"):
            path = f"/{path}"
        return path.rstrip("/") or "/"

    @staticmethod
    def _parse_mtime(raw_mtime) -> datetime:
        """Parse mtime from SeaFile API response.

        Handles:
            - Unix timestamp as int:  1575514722
            - Unix timestamp as str:  "1575514722"
            - ISO 8601 datetime str:  "2026-02-15T17:26:53+01:00"
            - None / missing
        """
        if not raw_mtime:
            return datetime.now(timezone.utc)

        # Try as unix timestamp (int or numeric string)
        if isinstance(raw_mtime, (int, float)):
            return datetime.fromtimestamp(raw_mtime, tz=timezone.utc)

        if isinstance(raw_mtime, str):
            # Try numeric string first
            try:
                return datetime.fromtimestamp(int(raw_mtime), tz=timezone.utc)
            except ValueError:
                pass

            # Try ISO 8601
            try:
                return datetime.fromisoformat(raw_mtime)
            except ValueError:
                pass

        logger.warning("Unparseable mtime %r, using current time", raw_mtime)
        return datetime.now(timezone.utc)
    
    def _validate_scope_params(self) -> None:
        if self.sync_scope in (SeafileSyncScope.LIBRARY, SeafileSyncScope.DIRECTORY):
            if not self.repo_id:
                raise ConnectorValidationError(
                    f"sync_scope={self.sync_scope.value!r} requires 'repo_id'."
                )
        if self.sync_scope == SeafileSyncScope.DIRECTORY:
            if self.sync_path == "/":
                raise ConnectorValidationError(
                    "sync_scope='directory' requires a non-root 'sync_path'. "
                    "Use sync_scope='library' to sync an entire library."
                )

    @property
    def _use_repo_token(self) -> bool:
        """Whether we should use repo-token endpoints."""
        return self.repo_token is not None


    def _account_headers(self) -> dict[str, str]:
        if not self.token:
            raise ConnectorMissingCredentialError("Account token not set")
        return {
            "Authorization": f"Token {self.token}",
            "Accept": "application/json",
        }

    def _repo_token_headers(self) -> dict[str, str]:
        if not self.repo_token:
            raise ConnectorMissingCredentialError("Repo token not set")
        return {
            "Authorization": f"Bearer {self.repo_token}",  # <-- Bearer, not Token
            "Accept": "application/json",
        }

    def _account_get(self, endpoint: str, params: Optional[dict] = None):
        """GET against /api2/... using the account token."""
        url = f"{self.seafile_url}/api2/{endpoint.lstrip('/')}"
        resp = rl_requests.get(
            url, headers=self._account_headers(), params=params, timeout=60,
        )
        return resp

    def _repo_token_get(self, endpoint: str, params: Optional[dict] = None):
        """GET against /api/v2.1/via-repo-token/... using the repo token."""
        url = f"{self.seafile_url}/api/v2.1/via-repo-token/{endpoint.lstrip('/')}"
        resp = rl_requests.get(
            url, headers=self._repo_token_headers(), params=params, timeout=60,
        )
        return resp


    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        logger.debug("Loading credentials for SeaFile server %s", self.seafile_url)

        token = credentials.get("seafile_token")
        repo_token = credentials.get("repo_token")
        username = credentials.get("username")
        password = credentials.get("password")

        if token:
            self.token = token
        elif username and password:
            self.token = self._authenticate_with_password(username, password)

        if repo_token and self.sync_scope in (SeafileSyncScope.LIBRARY, SeafileSyncScope.DIRECTORY):
            self.repo_token = repo_token
        elif repo_token:
            logger.debug(
                "repo_token supplied but scope=%s; ignoring.",
                self.sync_scope.value,
            )

        if not self.token and not self.repo_token:
            raise ConnectorMissingCredentialError(
                "SeaFile requires 'seafile_token', 'repo_token', "
                "or 'username'/'password'."
            )

        try:
            self._validate_credentials()
        except ConnectorMissingCredentialError:
            raise
        except Exception as e:
            raise CredentialExpiredError(
                f"SeaFile credential validation failed: {e}"
            )

        return None

    def _authenticate_with_password(self, username: str, password: str) -> str:
        try:
            resp = rl_requests.post(
                f"{self.seafile_url}/api2/auth-token/",
                data={"username": username, "password": password},
                timeout=30,
            )
            resp.raise_for_status()
            token = resp.json().get("token")
            if not token:
                raise CredentialExpiredError("No token returned")
            return token
        except Exception as e:
            raise ConnectorMissingCredentialError(
                f"Failed to authenticate with SeaFile: {e}"
            )

    def _validate_credentials(self) -> None:
        if self.token:
            self._validate_account_token()

        if self.repo_token:
            self._validate_repo_token()
        elif self.sync_scope in (SeafileSyncScope.LIBRARY, SeafileSyncScope.DIRECTORY):
            self._validate_repo_access_via_account()

    def _validate_account_token(self) -> dict:
        resp = self._account_get("/account/info/")
        resp.raise_for_status()
        info = resp.json()
        self.current_user_email = info.get("email")
        logger.info("SeaFile authenticated as: %s", self.current_user_email)
        return info

    def _validate_repo_token(self) -> None:
        """Validate repo token using /api/v2.1/via-repo-token/repo-info/"""
        try:
            resp = self._repo_token_get("repo-info/")
            resp.raise_for_status()
            info = resp.json()
            logger.info(
                "Repo token validated — library: %s (id: %s)",
                info.get("repo_name", "?"), info.get("repo_id", self.repo_id),
            )
            # Update repo_id from response if not set
            if not self.repo_id and info.get("repo_id"):
                self.repo_id = info["repo_id"]
        except Exception as e:
            raise CredentialExpiredError(
                f"Repo token validation failed: {e}"
            )

    def _validate_repo_access_via_account(self) -> None:
        repo_info = self._get_repo_info_via_account(self.repo_id)
        if not repo_info:
            raise ConnectorValidationError(
                f"Library {self.repo_id} not accessible with account token."
            )
        if self.sync_scope == SeafileSyncScope.DIRECTORY:
            entries = self._get_directory_entries(self.repo_id, self.sync_path)
            if entries is None:
                raise ConnectorValidationError(
                    f"Directory {self.sync_path!r} does not exist "
                    f"in library {self.repo_id}."
                )


    def validate_connector_settings(self) -> None:
        if not self.token and not self.repo_token:
            raise ConnectorMissingCredentialError("SeaFile credentials not loaded.")
        if not self.seafile_url:
            raise ConnectorValidationError("No SeaFile URL was provided.")

        try:
            if self.sync_scope == SeafileSyncScope.ACCOUNT:
                libs = self._get_libraries()
                logger.info("Validated (account scope). %d libraries.", len(libs))
            elif self.sync_scope == SeafileSyncScope.LIBRARY:
                info = self._get_repo_info()
                logger.info(
                    "Validated (library scope): %s", info.get("name", self.repo_id)
                )
            elif self.sync_scope == SeafileSyncScope.DIRECTORY:
                entries = self._get_directory_entries(self.repo_id, self.sync_path)
                logger.info(
                    "Validated (directory scope): %s:%s (%d entries)",
                    self.repo_id, self.sync_path, len(entries),
                )
        except (
            ConnectorValidationError, ConnectorMissingCredentialError,
            CredentialExpiredError, InsufficientPermissionsError,
        ):
            raise
        except Exception as e:
            status = getattr(getattr(e, "response", None), "status_code", None)
            if status == 401:
                raise CredentialExpiredError("Token invalid or expired.")
            if status == 403:
                raise InsufficientPermissionsError("Insufficient permissions.")
            raise ConnectorValidationError(f"Validation failed: {repr(e)}")


    @retry(tries=3, delay=1, backoff=2)
    def _get_libraries(self) -> list[dict]:
        """List all libraries (account token only)."""
        resp = self._account_get("/repos/")
        resp.raise_for_status()
        libraries = resp.json()

        if not self.include_shared and self.current_user_email:
            libraries = [
                lib for lib in libraries
                if lib.get("owner") == self.current_user_email
                or lib.get("owner_email") == self.current_user_email
            ]

        return libraries

    @retry(tries=3, delay=1, backoff=2)
    def _get_repo_info_via_account(self, repo_id: str) -> Optional[dict]:
        """GET /api2/repos/{repo_id}/ — account token."""
        try:
            resp = self._account_get(f"/repos/{repo_id}/")
            resp.raise_for_status()
            return resp.json()
        except Exception as e:
            logger.warning("Error fetching repo info for %s: %s", repo_id, e)
            return None

    @retry(tries=3, delay=1, backoff=2)
    def _get_repo_info_via_repo_token(self) -> Optional[dict]:
        """GET /api/v2.1/via-repo-token/repo-info/ — repo token."""
        try:
            resp = self._repo_token_get("repo-info/")
            resp.raise_for_status()
            return resp.json()
        except Exception as e:
            logger.warning("Error fetching repo info via repo token: %s", e)
            return None

    def _get_repo_info(self) -> Optional[dict]:
        """Get repo info using whichever token is available."""
        if self._use_repo_token:
            info = self._get_repo_info_via_repo_token()
            if info:
                # Normalise keys to match account-token response shape
                return {
                    "id": info.get("repo_id", self.repo_id),
                    "name": info.get("repo_name", self.repo_id),
                }
            return None
        return self._get_repo_info_via_account(self.repo_id)

    @retry(tries=3, delay=1, backoff=2)
    def _get_directory_entries(self, repo_id: str, path: str = "/") -> list[dict]:
        """List directory contents using the appropriate endpoint."""
        try:
            if self._use_repo_token:
                # GET /api/v2.1/via-repo-token/dir/?path=/foo
                resp = self._repo_token_get("dir/", params={"path": path})
            else:
                # GET /api2/repos/{repo_id}/dir/?p=/foo
                resp = self._account_get(
                    f"/repos/{repo_id}/dir/", params={"p": path},
                )
            resp.raise_for_status()
            data = resp.json()

            # v2.1 wraps entries in {"dirent_list": [...]}
            if isinstance(data, dict) and "dirent_list" in data:
                return data["dirent_list"]
            return data

        except Exception as e:
            logger.warning(
                "Error fetching directory %s in repo %s: %s", path, repo_id, e,
            )
            return []

    @retry(tries=3, delay=1, backoff=2)
    def _get_file_download_link(
        self, repo_id: str, path: str
    ) -> Optional[str]:
        """Get a temporary download URL for a file."""
        try:
            if self._use_repo_token:
                # GET /api/v2.1/via-repo-token/download-link/?path=/foo.pdf
                resp = self._repo_token_get(
                    "download-link/", params={"path": path},
                )
            else:
                # GET /api2/repos/{repo_id}/file/?p=/foo.pdf&reuse=1
                resp = self._account_get(
                    f"/repos/{repo_id}/file/", params={"p": path, "reuse": 1},
                )
            resp.raise_for_status()
            return resp.text.strip('"')
        except Exception as e:
            logger.warning("Error getting download link for %s: %s", path, e)
            return None


    def _list_files_recursive(
        self,
        repo_id: str,
        repo_name: str,
        path: str,
        start: datetime,
        end: datetime,
    ) -> list[tuple[str, dict, dict]]:
        files = []
        entries = self._get_directory_entries(repo_id, path)

        for entry in entries:
            entry_type = entry.get("type")
            entry_name = entry.get("name", "")
            entry_path = f"{path.rstrip('/')}/{entry_name}"

            if entry_type == "dir":
                files.extend(
                    self._list_files_recursive(
                        repo_id, repo_name, entry_path, start, end,
                    )
                )
            elif entry_type == "file":
                modified = self._parse_mtime(entry.get("mtime"))
                if start < modified <= end:
                    files.append(
                        (entry_path, entry,
                        {"id": repo_id, "name": repo_name})
                    )

        return files

    def _resolve_libraries_to_scan(self) -> list[dict]:
        if self.sync_scope == SeafileSyncScope.ACCOUNT:
            return [
                {"id": lib["id"], "name": lib.get("name", "Unknown")}
                for lib in self._get_libraries() if lib.get("id")
            ]

        info = self._get_repo_info()
        if info:
            return [{"id": info.get("id", self.repo_id),
                      "name": info.get("name", self.repo_id)}]
        return [{"id": self.repo_id, "name": self.repo_id}]

    def _root_path_for_repo(self, repo_id: str) -> str:
        if (self.sync_scope == SeafileSyncScope.DIRECTORY
                and repo_id == self.repo_id):
            return self.sync_path
        return "/"


    def _yield_seafile_documents(
        self, start: datetime, end: datetime,
    ) -> GenerateDocumentsOutput:
        libraries = self._resolve_libraries_to_scan()
        logger.info(
            "Processing %d library(ies) [scope=%s]",
            len(libraries), self.sync_scope.value,
        )

        all_files: list[tuple[str, dict, dict]] = []
        for lib in libraries:
            root = self._root_path_for_repo(lib["id"])
            logger.debug("Scanning %s starting at %s", lib["name"], root)
            try:
                files = self._list_files_recursive(
                    lib["id"], lib["name"], root, start, end,
                )
                all_files.extend(files)
            except Exception as e:
                logger.error("Error in library %s: %s", lib["name"], e)

        logger.info("Found %d file(s) matching criteria", len(all_files))

        batch: list[Document] = []
        for file_path, file_entry, library in all_files:
            file_name = file_entry.get("name", "")
            file_size = file_entry.get("size", 0)
            file_id = file_entry.get("id", "")
            repo_id = library["id"]
            repo_name = library["name"]

            modified = self._parse_mtime(file_entry.get("mtime"))
            
            if file_size > self.size_threshold:
                logger.warning("Skipping large file: %s (%d B)", file_path, file_size)
                continue

            try:
                download_link = self._get_file_download_link(repo_id, file_path)
                if not download_link:
                    continue

                resp = rl_requests.get(download_link, timeout=120)
                resp.raise_for_status()
                blob = resp.content
                if not blob:
                    continue

                batch.append(Document(
                    id=f"seafile:{repo_id}:{file_id}",
                    blob=blob,
                    source=DocumentSource.SEAFILE,
                    semantic_identifier=f"{repo_name}{file_path}",
                    extension=get_file_ext(file_name),
                    doc_updated_at=modified,          # <-- already parsed
                    size_bytes=len(blob),
                ))

                if len(batch) >= self.batch_size:
                    yield batch
                    batch = []

            except Exception as e:
                logger.error("Error downloading %s: %s", file_path, e)

        if batch:
            yield batch

    def load_from_state(self) -> GenerateDocumentsOutput:
        return self._yield_seafile_documents(
            start=datetime(1970, 1, 1, tzinfo=timezone.utc),
            end=datetime.now(timezone.utc),
        )

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch,
    ) -> GenerateDocumentsOutput:
        start_dt = datetime.fromtimestamp(start, tz=timezone.utc)
        end_dt = datetime.fromtimestamp(end, tz=timezone.utc)
        for batch in self._yield_seafile_documents(start_dt, end_dt):
            yield batch

    