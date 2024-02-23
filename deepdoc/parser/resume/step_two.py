# -*- coding: utf-8 -*-
import re, copy, time, datetime, demjson, \
    traceback, signal
import numpy as np
from deepdoc.parser.resume.entities import degrees, schools, corporations
from rag.nlp import huqie, surname
from xpinyin import Pinyin
from contextlib import contextmanager


class TimeoutException(Exception): pass


@contextmanager
def time_limit(seconds):
    def signal_handler(signum, frame):
        raise TimeoutException("Timed out!")

    signal.signal(signal.SIGALRM, signal_handler)
    signal.alarm(seconds)
    try:
        yield
    finally:
        signal.alarm(0)


ENV = None
PY = Pinyin()


def rmHtmlTag(line):
    return re.sub(r"<[a-z0-9.\"=';,:\+_/ -]+>", " ", line, 100000, re.IGNORECASE)


def highest_degree(dg):
    if not dg: return ""
    if type(dg) == type(""): dg = [dg]
    m = {"初中": 0, "高中": 1, "中专": 2, "大专": 3, "专升本": 4, "本科": 5, "硕士": 6, "博士": 7, "博士后": 8}
    return sorted([(d, m.get(d, -1)) for d in dg], key=lambda x: x[1] * -1)[0][0]


def forEdu(cv):
    if not cv.get("education_obj"):
        cv["integerity_flt"] *= 0.8
        return cv

    first_fea, fea, maj, fmaj, deg, fdeg, sch, fsch, st_dt, ed_dt = [], [], [], [], [], [], [], [], [], []
    edu_nst = []
    edu_end_dt = ""
    cv["school_rank_int"] = 1000000
    for ii, n in enumerate(sorted(cv["education_obj"], key=lambda x: x.get("start_time", "3"))):
        e = {}
        if n.get("end_time"):
            if n["end_time"] > edu_end_dt: edu_end_dt = n["end_time"]
            try:
                dt = n["end_time"]
                if re.match(r"[0-9]{9,}", dt): dt = turnTm2Dt(dt)
                y, m, d = getYMD(dt)
                ed_dt.append(str(y))
                e["end_dt_kwd"] = str(y)
            except Exception as e:
                pass
        if n.get("start_time"):
            try:
                dt = n["start_time"]
                if re.match(r"[0-9]{9,}", dt): dt = turnTm2Dt(dt)
                y, m, d = getYMD(dt)
                st_dt.append(str(y))
                e["start_dt_kwd"] = str(y)
            except Exception as e:
                pass

        r = schools.select(n.get("school_name", ""))
        if r:
            if str(r.get("type", "")) == "1": fea.append("211")
            if str(r.get("type", "")) == "2": fea.append("211")
            if str(r.get("is_abroad", "")) == "1": fea.append("留学")
            if str(r.get("is_double_first", "")) == "1": fea.append("双一流")
            if str(r.get("is_985", "")) == "1": fea.append("985")
            if str(r.get("is_world_known", "")) == "1": fea.append("海外知名")
            if r.get("rank") and cv["school_rank_int"] > r["rank"]: cv["school_rank_int"] = r["rank"]

        if n.get("school_name") and isinstance(n["school_name"], str):
            sch.append(re.sub(r"(211|985|重点大学|[,&;；-])", "", n["school_name"]))
            e["sch_nm_kwd"] = sch[-1]
        fea.append(huqie.qieqie(huqie.qie(n.get("school_name", ""))).split(" ")[-1])

        if n.get("discipline_name") and isinstance(n["discipline_name"], str):
            maj.append(n["discipline_name"])
            e["major_kwd"] = n["discipline_name"]

        if not n.get("degree") and "985" in fea and not first_fea: n["degree"] = "1"

        if n.get("degree"):
            d = degrees.get_name(n["degree"])
            if d: e["degree_kwd"] = d
            if d == "本科" and ("专科" in deg or "专升本" in deg or "中专" in deg or "大专" in deg or re.search(r"(成人|自考|自学考试)",
                                                                                                     n.get(
                                                                                                         "school_name",
                                                                                                         ""))): d = "专升本"
            if d: deg.append(d)

            # for first degree
            if not fdeg and d in ["中专", "专升本", "专科", "本科", "大专"]:
                fdeg = [d]
                if n.get("school_name"): fsch = [n["school_name"]]
                if n.get("discipline_name"): fmaj = [n["discipline_name"]]
                first_fea = copy.deepcopy(fea)

        edu_nst.append(e)

    cv["sch_rank_kwd"] = []
    if cv["school_rank_int"] <= 20 \
            or ("海外名校" in fea and cv["school_rank_int"] <= 200):
        cv["sch_rank_kwd"].append("顶尖学校")
    elif cv["school_rank_int"] <= 50 and cv["school_rank_int"] > 20 \
            or ("海外名校" in fea and cv["school_rank_int"] <= 500 and \
                cv["school_rank_int"] > 200):
        cv["sch_rank_kwd"].append("精英学校")
    elif cv["school_rank_int"] > 50 and ("985" in fea or "211" in fea) \
            or ("海外名校" in fea and cv["school_rank_int"] > 500):
        cv["sch_rank_kwd"].append("优质学校")
    else:
        cv["sch_rank_kwd"].append("一般学校")

    if edu_nst: cv["edu_nst"] = edu_nst
    if fea: cv["edu_fea_kwd"] = list(set(fea))
    if first_fea: cv["edu_first_fea_kwd"] = list(set(first_fea))
    if maj: cv["major_kwd"] = maj
    if fsch: cv["first_school_name_kwd"] = fsch
    if fdeg: cv["first_degree_kwd"] = fdeg
    if fmaj: cv["first_major_kwd"] = fmaj
    if st_dt: cv["edu_start_kwd"] = st_dt
    if ed_dt: cv["edu_end_kwd"] = ed_dt
    if ed_dt: cv["edu_end_int"] = max([int(t) for t in ed_dt])
    if deg:
        if "本科" in deg and "专科" in deg:
            deg.append("专升本")
            deg = [d for d in deg if d != '本科']
        cv["degree_kwd"] = deg
        cv["highest_degree_kwd"] = highest_degree(deg)
    if edu_end_dt:
        try:
            if re.match(r"[0-9]{9,}", edu_end_dt): edu_end_dt = turnTm2Dt(edu_end_dt)
            if edu_end_dt.strip("\n") == "至今": edu_end_dt = cv.get("updated_at_dt", str(datetime.date.today()))
            y, m, d = getYMD(edu_end_dt)
            cv["work_exp_flt"] = min(int(str(datetime.date.today())[0:4]) - int(y), cv.get("work_exp_flt", 1000))
        except Exception as e:
            print("EXCEPTION: ", e, edu_end_dt, cv.get("work_exp_flt"))
    if sch:
        cv["school_name_kwd"] = sch
        if (len(cv.get("degree_kwd", [])) >= 1 and "本科" in cv["degree_kwd"]) \
                or all([c.lower() in ["硕士", "博士", "mba", "博士后"] for c in cv.get("degree_kwd", [])]) \
                or not cv.get("degree_kwd"):
            for c in sch:
                if schools.is_good(c):
                    if "tag_kwd" not in cv: cv["tag_kwd"] = []
                    cv["tag_kwd"].append("好学校")
                    cv["tag_kwd"].append("好学历")
                    break
        if (len(cv.get("degree_kwd", [])) >= 1 and \
            "本科" in cv["degree_kwd"] and \
            any([d.lower() in ["硕士", "博士", "mba", "博士"] for d in cv.get("degree_kwd", [])])) \
                or all([d.lower() in ["硕士", "博士", "mba", "博士后"] for d in cv.get("degree_kwd", [])]) \
                or any([d in ["mba", "emba", "博士后"] for d in cv.get("degree_kwd", [])]):
            if "tag_kwd" not in cv: cv["tag_kwd"] = []
            if "好学历" not in cv["tag_kwd"]: cv["tag_kwd"].append("好学历")

    if cv.get("major_kwd"): cv["major_tks"] = huqie.qie(" ".join(maj))
    if cv.get("school_name_kwd"): cv["school_name_tks"] = huqie.qie(" ".join(sch))
    if cv.get("first_school_name_kwd"): cv["first_school_name_tks"] = huqie.qie(" ".join(fsch))
    if cv.get("first_major_kwd"): cv["first_major_tks"] = huqie.qie(" ".join(fmaj))

    return cv


def forProj(cv):
    if not cv.get("project_obj"): return cv

    pro_nms, desc = [], []
    for i, n in enumerate(
            sorted(cv.get("project_obj", []), key=lambda x: str(x.get("updated_at", "")) if type(x) == type({}) else "",
                   reverse=True)):
        if n.get("name"): pro_nms.append(n["name"])
        if n.get("describe"): desc.append(str(n["describe"]))
        if n.get("responsibilities"): desc.append(str(n["responsibilities"]))
        if n.get("achivement"): desc.append(str(n["achivement"]))

    if pro_nms:
        # cv["pro_nms_tks"] = huqie.qie(" ".join(pro_nms))
        cv["project_name_tks"] = huqie.qie(pro_nms[0])
    if desc:
        cv["pro_desc_ltks"] = huqie.qie(rmHtmlTag(" ".join(desc)))
        cv["project_desc_ltks"] = huqie.qie(rmHtmlTag(desc[0]))

    return cv


def json_loads(line):
    return demjson.decode(re.sub(r": *(True|False)", r": '\1'", line))


def forWork(cv):
    if not cv.get("work_obj"):
        cv["integerity_flt"] *= 0.7
        return cv

    flds = ["position_name", "corporation_name", "corporation_id", "responsibilities",
            "industry_name", "subordinates_count"]
    duas = []
    scales = []
    fea = {c: [] for c in flds}
    latest_job_tm = ""
    goodcorp = False
    goodcorp_ = False
    work_st_tm = ""
    corp_tags = []
    for i, n in enumerate(
            sorted(cv.get("work_obj", []), key=lambda x: str(x.get("start_time", "")) if type(x) == type({}) else "",
                   reverse=True)):
        if type(n) == type(""):
            try:
                n = json_loads(n)
            except Exception as e:
                continue

        if n.get("start_time") and (not work_st_tm or n["start_time"] < work_st_tm): work_st_tm = n["start_time"]
        for c in flds:
            if not n.get(c) or str(n[c]) == '0':
                fea[c].append("")
                continue
            if c == "corporation_name":
                n[c] = corporations.corpNorm(n[c], False)
                if corporations.is_good(n[c]):
                    if i == 0:
                        goodcorp = True
                    else:
                        goodcorp_ = True
                ct = corporations.corp_tag(n[c])
                if i == 0:
                    corp_tags.extend(ct)
                elif ct and ct[0] != "软外":
                    corp_tags.extend([f"{t}(曾)" for t in ct])

            fea[c].append(rmHtmlTag(str(n[c]).lower()))

        y, m, d = getYMD(n.get("start_time"))
        if not y or not m: continue
        st = "%s-%02d-%02d" % (y, int(m), int(d))
        latest_job_tm = st

        y, m, d = getYMD(n.get("end_time"))
        if (not y or not m) and i > 0: continue
        if not y or not m or int(y) > 2022:  y, m, d = getYMD(str(n.get("updated_at", "")))
        if not y or not m: continue
        ed = "%s-%02d-%02d" % (y, int(m), int(d))

        try:
            duas.append((datetime.datetime.strptime(ed, "%Y-%m-%d") - datetime.datetime.strptime(st, "%Y-%m-%d")).days)
        except Exception as e:
            print("kkkkkkkkkkkkkkkkkkkk", n.get("start_time"), n.get("end_time"))

        if n.get("scale"):
            r = re.search(r"^([0-9]+)", str(n["scale"]))
            if r: scales.append(int(r.group(1)))

    if goodcorp:
        if "tag_kwd" not in cv: cv["tag_kwd"] = []
        cv["tag_kwd"].append("好公司")
    if goodcorp_:
        if "tag_kwd" not in cv: cv["tag_kwd"] = []
        cv["tag_kwd"].append("好公司(曾)")

    if corp_tags:
        if "tag_kwd" not in cv: cv["tag_kwd"] = []
        cv["tag_kwd"].extend(corp_tags)
        cv["corp_tag_kwd"] = [c for c in corp_tags if re.match(r"(综合|行业)", c)]

    if latest_job_tm: cv["latest_job_dt"] = latest_job_tm
    if fea["corporation_id"]: cv["corporation_id"] = fea["corporation_id"]

    if fea["position_name"]:
        cv["position_name_tks"] = huqie.qie(fea["position_name"][0])
        cv["position_name_sm_tks"] = huqie.qieqie(cv["position_name_tks"])
        cv["pos_nm_tks"] = huqie.qie(" ".join(fea["position_name"][1:]))

    if fea["industry_name"]:
        cv["industry_name_tks"] = huqie.qie(fea["industry_name"][0])
        cv["industry_name_sm_tks"] = huqie.qieqie(cv["industry_name_tks"])
        cv["indu_nm_tks"] = huqie.qie(" ".join(fea["industry_name"][1:]))

    if fea["corporation_name"]:
        cv["corporation_name_kwd"] = fea["corporation_name"][0]
        cv["corp_nm_kwd"] = fea["corporation_name"]
        cv["corporation_name_tks"] = huqie.qie(fea["corporation_name"][0])
        cv["corporation_name_sm_tks"] = huqie.qieqie(cv["corporation_name_tks"])
        cv["corp_nm_tks"] = huqie.qie(" ".join(fea["corporation_name"][1:]))

    if fea["responsibilities"]:
        cv["responsibilities_ltks"] = huqie.qie(fea["responsibilities"][0])
        cv["resp_ltks"] = huqie.qie(" ".join(fea["responsibilities"][1:]))

    if fea["subordinates_count"]: fea["subordinates_count"] = [int(i) for i in fea["subordinates_count"] if
                                                               re.match(r"[^0-9]+$", str(i))]
    if fea["subordinates_count"]: cv["max_sub_cnt_int"] = np.max(fea["subordinates_count"])

    if type(cv.get("corporation_id")) == type(1): cv["corporation_id"] = [str(cv["corporation_id"])]
    if not cv.get("corporation_id"): cv["corporation_id"] = []
    for i in cv.get("corporation_id", []):
        cv["baike_flt"] = max(corporations.baike(i), cv["baike_flt"] if "baike_flt" in cv else 0)

    if work_st_tm:
        try:
            if re.match(r"[0-9]{9,}", work_st_tm): work_st_tm = turnTm2Dt(work_st_tm)
            y, m, d = getYMD(work_st_tm)
            cv["work_exp_flt"] = min(int(str(datetime.date.today())[0:4]) - int(y), cv.get("work_exp_flt", 1000))
        except Exception as e:
            print("EXCEPTION: ", e, work_st_tm, cv.get("work_exp_flt"))

    cv["job_num_int"] = 0
    if duas:
        cv["dua_flt"] = np.mean(duas)
        cv["cur_dua_int"] = duas[0]
        cv["job_num_int"] = len(duas)
    if scales: cv["scale_flt"] = np.max(scales)
    return cv


def turnTm2Dt(b):
    if not b: return
    b = str(b).strip()
    if re.match(r"[0-9]{10,}", b): b = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(int(b[:10])))
    return b


def getYMD(b):
    y, m, d = "", "", "01"
    if not b: return (y, m, d)
    b = turnTm2Dt(b)
    if re.match(r"[0-9]{4}", b): y = int(b[:4])
    r = re.search(r"[0-9]{4}.?([0-9]{1,2})", b)
    if r: m = r.group(1)
    r = re.search(r"[0-9]{4}.?[0-9]{,2}.?([0-9]{1,2})", b)
    if r: d = r.group(1)
    if not d or int(d) == 0 or int(d) > 31: d = "1"
    if not m or int(m) > 12 or int(m) < 1: m = "1"
    return (y, m, d)


def birth(cv):
    if not cv.get("birth"):
        cv["integerity_flt"] *= 0.9
        return cv
    y, m, d = getYMD(cv["birth"])
    if not m or not y: return cv
    b = "%s-%02d-%02d" % (y, int(m), int(d))
    cv["birth_dt"] = b
    cv["birthday_kwd"] = "%02d%02d" % (int(m), int(d))

    cv["age_int"] = datetime.datetime.now().year - int(y)
    return cv


def parse(cv):
    for k in cv.keys():
        if cv[k] == '\\N': cv[k] = ''
    # cv = cv.asDict()
    tks_fld = ["address", "corporation_name", "discipline_name", "email", "expect_city_names",
               "expect_industry_name", "expect_position_name", "industry_name", "industry_names", "name",
               "position_name", "school_name", "self_remark", "title_name"]
    small_tks_fld = ["corporation_name", "expect_position_name", "position_name", "school_name", "title_name"]
    kwd_fld = ["address", "city", "corporation_type", "degree", "discipline_name", "expect_city_names", "email",
               "expect_industry_name", "expect_position_name", "expect_type", "gender", "industry_name",
               "industry_names", "political_status", "position_name", "scale", "school_name", "phone", "tel"]
    num_fld = ["annual_salary", "annual_salary_from", "annual_salary_to", "expect_annual_salary", "expect_salary_from",
               "expect_salary_to", "salary_month"]

    is_fld = [
        ("is_fertility", "已育", "未育"),
        ("is_house", "有房", "没房"),
        ("is_management_experience", "有管理经验", "无管理经验"),
        ("is_marital", "已婚", "未婚"),
        ("is_oversea", "有海外经验", "无海外经验")
    ]

    rmkeys = []
    for k in cv.keys():
        if cv[k] is None: rmkeys.append(k)
        if (type(cv[k]) == type([]) or type(cv[k]) == type("")) and len(cv[k]) == 0: rmkeys.append(k)
    for k in rmkeys: del cv[k]

    integerity = 0.
    flds_num = 0.

    def hasValues(flds):
        nonlocal integerity, flds_num
        flds_num += len(flds)
        for f in flds:
            v = str(cv.get(f, ""))
            if len(v) > 0 and v != '0' and v != '[]': integerity += 1

    hasValues(tks_fld)
    hasValues(small_tks_fld)
    hasValues(kwd_fld)
    hasValues(num_fld)
    cv["integerity_flt"] = integerity / flds_num

    if cv.get("corporation_type"):
        for p, r in [(r"(公司|企业|其它|其他|Others*|\n|未填写|Enterprises|Company|companies)", ""),
                     (r"[／/．·　<\(（]+.*", ""),
                     (r".*(合资|民企|股份制|中外|私营|个体|Private|创业|Owned|投资).*", "民营"),
                     (r".*(机关|事业).*", "机关"),
                     (r".*(非盈利|Non-profit).*", "非盈利"),
                     (r".*(外企|外商|欧美|foreign|Institution|Australia|港资).*", "外企"),
                     (r".*国有.*", "国企"),
                     (r"[ （）\(\)人/·0-9-]+", ""),
                     (r".*(元|规模|于|=|北京|上海|至今|中国|工资|州|shanghai|强|餐饮|融资|职).*", "")]:
            cv["corporation_type"] = re.sub(p, r, cv["corporation_type"], 1000, re.IGNORECASE)
        if len(cv["corporation_type"]) < 2: del cv["corporation_type"]

    if cv.get("political_status"):
        for p, r in [
            (r".*党员.*", "党员"),
            (r".*(无党派|公民).*", "群众"),
            (r".*团员.*", "团员")]:
            cv["political_status"] = re.sub(p, r, cv["political_status"])
        if not re.search(r"[党团群]", cv["political_status"]): del cv["political_status"]

    if cv.get("phone"): cv["phone"] = re.sub(r"^0*86([0-9]{11})", r"\1", re.sub(r"[^0-9]+", "", cv["phone"]))

    keys = list(cv.keys())
    for k in keys:
        # deal with json objects
        if k.find("_obj") > 0:
            try:
                cv[k] = json_loads(cv[k])
                cv[k] = [a for _, a in cv[k].items()]
                nms = []
                for n in cv[k]:
                    if type(n) != type({}) or "name" not in n or not n.get("name"): continue
                    n["name"] = re.sub(r"(（442）|\t )", "", n["name"]).strip().lower()
                    if not n["name"]: continue
                    nms.append(n["name"])
                if nms:
                    t = k[:-4]
                    cv[f"{t}_kwd"] = nms
                    cv[f"{t}_tks"] = huqie.qie(" ".join(nms))
            except Exception as e:
                print("【EXCEPTION】:", str(traceback.format_exc()), cv[k])
                cv[k] = []

        # tokenize fields
        if k in tks_fld:
            cv[f"{k}_tks"] = huqie.qie(cv[k])
            if k in small_tks_fld: cv[f"{k}_sm_tks"] = huqie.qie(cv[f"{k}_tks"])

        # keyword fields
        if k in kwd_fld: cv[f"{k}_kwd"] = [n.lower()
                                           for n in re.split(r"[\t,，；;. ]",
                                                             re.sub(r"([^a-zA-Z])[ ]+([^a-zA-Z ])", r"\1，\2", cv[k])
                                                             ) if n]

        if k in num_fld and cv.get(k): cv[f"{k}_int"] = cv[k]

    cv["email_kwd"] = cv.get("email_tks", "").replace(" ", "")
    # for name field
    if cv.get("name"):
        nm = re.sub(r"[\n——\-\(（\+].*", "", cv["name"].strip())
        nm = re.sub(r"[ \t　]+", " ", nm)
        if re.match(r"[a-zA-Z ]+$", nm):
            if len(nm.split(" ")) > 1:
                cv["name"] = nm
            else:
                nm = ""
        elif nm and (surname.isit(nm[0]) or surname.isit(nm[:2])):
            nm = re.sub(r"[a-zA-Z]+.*", "", nm[:5])
        else:
            nm = ""
        cv["name"] = nm.strip()
        name = cv["name"]

        # name pingyin and its prefix
        cv["name_py_tks"] = " ".join(PY.get_pinyins(nm[:20], '')) + " " + " ".join(PY.get_pinyins(nm[:20], ' '))
        cv["name_py_pref0_tks"] = ""
        cv["name_py_pref_tks"] = ""
        for py in PY.get_pinyins(nm[:20], ''):
            for i in range(2, len(py) + 1): cv["name_py_pref_tks"] += " " + py[:i]
        for py in PY.get_pinyins(nm[:20], ' '):
            py = py.split(" ")
            for i in range(1, len(py) + 1): cv["name_py_pref0_tks"] += " " + "".join(py[:i])

        cv["name_kwd"] = name
        cv["name_pinyin_kwd"] = PY.get_pinyins(nm[:20], ' ')[:3]
        cv["name_tks"] = (
                huqie.qie(name) + " " + (" ".join(list(name)) if not re.match(r"[a-zA-Z ]+$", name) else "")
        ) if name else ""
    else:
        cv["integerity_flt"] /= 2.

    if cv.get("phone"):
        r = re.search(r"(1[3456789][0-9]{9})", cv["phone"])
        if not r:
            cv["phone"] = ""
        else:
            cv["phone"] = r.group(1)

    # deal with date  fields
    if cv.get("updated_at") and isinstance(cv["updated_at"], datetime.datetime):
        cv["updated_at_dt"] = cv["updated_at"].strftime('%Y-%m-%d %H:%M:%S')
    else:
        y, m, d = getYMD(str(cv.get("updated_at", "")))
        if not y: y = "2012"
        if not m: m = "01"
        if not d: d = "01"
        cv["updated_at_dt"] = f"%s-%02d-%02d 00:00:00" % (y, int(m), int(d))
        # long text tokenize

    if cv.get("responsibilities"): cv["responsibilities_ltks"] = huqie.qie(rmHtmlTag(cv["responsibilities"]))

    # for yes or no field
    fea = []
    for f, y, n in is_fld:
        if f not in cv: continue
        if cv[f] == '是': fea.append(y)
        if cv[f] == '否': fea.append(n)

    if fea: cv["tag_kwd"] = fea

    cv = forEdu(cv)
    cv = forProj(cv)
    cv = forWork(cv)
    cv = birth(cv)

    cv["corp_proj_sch_deg_kwd"] = [c for c in cv.get("corp_tag_kwd", [])]
    for i in range(len(cv["corp_proj_sch_deg_kwd"])):
        for j in cv.get("sch_rank_kwd", []): cv["corp_proj_sch_deg_kwd"][i] += "+" + j
    for i in range(len(cv["corp_proj_sch_deg_kwd"])):
        if cv.get("highest_degree_kwd"): cv["corp_proj_sch_deg_kwd"][i] += "+" + cv["highest_degree_kwd"]

    try:
        if not cv.get("work_exp_flt") and cv.get("work_start_time"):
            if re.match(r"[0-9]{9,}", str(cv["work_start_time"])):
                cv["work_start_dt"] = turnTm2Dt(cv["work_start_time"])
                cv["work_exp_flt"] = (time.time() - int(int(cv["work_start_time"]) / 1000)) / 3600. / 24. / 365.
            elif re.match(r"[0-9]{4}[^0-9]", str(cv["work_start_time"])):
                y, m, d = getYMD(str(cv["work_start_time"]))
                cv["work_start_dt"] = f"%s-%02d-%02d 00:00:00" % (y, int(m), int(d))
                cv["work_exp_flt"] = int(str(datetime.date.today())[0:4]) - int(y)
    except Exception as e:
        print("【EXCEPTION】", e, "==>", cv.get("work_start_time"))
    if "work_exp_flt" not in cv and cv.get("work_experience", 0): cv["work_exp_flt"] = int(cv["work_experience"]) / 12.

    keys = list(cv.keys())
    for k in keys:
        if not re.search(r"_(fea|tks|nst|dt|int|flt|ltks|kwd|id)$", k): del cv[k]
    for k in cv.keys():
        if not re.search("_(kwd|id)$", k) or type(cv[k]) != type([]): continue
        cv[k] = list(set([re.sub("(市)$", "", str(n)) for n in cv[k] if n not in ['中国', '0']]))
    keys = [k for k in cv.keys() if re.search(r"_feas*$", k)]
    for k in keys:
        if cv[k] <= 0: del cv[k]

    cv["tob_resume_id"] = str(cv["tob_resume_id"])
    cv["id"] = cv["tob_resume_id"]
    print("CCCCCCCCCCCCCCC")

    return dealWithInt64(cv)


def dealWithInt64(d):
    if isinstance(d, dict):
        for n, v in d.items():
            d[n] = dealWithInt64(v)

    if isinstance(d, list):
        d = [dealWithInt64(t) for t in d]

    if isinstance(d, np.integer): d = int(d)
    return d

