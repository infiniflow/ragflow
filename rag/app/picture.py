#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import io

import numpy as np
from PIL import Image

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from rag.nlp import tokenize
from deepdoc.vision import OCR

ocr = OCR()


def chunk(filename, binary, tenant_id, lang, callback=None, **kwargs):
    img = Image.open(io.BytesIO(binary)).convert('RGB')
    doc = {
        "docnm_kwd": filename,
        "image": img
    }
    bxs = ocr(np.array(img))
    txt = "\n".join([t[0] for _, t in bxs if t[0]])
    eng = lang.lower() == "english"
    callback(0.4, "Finish OCR: (%s ...)" % txt[:12])
    if (eng and len(txt.split()) > 32) or len(txt) > 32:
        tokenize(doc, txt, eng)
        callback(0.8, "OCR results is too long to use CV LLM.")
        return [doc]

    try:
        callback(0.4, "Use CV LLM to describe the picture.")
        cv_mdl = LLMBundle(tenant_id, LLMType.IMAGE2TEXT, lang=lang)
        img_binary = io.BytesIO()
        img.save(img_binary, format='JPEG')
        img_binary.seek(0)
        ans = cv_mdl.describe(img_binary.read())
        callback(0.8, "CV LLM respond: %s ..." % ans[:32])
        txt += "\n" + ans
        tokenize(doc, txt, eng)
        return [doc]
    except Exception as e:
        callback(prog=-1, msg=str(e))

    return []
