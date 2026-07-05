with open("common/metadata_utils.py", "r") as f:
    content = f.read()

content = content.replace('''<<<<<<< HEAD
        meta_data_filter: dict | None,
        metas: dict | None = None,
        question: str = "",
        chat_mdl: Any = None,
        base_doc_ids: list[str] | None = None,
        manual_value_resolver: Callable[[dict], dict] | None = None,
        kb_ids: list[str] | None = None,
        metas_loader: Callable[[], dict] | None = None,
        extra_conditions: list[dict] | None = None,
=======
    meta_data_filter: dict | None,
    metas: dict | None = None,
    question: str = "",
    chat_mdl: Any = None,
    base_doc_ids: list[str] | None = None,
    manual_value_resolver: Callable[[dict], dict] | None = None,
    kb_ids: list[str] | None = None,
    metas_loader: Callable[[], dict] | None = None,
>>>>>>> origin/infinitiflow-main''', '''    meta_data_filter: dict | None,
    metas: dict | None = None,
    question: str = "",
    chat_mdl: Any = None,
    base_doc_ids: list[str] | None = None,
    manual_value_resolver: Callable[[dict], dict] | None = None,
    kb_ids: list[str] | None = None,
    metas_loader: Callable[[], dict] | None = None,
    extra_conditions: list[dict] | None = None,''')

with open("common/metadata_utils.py", "w") as f:
    f.write(content)
