#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import importlib.util
import sys
import types
from pathlib import Path

# Importing ``api.apps.auth.ldap`` the normal way runs ``api/apps/__init__.py``,
# which eagerly pulls the entire Flask/Quart app together with
# ``rag.llm`` / ``deepdoc.parser`` / ``xgboost`` / ``cv2``. Several of
# those modules fail to import in slim CI environments (xgboost needs
# ``pkg_resources``, cv2 needs ``libGL.so.1`` at the OS level, etc.).
#
# These LDAP tests only need ``LDAPClient`` and ``LDAPAuthError`` from
# the small ``ldap.py`` module, so we pre-populate ``sys.modules`` with
# the bare minimum and load ``ldap.py`` from disk directly. The test
# module's own ``from api.apps.auth.ldap import LDAPAuthError, LDAPClient``
# then finds the already-loaded module and skips the heavy package init.

_REPO_ROOT = Path(__file__).resolve().parents[4]
_LDAP_PATH = _REPO_ROOT / "api" / "apps" / "auth" / "ldap.py"


def _ensure_pkg(name: str, path: Path) -> types.ModuleType:
    mod = sys.modules.get(name)
    if mod is None:
        mod = types.ModuleType(name)
        mod.__path__ = [str(path)]
        sys.modules[name] = mod
    return mod


if "api.apps.auth.ldap" not in sys.modules:
    _ensure_pkg("api", _REPO_ROOT / "api")
    _ensure_pkg("api.apps", _REPO_ROOT / "api" / "apps")
    _ensure_pkg("api.apps.auth", _REPO_ROOT / "api" / "apps" / "auth")

    # ``ldap.py`` does ``from .oauth import UserInfo``. ``oauth.py`` itself
    # depends on the common HTTP stack, so substitute a tiny ``UserInfo``
    # carrier with the same shape.
    if "api.apps.auth.oauth" not in sys.modules:
        oauth_stub = types.ModuleType("api.apps.auth.oauth")

        class UserInfo:
            __slots__ = ("email", "username", "nickname", "avatar_url")

            def __init__(self, email=None, username=None, nickname=None, avatar_url=None):
                self.email = email
                self.username = username
                self.nickname = nickname
                self.avatar_url = avatar_url

        oauth_stub.UserInfo = UserInfo
        sys.modules["api.apps.auth.oauth"] = oauth_stub

    _spec = importlib.util.spec_from_file_location("api.apps.auth.ldap", _LDAP_PATH)
    _ldap_mod = importlib.util.module_from_spec(_spec)
    sys.modules["api.apps.auth.ldap"] = _ldap_mod
    _spec.loader.exec_module(_ldap_mod)
