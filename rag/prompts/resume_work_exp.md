请从以下带行号索引的简历文本中提取工作经历。

{indexed_text}

提取为 JSON，每段工作经历包含:
{{
  "workExperience": [
    {{
      "company": "",
      "position": "",
      "internship": 0,
      "start_date": "",
      "end_date": "",
      "desc_lines": [start_index, end_index]
    }}
  ]
}}

字段说明:
- company: 公司全称（含括号内地区信息），如"阿里巴巴(中国)有限公司"
- position: 职位名称，遵循原文不要编造或推测
- internship: 该段经历是否是实习，是实习为1，不是为0
- start_date: 入职时间，格式为 %Y.%m 或 %Y，如 "2024.1"
- end_date: 离职时间，若至今填写"至今"，若不存在填写""
- desc_lines: [起始行号, 结束行号]，工作描述对应的行号范围（整数数组）
  - 指工作经历描述的原文引用段落 index 范围，包括工作成果、业绩、主要工作、技术栈等
  - 不包括 company、position、start_date、end_date 所在行
  - 尽可能写全，直到下一段工作经历为止
  - 如果不存在就写 []

示例:
[22]: 阿里巴巴 2021.11-2022.11 高级工程师
[23]: 工作描述: 从事地推工作完成xx业绩
[24]: 在地推任务中考核为A
则 desc_lines 应为 [23, 24]

只返回 JSON。 /no_think