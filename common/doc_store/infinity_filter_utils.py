def build_fulltext_filter(fields: str, matching_text: str) -> str:
    escaped_text = matching_text.replace("'", "''")
    return f"filter_fulltext('{fields}', '{escaped_text}')"
