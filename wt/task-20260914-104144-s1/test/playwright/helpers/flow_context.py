from dataclasses import dataclass
from typing import Any


@dataclass
class FlowContext:
    page: Any
    state: dict
    base_url: str
    login_url: str
    smoke_login_url: str | None = None
    active_auth_context: Any | None = None
    auth_click: Any | None = None
    seeded_user_credentials: Any | None = None
