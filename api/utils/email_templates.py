#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
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

"""
Reusable HTML email templates and registry.
"""

# Invitation email template
INVITE_EMAIL_TMPL = """
Hi {{email}},
{{inviter}} has invited you to join their team (ID: {{tenant_id}}).
Click the link below to complete your registration:
{{invite_url}}
If you did not request this, please ignore this email.
"""

# Password reset code template
RESET_CODE_EMAIL_TMPL = """
Hello,
Your password reset code is: {{ code }}
This code will expire in {{ ttl_min }} minutes.
"""

# Template registry
EMAIL_TEMPLATES = {
    "invite": INVITE_EMAIL_TMPL,
    "reset_code": RESET_CODE_EMAIL_TMPL,
}
