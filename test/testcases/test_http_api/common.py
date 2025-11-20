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
from pathlib import Path
from typing import Any, Dict, Optional, Union

import requests
from configs import HOST_ADDRESS, VERSION
from requests.auth import AuthBase
from requests_toolbelt import MultipartEncoder
from utils.file_utils import create_txt_file

# Import login_as_user and encrypt_password from root conftest
import importlib.util
_root_conftest_path = Path(__file__).parent.parent / "conftest.py"
_root_spec = importlib.util.spec_from_file_location("root_conftest", _root_conftest_path)
_root_conftest_module = importlib.util.module_from_spec(_root_spec)
_root_spec.loader.exec_module(_root_conftest_module)
login_as_user = _root_conftest_module.login_as_user
encrypt_password = _root_conftest_module.encrypt_password

HEADERS = {"Content-Type": "application/json"}
DATASETS_API_URL = f"/api/{VERSION}/datasets"
FILE_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/documents"
FILE_CHUNK_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/chunks"
CHUNK_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/documents/{{document_id}}/chunks"
CHAT_ASSISTANT_API_URL = f"/api/{VERSION}/chats"
SESSION_WITH_CHAT_ASSISTANT_API_URL = f"/api/{VERSION}/chats/{{chat_id}}/sessions"
SESSION_WITH_AGENT_API_URL = f"/api/{VERSION}/agents/{{agent_id}}/sessions"


# DATASET MANAGEMENT
def create_dataset(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DATASETS_API_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_datasets(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_API_URL}", headers=headers, auth=auth, params=params)
    return res.json()


def update_dataset(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.put(url=f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def delete_datasets(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.delete(url=f"{HOST_ADDRESS}{DATASETS_API_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def batch_create_datasets(auth, num):
    ids = []
    for i in range(num):
        res = create_dataset(auth, {"name": f"dataset_{i}"})
        ids.append(res["data"]["id"])
    return ids


# FILE MANAGEMENT WITHIN DATASET
def upload_documents(auth, dataset_id, files_path=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)

    if files_path is None:
        files_path = []

    fields = []
    file_objects = []
    try:
        for fp in files_path:
            p = Path(fp)
            f = p.open("rb")
            fields.append(("file", (p.name, f)))
            file_objects.append(f)
        m = MultipartEncoder(fields=fields)

        res = requests.post(
            url=url,
            headers={"Content-Type": m.content_type},
            auth=auth,
            data=m,
        )
        return res.json()
    finally:
        for f in file_objects:
            f.close()


def download_document(auth, dataset_id, document_id, save_path):
    url = f"{HOST_ADDRESS}{FILE_API_URL}/{document_id}".format(dataset_id=dataset_id)
    res = requests.get(url=url, auth=auth, stream=True)
    try:
        if res.status_code == 200:
            with open(save_path, "wb") as f:
                for chunk in res.iter_content(chunk_size=8192):
                    f.write(chunk)
    finally:
        res.close()

    return res


def list_documents(auth, dataset_id, params=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def update_document(auth, dataset_id, document_id, payload=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}/{document_id}".format(dataset_id=dataset_id)
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_documents(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def parse_documents(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{FILE_CHUNK_API_URL}".format(dataset_id=dataset_id)
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def stop_parse_documents(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{FILE_CHUNK_API_URL}".format(dataset_id=dataset_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def bulk_upload_documents(auth, dataset_id, num, tmp_path):
    fps = []
    for i in range(num):
        fp = create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt")
        fps.append(fp)
    res = upload_documents(auth, dataset_id, fps)
    document_ids = []
    for document in res["data"]:
        document_ids.append(document["id"])
    return document_ids


# CHUNK MANAGEMENT WITHIN DATASET
def add_chunk(auth, dataset_id, document_id, payload=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def list_chunks(auth, dataset_id, document_id, params=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def update_chunk(auth, dataset_id, document_id, chunk_id, payload=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}/{chunk_id}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_chunks(auth, dataset_id, document_id, payload=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def retrieval_chunks(auth, payload=None):
    url = f"{HOST_ADDRESS}/api/v1/retrieval"
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def batch_add_chunks(auth, dataset_id, document_id, num):
    chunk_ids = []
    for i in range(num):
        res = add_chunk(auth, dataset_id, document_id, {"content": f"chunk test {i}"})
        chunk_ids.append(res["data"]["chunk"]["id"])
    return chunk_ids


# CHAT ASSISTANT MANAGEMENT
def create_chat_assistant(auth, payload=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}"
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def list_chat_assistants(auth, params=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}"
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def update_chat_assistant(auth, chat_assistant_id, payload=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}/{chat_assistant_id}"
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_chat_assistants(auth, payload=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}"
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def batch_create_chat_assistants(auth, num):
    chat_assistant_ids = []
    for i in range(num):
        res = create_chat_assistant(auth, {"name": f"test_chat_assistant_{i}", "dataset_ids": []})
        chat_assistant_ids.append(res["data"]["id"])
    return chat_assistant_ids


# SESSION MANAGEMENT
def create_session_with_chat_assistant(auth, chat_assistant_id, payload=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}".format(chat_id=chat_assistant_id)
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def list_session_with_chat_assistants(auth, chat_assistant_id, params=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}".format(chat_id=chat_assistant_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def update_session_with_chat_assistant(auth, chat_assistant_id, session_id, payload=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}/{session_id}".format(chat_id=chat_assistant_id)
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_session_with_chat_assistants(auth, chat_assistant_id, payload=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}".format(chat_id=chat_assistant_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def batch_add_sessions_with_chat_assistant(auth, chat_assistant_id, num):
    session_ids = []
    for i in range(num):
        res = create_session_with_chat_assistant(auth, chat_assistant_id, {"name": f"session_with_chat_assistant_{i}"})
        session_ids.append(res["data"]["id"])
    return session_ids


# USER MANAGEMENT
USER_API_URL: str = f"/{VERSION}/user"


def create_user(
    auth: Union[AuthBase, str, None],
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Create a new user.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        payload: Optional JSON payload containing user data.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the created user data.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{USER_API_URL}/create"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def update_user(
    auth: Union[AuthBase, str, None],
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Update an existing user.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        payload: Optional JSON payload containing updated user data.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the updated user data.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{USER_API_URL}/update"
    res: requests.Response = requests.put(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def list_users(
    auth: Union[AuthBase, str, None],
    params: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """List users with optional filtering.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        params: Optional query parameters for filtering/pagination.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the list of users.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{USER_API_URL}/list"
    res: requests.Response = requests.get(
        url=url, headers=headers, auth=auth, params=params
    )
    return res.json()


def delete_user(
    auth: Union[AuthBase, str, None],
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Delete a user.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        payload: Optional JSON payload containing user identification data.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the deletion result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{USER_API_URL}/delete"
    res: requests.Response = requests.delete(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def get_user_info(
    auth: Union[AuthBase, str, None],
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Get the current authenticated user's information.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the user information.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{USER_API_URL}/info"
    res: requests.Response = requests.get(
        url=url, headers=headers, auth=auth
    )
    return res.json()


# TEAM MANAGEMENT
TEAM_API_URL: str = f"/{VERSION}/tenant"


def create_team(
    auth: Union[AuthBase, str, None],
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Create a new team (tenant).

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        payload: Optional JSON payload containing team data (e.g., name, user_id).
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the created team data.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/create"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def add_users_to_team(
    auth: Union[AuthBase, str, None],
    tenant_id: str,
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Add users to a team (tenant).

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        tenant_id: The tenant/team ID to add users to.
        payload: Optional JSON payload containing users list (emails).
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing added and failed user lists.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/{tenant_id}/users/add"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def remove_user_from_team(
    auth: Union[AuthBase, str, None],
    tenant_id: str,
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Remove a user from a team (tenant).

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        tenant_id: The tenant/team ID to remove user from.
        payload: Optional JSON payload containing user_id string.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing removal results.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/{tenant_id}/user/remove"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def accept_team_invitation(
    auth: Union[AuthBase, str, None],
    tenant_id: str,
    role: Optional[str] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Accept a team invitation.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        tenant_id: The tenant/team ID to accept invitation for.
        role: Optional role to assign after acceptance (normal, admin). Defaults to normal.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the acceptance result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/update-request/{tenant_id}"
    payload: Dict[str, Any] = {"accept": True}
    if role:
        payload["role"] = role
    res: requests.Response = requests.put(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def reject_team_invitation(
    auth: Union[AuthBase, str, None],
    tenant_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Reject a team invitation.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        tenant_id: The tenant/team ID to reject invitation for.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the rejection result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/update-request/{tenant_id}"
    payload: Dict[str, bool] = {"accept": False}
    res: requests.Response = requests.put(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def update_team(
    auth: Union[AuthBase, str, None],
    tenant_id: str,
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Update a team (tenant).

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        tenant_id: The team ID to update.
        payload: Optional JSON payload containing update data (e.g., name, llm_id, embd_id, credit).
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the updated team data.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/{tenant_id}"
    res: requests.Response = requests.put(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def promote_admin(
    auth: Union[AuthBase, str, None],
    tenant_id: str,
    user_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Promote a user to admin in a team.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        tenant_id: The team ID.
        user_id: The user ID to promote to admin.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the promotion result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/{tenant_id}/admin/{user_id}/promote"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth
    )
    return res.json()


def demote_admin(
    auth: Union[AuthBase, str, None],
    tenant_id: str,
    user_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Demote an admin to normal member in a team.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        tenant_id: The team ID.
        user_id: The user ID to demote from admin.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the demotion result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{TEAM_API_URL}/{tenant_id}/admin/{user_id}/demote"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth
    )
    return res.json()


# DEPARTMENT MANAGEMENT
DEPARTMENT_API_URL: str = f"/{VERSION}/department"
GROUP_API_URL: str = f"/{VERSION}/group"


def create_department(
    auth: Union[AuthBase, str, None],
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Create a new department.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        payload: Optional JSON payload containing department data (e.g., name, tenant_id, description).
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the created department data.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{DEPARTMENT_API_URL}/create"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def add_department_members(
    auth: Union[AuthBase, str, None],
    department_id: str,
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Add members to a department.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        department_id: The department ID to add members to.
        payload: Optional JSON payload containing user_ids list.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing added and failed user lists.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{DEPARTMENT_API_URL}/{department_id}/members/add"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def remove_department_member(
    auth: Union[AuthBase, str, None],
    department_id: str,
    user_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Remove a member from a department.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        department_id: The department ID to remove member from.
        user_id: The user ID to remove from the department.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the removal result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{DEPARTMENT_API_URL}/{department_id}/members/{user_id}"
    res: requests.Response = requests.delete(
        url=url, headers=headers, auth=auth
    )
    return res.json()


def update_department(
    auth: Union[AuthBase, str, None],
    department_id: str,
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Update a department's details.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        department_id: The department ID to update.
        payload: Optional JSON payload containing update data (e.g., name, description).
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the updated department data.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{DEPARTMENT_API_URL}/{department_id}"
    res: requests.Response = requests.put(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def delete_department(
    auth: Union[AuthBase, str, None],
    department_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Delete a department.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        department_id: The department ID to delete.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the deletion result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{DEPARTMENT_API_URL}/{department_id}"
    res: requests.Response = requests.delete(
        url=url, headers=headers, auth=auth
    )
    return res.json()


def list_department_members(
    auth: Union[AuthBase, str, None],
    department_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """List all members in a department.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        department_id: The department ID to list members from.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the list of department members.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{DEPARTMENT_API_URL}/{department_id}/members"
    res: requests.Response = requests.get(
        url=url, headers=headers, auth=auth
    )
    return res.json()


# GROUP MANAGEMENT
def create_group(
    auth: Union[AuthBase, str, None],
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Create a new group.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        payload: Optional JSON payload containing group data (e.g., name, tenant_id, description).
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the created group data.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{GROUP_API_URL}/create"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def add_group_members(
    auth: Union[AuthBase, str, None],
    group_id: str,
    payload: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Add members to a group.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        group_id: The group ID to add members to.
        payload: Optional JSON payload containing user_ids list.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing added and failed user lists.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{GROUP_API_URL}/{group_id}/members/add"
    res: requests.Response = requests.post(
        url=url, headers=headers, auth=auth, json=payload
    )
    return res.json()


def remove_group_member(
    auth: Union[AuthBase, str, None],
    group_id: str,
    user_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Remove a user from a group.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        group_id: The group ID to remove member from.
        user_id: The user ID to remove from the group.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the removal result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{GROUP_API_URL}/{group_id}/members/{user_id}"
    res: requests.Response = requests.delete(
        url=url, headers=headers, auth=auth
    )
    return res.json()


def list_group_members(
    auth: Union[AuthBase, str, None],
    group_id: str,
    params: Optional[Dict[str, Any]] = None,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """List all members of a group.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        group_id: The group ID to list members for.
        params: Optional query parameters for filtering/pagination.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the list of members.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{GROUP_API_URL}/{group_id}/members"
    res: requests.Response = requests.get(
        url=url, headers=headers, auth=auth, params=params
    )
    return res.json()


def delete_group(
    auth: Union[AuthBase, str, None],
    group_id: str,
    *,
    headers: Dict[str, str] = HEADERS,
) -> Dict[str, Any]:
    """Delete a group.

    Args:
        auth: Authentication object (AuthBase subclass), token string, or None.
        group_id: The group ID to delete.
        headers: Optional HTTP headers. Defaults to HEADERS.

    Returns:
        JSON response as a dictionary containing the deletion result.

    Raises:
        requests.RequestException: If the HTTP request fails.
    """
    url: str = f"{HOST_ADDRESS}{GROUP_API_URL}/{group_id}"
    res: requests.Response = requests.delete(
        url=url, headers=headers, auth=auth
    )
    return res.json()
