# -*- coding: utf-8 -*-
from rag.nlp import find_codec
import readability
import html_text
import chardet

def get_encoding(file):
    with open(file,'rb') as f:
        tmp = chardet.detect(f.read())
        return tmp['encoding']
    
class RAGFlowHtmlParser:
    def __call__(self, fnm, binary=None):
        txt = ""
        if binary:
            encoding = find_codec(binary)
            txt = binary.decode(encoding, errors="ignore")
        else:
            with open(fnm, "r",encoding=get_encoding(fnm)) as f:
                txt = f.read()
            
        html_doc = readability.Document(txt)
        title = html_doc.title()
        content = html_text.extract_text(html_doc.summary(html_partial=True))
        txt = f'{title}\n{content}'
        sections = txt.split("\n")
        return sections
