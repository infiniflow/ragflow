
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
        return self.code != ""


class Coder(ComponentBase, ABC):
    component_name = "Coder"

    def get_dependent_components(self):
        inputs = self.get_input_elements()
        cpnts = set([i["key"] for i in inputs if i["key"].lower().find("answer") < 0 and i["key"].lower().find("begin") < 0])
        return list(cpnts)
    
    def get_input_elements(self):
        key_set = set([])
        res = []
        for r in re.finditer(r"\{([a-z]+[:@][a-z0-9_-]+)\}", self._param.code, flags=re.IGNORECASE):
            cpn_id = r.group(1)
            if cpn_id in key_set:
                continue
            if cpn_id.lower().find("begin@") == 0:
                cpn_id, key = cpn_id.split("@")
                for p in self._canvas.get_component(cpn_id)["obj"]._param.query:
                    if p["key"] != key:
                        continue
                    res.append({"key": r.group(1), "name": p["name"]})
                    key_set.add(r.group(1))
                continue
            cpn_nm = self._canvas.get_component_name(cpn_id)
            if not cpn_nm:
                continue
            res.append({"key": cpn_id, "name": cpn_nm})
            key_set.add(cpn_id)
        return res
    
    def _run(self, history, **kwargs):
        code = self._param.code

        self._param.inputs = []
        for para in self.get_input_elements():
            if para["key"].lower().find("begin@") == 0:
                cpn_id, key = para["key"].split("@")
                for p in self._canvas.get_component(cpn_id)["obj"]._param.query:
                    if p["key"] == key:
                        value = p.get("value", "")
                        self.make_inputs(para, value)
                        break
                else:
                    assert False, f"Can't find parameter '{key}' for {cpn_id}"
                continue

            component_id = para["key"]
            cpn = self._canvas.get_component(component_id)["obj"]
            if cpn.component_name.lower() == "answer":
                hist = self._canvas.get_history(1)
                if hist:
                    hist = hist[0]["content"]
                else:
                    hist = ""
                self.make_inputs(para, hist)
                continue

            _, out = cpn.output(allow_partial=False)

            result = ""
            if "content" in out.columns:
                result = "\n".join(
                    [o if isinstance(o, str) else str(o) for o in out["content"]]
                )

            self.make_inputs(para, result)
        
        code = self.replace_all_inputs(code)

        return self.exec_code(code)
    
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

    def make_inputs(self, para, value):
        self._param.inputs.append(
            {"component_id": para["key"], "content": value}
        )