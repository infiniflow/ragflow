"""Create and verify the initial RAGFlow superuser."""

import os

from common import settings


def required(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        raise RuntimeError(f"{name} is required")
    return value


def main() -> None:
    settings.init_settings()

    from api.db.db_models import DB
    from api.db.init_data import init_superuser
    from api.db.services import UserService
    from api.db.services.managed_resource_service import MANAGED_RESOURCE_OWNER_SETTING
    from api.db.services.system_settings_service import SystemSettingsService

    email = required("BOOTSTRAP_ADMIN_EMAIL")
    password = required("BOOTSTRAP_ADMIN_PASSWORD")
    nickname = os.environ.get("BOOTSTRAP_ADMIN_NICKNAME", "RAGFlow Admin").strip() or "RAGFlow Admin"

    with DB.connection_context():
        init_superuser(nickname=nickname, email=email, password=password)
        users = list(UserService.query(email=email))
        if len(users) != 1:
            raise RuntimeError(f"Expected one bootstrap user for {email}, found {len(users)}")
        if not users[0].is_superuser:
            raise RuntimeError("Bootstrap user exists but is not a superuser")
        owner_settings = list(SystemSettingsService.get_by_name(MANAGED_RESOURCE_OWNER_SETTING))
        if owner_settings:
            SystemSettingsService.update_by_name(MANAGED_RESOURCE_OWNER_SETTING, {"value": email})
        else:
            SystemSettingsService.save(
                name=MANAGED_RESOURCE_OWNER_SETTING,
                source="admin",
                data_type="string",
                value=email,
            )

    print(f"bootstrap_admin={email}")
    print("bootstrap_superuser=true")


if __name__ == "__main__":
    main()
