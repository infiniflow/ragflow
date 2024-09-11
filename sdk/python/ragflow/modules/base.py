class Base(object):
    def __init__(self, rag, res_dict):
        self.rag = rag
        for k, v in res_dict.items():
            if isinstance(v, dict):
                self.__dict__[k] = Base(rag, v)
            else:
                self.__dict__[k] = v

    def to_json(self):
        pr = {}
        for name in dir(self):
            value = getattr(self, name)
            if not name.startswith('__') and not callable(value) and name != "rag":
                if isinstance(value, Base):
                    pr[name] = value.to_json()
                else:
                    pr[name] = value
        return pr

    def post(self, path, param, stream=False):
        res = self.rag.post(path, param, stream=stream)
        return res

    def get(self, path, params):
        res = self.rag.get(path, params)
        return res

    def rm(self, path, params):
        res = self.rag.delete(path, params)
        return res

    def __str__(self):
        return str(self.to_json())
