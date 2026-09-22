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
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def test_docker_template_defines_vb_config_block():
    template = (ROOT / "docker" / "service_conf.yaml.template").read_text(encoding="utf-8")
    assert "vb:" in template
    assert "VB_HOST" in template
    assert "VB_VEC_DBNAME" in template
    assert "VB_DBCOMPATIBILITY" in template


def test_compose_declares_vastbase_profile_service():
    compose = (ROOT / "docker" / "docker-compose-base.yml").read_text(encoding="utf-8")
    assert "vastbase:" in compose
    assert "- vastbase" in compose
    assert "vastbase_data:" in compose
