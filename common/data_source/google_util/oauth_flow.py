import json
import os
import threading
from typing import Any, Callable

import requests

from common.data_source.config import DocumentSource
from common.data_source.google_util.constant import GOOGLE_SCOPES

GOOGLE_DEVICE_CODE_URL = "https://oauth2.googleapis.com/device/code"
GOOGLE_DEVICE_TOKEN_URL = "https://oauth2.googleapis.com/token"
DEFAULT_DEVICE_INTERVAL = 5


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


def _extract_client_info(credentials: dict[str, Any]) -> tuple[str, str | None]:
    if "client_id" in credentials:
        return credentials["client_id"], credentials.get("client_secret")
    for key in ("installed", "web"):
        if key in credentials and isinstance(credentials[key], dict):
            nested = credentials[key]
            if "client_id" not in nested:
                break
            return nested["client_id"], nested.get("client_secret")
    raise ValueError("Provided Google OAuth credentials are missing client_id.")


def start_device_authorization_flow(
    credentials: dict[str, Any],
    source: DocumentSource,
) -> tuple[dict[str, Any], dict[str, Any]]:
    client_id, client_secret = _extract_client_info(credentials)
    data = {
        "client_id": client_id,
        "scope": " ".join(_get_requested_scopes(source)),
    }
    if client_secret:
        data["client_secret"] = client_secret
    resp = requests.post(GOOGLE_DEVICE_CODE_URL, data=data, timeout=15)
    resp.raise_for_status()
    payload = resp.json()
    state = {
        "client_id": client_id,
        "client_secret": client_secret,
        "device_code": payload.get("device_code"),
        "interval": payload.get("interval", DEFAULT_DEVICE_INTERVAL),
    }
    response_data = {
        "user_code": payload.get("user_code"),
        "verification_url": payload.get("verification_url") or payload.get("verification_uri"),
        "verification_url_complete": payload.get("verification_url_complete")
        or payload.get("verification_uri_complete"),
        "expires_in": payload.get("expires_in"),
        "interval": state["interval"],
    }
    return state, response_data


def poll_device_authorization_flow(state: dict[str, Any]) -> dict[str, Any]:
    data = {
        "client_id": state["client_id"],
        "device_code": state["device_code"],
        "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
    }
    if state.get("client_secret"):
        data["client_secret"] = state["client_secret"]
    resp = requests.post(GOOGLE_DEVICE_TOKEN_URL, data=data, timeout=20)
    resp.raise_for_status()
    return resp.json()


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
    timeout_message = (
        f"Google OAuth verification timed out after {timeout_secs} seconds. "
        "Close any pending consent windows and rerun the connector configuration to try again."
    )

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
                "  1. In Google Cloud Console, open APIs & Services > OAuth consent screen and add the missing scopes "
                "     (Drive metadata + Admin Directory read scopes), then re-run the flow.",
                "  2. Set GOOGLE_OAUTH_SCOPE_OVERRIDE to a comma-separated list of scopes you are allowed to request.",
                "  3. For quick local testing only, export OAUTHLIB_RELAX_TOKEN_SCOPE=1 to accept the reduced scopes "
                "     (be aware the connector may lose functionality).",
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
        raise ValueError(
            "Provided Google OAuth credentials are missing both tokens and a client configuration."
        )

    return _run_local_server_flow(client_config, source)
