import datetime


def refactor(cv):
    for n in ["raw_txt", "parser_name", "inference", "ori_text", "use_time", "time_stat"]:
        if n in cv and cv[n] is not None: del cv[n]
    cv["is_deleted"] = 0
    if "basic" not in cv: cv["basic"] = {}
    if cv["basic"].get("photo2"): del cv["basic"]["photo2"]

    for n in ["education", "work", "certificate", "project", "language", "skill", "training"]:
        if n not in cv or cv[n] is None: continue
        if type(cv[n]) == type({}): cv[n] = [v for _, v in cv[n].items()]
        if type(cv[n]) != type([]):
            del cv[n]
            continue
        vv = []
        for v in cv[n]:
            if "external" in v and v["external"] is not None: del v["external"]
            vv.append(v)
        cv[n] = {str(i): vv[i] for i in range(len(vv))}

    basics = [
        ("basic_salary_month", "salary_month"),
        ("expect_annual_salary_from", "expect_annual_salary"),
    ]
    for n, t in basics:
        if cv["basic"].get(n):
            cv["basic"][t] = cv["basic"][n]
            del cv["basic"][n]

    work = sorted([v for _, v in cv.get("work", {}).items()], key=lambda x: x.get("start_time", ""))
    edu = sorted([v for _, v in cv.get("education", {}).items()], key=lambda x: x.get("start_time", ""))

    if work:
        cv["basic"]["work_start_time"] = work[0].get("start_time", "")
        cv["basic"]["management_experience"] = 'Y' if any(
            [w.get("management_experience", '') == 'Y' for w in work]) else 'N'
        cv["basic"]["annual_salary"] = work[-1].get("annual_salary_from", "0")

        for n in ["annual_salary_from", "annual_salary_to", "industry_name", "position_name", "responsibilities",
                  "corporation_type", "scale", "corporation_name"]:
            cv["basic"][n] = work[-1].get(n, "")

    if edu:
        for n in ["school_name", "discipline_name"]:
            if n in edu[-1]: cv["basic"][n] = edu[-1][n]

    cv["basic"]["updated_at"] = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    if "contact" not in cv: cv["contact"] = {}
    if not cv["contact"].get("name"): cv["contact"]["name"] = cv["basic"].get("name", "")
    return cv