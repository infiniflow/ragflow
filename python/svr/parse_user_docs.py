import json, re, sys, os, hashlib, copy, glob, util, time, random
from util.es_conn import HuEs, Postgres
from util import rmSpace, findMaxDt
from FlagEmbedding import FlagModel
from nlp import huchunk, huqie
import base64, hashlib
from io import BytesIO
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

PDF = PdfChunker(PdfParser())
DOC = DocxChunker(DocxParser())
EXC = ExcelChunker(ExcelParser())
PPT = PptChunker()


def chuck_doc(name):
    name = os.path.split(name)[-1].lower().split(".")[-1]
    if name.find("pdf") >= 0: return PDF(name)
    if name.find("doc") >= 0: return DOC(name)
    if name.find("xlsx") >= 0: return EXC(name)
    if name.find("ppt") >= 0: return PDF(name)
    if name.find("pdf") >= 0: return PPT(name)
    
    if re.match(r"(txt|csv)", name): return TextChunker(name)


def collect(comm, mod, tm):
    sql = f"""
    select 
    did,
    uid,
    doc_name,
    location,
    updated_at
    from docinfo
    where 
    updated_at >= '{tm}' 
    and kb_progress = 0
    and type = 'doc'
    and MOD(uid, {comm}) = {mod}
    order by updated_at asc
    limit 1000
    """
    df = PG.select(sql)
    df = df.fillna("")
    mtm = str(df["updated_at"].max())[:19]
    print("TOTAL:", len(df), "To: ", mtm)
    return df, mtm


def set_progress(did, prog, msg):
    sql = f"""
    update docinfo set kb_progress={prog}, kb_progress_msg='{msg}' where did={did}
    """
    PG.update(sql)


def build(row):
    if row["size"] > 256000000:
        set_progress(row["did"], -1, "File size exceeds( <= 256Mb )")
        return  []
    doc = {
        "doc_id": row["did"],
        "title_tks": huqie.qie(os.path.split(row["location"])[-1]),
        "updated_at": row["updated_at"]
    }
    random.seed(time.time())
    set_progress(row["did"], random.randint(0, 20)/100., "Finished preparing! Start to slice file!")
    obj = chuck_doc(row["location"])
    if not obj: 
        set_progress(row["did"], -1, "Unsuported file type.")
        return  []

    set_progress(row["did"], random.randint(20, 60)/100.)

    output_buffer = BytesIO()
    docs = []
    md5 = hashlib.md5()
    for txt, img in obj.text_chunks:
        d = copy.deepcopy(doc)
        md5.update((txt + str(d["doc_id"])).encode("utf-8"))
        d["_id"] = md5.hexdigest()
        d["content_ltks"] = huqie.qie(txt)
        d["docnm_kwd"] = rmSpace(d["docnm_tks"])
        if not img:
            docs.append(d)
            continue
        img.save(output_buffer, format='JPEG')
        d["img_bin"] = base64.b64encode(output_buffer.getvalue())
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
            d["img_bin"] = base64.b64encode(output_buffer.getvalue())
            docs.append(d)
    set_progress(row["did"], random.randint(60, 70)/100., "Finished slicing. Start to embedding the content.")

    return docs


def index_name(uid):return f"docgpt_{uid}"

def init_kb(row):
    idxnm = index_name(row["uid"])
    if ES.indexExist(idxnm): return
    return ES.createIdx(idxnm, json.load(open("res/mapping.json", "r")))


model = None
def embedding(docs):
    global model
    tts = model.encode([rmSpace(d["title_tks"]) for d in docs])
    cnts = model.encode([rmSpace(d["content_ltks"]) for d in docs])
    vects = 0.1 * tts + 0.9 * cnts
    assert len(vects) == len(docs)
    for i,d in enumerate(docs):d["q_vec"] = vects[i].tolist()
    for d in docs:
        set_progress(d["doc_id"], random.randint(70, 95)/100., 
                     "Finished embedding! Start to build index!")


def main(comm, mod):
    tm_fnm = f"res/{comm}-{mod}.tm"
    tmf = open(tm_fnm, "a+")
    tm = findMaxDt(tm_fnm)
    rows, tm = collect(comm, mod, tm)
    for r in rows:
        if r["is_deleted"]: 
            ES.deleteByQuery(Q("term", dock_id=r["did"]), index_name(r["uid"]))
            continue

        cks = build(r)
        ## TODO: exception handler
        ## set_progress(r["did"], -1, "ERROR: ")
        embedding(cks)
        if cks: init_kb(r)
        ES.bulk(cks, index_name(r["uid"]))
        tmf.write(str(r["updated_at"]) + "\n")
    tmf.close()


if __name__ == "__main__":
    from mpi4py import MPI
    comm = MPI.COMM_WORLD
    rank = comm.Get_rank()
    main(comm, rank)

