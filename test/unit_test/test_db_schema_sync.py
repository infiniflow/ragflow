import ast

from tools.scripts.migration_sql_renderer import render_migrator_sql


def test_render_migrator_sql_escapes_json_defaults():
    sql = "ALTER TABLE `demo` ADD `config` LONGTEXT DEFAULT '{\"pages\": [1, 2]} '"

    generated = render_migrator_sql(sql)

    ast.parse(f"def upgrade(migrator):\n    {generated.strip()}\n")


def test_render_migrator_sql_preserves_sql_text():
    sql = "ALTER TABLE `demo` DROP COLUMN `old_column`"

    assert render_migrator_sql(sql) == f"    migrator.sql({sql!r})"
