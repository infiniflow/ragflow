from web_server.settings import RetCode


class ParametersBase:
    def to_dict(self):
        d = {}
        for k, v in self.__dict__.items():
            d[k] = v
        return d


class ClientAuthenticationParameters(ParametersBase):
    def __init__(self, full_path, headers, form, data, json):
        self.full_path = full_path
        self.headers = headers
        self.form = form
        self.data = data
        self.json = json


class ClientAuthenticationReturn(ParametersBase):
    def __init__(self, code=RetCode.SUCCESS, message="success"):
        self.code = code
        self.message = message


class SignatureParameters(ParametersBase):
    def __init__(self, party_id, body):
        self.party_id = party_id
        self.body = body


class SignatureReturn(ParametersBase):
    def __init__(self, code=RetCode.SUCCESS, site_signature=None):
        self.code = code
        self.site_signature = site_signature


class AuthenticationParameters(ParametersBase):
    def __init__(self, site_signature, body):
        self.site_signature = site_signature
        self.body = body


class AuthenticationReturn(ParametersBase):
    def __init__(self, code=RetCode.SUCCESS, message="success"):
        self.code = code
        self.message = message


class PermissionReturn(ParametersBase):
    def __init__(self, code=RetCode.SUCCESS, message="success"):
        self.code = code
        self.message = message


