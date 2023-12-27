import sys, datetime, random, re, cv2
from os.path import dirname, realpath
sys.path.append(dirname(realpath(__file__)) + "/../")
from util.db_conn import Postgres
from util.minio_conn import HuMinio
from util import findMaxDt
import base64
from io import BytesIO
import pandas as pd
from PIL import Image
import pdfplumber


PG = Postgres("infiniflow", "docgpt")
MINIO = HuMinio("infiniflow")
def set_thumbnail(did, base64):
    sql = f"""
    update doc_info set thumbnail_base64='{base64}' 
    where
    did={did}
    """
    PG.update(sql)


def collect(comm, mod, tm):
    sql = f"""
    select 
    did, uid, doc_name, location, updated_at
    from doc_info
    where
    updated_at >= '{tm}'
    and MOD(did, {comm}) = {mod}
    and is_deleted=false
    and type <> 'folder'
    and thumbnail_base64=''
    order by updated_at asc
    limit 10
    """
    docs = PG.select(sql)
    if len(docs) == 0:return pd.DataFrame()

    mtm = str(docs["updated_at"].max())[:19]
    print("TOTAL:", len(docs), "To: ", mtm)
    return docs


def build(row):
    if not re.search(r"\.(pdf|jpg|jpeg|png|gif|svg|apng|icon|ico|webp|mpg|mpeg|avi|rm|rmvb|mov|wmv|mp4)$",
              row["doc_name"].lower().strip()):
        set_thumbnail(row["did"], "_")
        return

    def thumbnail(img, SIZE=128):
        w,h = img.size
        p = SIZE/max(w, h)
        w, h = int(w*p), int(h*p)
        img.thumbnail((w, h))
        buffered = BytesIO()
        try:
            img.save(buffered, format="JPEG")
        except Exception as e:
            try:
                img.save(buffered, format="PNG")
            except Exception as ee:
                pass
        return base64.b64encode(buffered.getvalue()).decode("utf-8")


    iobytes = BytesIO(MINIO.get("%s-upload"%str(row["uid"]), row["location"]))
    if re.search(r"\.pdf$", row["doc_name"].lower().strip()):
        pdf = pdfplumber.open(iobytes)
        img = pdf.pages[0].to_image().annotated
        set_thumbnail(row["did"], thumbnail(img))

    if re.search(r"\.(jpg|jpeg|png|gif|svg|apng|webp|icon|ico)$", row["doc_name"].lower().strip()):
        img = Image.open(iobytes)
        set_thumbnail(row["did"], thumbnail(img))

    if re.search(r"\.(mpg|mpeg|avi|rm|rmvb|mov|wmv|mp4)$", row["doc_name"].lower().strip()):
        url  = MINIO.get_presigned_url("%s-upload"%str(row["uid"]),
                                       row["location"],
                                       expires=datetime.timedelta(seconds=60)
                                      )
        cap = cv2.VideoCapture(url)
        succ = cap.isOpened()
        i = random.randint(1, 11)
        while succ:
            ret, frame = cap.read()
            if not ret: break
            if i > 0:
                i -= 1
                continue
            img = Image.fromarray(cv2.cvtColor(frame, cv2.COLOR_BGR2RGB))
            print(img.size)
            set_thumbnail(row["did"], thumbnail(img))
        cap.release()
        cv2.destroyAllWindows()


def main(comm, mod):
    global model
    tm_fnm = f"res/thumbnail-{comm}-{mod}.tm"
    tm = findMaxDt(tm_fnm)
    rows = collect(comm, mod, tm)
    if len(rows) == 0:return

    tmf = open(tm_fnm, "a+")
    for _, r in rows.iterrows():
        build(r)
        tmf.write(str(r["updated_at"]) + "\n")
    tmf.close()


if __name__ == "__main__":
    from mpi4py import MPI
    comm = MPI.COMM_WORLD
    main(comm.Get_size(), comm.Get_rank())

