Please extract project experience from the following line-indexed resume text.

{indexed_text}

Extract into JSON, each project experience entry contains:
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

Field descriptions:
- project_name: Project name
- role: Role/responsibility, e.g. "Project Lead", "Backend Developer"
- start_date: Start date, format %Y.%m or %Y
- end_date: End date, use "Present" if ongoing, "" if not available
- desc_lines: [start_line, end_line], line number range for project description (integer array)
  - Refers to the original text reference range for project description, including project content, tech stack, achievements, etc.
  - Does not include lines containing project_name, role, start_date, end_date
  - Include as much as possible until the next project experience entry
  - Use [] if not available

Return JSON only. /no_think