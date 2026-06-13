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

from unittest.mock import patch

from core.config import AppConfig


def test_storage_old_yaml():
    return_value = {
        "minio": {"host": "127.0.0.1:9000", "user": "minio", "password": "pass"},
        "s3": {"access_key": "old", "secret_key": "oldsecret", "bucket": "oldbucket"},
        "oss": {"access_key": "old", "secret_key": "oldsecret", "bucket": "oldbucket"}
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()
    minio_cfg = config.storage.minio
    s3_cfg = config.storage.s3
    oss_cfg = config.storage.oss
    assert minio_cfg.host.startswith("127.0.0.1")
    assert s3_cfg.bucket == "oldbucket"
    assert oss_cfg.bucket == "oldbucket"

def test_storage_new_yaml():
    return_value = {
        "storage": {
            "minio": {"host": "127.0.0.2:9000", "user": "new", "password": "newpass"},
            "s3": {"access_key": "new", "secret_key": "newsecret", "bucket": "newbucket"},
            "oss": {"access_key": "new", "secret_key": "newsecret", "bucket": "newbucket"}
        }
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()
    minio_cfg = config.storage.minio
    s3_cfg = config.storage.s3
    oss_cfg = config.storage.oss
    assert minio_cfg.host.startswith("127.0.0.2")
    assert s3_cfg.bucket == "newbucket"
    assert oss_cfg.bucket == "newbucket"
