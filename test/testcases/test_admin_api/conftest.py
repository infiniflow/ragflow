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

import os
from typing import Any

import pytest
import requests
from configs import VERSION

# Admin API runs on port 9381
ADMIN_HOST_ADDRESS = os.getenv("ADMIN_HOST_ADDRESS", "http://127.0.0.1:9381")

# password is "admin"
ENCRYPTED_ADMIN_PASSWORD: str = """WBPsJbL/W+1HN+hchm5pgu1YC3yMEb/9MFtsanZrpKEE9kAj4u09EIIVDtIDZhJOdTjz5pp5QW9TwqXBfQ2qzDqVJiwK7HGcNsoPi4wQPCmnLo0fs62QklMlg7l1Q7fjGRgV+KWtvNUce2PFzgrcAGDqRIuA/slSclKUEISEiK4z62rdDgvHT8LyuACuF1lPUY5wV0m/MbmGijRJlgvglAF8BX0BP8rQr8wZeaJdcnAy/keuODCjltMZDL06tYluN7HoiU+qlhBB+ltqG411oO/+vVhBgWsuVVOHd8uMjJEL320GUWUicprDUZvjlLaSSqVyyOiRMHpqAE9eHEecWg=="""


def admin_login(session: requests.Session, email: str = "admin@ragflow.io", password: str = "admin") -> str:
    """Helper function to login as admin and return authorization token"""
    url: str = f"{ADMIN_HOST_ADDRESS}/api/{VERSION}/admin/login"
    response: requests.Response = session.post(url, json={"email": email, "password": ENCRYPTED_ADMIN_PASSWORD})
    res_json: dict[str, Any] = response.json()
    if res_json.get("code") != 0:
        raise Exception(res_json.get("message"))
    # Admin login uses session cookies and Authorization header
    # Set Authorization header for subsequent requests
    auth: str = response.headers.get("Authorization", "")
    if auth:
        session.headers.update({"Authorization": auth})
    return auth


@pytest.fixture(scope="session")
def admin_session() -> requests.Session:
    """Fixture to create an admin session with login"""
    session: requests.Session = requests.Session()
    try:
        admin_login(session)
    except Exception as e:
        pytest.skip(f"Admin login failed: {e}")
    return session
