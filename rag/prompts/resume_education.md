请从以下带行号索引的简历文本中提取教育背景。

{indexed_text}

提取为 JSON:
{{
  "education": [
    {{
      "school": "",
      "major": "",
      "degree": "",
      "department": "",
      "start_date": "",
      "end_date": "",
      "desc_lines": [start_index, end_index]
    }}
  ]
}}

字段说明:
- school: 学校全称，如"厦门大学"，中英文都可以
- major: 专业，如"机械工程"
- degree: 学位，本科/硕士/博士/专科/高中/初中，若不存在则填""
- department: 系/学院，如"信息工程系"
- start_date: 开始时间，格式为 %Y.%m 或 %Y
- end_date: 结束时间，若至今填写"至今"，若不存在填写""
- desc_lines: [起始行号, 结束行号]，教育描述对应的行号范围（可选）
  - 包括课程成绩、研究方向、GPA、荣誉奖项等
  - 不存在则填 []

只返回 JSON。 /no_think