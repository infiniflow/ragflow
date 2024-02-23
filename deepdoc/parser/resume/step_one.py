# -*- coding: utf-8 -*-
import json
from deepdoc.parser.resume.entities import degrees, regions, industries

FIELDS = [
"address STRING",
"annual_salary int",
"annual_salary_from int",
"annual_salary_to int",
"birth STRING",
"card STRING",
"certificate_obj string",
"city STRING",
"corporation_id int",
"corporation_name STRING",
"corporation_type STRING",
"degree STRING",
"discipline_name STRING",
"education_obj string",
"email STRING",
"expect_annual_salary int",
"expect_city_names string",
"expect_industry_name STRING",
"expect_position_name STRING",
"expect_salary_from int",
"expect_salary_to int",
"expect_type STRING",
"gender STRING",
"industry_name STRING",
"industry_names STRING",
"is_deleted STRING",
"is_fertility STRING",
"is_house STRING",
"is_management_experience STRING",
"is_marital STRING",
"is_oversea STRING",
"language_obj string",
"name STRING",
"nation STRING",
"phone STRING",
"political_status STRING",
"position_name STRING",
"project_obj string",
"responsibilities string",
"salary_month int",
"scale STRING",
"school_name STRING",
"self_remark string",
"skill_obj string",
"title_name STRING",
"tob_resume_id STRING",
"updated_at Timestamp",
"wechat STRING",
"work_obj string",
"work_experience int",
"work_start_time BIGINT"
]

def refactor(df):
    def deal_obj(obj, k, kk):
        if not isinstance(obj, type({})):
            return ""
        obj = obj.get(k, {})
        if not isinstance(obj, type({})):
            return ""
        return obj.get(kk, "")

    def loadjson(line):
        try:
            return json.loads(line)
        except Exception as e:
            pass
        return {}

    df["obj"] = df["resume_content"].map(lambda x: loadjson(x))
    df.fillna("", inplace=True)

    clms = ["tob_resume_id", "updated_at"]

    def extract(nms, cc=None):
        nonlocal clms
        clms.extend(nms)
        for c in nms:
            if cc:
                df[c] = df["obj"].map(lambda x: deal_obj(x, cc, c))
            else:
                df[c] = df["obj"].map(
                    lambda x: json.dumps(
                        x.get(
                            c,
                            {}),
                        ensure_ascii=False) if isinstance(
                        x,
                        type(
                            {})) and (
                        isinstance(
                            x.get(c),
                            type(
                                {})) or not x.get(c)) else str(x).replace(
                                    "None",
                        ""))

    extract(["education", "work", "certificate", "project", "language",
             "skill"])
    extract(["wechat", "phone", "is_deleted",
            "name", "tel", "email"], "contact")
    extract(["nation", "expect_industry_name", "salary_month",
             "industry_ids", "is_house", "birth", "annual_salary_from",
             "annual_salary_to", "card",
             "expect_salary_to", "expect_salary_from",
             "expect_position_name", "gender", "city",
             "is_fertility", "expect_city_names",
             "political_status", "title_name", "expect_annual_salary",
             "industry_name", "address", "position_name", "school_name",
             "corporation_id",
             "is_oversea", "responsibilities",
             "work_start_time", "degree", "management_experience",
             "expect_type", "corporation_type", "scale", "corporation_name",
             "self_remark", "annual_salary", "work_experience",
             "discipline_name", "marital", "updated_at"], "basic")

    df["degree"] = df["degree"].map(lambda x: degrees.get_name(x))
    df["address"] = df["address"].map(lambda x: " ".join(regions.get_names(x)))
    df["industry_names"] = df["industry_ids"].map(lambda x: " ".join([" ".join(industries.get_names(i)) for i in
                                                                      str(x).split(",")]))
    clms.append("industry_names")

    def arr2str(a):
        if not a:
            return ""
        if isinstance(a, list):
            a = " ".join([str(i) for i in a])
        return str(a).replace(",", " ")

    df["expect_industry_name"] = df["expect_industry_name"].map(
        lambda x: arr2str(x))
    df["gender"] = df["gender"].map(
        lambda x: "男" if x == 'M' else (
            "女" if x == 'F' else ""))
    for c in ["is_fertility", "is_oversea", "is_house",
              "management_experience", "marital"]:
        df[c] = df[c].map(
            lambda x: '是' if x == 'Y' else (
                '否' if x == 'N' else ""))
    df["is_management_experience"] = df["management_experience"]
    df["is_marital"] = df["marital"]
    clms.extend(["is_management_experience", "is_marital"])

    df.fillna("", inplace=True)
    for i in range(len(df)):
        if not df.loc[i, "phone"].strip() and df.loc[i, "tel"].strip():
            df.loc[i, "phone"] = df.loc[i, "tel"].strip()

    for n in ["industry_ids", "management_experience", "marital", "tel"]:
        for i in range(len(clms)):
            if clms[i] == n:
                del clms[i]
                break

    clms = list(set(clms))

    df = df.reindex(sorted(clms), axis=1)
    #print(json.dumps(list(df.columns.values)), "LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL")
    for c in clms:
        df[c] = df[c].map(
            lambda s: str(s).replace(
                "\t",
                " ").replace(
                "\n",
                "\\n").replace(
                "\r",
                "\\n"))
    # print(df.values.tolist())
    return dict(zip([n.split(" ")[0] for n in FIELDS], df.values.tolist()[0]))
