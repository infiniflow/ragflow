#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
from abc import ABC
import ast
import os
from agent.component.base import ComponentBase, ComponentParamBase
from api.utils.api_utils import timeout

class DataOperationsParam(ComponentParamBase):
    """
    Define the Data Operations component parameters.
    """
    def __init__(self):
        super().__init__()
        self.query = []
        self.operations = "literal_eval"
        self.select_keys = []
        self.filter_values=[]
        self.updates=[]
        self.remove_keys=[]
        self.rename_keys=[]
        self.outputs = {
            "result": {
                "value": [],
                "type": "Array of Object"
            }
        }
    
    def check(self):
        self.check_valid_value(self.operations, "Support operations", ["select_keys", "literal_eval","combine","filter_values","append_or_update","remove_keys","rename_keys"])
    
    

class DataOperations(ComponentBase,ABC):
    component_name = "DataOperations"

    def get_input_form(self) -> dict[str, dict]:
        return {
            k: {"name": o.get("name", ""), "type": "line"}
            for input_item in (self._param.query or [])
            for k, o in self.get_input_elements_from_text(input_item).items()
        }

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
    def _invoke(self, **kwargs):
        self.input_objects=[]
        inputs = getattr(self._param, "query", None)
        if not isinstance(inputs, (list, tuple)):
            inputs = [inputs]
        for input_ref in inputs:
            input_object=self._canvas.get_variable_value(input_ref)
            self.set_input_value(input_ref, input_object)
            if input_object is None:
                continue
            if isinstance(input_object,dict):
                self.input_objects.append(input_object)
            elif isinstance(input_object,list):
                self.input_objects.extend(x for x in input_object if isinstance(x, dict))
            else:
                continue
        if self._param.operations == "select_keys":
            self._select_keys()
        elif self._param.operations == "recursive_eval":
            self._literal_eval()
        elif self._param.operations == "combine":
            self._combine()
        elif self._param.operations == "filter_values":
            self._filter_values()
        elif self._param.operations == "append_or_update":
            self._append_or_update()
        elif self._param.operations == "remove_keys":
            self._remove_keys()
        else:
            self._rename_keys()
    
    def _select_keys(self):
        filter_criteria: list[str] = self._param.select_keys
        results = [{key: value for key, value in data_dict.items() if key in filter_criteria} for data_dict in self.input_objects]
        self.set_output("result", results)


    def _recursive_eval(self, data):
        if isinstance(data, dict):
            return {k: self.recursive_eval(v) for k, v in data.items()}
        if isinstance(data, list):
            return [self.recursive_eval(item) for item in data]
        if isinstance(data, str):
            try:
                if (
                    data.strip().startswith(("{", "[", "(", "'", '"'))
                    or data.strip().lower() in ("true", "false", "none")
                    or data.strip().replace(".", "").isdigit()
                ):
                    return ast.literal_eval(data)
            except (ValueError, SyntaxError, TypeError, MemoryError):
                return data
            else:
                return data
        return data
    
    def _literal_eval(self):
        self.set_output("result", self._recursive_eval(self.input_objects))

    def _combine(self):
        result={}
        for obj in self.input_objects:
            for key, value in obj.items():
                if key not in result:
                    result[key] = value
                elif isinstance(result[key], list):
                    if isinstance(value, list):
                        result[key].extend(value)
                    else:
                        result[key].append(value)
                else:
                    result[key] = (
                        [result[key], value] if not isinstance(value, list) else [result[key], *value]
                    )
        self.set_output("result", result)
    
    def norm(self,v):
        s = "" if v is None else str(v)
        return s
    
    def match_rule(self, obj, rule):
        key = rule.get("key")
        op = (rule.get("operator") or "equals").lower()
        target = self.norm(rule.get("value"))
        target = self._canvas.get_value_with_variable(target) or target
        if key not in obj:
            return False
        val = obj.get(key, None)
        v = self.norm(val)
        if op == "=":
            return v == target
        if op == "≠":
            return v != target
        if op == "contains":
            return target in v
        if op == "start with":
            return v.startswith(target)
        if op == "end with":
            return v.endswith(target)
        return False
        
    def _filter_values(self):
        results=[]
        rules = (getattr(self._param, "filter_values", None) or [])
        for obj in self.input_objects:
            if not rules:
                results.append(obj)
                continue
            if all(self.match_rule(obj, r) for r in rules):
                results.append(obj)
        self.set_output("result", results)
            
                
    def _append_or_update(self):
        results=[]
        updates = getattr(self._param, "updates", []) or [] 
        for obj in self.input_objects:
            new_obj = dict(obj)
            for item in updates:
                if not isinstance(item, dict):
                    continue
                k = (item.get("key") or "").strip()
                if not k:
                    continue
                new_obj[k] = self._canvas.get_value_with_variable(item.get("value")) or item.get("value")
            results.append(new_obj)
        self.set_output("result", results)

    def _remove_keys(self):
        results = []
        remove_keys = getattr(self._param, "remove_keys", []) or []

        for obj in (self.input_objects or []):
            new_obj = dict(obj)
            for k in remove_keys:
                if not isinstance(k, str):
                    continue
                new_obj.pop(k, None)
            results.append(new_obj)
        self.set_output("result", results)

    def _rename_keys(self):
        results = []
        rename_pairs = getattr(self._param, "rename_keys", []) or []

        for obj in (self.input_objects or []):
            new_obj = dict(obj)
            for pair in rename_pairs:
                if not isinstance(pair, dict):
                    continue
                old = (pair.get("old_key") or "").strip()
                new = (pair.get("new_key") or "").strip()
                if not old or not new or old == new:
                    continue
                if old in new_obj:
                    new_obj[new] = new_obj.pop(old)
            results.append(new_obj)
        self.set_output("result", results)

    def thoughts(self) -> str:
        return "DataOperation in progress"
