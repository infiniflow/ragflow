import json
import os
import threading
from typing import Any, Callable

from common.data_source.config import DocumentSource
from common.data_source.google_util.constant import GOOGLE_SCOPES


def _get_requested_scopes(source: DocumentSource) -> list[str]:
    """Return the scopes to request, honoring an optional override env var."""
    override = os.environ.get("GOOGLE_OAUTH_SCOPE_OVERRIDE", "")
    if override.strip():
        scopes = [scope.strip() for scope in override.split(",") if scope.strip()]
        if scopes:
            return scopes
    return GOOGLE_SCOPES[source]


def _get_oauth_timeout_secs() -> int:
    raw_timeout = os.environ.get("GOOGLE_OAUTH_FLOW_TIMEOUT_SECS", "300").strip()
    try:
        timeout = int(raw_timeout)
    except ValueError:
        timeout = 300
    return timeout


def _run_with_timeout(func: Callable[[], Any], timeout_secs: int, timeout_message: str) -> Any:
    if timeout_secs <= 0:
        return func()

    result: dict[str, Any] = {}
    error: dict[str, BaseException] = {}

    def _target() -> None:
        try:
            result["value"] = func()
        except BaseException as exc:  # pragma: no cover
            error["error"] = exc

    thread = threading.Thread(target=_target, daemon=True)
    thread.start()
    thread.join(timeout_secs)
    if thread.is_alive():
        raise TimeoutError(timeout_message)
    if "error" in error:
        raise error["error"]
    return result.get("value")


def _run_local_server_flow(client_config: dict[str, Any], source: DocumentSource) -> dict[str, Any]:
    """Launch the standard Google OAuth local-server flow to mint user tokens."""
    from google_auth_oauthlib.flow import InstalledAppFlow  # type: ignore

    scopes = _get_requested_scopes(source)
    flow = InstalledAppFlow.from_client_config(
        client_config,
        scopes=scopes,
    )

    open_browser = os.environ.get("GOOGLE_OAUTH_OPEN_BROWSER", "true").lower() != "false"
    preferred_port = os.environ.get("GOOGLE_OAUTH_LOCAL_SERVER_PORT")
    port = int(preferred_port) if preferred_port else 0
    timeout_secs = _get_oauth_timeout_secs()
    timeout_message = f"Google OAuth verification timed out after {timeout_secs} seconds. Close any pending consent windows and rerun the connector configuration to try again."

    print("Launching Google OAuth flow. A browser window should open shortly.")
    print("If it does not, copy the URL shown in the console into your browser manually.")
    if timeout_secs > 0:
        print(f"You have {timeout_secs} seconds to finish granting access before the request times out.")

    try:
        creds = _run_with_timeout(
            lambda: flow.run_local_server(port=port, open_browser=open_browser, prompt="consent"),
            timeout_secs,
            timeout_message,
        )
    except OSError as exc:
        allow_console = os.environ.get("GOOGLE_OAUTH_ALLOW_CONSOLE_FALLBACK", "true").lower() != "false"
        if not allow_console:
            raise
        print(f"Local server flow failed ({exc}). Falling back to console-based auth.")
        creds = _run_with_timeout(flow.run_console, timeout_secs, timeout_message)
    except Warning as warning:
        warning_msg = str(warning)
        if "Scope has changed" in warning_msg:
            instructions = [
                "Google rejected one or more of the requested OAuth scopes.",
                "Fix options:",
                "  1. In Google Cloud Console, open APIs & Services > OAuth consent screen and add the missing scopes      (Drive metadata + Admin Directory read scopes), then re-run the flow.",
                "  2. Set GOOGLE_OAUTH_SCOPE_OVERRIDE to a comma-separated list of scopes you are allowed to request.",
            ]
            raise RuntimeError("\n".join(instructions)) from warning
        raise

    token_dict: dict[str, Any] = json.loads(creds.to_json())

    print("\nGoogle OAuth flow completed successfully.")
    print("Copy the JSON blob below into GOOGLE_DRIVE_OAUTH_CREDENTIALS_JSON_STR to reuse these tokens without re-authenticating:\n")
    print(json.dumps(token_dict, indent=2))
    print()

    return token_dict


def ensure_oauth_token_dict(credentials: dict[str, Any], source: DocumentSource) -> dict[str, Any]:
    """Return a dict that contains OAuth tokens, running the flow if only a client config is provided."""
    if "refresh_token" in credentials and "token" in credentials:
        return credentials

    client_config: dict[str, Any] | None = None
    if "installed" in credentials:
        client_config = {"installed": credentials["installed"]}
    elif "web" in credentials:
        client_config = {"web": credentials["web"]}

    if client_config is None:
        raise ValueError("Provided Google OAuth credentials are missing both tokens and a client configuration.")

    return _run_local_server_flow(client_config, source)
