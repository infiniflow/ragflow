from test.playwright.helpers.env_utils import env_bool


def debug(msg: str) -> None:
    if env_bool("PW_DEBUG_DUMP"):
        print(msg, flush=True)
