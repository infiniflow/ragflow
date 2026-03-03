"""Auth UI selectors for Playwright suite. Keep stable testids."""

AUTH_FORM = "form[data-testid='auth-form']"
AUTH_ACTIVE_FORM = "form[data-testid='auth-form'][data-active='true']"

EMAIL_INPUT = "input[data-testid='auth-email'], [data-testid='auth-email'] input"
PASSWORD_INPUT = "input[data-testid='auth-password'], [data-testid='auth-password'] input"
NICKNAME_INPUT = "input[data-testid='auth-nickname'], [data-testid='auth-nickname'] input"

SUBMIT_BUTTON = (
    "button[data-testid='auth-submit'], [data-testid='auth-submit'] button, "
    "[data-testid='auth-submit']"
)

REGISTER_TAB = "[data-testid='auth-toggle-register']"
LOGIN_TAB = "[data-testid='auth-toggle-login']"
AUTH_STATUS = "[data-testid='auth-status']"
