import urllib.parse
from .oauth import OAuthClient, UserInfo


class YandexOAuthClient(OAuthClient):
    def get_authorization_url(self, state=None):
        params = {
            "client_id": self.client_id,
            "redirect_uri": self.redirect_uri,
            "response_type": "code",
        }
        if self.scope:
            params["scope"] = self.scope
        if state:
            params["state"] = state
        params["force_confirm"] = "yes"
        return f"{self.authorization_url}?{urllib.parse.urlencode(params)}"

    def normalize_user_info(self, user_info):
        email = user_info.get("default_email") or (user_info.get("emails") or [None])[0]
        username = user_info.get("login", str(email).split("@")[0])
        nickname = " ".join(filter(None, [user_info.get("first_name"), user_info.get("last_name")])) or username
        avatar_id = user_info.get("avatar_id")
        avatar_url = f"https://avatars.yandex.net/get-yapic/{avatar_id}/islands-200" if avatar_id else ""
        return UserInfo(email=email, username=username, nickname=nickname, avatar_url=avatar_url)
