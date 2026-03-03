Please extract education background from the following line-indexed resume text.

{indexed_text}

Extract into JSON:
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

Field descriptions:
- school: Full school name, e.g. "Stanford University", both Chinese and English are acceptable
- major: Major/field of study, e.g. "Computer Science"
- degree: Degree level - Bachelor/Master/PhD/Associate/High School/Middle School, leave "" if not available
- department: Department/College, e.g. "School of Engineering"
- start_date: Start date, format %Y.%m or %Y
- end_date: End date, use "Present" if still enrolled, "" if not available
- desc_lines: [start_line, end_line], line number range for education description (optional)
  - Includes coursework, research focus, GPA, honors/awards, etc.
  - Use [] if not available

Return JSON only. /no_think