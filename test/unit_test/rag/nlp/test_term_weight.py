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

import builtins
from pathlib import Path

from rag.nlp import term_weight


def test_dealer_loads_resource_files_as_utf8(monkeypatch, tmp_path):
    """Load non-ASCII term-weight resources with an explicit UTF-8 encoding."""
    resource_dir = tmp_path / "rag" / "res"
    resource_dir.mkdir(parents=True)
    (resource_dir / "ner.json").write_text('{"\u5317\u4eac": "loca"}', encoding="utf-8")
    (resource_dir / "term.freq").write_text("caf\u00e9\t42\n", encoding="utf-8")

    monkeypatch.setattr(term_weight, "get_project_base_directory", lambda: str(tmp_path))
    real_open = builtins.open

    def require_explicit_utf8(file, *args, **kwargs):
        """Reject resource reads that rely on the platform default encoding."""
        if Path(file).parent == resource_dir:
            assert kwargs.get("encoding") == "utf-8"
        return real_open(file, *args, **kwargs)

    monkeypatch.setattr(builtins, "open", require_explicit_utf8)

    dealer = term_weight.Dealer()

    assert dealer.ne == {"\u5317\u4eac": "loca"}
    assert dealer.df == {"caf\u00e9": 42}
