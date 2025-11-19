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
import os
import numbers
from agent.component.base import ComponentBase, ComponentParamBase
from api.utils.api_utils import timeout

class VariableAssignerParam(ComponentParamBase):
    """
    Define the Variable Assigner component parameters.
    """
    def __init__(self):
        super().__init__()
        self.variables=[]

    def check(self):
        return True
    
    def get_input_form(self) -> dict[str, dict]:
        return {
            "items": {
                "type": "json",
                "name": "Items"
            }
        }

class VariableAssigner(ComponentBase,ABC):
    component_name = "VariableAssigner"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
    def _invoke(self, **kwargs):
        if not isinstance(self._param.variables,list):
            return
        else:
            for item in self._param.variables:
                if any([not item.get("variable"), not item.get("operator"), not item.get("parameter")]):
                    assert "Variable is not complete."
                variable=item["variable"]
                operator=item["operator"]
                parameter=item["parameter"]
                variable_value=self._canvas.get_variable_value(variable)
                new_variable=self._operate(variable_value,operator,parameter)
                self._canvas.set_variable_value(variable, new_variable)

    def _operate(self,variable,operator,parameter):
        if operator == "overwrite":
            return self._overwrite(parameter)
        elif operator == "clear":
            return self._clear(variable)
        elif operator == "set":
            return self._set(variable,parameter)
        elif operator == "append":
            return self._append(variable,parameter)
        elif operator == "extend":
            return self._extend(variable,parameter)
        elif operator == "remove_first":
            return self._remove_first(variable)
        elif operator == "remove_last":
            return self._remove_last(variable)
        elif operator == "+=":
            return self._add(variable,parameter)
        elif operator == "-=":
            return self._subtract(variable,parameter)
        elif operator == "*=":
            return self._multiply(variable,parameter)
        elif operator == "/=":
            return self._divide(variable,parameter)
        else:
            return
    
    def _overwrite(self,parameter):
        return self._canvas.get_variable_value(parameter)

    def _clear(self,variable):
        if isinstance(variable,list):
            return []
        elif isinstance(variable,str):
            return ""
        elif isinstance(variable,dict):
            return {}
        elif isinstance(variable,int):
            return 0
        elif isinstance(variable,float):
            return 0.0
        elif isinstance(variable,bool):
            return False
        else:
            return None

    def _set(self,variable,parameter):
        if variable is None:
            return self._canvas.get_value_with_variable(parameter)
        elif isinstance(variable,str):
            return self._canvas.get_value_with_variable(parameter)
        elif isinstance(variable,bool):
            return parameter
        elif isinstance(variable,int):
            return parameter
        elif isinstance(variable,float):
            return parameter
        else:
            return parameter

    def _append(self,variable,parameter):
        parameter=self._canvas.get_variable_value(parameter)
        if variable is None:
            variable=[]
        if not isinstance(variable,list):
            return "ERROR:VARIABLE_NOT_LIST"
        elif len(variable)!=0 and not isinstance(parameter,type(variable[0])):
            return "ERROR:PARAMETER_NOT_LIST_ELEMENT_TYPE"
        else:
            variable.append(parameter)
            return variable

    def _extend(self,variable,parameter):
        parameter=self._canvas.get_variable_value(parameter)
        if variable is None:
            variable=[]
        if not isinstance(variable,list):
            return "ERROR:VARIABLE_NOT_LIST"
        elif not isinstance(parameter,list):
            return "ERROR:PARAMETER_NOT_LIST"
        elif len(variable)!=0 and len(parameter)!=0 and not isinstance(parameter[0],type(variable[0])):
            return "ERROR:PARAMETER_NOT_LIST_ELEMENT_TYPE"
        else:
            return variable + parameter

    def _remove_first(self,variable):
        if len(variable)==0:
            return variable
        if not isinstance(variable,list):
            return "ERROR:VARIABLE_NOT_LIST"
        else:
            return variable[1:]

    def _remove_last(self,variable):
        if len(variable)==0:
            return variable
        if not isinstance(variable,list):
            return "ERROR:VARIABLE_NOT_LIST"
        else:
            return variable[:-1]

    def is_number(self, value):
        if isinstance(value, bool):
            return False
        return isinstance(value, numbers.Number)

    def _add(self,variable,parameter):
        if self.is_number(variable) and self.is_number(parameter):
            return variable + parameter
        else:
            return "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"

    def _subtract(self,variable,parameter):
        if self.is_number(variable) and self.is_number(parameter):
            return variable - parameter
        else:
            return "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"

    def _multiply(self,variable,parameter):
        if self.is_number(variable) and self.is_number(parameter):
            return variable * parameter
        else:
            return "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"

    def _divide(self,variable,parameter):
        if self.is_number(variable) and self.is_number(parameter):
            if  parameter==0:
                return "ERROR:DIVIDE_BY_ZERO"
            else:
                return variable/parameter
        else:
            return  "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER"

    def thoughts(self) -> str:
        return "Assign variables from canvas."