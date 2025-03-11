
# read file content from local、remote or dataset
from abc import ABC

import pandas as pd


from agent.component.base import ComponentBase, ComponentParamBase

from api.db import LLMType
from api.db.services.llm_service import LLMBundle



class OcrParam(ComponentParamBase):
    """
    Define the Coder component parameters.
    """
    def __init__(self):
        super().__init__()
        self.llm_id = ""

    def get_query_names(self): 
        names = []
        for query in self.query:
            names.append(query['name'])
        return names

    def check(self):
        # 获取所有query的name属性
        self.check_defined_type(self.llm_id, "llm_id", ['str'])
        self.check_empty(self.llm_id, "llm_id")
        # check query proper
        query_names = self.get_query_names()
        if not query_names.__contains__('datasource'):
            self.error = "缺少datasource属性"
            return False
        return True




class Ocr(ComponentBase, ABC):
    component_name = "Ocr"

    def _run(self, history, **kwargs):
        cv_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.IMAGE2TEXT, self._param.llm_id)
        binary = self.get_query('datasource')
        # 如果从pd取出来的可能是Series 需要判断并转换
        if isinstance(binary, pd.Series):
            binary = binary[0]
        ans = cv_mdl.describe(binary, binary.__sizeof__())
        return Ocr.be_output(ans)