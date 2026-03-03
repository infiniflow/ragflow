请从以下带行号索引的简历文本中提取项目经验。

{indexed_text}

提取为 JSON，每段项目经验包含:
{{
  "projectExperience": [
    {{
      "project_name": "",
      "role": "",
      "start_date": "",
      "end_date": "",
      "desc_lines": [start_index, end_index]
    }}
  ]
}}

字段说明:
- project_name: 项目名称
- role: 担任角色/职责，如"项目负责人"、"后端开发"
- start_date: 开始时间，格式为 %Y.%m 或 %Y
- end_date: 结束时间，若至今填写"至今"，若不存在填写""
- desc_lines: [起始行号, 结束行号]，项目描述对应的行号范围（整数数组）
  - 指项目描述的原文引用段落 index 范围，包括项目内容、技术栈、成果等
  - 不包括 project_name、role、start_date、end_date 所在行
  - 尽可能写全，直到下一段项目经验或其他段落标题为止
  - 遇到以下段落标题时必须截止，不要将其包含在 desc_lines 中：
    个人评价、自我评价、个人总结、个人优势、自我描述、技能特长、专业技能、教育背景、教育经历、工作经历、工作经验、证书资质、语言能力、兴趣爱好、求职意向
  - 如果不存在就写 []

只返回 JSON。 /no_think