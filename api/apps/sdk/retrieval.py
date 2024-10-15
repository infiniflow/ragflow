from flask import request, jsonify

from db import LLMType, ParserType
from db.services.knowledgebase_service import KnowledgebaseService
from db.services.llm_service import LLMBundle
from settings import retrievaler, kg_retrievaler, RetCode
from utils.api_utils import validate_request, build_error_result, apikey_required


@manager.route('/retrieval', methods=['POST'])
@apikey_required
@validate_request("knowledge_id", "query")
def retrieval(tenant_id):
    req = request.json
    question = req["query"]
    kb_id = req["knowledge_id"]
    retrieval_setting = req.get("retrieval_setting", {})
    similarity_threshold = float(retrieval_setting.get("score_threshold", 0.0))
    top = int(retrieval_setting.get("top_k", 1024))

    try:

        e, kb = KnowledgebaseService.get_by_id(kb_id)
        if not e:
            return build_error_result(error_msg="Knowledgebase not found!", retcode=RetCode.NOT_FOUND)

        if kb.tenant_id != tenant_id:
            return build_error_result(error_msg="Knowledgebase not found!", retcode=RetCode.NOT_FOUND)

        embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id)

        retr = retrievaler if kb.parser_id != ParserType.KG else kg_retrievaler
        ranks = retr.retrieval(
            question,
            embd_mdl,
            kb.tenant_id,
            [kb_id],
            page=1,
            size=top,
            similarity_threshold=similarity_threshold,
            vector_similarity_weight=0.3,
            top=top
        )
        records = []
        for c in ranks["chunks"]:
            if "vector" in c:
                del c["vector"]
            records.append({
                "content": c["content_ltks"],
                "score": c["similarity"],
                "title": c["docnm_kwd"],
                "metadata": ""
            })

        return jsonify({"records": records})
    except Exception as e:
        if str(e).find("not_found") > 0:
            return build_error_result(
                error_msg=f'No chunk found! Check the chunk status please!',
                retcode=RetCode.NOT_FOUND
            )
        return build_error_result(error_msg=str(e), retcode=RetCode.SERVER_ERROR)
