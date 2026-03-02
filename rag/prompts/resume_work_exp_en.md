Please extract work experience from the following line-indexed resume text.

{indexed_text}

Extract into JSON, each work experience entry contains:
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

Field descriptions:
- company: Full company name (including region info in brackets), e.g. "Google Inc."
- position: Job title, follow original text, do not fabricate or guess
- internship: Whether this is an internship, 1 for yes, 0 for no
- start_date: Start date, format %Y.%m or %Y, e.g. "2024.1"
- end_date: End date, use "Present" if still employed, "" if not available
- desc_lines: [start_line, end_line], line number range for job description (integer array)
  - Refers to the original text reference range for job description, including achievements, responsibilities, tech stack, etc.
  - Does not include lines containing company, position, start_date, end_date
  - Include as much as possible until the next work experience entry
  - Use [] if not available

Example:
[22]: Google Inc. 2021.11-2022.11 Senior Engineer
[23]: Job description: Responsible for backend development
[24]: Achieved 99.9% uptime for core services
Then desc_lines should be [23, 24]

Return JSON only. /no_think