with open("api/apps/restful_apis/bot_api.py", "r") as f:
    content = f.read()

content = content.replace('''<<<<<<< HEAD
            _question, embd_mdl, tenant_ids, kb_ids, page, size, similarity_threshold, vector_similarity_weight, top,
            local_doc_ids, rerank_mdl=rerank_mdl, highlight=req.get("highlight"), rank_feature=labels,
            temporal_rank_policy=temporal_ctx.temporal_rank_policy
=======
            _question,
            embd_mdl,
            tenant_ids,
            kb_ids,
            page,
            size,
            similarity_threshold,
            vector_similarity_weight,
            top,
            local_doc_ids,
            rerank_mdl=rerank_mdl,
            highlight=req.get("highlight"),
            rank_feature=labels,
>>>>>>> origin/infinitiflow-main''', '''            _question,
            embd_mdl,
            tenant_ids,
            kb_ids,
            page,
            size,
            similarity_threshold,
            vector_similarity_weight,
            top,
            local_doc_ids,
            rerank_mdl=rerank_mdl,
            highlight=req.get("highlight"),
            rank_feature=labels,
            temporal_rank_policy=temporal_ctx.temporal_rank_policy,''')
with open("api/apps/restful_apis/bot_api.py", "w") as f:
    f.write(content)
