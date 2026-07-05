with open("api/apps/restful_apis/chunk_api.py", "r") as f:
    content = f.read()

content = content.replace('''<<<<<<< HEAD
from common.metadata_utils import convert_conditions
from common.temporal_retrieval import merge_temporal_reference_fields, resolve_temporal_retrieval_context
=======
from common.doc_store.doc_store_base import OrderByExpr
from common.metadata_utils import convert_conditions, meta_filter
>>>>>>> origin/infinitiflow-main''', '''from common.doc_store.doc_store_base import OrderByExpr
from common.metadata_utils import convert_conditions, meta_filter
from common.temporal_retrieval import merge_temporal_reference_fields, resolve_temporal_retrieval_context''')

content = content.replace('''<<<<<<< HEAD
            question, embd_mdl, tenant_ids, kb_ids, page, size, similarity_threshold,
            vector_similarity_weight, top, doc_ids, rerank_mdl=rerank_mdl,
            highlight=highlight, rank_feature=label_question(question, kbs),
            temporal_rank_policy=temporal_ctx.temporal_rank_policy,
=======
            question,
            embd_mdl,
            tenant_ids,
            kb_ids,
            page,
            size,
            similarity_threshold,
            vector_similarity_weight,
            top,
            doc_ids,
            rerank_mdl=rerank_mdl,
            highlight=highlight,
            rank_feature=label_question(question, kbs),
>>>>>>> origin/infinitiflow-main''', '''            question,
            embd_mdl,
            tenant_ids,
            kb_ids,
            page,
            size,
            similarity_threshold,
            vector_similarity_weight,
            top,
            doc_ids,
            rerank_mdl=rerank_mdl,
            highlight=highlight,
            rank_feature=label_question(question, kbs),
            temporal_rank_policy=temporal_ctx.temporal_rank_policy,''')
with open("api/apps/restful_apis/chunk_api.py", "w") as f:
    f.write(content)
