with open("api/apps/services/dataset_api_service.py", "r") as f:
    content = f.read()

content = content.replace('''<<<<<<< HEAD
=======
    if meta_data_filter:
        logging.debug("Metadata filter applied: %s, question length: %d, chat_mdl=%s", meta_data_filter, len(question), "None" if chat_mdl is None else "configured")
        local_doc_ids = await apply_meta_data_filter(
            meta_data_filter,
            None,
            question,
            chat_mdl,
            local_doc_ids,
            kb_ids=kb_ids,
            metas_loader=lambda: DocMetadataService.get_flatted_meta_by_kbs(kb_ids),
        )

>>>>>>> origin/infinitiflow-main''', '')
with open("api/apps/services/dataset_api_service.py", "w") as f:
    f.write(content)
