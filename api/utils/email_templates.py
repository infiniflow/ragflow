"""
Reusable HTML email templates and registry.
"""

# Invitation email template
INVITE_EMAIL_TMPL = """
<p>Hi {{email}},</p>
<p>{{inviter}} has invited you to join their team (ID: {{tenant_id}}).</p>
<p>Click the link below to complete your registration:<br>
<a href="{{invite_url}}">{{invite_url}}</a></p>
<p>If you did not request this, please ignore this email.</p>
"""

# Password reset code template
RESET_CODE_EMAIL_TMPL = """
<p>Hello,</p>
<p>Your password reset code is: <b>{{ code }}</b></p>
<p>This code will expire in {{ ttl_min }} minutes.</p>
"""

# Template registry
EMAIL_TEMPLATES = {
    "invite": INVITE_EMAIL_TMPL,
    "reset_code": RESET_CODE_EMAIL_TMPL,
}
