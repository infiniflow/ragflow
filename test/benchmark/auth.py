from typing import Any, Dict, Optional

from .http_client import HttpClient


class AuthError(RuntimeError):
    pass


def encrypt_password(password_plain: str) -> str:
    try:
        from api.utils.crypt import crypt
    except Exception as exc:
        raise AuthError(
            "Password encryption unavailable; install pycryptodomex (uv sync --python 3.13 --group test)."
        ) from exc
    return crypt(password_plain)

def register_user(client: HttpClient, email: str, nickname: str, password_enc: str) -> None:
    payload = {"email": email, "nickname": nickname, "password": password_enc}
    res = client.request_json("POST", "/users", use_api_base=True, auth_kind=None, json_body=payload)
    if res.get("code") == 0:
        return
    msg = res.get("message", "")
    if "has already registered" in msg:
        return
    raise AuthError(f"Register failed: {msg}")


def login_user(client: HttpClient, email: str, password_enc: str) -> str:
    payload = {"email": email, "password": password_enc}
    response = client.request("POST", "/auth/login", use_api_base=True, auth_kind=None, json_body=payload)
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
    res = client.request_json("POST", "/system/tokens", use_api_base=False, auth_kind="login", params=params)
    if res.get("code") != 0:
        raise AuthError(f"API token creation failed: {res.get('message')}")
    token = res.get("data", {}).get("token")
    if not token:
        raise AuthError("API token creation failed: missing token in response")
    return token


def get_my_llms(client: HttpClient) -> Dict[str, Any]:
    """List tenant-configured providers. Returns a dict keyed by provider name."""
    res = client.request_json("GET", "/providers", use_api_base=True, auth_kind="login")
    if res.get("code") != 0:
        raise AuthError(f"Failed to list providers: {res.get('message')}")
    providers = res.get("data", [])
    return {p.get("name", ""): p for p in providers} if isinstance(providers, list) else {}


def set_llm_api_key(
    client: HttpClient,
    llm_factory: str,
    api_key: str,
    base_url: Optional[str] = None,
) -> None:
    """Add a provider (PUT /providers) and create a default instance (POST /providers/{name}/instances)."""
    provider_payload = {"provider_name": llm_factory}
    provider_res = client.request_json("PUT", "/providers", use_api_base=True, auth_kind="login", json_body=provider_payload)
    provider_msg = provider_res.get("message", "")
    if provider_res.get("code") != 0 and "duplicated" not in provider_msg.lower() and "already exist" not in provider_msg.lower():
        raise AuthError(f"Failed to add provider: {provider_msg}")

    instance_payload = {
        "instance_name": "default",
        "api_key": api_key,
        "region": "default",
        "base_url": base_url or "",
    }
    instance_res = client.request_json("POST", f"/providers/{llm_factory}/instances", use_api_base=True,
                                      auth_kind="login", json_body=instance_payload)
    instance_msg = instance_res.get("message", "")
    if instance_res.get("code") != 0 and "already exist" not in instance_msg.lower():
        raise AuthError(f"Failed to add instance: {instance_msg}")


def get_default_models(client: HttpClient) -> Dict[str, Any]:
    """List tenant default models."""
    res = client.request_json("GET", "/models/default", use_api_base=True, auth_kind="login")
    if res.get("code") != 0:
        raise AuthError(f"Failed to get default models: {res.get('message')}")
    return res.get("data", {})


def set_default_model(
    client: HttpClient,
    model_provider: str,
    model_instance: str,
    model_name: str,
    model_type: str,
) -> None:
    """Set a tenant default model via PATCH /models/default."""
    payload = {
        "model_provider": model_provider,
        "model_instance": model_instance,
        "model_name": model_name,
        "model_type": model_type,
    }
    res = client.request_json("PATCH", "/models/default", use_api_base=True, auth_kind="login", json_body=payload)
    if res.get("code") != 0:
        raise AuthError(f"Failed to set default model: {res.get('message')}")
