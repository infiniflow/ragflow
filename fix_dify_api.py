with open("api/apps/restful_apis/dify_retrieval_api.py", "r") as f:
    content = f.read()

content = content.replace('''<<<<<<< HEAD
            temporal_rank_policy=temporal_ctx.temporal_rank_policy,
=======
>>>>>>> origin/infinitiflow-main''', '''            temporal_rank_policy=temporal_ctx.temporal_rank_policy,''')
with open("api/apps/restful_apis/dify_retrieval_api.py", "w") as f:
    f.write(content)
