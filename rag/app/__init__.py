import re

from nltk import word_tokenize

from rag.nlp import stemmer, huqie

BULLET_PATTERN = [[
        r"第[零一二三四五六七八九十百]+(编|部分)",
        r"第[零一二三四五六七八九十百]+章",
        r"第[零一二三四五六七八九十百]+节",
        r"第[零一二三四五六七八九十百]+条",
        r"[\(（][零一二三四五六七八九十百]+[\)）]",
    ], [
        r"[0-9]{,3}[\. 、]",
        r"[0-9]{,2}\.[0-9]{,2}",
        r"[0-9]{,2}\.[0-9]{,2}\.[0-9]{,2}",
        r"[0-9]{,2}\.[0-9]{,2}\.[0-9]{,2}\.[0-9]{,2}",
    ], [
        r"第[零一二三四五六七八九十百]+章",
        r"第[零一二三四五六七八九十百]+节",
        r"[零一二三四五六七八九十百]+[ 、]",
        r"[\(（][零一二三四五六七八九十百]+[\)）]",
        r"[\(（][0-9]{,2}[\)）]",
    ] ,[
        r"PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)",
        r"Chapter (I+V?|VI*|XI|IX|X)",
        r"Section [0-9]+",
        r"Article [0-9]+"
    ]
    ]


def bullets_category(sections):
    global BULLET_PATTERN
    hits = [0] * len(BULLET_PATTERN)
    for i, pro in enumerate(BULLET_PATTERN):
        for sec in sections:
            for p in pro:
                if re.match(p, sec):
                    hits[i] += 1
                    break
    maxium = 0
    res = -1
    for i,h in enumerate(hits):
        if h <= maxium:continue
        res = i
        maxium = h
    return res

def is_english(texts):
    eng = 0
    for t in texts:
        if re.match(r"[a-zA-Z]{2,}", t.strip()):
            eng += 1
    if eng / len(texts) > 0.8:
        return True
    return False

def tokenize(d, t, eng):
    d["content_with_weight"] = t
    if eng:
        t = re.sub(r"([a-z])-([a-z])", r"\1\2", t)
        d["content_ltks"] = " ".join([stemmer.stem(w) for w in word_tokenize(t)])
    else:
        d["content_ltks"] = huqie.qie(t)
        d["content_sm_ltks"] = huqie.qieqie(d["content_ltks"])


def remove_contents_table(sections, eng=False):
    i = 0
    while i < len(sections):
        def get(i):
            nonlocal sections
            return (sections[i] if type(sections[i]) == type("") else sections[i][0]).strip()
        if not re.match(r"(contents|目录|目次|table of contents|致谢|acknowledge)$", re.sub(r"( | |\u3000)+", "", get(i).split("@@")[0], re.IGNORECASE)):
            i += 1
            continue
        sections.pop(i)
        if i >= len(sections): break
        prefix = get(i)[:3] if not eng else " ".join(get(i).split(" ")[:2])
        while not prefix:
            sections.pop(i)
            if i >= len(sections): break
            prefix = get(i)[:3] if not eng else " ".join(get(i).split(" ")[:2])
        sections.pop(i)
        if i >= len(sections) or not prefix: break
        for j in range(i, min(i+128, len(sections))):
            if not re.match(prefix, get(j)):
                continue
            for _ in range(i, j):sections.pop(i)
            break