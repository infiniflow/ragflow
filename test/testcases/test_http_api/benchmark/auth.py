from typing import Any, Dict, Optional

from .http_client import HttpClient


class AuthError(RuntimeError):
    pass


def encrypt_password(password_plain: str) -> str:
    try:
        from api.utils.crypt import crypt
    except Exception as exc:
        raise AuthError(
            "Password encryption unavailable; install pycryptodomex (uv sync --python 3.12 --group test)."
        ) from exc
    return crypt(password_plain)

def register_user(client: HttpClient, email: str, nickname: str, password_enc: str) -> None:
    payload = {"email": email, "nickname": nickname, "password": password_enc}
    res = client.request_json("POST", "/user/register", use_api_base=False, auth_kind=None, json_body=payload)
    if res.get("code") == 0:
        return
    msg = res.get("message", "")
    if "has already registered" in msg:
        return
    raise AuthError(f"Register failed: {msg}")


def login_user(client: HttpClient, email: str, password_enc: str) -> str:
    payload = {"email": email, "password": password_enc}
    response = client.request("POST", "/user/login", use_api_base=False, auth_kind=None, json_body=payload)
    try:
        res = response.json()
    except Exception as exc:
        raise AuthError(f"Login failed: invalid JSON response ({exc})") from exc
    if res.get("code") != 0:
        raise AuthError(f"Login failed: {res.get('message')}")
    token = response.headers.get("Authorization")
    if not token:
        raise AuthError("Login failed: missing Authorization header")
    return token


def create_api_token(client: HttpClient, login_token: str, token_name: Optional[str] = None) -> str:
    client.login_token = login_token
    params = {"name": token_name} if token_name else None
    res = client.request_json("POST", "/system/new_token", use_api_base=False, auth_kind="login", params=params)
    if res.get("code") != 0:
        raise AuthError(f"API token creation failed: {res.get('message')}")
    token = res.get("data", {}).get("token")
    if not token:
        raise AuthError("API token creation failed: missing token in response")
    return token


def get_my_llms(client: HttpClient) -> Dict[str, Any]:
    res = client.request_json("GET", "/llm/my_llms", use_api_base=False, auth_kind="login")
    if res.get("code") != 0:
        raise AuthError(f"Failed to list LLMs: {res.get('message')}")
    return res.get("data", {})


def set_llm_api_key(
    client: HttpClient,
    llm_factory: str,
    api_key: str,
    base_url: Optional[str] = None,
) -> None:
    payload = {"llm_factory": llm_factory, "api_key": api_key}
    if base_url:
        payload["base_url"] = base_url
    res = client.request_json("POST", "/llm/set_api_key", use_api_base=False, auth_kind="login", json_body=payload)
    if res.get("code") != 0:
        raise AuthError(f"Failed to set LLM API key: {res.get('message')}")


def get_tenant_info(client: HttpClient) -> Dict[str, Any]:
    res = client.request_json("GET", "/user/tenant_info", use_api_base=False, auth_kind="login")
    if res.get("code") != 0:
        raise AuthError(f"Failed to get tenant info: {res.get('message')}")
    return res.get("data", {})


def set_tenant_info(client: HttpClient, payload: Dict[str, Any]) -> None:
    res = client.request_json("POST", "/user/set_tenant_info", use_api_base=False, auth_kind="login", json_body=payload)
    if res.get("code") != 0:
        raise AuthError(f"Failed to set tenant info: {res.get('message')}")
