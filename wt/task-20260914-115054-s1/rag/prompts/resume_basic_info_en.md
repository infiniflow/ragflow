Please extract basic information from the following line-indexed resume text.

{indexed_text}

Extract the following information into JSON. If a field does not exist, output "" or 0:
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

Field descriptions:
- name_kwd: Full name, e.g. "John Smith"
- gender_kwd: Male/Female, leave empty if not present
- age_int: Current age, integer
- phone_kwd: Phone number, keep original format including country code and brackets
- email_tks: Email address, e.g. "xxx@gmail.com"
- birth_dt: Date of birth, e.g. "1996-11"
- work_exp_flt: Years of work experience, float
- current_location: Current city/location, do not infer from work experience, must be explicitly stated
- expect_city_names_tks: List of preferred work cities, must be explicitly stated in the resume
- expect_position_name_tks: List of desired positions
- skill_tks: List of skills/tech stack
- language_tks: List of language proficiencies
- certificate_tks: List of certificates/qualifications
- self_evaluation_tks: Self-evaluation/personal strengths/summary, extract full original text

Return JSON only. /no_think