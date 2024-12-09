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

import datetime


def refactor(cv):
    for n in [
        "raw_txt",
        "parser_name",
        "inference",
        "ori_text",
        "use_time",
        "time_stat",
    ]:
        if n in cv and cv[n] is not None:
            del cv[n]
    cv["is_deleted"] = 0
    if "basic" not in cv:
        cv["basic"] = {}
    if cv["basic"].get("photo2"):
        del cv["basic"]["photo2"]

    for n in [
        "education",
        "work",
        "certificate",
        "project",
        "language",
        "skill",
        "training",
    ]:
        if n not in cv or cv[n] is None:
            continue
        if isinstance(cv[n], dict):
            cv[n] = [v for _, v in cv[n].items()]
        if not isinstance(cv[n], list):
            del cv[n]
            continue
        vv = []
        for v in cv[n]:
            if "external" in v and v["external"] is not None:
                del v["external"]
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

    work = sorted(
        [v for _, v in cv.get("work", {}).items()],
        key=lambda x: x.get("start_time", ""),
    )
    edu = sorted(
        [v for _, v in cv.get("education", {}).items()],
        key=lambda x: x.get("start_time", ""),
    )

    if work:
        cv["basic"]["work_start_time"] = work[0].get("start_time", "")
        cv["basic"]["management_experience"] = (
            "Y"
            if any([w.get("management_experience", "") == "Y" for w in work])
            else "N"
        )
        cv["basic"]["annual_salary"] = work[-1].get("annual_salary_from", "0")

        for n in [
            "annual_salary_from",
            "annual_salary_to",
            "industry_name",
            "position_name",
            "responsibilities",
            "corporation_type",
            "scale",
            "corporation_name",
        ]:
            cv["basic"][n] = work[-1].get(n, "")

    if edu:
        for n in ["school_name", "discipline_name"]:
            if n in edu[-1]:
                cv["basic"][n] = edu[-1][n]

    cv["basic"]["updated_at"] = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    if "contact" not in cv:
        cv["contact"] = {}
    if not cv["contact"].get("name"):
        cv["contact"]["name"] = cv["basic"].get("name", "")
    return cv
