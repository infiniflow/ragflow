import re


def callback__(progress, msg, func):
    if not func :return
    func(progress, msg)


BULLET_PATTERN = [[
        r"第[零一二三四五六七八九十百]+编",
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
