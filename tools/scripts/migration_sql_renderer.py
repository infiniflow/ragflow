def render_migrator_sql(sql: str) -> str:
    """Render SQL as a valid Python string literal in generated migrations."""
    return f"    migrator.sql({sql!r})"
