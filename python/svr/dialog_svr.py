#-*- coding:utf-8 -*-
import sys, os, re,inspect,json,traceback,logging,argparse, copy
sys.path.append(os.path.realpath(os.path.dirname(inspect.getfile(inspect.currentframe())))+"/../")
from tornado.web import RequestHandler,Application
from tornado.ioloop import IOLoop
from tornado.httpserver import HTTPServer
from tornado.options import define,options
from util import es_conn, setup_logging
from sklearn.metrics.pairwise import cosine_similarity as CosineSimilarity
from nlp import huqie
from nlp import query as Query
from nlp import search
from llm import HuEmbedding, GptTurbo
import numpy as np
from io import BytesIO
from util import config
from timeit import default_timer as timer
from collections import OrderedDict
from llm import ChatModel, EmbeddingModel

SE = None
CFIELD="content_ltks"
EMBEDDING = EmbeddingModel
LLM = ChatModel

def get_QA_pairs(hists):
    pa = []
    for h in hists:
        for k in ["user", "assistant"]:
            if h.get(k):
                pa.append({
                    "content": h[k],
                    "role": k,
                })

    for p in pa[:-1]: assert len(p) == 2, p
    return pa



def get_instruction(sres, top_i, max_len=8096, fld="content_ltks"):
    max_len //= len(top_i)
    # add instruction to prompt
    instructions = [re.sub(r"[\r\n]+", " ", sres.field[sres.ids[i]][fld]) for i in top_i]
    if len(instructions)>2:
        # Said that LLM is sensitive to the first and the last one, so
        # rearrange the order of references
        instructions.append(copy.deepcopy(instructions[1]))
        instructions.pop(1)

    def token_num(txt):
        c = 0
        for tk in re.split(r"[，。/？‘’”“：；:;!！]", txt):
            if re.match(r"[a-zA-Z-]+$", tk):
                c += 1
                continue
            c += len(tk)
        return c

    _inst = ""
    for ins in instructions:
        if token_num(_inst) > 4096:
            _inst += "\n知识库：" + instructions[-1][:max_len]
            break
        _inst += "\n知识库：" + ins[:max_len]
    return _inst


def prompt_and_answer(history, inst):
    hist = get_QA_pairs(history)
    chks = []
    for s in re.split(r"[：:;；。\n\r]+", inst):
        if s: chks.append(s)
    chks = len(set(chks))/(0.1+len(chks))
    print("Duplication portion:", chks)
    
    system = """
你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答%s。当所有知识库内容都与问题无关时，你的回答必须包括"知识库中未找到您要的答案！这是我所知道的，仅作参考。"这句话。回答需要考虑聊天历史。
以下是知识库：
%s
以上是知识库。
"""%(("，最好总结成表格" if chks<0.6 and chks>0 else ""), inst)

    print("【PROMPT】:", system)
    start = timer()
    response = LLM.chat(system, hist, {"temperature": 0.2, "max_tokens": 512})
    print("GENERATE: ", timer()-start)
    print("===>>", response)
    return response


class Handler(RequestHandler):
    def post(self):
        global SE,MUST_TK_NUM
        param = json.loads(self.request.body.decode('utf-8'))
        try:
            question = param.get("history",[{"user": "Hi!"}])[-1]["user"]
            res = SE.search({
                    "question": question,
                    "kb_ids": param.get("kb_ids", []),
                    "size": param.get("topn", 15)},
               search.index_name(param["uid"]) 
            )

            sim = SE.rerank(res, question)  
            rk_idx = np.argsort(sim*-1)
            topidx = [i for i in rk_idx if sim[i] >= aram.get("similarity", 0.5)][:param.get("topn",12)]
            inst = get_instruction(res, topidx)

            ans, topidx = prompt_and_answer(param["history"], inst)
            ans = SE.insert_citations(ans, topidx, res)

            refer = OrderedDict()
            docnms = {}
            for i in rk_idx:
                 did = res.field[res.ids[i]]["doc_id"]
                 if did not in docnms: docnms[did] = res.field[res.ids[i]]["docnm_kwd"]
                 if did not in refer: refer[did] = []
                 refer[did].append({
                     "chunk_id": res.ids[i],
                     "content": res.field[res.ids[i]]["content_ltks"],
                     "image": ""
                 })

            print("::::::::::::::", ans)
            self.write(json.dumps({
                "code":0,
                "msg":"success",
                "data":{
                    "uid": param["uid"],
                    "dialog_id": param["dialog_id"],
                    "assistant": ans,
                    "refer": [{
                        "did": did,
                        "doc_name": docnms[did],
                        "chunks": chunks
                    } for did, chunks in refer.items()]
                }
            }))
            logging.info("SUCCESS[%d]"%(res.total)+json.dumps(param, ensure_ascii=False))

        except Exception as e:
            logging.error("Request 500: "+str(e))
            self.write(json.dumps({
                "code":500,
                "msg":str(e),
                "data":{}
            }))
            print(traceback.format_exc())


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", default=4455, type=int, help="Port used for service")
    ARGS = parser.parse_args()
    
    SE = search.Dealer(es_conn.HuEs("infiniflow"), EMBEDDING)

    app = Application([(r'/v1/chat/completions', Handler)],debug=False)
    http_server = HTTPServer(app)
    http_server.bind(ARGS.port)
    http_server.start(3)

    IOLoop.current().start()

