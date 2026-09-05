import sys
from pathlib import Path
from types import SimpleNamespace

from flask import Flask
from flask_login import LoginManager, UserMixin

ADMIN_SERVER = Path(__file__).parents[3] / "admin" / "server"
if str(ADMIN_SERVER) not in sys.path:
    sys.path.insert(0, str(ADMIN_SERVER))

import auth
import routes
from common.constants import ActiveEnum


def _application(monkeypatch):
    app = Flask(__name__)
    app.config.update(TESTING=True, SECRET_KEY="test")
    manager = LoginManager(app)

    class User(UserMixin):
        id = "admin"

    @manager.request_loader
    def load_user(request):
        return User() if request.headers.get("Authorization") else None

    monkeypatch.setattr(auth.UserService, "filter_by_id", lambda _id: SimpleNamespace(is_superuser=True, is_active=ActiveEnum.ACTIVE.value))

    def admin_error(error):
        return {"code": error.code, "message": error.message}, error.code

    for exception_type in {auth.AdminException, routes.AdminException}:
        app.register_error_handler(exception_type, admin_error)

    app.register_blueprint(routes.admin_bp)
    return app


def test_access_group_crud_routes(monkeypatch):
    app = _application(monkeypatch)
    stored = {"id": "g1", "name": "Legal", "description": "", "user_ids": [], "dataset_ids": [], "sections": ["dataset"]}
    monkeypatch.setattr(routes.AccessGroupMgr, "list_groups", lambda: [stored])
    monkeypatch.setattr(routes.AccessGroupMgr, "create", lambda payload: {**stored, **payload})
    monkeypatch.setattr(routes.AccessGroupMgr, "update", lambda group_id, payload: {**stored, **payload, "id": group_id})
    monkeypatch.setattr(routes.AccessGroupMgr, "delete", lambda group_id: None)
    client = app.test_client()
    headers = {"Authorization": "test"}

    assert client.get("/api/v1/admin/access-groups", headers=headers).json["data"]["items"][0]["id"] == "g1"
    assert client.post("/api/v1/admin/access-groups", json=stored, headers=headers).status_code == 200
    assert client.put("/api/v1/admin/access-groups/g1", json={"name": "Legal 2"}, headers=headers).json["data"]["name"] == "Legal 2"
    assert client.delete("/api/v1/admin/access-groups/g1", headers=headers).json["data"] is True


def test_access_group_routes_require_admin_login(monkeypatch):
    app = _application(monkeypatch)
    client = app.test_client()
    assert client.get("/api/v1/admin/access-groups").status_code == 401
    monkeypatch.setattr(auth.UserService, "filter_by_id", lambda _id: SimpleNamespace(is_superuser=False, is_active=ActiveEnum.ACTIVE.value))
    assert client.get("/api/v1/admin/access-groups", headers={"Authorization": "test"}).status_code == 403
