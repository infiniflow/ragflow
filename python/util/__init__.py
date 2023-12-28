import re


def rmSpace(txt):
    txt = re.sub(r"([^a-z0-9.,]) +([^ ])", r"\1\2", txt)
    return re.sub(r"([^ ]) +([^a-z0-9.,])", r"\1\2", txt)


def findMaxDt(fnm):
    m = "1970-01-01 00:00:00"
    try:
        with open(fnm, "r") as f:
            while True:
                l = f.readline()
                if not l:
                    break
                l = l.strip("\n")
                if l == 'nan':
                    continue
                if l > m:
                    m = l
    except Exception as e:
        print("WARNING: can't find " + fnm)
    return m
