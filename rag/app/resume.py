import copy
import json
import os
import re
import requests
from api.db.services.knowledgebase_service import KnowledgebaseService
from rag.nlp import huqie

from rag.settings import cron_logger
from rag.utils import rmSpace


def chunk(filename, binary=None, callback=None, **kwargs):
    if not re.search(r"\.(pdf|doc|docx|txt)$", filename, flags=re.IGNORECASE): raise NotImplementedError("file type not supported yet(pdf supported)")

    url = os.environ.get("INFINIFLOW_SERVER")
    if not url:raise EnvironmentError("Please set environment variable: 'INFINIFLOW_SERVER'")
    token = os.environ.get("INFINIFLOW_TOKEN")
    if not token:raise EnvironmentError("Please set environment variable: 'INFINIFLOW_TOKEN'")

    if not binary:
        with open(filename, "rb") as f: binary = f.read()
    def remote_call():
        nonlocal filename, binary
        for _ in range(3):
            try:
                res = requests.post(url + "/v1/layout/resume/", files=[(filename, binary)],
                                    headers={"Authorization": token}, timeout=180)
                res = res.json()
                if res["retcode"] != 0: raise RuntimeError(res["retmsg"])
                return res["data"]
            except RuntimeError as e:
                raise e
            except Exception as e:
                cron_logger.error("resume parsing:" + str(e))

    resume = remote_call()
    print(json.dumps(resume, ensure_ascii=False, indent=2))

    field_map = {
        "name_kwd": "姓名/名字",
        "gender_kwd": "性别（男，女）",
        "age_int": "年龄/岁/年纪",
        "phone_kwd": "电话/手机/微信",
        "email_tks": "email/e-mail/邮箱",
        "position_name_tks": "职位/职能/岗位/职责",
        "expect_position_name_tks": "期望职位/期望职能/期望岗位",
    
        "hightest_degree_kwd": "最高学历（高中，职高，硕士，本科，博士，初中，中技，中专，专科，专升本，MPA，MBA，EMBA）",
        "first_degree_kwd": "第一学历（高中，职高，硕士，本科，博士，初中，中技，中专，专科，专升本，MPA，MBA，EMBA）",
        "first_major_tks": "第一学历专业",
        "first_school_name_tks": "第一学历毕业学校",
        "edu_first_fea_kwd": "第一学历标签（211，留学，双一流，985，海外知名，重点大学，中专，专升本，专科，本科，大专）",
    
        "degree_kwd": "过往学历（高中，职高，硕士，本科，博士，初中，中技，中专，专科，专升本，MPA，MBA，EMBA）",
        "major_tks": "学过的专业/过往专业",
        "school_name_tks": "学校/毕业院校",
        "sch_rank_kwd": "学校标签（顶尖学校，精英学校，优质学校，一般学校）",
        "edu_fea_kwd": "教育标签（211，留学，双一流，985，海外知名，重点大学，中专，专升本，专科，本科，大专）",
    
        "work_exp_flt": "工作年限/工作年份/N年经验/毕业了多少年",
        "birth_dt": "生日/出生年份",
        "corp_nm_tks": "就职过的公司/之前的公司/上过班的公司",
        "corporation_name_tks": "最近就职(上班)的公司/上一家公司",
        "edu_end_int": "毕业年份",
        "expect_city_names_tks": "期望城市",
        "industry_name_tks": "所在行业"
    }
    titles = []
    for n in ["name_kwd", "gender_kwd", "position_name_tks", "age_int"]:
        v = resume.get(n, "")
        if isinstance(v, list):v = v[0]
        if n.find("tks") > 0: v = rmSpace(v)
        titles.append(str(v))
    doc = {
        "docnm_kwd": filename,
        "title_tks": huqie.qie("-".join(titles)+"-简历")
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    pairs = []
    for n,m in field_map.items():
        if not resume.get(n):continue
        v = resume[n]
        if isinstance(v, list):v = " ".join(v)
        if n.find("tks") > 0: v = rmSpace(v)
        pairs.append((m, str(v)))

    doc["content_with_weight"] = "\n".join(["{}: {}".format(re.sub(r"（[^（）]+）", "", k), v) for k,v in pairs])
    doc["content_ltks"] = huqie.qie(doc["content_with_weight"])
    doc["content_sm_ltks"] = huqie.qieqie(doc["content_ltks"])
    for n, _ in field_map.items(): doc[n] = resume[n]

    print(doc)
    KnowledgebaseService.update_parser_config(kwargs["kb_id"], {"field_map": field_map})
    return [doc]


if __name__ == "__main__":
    import sys
    def dummy(a, b):
        pass
    chunk(sys.argv[1], callback=dummy)
