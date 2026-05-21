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
import pytest

from common.sql_guard import ReadOnlySqlViolation, ensure_read_only_sql


@pytest.mark.parametrize(
    "sql,doc_engine",
    [
        ("SELECT doc_id, docnm FROM ragflow_t_kb", "infinity"),
        ("SELECT COUNT(*) AS rows FROM ragflow_t_kb", "infinity"),
        (
            "SELECT doc_id FROM t UNION SELECT doc_id FROM t2",
            "es",
        ),
    ],
)
def test_ensure_read_only_sql_allows_valid_select(sql, doc_engine):
    ensure_read_only_sql(sql, doc_engine=doc_engine)


@pytest.mark.parametrize(
    "sql,doc_engine",
    [
        ("DROP TABLE ragflow_t_kb", "infinity"),
        ("DELETE FROM ragflow_t_kb", "infinity"),
        ("INSERT INTO ragflow_t_kb VALUES (1)", "oceanbase"),
        ("SELECT 1; DROP TABLE ragflow_t_kb", "infinity"),
        ("UPDATE ragflow_t_kb SET x = 1", "es"),
        ("TRUNCATE TABLE ragflow_t_kb", "infinity"),
    ],
)
def test_ensure_read_only_sql_rejects_destructive(sql, doc_engine):
    with pytest.raises(ReadOnlySqlViolation):
        ensure_read_only_sql(sql, doc_engine=doc_engine)
