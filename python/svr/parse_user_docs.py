import json, os, sys, hashlib, copy, time, random, re, logging, torch
from os.path import dirname, realpath
sys.path.append(dirname(realpath(__file__)) + "/../")
from util.es_conn import HuEs
from util.db_conn import Postgres
from util.minio_conn import HuMinio
from util import rmSpace, findMaxDt
from FlagEmbedding import FlagModel
from nlp import huchunk, huqie, search
import base64, hashlib
from io import BytesIO
import pandas as pd
from elasticsearch_dsl import Q
from parser import (
    PdfParser,
    DocxParser,
    ExcelParser
)
from nlp.huchunk import (
    PdfChunker,
    DocxChunker,
    ExcelChunker,
    PptChunker,
    TextChunker
)

ES = HuEs("infiniflow")
BATCH_SIZE = 64
PG = Postgres("infiniflow", "docgpt")
MINIO = HuMinio("infiniflow")

PDF = PdfChunker(PdfParser())
DOC = DocxChunker(DocxParser())
EXC = ExcelChunker(ExcelParser())
PPT = PptChunker()

def chuck_doc(name, binary):
    suff = os.path.split(name)[-1].lower().split(".")[-1]
    if suff.find("pdf") >= 0: return PDF(binary)
    if suff.find("doc") >= 0: return DOC(binary)
    if re.match(r"(xlsx|xlsm|xltx|xltm)", suff): return EXC(binary)
    if suff.find("ppt") >= 0: return PPT(binary)
    
    return TextChunker()(binary)


def collect(comm, mod, tm):
    sql = f"""
    select 
    id as kb2doc_id,
    kb_id,
    did,
    updated_at,
    is_deleted
    from kb2_doc
    where
    updated_at >= '{tm}'
    and kb_progress = 0
    and MOD(did, {comm}) = {mod}
    order by updated_at asc
    limit 1000
    """
    kb2doc = PG.select(sql)
    if len(kb2doc) == 0:return pd.DataFrame()

    sql = """
    select 
    did,
    uid,
    doc_name,
    location,
    size
    from doc_info
    where 
    did in (%s)
    """%",".join([str(i) for i in kb2doc["did"].unique()])
    docs = PG.select(sql)
    docs = docs.fillna("")
    docs = docs.join(kb2doc.set_index("did"), on="did", how="left")

    mtm = str(docs["updated_at"].max())[:19]
    print("TOTAL:", len(docs), "To: ", mtm)
    return docs


def set_progress(kb2doc_id, prog, msg="Processing..."):
    sql = f"""
    update kb2_doc set kb_progress={prog}, kb_progress_msg='{msg}' 
    where
    id={kb2doc_id}
    """
    PG.update(sql)


def build(row):
    if row["size"] > 256000000:
        set_progress(row["kb2doc_id"], -1, "File size exceeds( <= 256Mb )")
        return  []
    res = ES.search(Q("term", doc_id=row["did"]))
    if ES.getTotal(res) > 0:
        ES.updateScriptByQuery(Q("term", doc_id=row["did"]), 
                               scripts="""
                               if(!ctx._source.kb_id.contains('%s'))
                                 ctx._source.kb_id.add('%s');
                               """%(str(row["kb_id"]), str(row["kb_id"])),
                               idxnm = search.index_name(row["uid"])
                              )
        set_progress(row["kb2doc_id"], 1, "Done")
        return []

    random.seed(time.time())
    set_progress(row["kb2doc_id"], random.randint(0, 20)/100., "Finished preparing! Start to slice file!")
    try:
        obj = chuck_doc(row["doc_name"], MINIO.get("%s-upload"%str(row["uid"]), row["location"]))
    except Exception as e:
        if re.search("(No such file|not found)", str(e)):
            set_progress(row["kb2doc_id"], -1, "Can not find file <%s>"%row["doc_name"])
        else:
            set_progress(row["kb2doc_id"], -1, f"Internal system error: %s"%str(e).replace("'", ""))
        return []

    print(row["doc_name"], obj)
    if not obj.text_chunks and not obj.table_chunks: 
        set_progress(row["kb2doc_id"], 1, "Nothing added! Mostly, file type unsupported yet.")
        return  []

    set_progress(row["kb2doc_id"], random.randint(20, 60)/100., "Finished slicing files. Start to embedding the content.")

    doc = {
        "doc_id": row["did"],
        "kb_id": [str(row["kb_id"])],
        "docnm_kwd": os.path.split(row["location"])[-1],
        "title_tks": huqie.qie(os.path.split(row["location"])[-1]),
        "updated_at": str(row["updated_at"]).replace("T", " ")[:19]
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    output_buffer = BytesIO()
    docs = []
    md5 = hashlib.md5()
    for txt, img in obj.text_chunks:
        d = copy.deepcopy(doc)
        md5.update((txt + str(d["doc_id"])).encode("utf-8"))
        d["_id"] = md5.hexdigest()
        d["content_ltks"] = huqie.qie(txt)
        d["content_sm_ltks"] = huqie.qieqie(d["content_ltks"])
        if not img:
            docs.append(d)
            continue
        img.save(output_buffer, format='JPEG')
        MINIO.put("{}-{}".format(row["uid"], row["kb_id"]), d["_id"],
                      output_buffer.getvalue())
        d["img_id"] = "{}-{}".format(row["uid"], row["kb_id"])
        docs.append(d)

    for arr, img in obj.table_chunks:
        for i, txt in enumerate(arr):
            d = copy.deepcopy(doc)
            d["content_ltks"] = huqie.qie(txt)
            md5.update((txt + str(d["doc_id"])).encode("utf-8"))
            d["_id"] = md5.hexdigest()
            if not img:
                docs.append(d)
                continue
            img.save(output_buffer, format='JPEG')
            MINIO.put("{}-{}".format(row["uid"], row["kb_id"]), d["_id"],
                      output_buffer.getvalue())
            d["img_id"] = "{}-{}".format(row["uid"], row["kb_id"])
            docs.append(d)
    set_progress(row["kb2doc_id"], random.randint(60, 70)/100., "Continue embedding the content.")

    return docs


def init_kb(row):
    idxnm = search.index_name(row["uid"])
    if ES.indexExist(idxnm): return
    return ES.createIdx(idxnm, json.load(open("conf/mapping.json", "r")))


model = None
def embedding(docs):
    global model
    tts = model.encode([rmSpace(d["title_tks"]) for d in docs])
    cnts = model.encode([rmSpace(d["content_ltks"]) for d in docs])
    vects = 0.1 * tts + 0.9 * cnts
    assert len(vects) == len(docs)
    for i,d in enumerate(docs):d["q_vec"] = vects[i].tolist()


def rm_doc_from_kb(df):
    if len(df) == 0:return
    for _,r in df.iterrows():
        ES.updateScriptByQuery(Q("term", doc_id=r["did"]), 
                               scripts="""
                               if(ctx._source.kb_id.contains('%s'))
                                 ctx._source.kb_id.remove(
                                     ctx._source.kb_id.indexOf('%s')
                               );
                                """%(str(r["kb_id"]),str(r["kb_id"])),
                               idxnm = search.index_name(r["uid"])
                              )
    if len(df) == 0:return
    sql = """
    delete from kb2_doc where id in (%s)
    """%",".join([str(i) for i in df["kb2doc_id"]])
    PG.update(sql)


def main(comm, mod):
    global model
    from llm import HuEmbedding
    model = HuEmbedding()
    tm_fnm = f"res/{comm}-{mod}.tm"
    tm = findMaxDt(tm_fnm)
    rows = collect(comm, mod, tm)
    if len(rows) == 0:return

    rm_doc_from_kb(rows.loc[rows.is_deleted == True])
    rows = rows.loc[rows.is_deleted == False].reset_index(drop=True)
    if len(rows) == 0:return
    tmf = open(tm_fnm, "a+")
    for _, r in rows.iterrows():
        cks = build(r)
        if not cks:
            tmf.write(str(r["updated_at"]) + "\n")
            continue
        ## TODO: exception handler
        ## set_progress(r["did"], -1, "ERROR: ")
        embedding(cks)

        set_progress(r["kb2doc_id"], random.randint(70, 95)/100., 
                     "Finished embedding! Start to build index!")
        init_kb(r)
        es_r = ES.bulk(cks, search.index_name(r["uid"]))
        if es_r:
            set_progress(r["kb2doc_id"], -1, "Index failure!")
            print(es_r)
        else: set_progress(r["kb2doc_id"], 1., "Done!")
        tmf.write(str(r["updated_at"]) + "\n")
    tmf.close()


if __name__ == "__main__":
    from mpi4py import MPI
    comm = MPI.COMM_WORLD
    main(comm.Get_size(), comm.Get_rank())

