#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""Global account-role permission enforcement (issue #5965).

Accounts have a global tier: ``admin`` (full access) or ``user`` (read-only).
A read-only account may use assistants/models but must not create, modify, or
delete any resource.
"""

import inspect
import logging
from functools import wraps

from api.db import AccountRole


def is_readonly_account(user) -> bool:
    """True if ``user`` is a read-only ("user" tier) account.

    Accounts predating this feature have no ``account_role``; they default to
    administrator so existing installs keep full access.
    """
    return getattr(user, "account_role", AccountRole.ADMIN) == AccountRole.USER


def require_admin_account(func):
    """Reject read-only accounts from a mutation route.

    Layer this *after* ``@login_required`` (so ``current_user`` is populated)::

        @manager.route("/datasets", methods=["POST"])
        @login_required
        @require_admin_account
        async def create_dataset():
            ...

    Sync- and async-aware, mirroring ``add_tenant_id_to_kwargs``.
    """

    @wraps(func)
    async def wrapper(*args, **kwargs):
        from api.apps import current_user
        from api.utils.api_utils import get_error_permission_result

        if is_readonly_account(current_user):
            logging.warning("Read-only account %s blocked from %s", getattr(current_user, "id", "?"), func.__name__)
            return get_error_permission_result(message="This account is read-only and cannot modify resources.")
        if inspect.iscoroutinefunction(func):
            return await func(*args, **kwargs)
        return func(*args, **kwargs)

    return wrapper
