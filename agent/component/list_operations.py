from abc import ABC
import os
from agent.component.base import ComponentBase, ComponentParamBase
from api.utils.api_utils import timeout

class ListOperationsParam(ComponentParamBase):
    """
    Define the List Operations component parameters.
    """
    def __init__(self):
        super().__init__()
        self.query = ""
        self.operations = "topN"
        self.n=0
        self.sort_method = "asc"
        self.filter = {
            "operator": "=",
            "value": ""
        }
        self.outputs = {
            "result": {
                "value": [],
                "type": "Array of ?"
            },
            "first": {
                "value": "",
                "type": "?"
            },
            "last": {
                "value": "",
                "type": "?"
            }
        }
    
    def check(self):
        self.check_empty(self.query, "query")
        self.check_valid_value(self.operations, "Support operations", ["topN","head","tail","filter","sort","drop_duplicates"])

    def get_input_form(self) -> dict[str, dict]:
        return {}
    

class ListOperations(ComponentBase,ABC):
    component_name = "ListOperations"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
    def _invoke(self, **kwargs):
        self.input_objects=[]
        inputs = getattr(self._param, "query", None)
        self.inputs = self._canvas.get_variable_value(inputs)
        if not isinstance(self.inputs, list):
            raise TypeError("The input of List Operations should be an array.")
        self.set_input_value(inputs, self.inputs)
        if self._param.operations == "topN":
            self._topN()
        elif self._param.operations == "head":
            self._head()
        elif self._param.operations == "tail":
            self._tail()
        elif self._param.operations == "filter":
            self._filter()
        elif self._param.operations == "sort":
            self._sort()
        elif self._param.operations == "drop_duplicates":
            self._drop_duplicates()


    def _coerce_n(self):
        try:
            return int(getattr(self._param, "n", 0))
        except Exception:
            return 0
        
    def _set_outputs(self, outputs):
        self._param.outputs["result"]["value"] = outputs
        self._param.outputs["first"]["value"] = outputs[0] if outputs else None
        self._param.outputs["last"]["value"]  = outputs[-1] if outputs else None
        
    def _topN(self):
        n = self._coerce_n()
        if n < 1:
            outputs = []
        else:
            n = min(n, len(self.inputs))
            outputs = self.inputs[:n]
        self._set_outputs(outputs)

    def _head(self):
        n = self._coerce_n()
        if 1 <= n <= len(self.inputs):
            outputs = [self.inputs[n - 1]]
        else:
            outputs = []
        self._set_outputs(outputs)

    def _tail(self):
        n = self._coerce_n()
        if 1 <= n <= len(self.inputs):
            outputs = [self.inputs[-n]]
        else:
            outputs = []
        self._set_outputs(outputs)

    def _filter(self):
        self._set_outputs([i for i in self.inputs if self._eval(self._norm(i),self._param.filter["operator"],self._param.filter["value"])])

    def _norm(self,v):
        s = "" if v is None else str(v)
        return s
    
    def _eval(self, v, operator, value):
        if operator == "=":
            return v == value
        elif operator == "≠":
            return v != value
        elif operator == "contains":
            return value in v
        elif operator == "start with":
            return v.startswith(value)
        elif operator == "end with":
            return v.endswith(value)
        else:
            return False

    def _sort(self):
        items = self.inputs or []
        method = getattr(self._param, "sort_method", "asc") or "asc"
        reverse = method == "desc"

        if not items:
            self._set_outputs([])
            return

        first = items[0]

        if isinstance(first, dict):
            outputs = sorted(
                items,
                key=lambda x: self._hashable(x),
                reverse=reverse,
            )
        else:
            outputs = sorted(items, reverse=reverse)

        self._set_outputs(outputs)

    def _drop_duplicates(self):
        seen = set()
        outs = []
        for item in self.inputs:
            k = self._hashable(item)
            if k in seen:
                continue
            seen.add(k)
            outs.append(item)
        self._set_outputs(outs)

    def _hashable(self,x):
        if isinstance(x, dict):
            return tuple(sorted((k, self._hashable(v)) for k, v in x.items()))
        if isinstance(x, (list, tuple)):
            return tuple(self._hashable(v) for v in x)
        if isinstance(x, set):
            return tuple(sorted(self._hashable(v) for v in x))
        return x
    
    def thoughts(self) -> str:
        return "ListOperation in progress"
