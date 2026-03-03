请从以下带行号索引的简历文本中提取基本信息。

{indexed_text}

提取如下信息到 JSON，若某些字段不存在则输出 "" 空或 0:
{{
  "name_kwd": "",
  "gender_kwd": "",
  "age_int": 0,
  "phone_kwd": "",
  "email_tks": "",
  "birth_dt": "",
  "work_exp_flt": 0,
  "current_location": "",
  "expect_city_names_tks": [],
  "expect_position_name_tks": [],
  "skill_tks": [],
  "language_tks": [],
  "certificate_tks": [],
  "self_evaluation_tks": ""
}}

字段说明:
- name_kwd: 姓名，如"张三"
- gender_kwd: 男/女，若不存在则不填
- age_int: 当前年龄，整数
- phone_kwd: 电话/手机，请保留原文中的形式，保留国家码区号括号
- email_tks: 邮箱，如 "xxx@qq.com"
- birth_dt: 出生年月，如 "1996-11"
- work_exp_flt: 工作年限，浮点数
- current_location: 现居地/当前城市，不要从工作经历中推测，要写明现居地
- expect_city_names_tks: 期望工作城市列表，简历中需要明确说明是期望城市
- expect_position_name_tks: 期望职位列表
- skill_tks: 技能/技术栈列表
- language_tks: 语言能力列表
- certificate_tks: 证书/资质列表
- self_evaluation_tks: 自我评价/个人优势/个人总结，完整提取原文内容

只返回 JSON。 /no_think