
# python code executor

from abc import ABC
import ast
import logging
import re

from agent.component.base import ComponentBase, ComponentParamBase
from RestrictedPython import safe_builtins, compile_restricted_exec


class CoderParam(ComponentParamBase):
    """
    Define the Coder component parameters.
    """
    def __init__(self):
        super().__init__()
        self.code = ""

    def check(self):
        self.check_empty(self.code, "code")


class Coder(ComponentBase, ABC):
    component_name = "Coder"

    def get_dependent_components(self):
        inputs = self.get_input_elements()
        cpnts = set([i["key"] for i in inputs if i["key"].lower().find("answer") < 0 and i["key"].lower().find("begin") < 0])
        return list(cpnts)
    
    def get_input_elements(self):
        key_set = set([])
        res = []
        paramList = ComponentBase.get_dynamic_params(self._param.code);
        for p in paramList:
            cpn_id, key = ComponentBase.split_param(p)
            if cpn_id in key_set:
                continue
            key_set.add(cpn_id)
            res.append({"key": p, "name": self.get_component_name(cpn_id)})
            continue
        return res
    

    def _run(self, history, **kwargs):
        code = self._param.code
        code = self.replace_all_inputs(code)
        return Coder.be_output(self.exec_code(code)) 
    
    def exec_code(self, user_code: str):
        try:
            # 定义安全的内置函数
            local_safe_builtins = safe_builtins.copy()


            ast.parse(user_code)

            # 编译受限代码
            bytecode = compile_restricted_exec(user_code)
            if bytecode.errors:
                print(f"编译错误：{bytecode.errors}")
                return f"编译错误：{bytecode.errors}"
            # 定义执行环境
            restricted_globals = {'__builtins__': local_safe_builtins}
            exec(bytecode.code, restricted_globals)
            return restricted_globals.get("result", "")
        except Exception as e:
            logging.error(f"代码执行错误: {str(e)}")
            return f"执行错误: {str(e)}"

    def replace_all_inputs(self, code):
        for para in self._param.inputs:
            value = para["content"]
            if isinstance(para["content"], list):
                value = str(value)
            elif isinstance(para["content"], dict):
                value = str(value)
            else:
                value = f"'{value}'"
            code = code.replace(f"{{{para['component_id']}}}", value)
        return code
