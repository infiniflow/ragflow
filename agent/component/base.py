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

import re
import time
from abc import ABC, abstractmethod
import builtins
import json
import os
import logging
from typing import Any, List, Union
import pandas as pd
import trio
from agent import settings
from api.utils.api_utils import timeout


_FEEDED_DEPRECATED_PARAMS = "_feeded_deprecated_params"
_DEPRECATED_PARAMS = "_deprecated_params"
_USER_FEEDED_PARAMS = "_user_feeded_params"
_IS_RAW_CONF = "_is_raw_conf"


class ComponentParamBase(ABC):
    def __init__(self):
        self.message_history_window_size = 13
        self.inputs = {}
        self.outputs = {}
        self.description = ""
        self.max_retries = 0
        self.delay_after_error = 2.0
        self.exception_method = None
        self.exception_default_value = None
        self.exception_goto = None
        self.debug_inputs = {}

    def set_name(self, name: str):
        self._name = name
        return self

    def check(self):
        raise NotImplementedError("Parameter Object should be checked.")

    @classmethod
    def _get_or_init_deprecated_params_set(cls):
        if not hasattr(cls, _DEPRECATED_PARAMS):
            setattr(cls, _DEPRECATED_PARAMS, set())
        return getattr(cls, _DEPRECATED_PARAMS)

    def _get_or_init_feeded_deprecated_params_set(self, conf=None):
        if not hasattr(self, _FEEDED_DEPRECATED_PARAMS):
            if conf is None:
                setattr(self, _FEEDED_DEPRECATED_PARAMS, set())
            else:
                setattr(
                    self,
                    _FEEDED_DEPRECATED_PARAMS,
                    set(conf[_FEEDED_DEPRECATED_PARAMS]),
                )
        return getattr(self, _FEEDED_DEPRECATED_PARAMS)

    def _get_or_init_user_feeded_params_set(self, conf=None):
        if not hasattr(self, _USER_FEEDED_PARAMS):
            if conf is None:
                setattr(self, _USER_FEEDED_PARAMS, set())
            else:
                setattr(self, _USER_FEEDED_PARAMS, set(conf[_USER_FEEDED_PARAMS]))
        return getattr(self, _USER_FEEDED_PARAMS)

    def get_user_feeded(self):
        return self._get_or_init_user_feeded_params_set()

    def get_feeded_deprecated_params(self):
        return self._get_or_init_feeded_deprecated_params_set()

    @property
    def _deprecated_params_set(self):
        return {name: True for name in self.get_feeded_deprecated_params()}

    def __str__(self):
        return json.dumps(self.as_dict(), ensure_ascii=False)

    def as_dict(self):
        def _recursive_convert_obj_to_dict(obj):
            ret_dict = {}
            if isinstance(obj, dict):
                for k,v in obj.items():
                    if isinstance(v, dict) or (v and type(v).__name__ not in dir(builtins)):
                        ret_dict[k] = _recursive_convert_obj_to_dict(v)
                    else:
                        ret_dict[k] = v
                return ret_dict

            for attr_name in list(obj.__dict__):
                if attr_name in [_FEEDED_DEPRECATED_PARAMS, _DEPRECATED_PARAMS, _USER_FEEDED_PARAMS, _IS_RAW_CONF]:
                    continue
                # get attr
                attr = getattr(obj, attr_name)
                if isinstance(attr, pd.DataFrame):
                    ret_dict[attr_name] = attr.to_dict()
                    continue
                if isinstance(attr, dict) or (attr and type(attr).__name__ not in dir(builtins)):
                    ret_dict[attr_name] = _recursive_convert_obj_to_dict(attr)
                else:
                    ret_dict[attr_name] = attr

            return ret_dict

        return _recursive_convert_obj_to_dict(self)

    def update(self, conf, allow_redundant=False):
        update_from_raw_conf = conf.get(_IS_RAW_CONF, True)
        if update_from_raw_conf:
            deprecated_params_set = self._get_or_init_deprecated_params_set()
            feeded_deprecated_params_set = (
                self._get_or_init_feeded_deprecated_params_set()
            )
            user_feeded_params_set = self._get_or_init_user_feeded_params_set()
            setattr(self, _IS_RAW_CONF, False)
        else:
            feeded_deprecated_params_set = (
                self._get_or_init_feeded_deprecated_params_set(conf)
            )
            user_feeded_params_set = self._get_or_init_user_feeded_params_set(conf)

        def _recursive_update_param(param, config, depth, prefix):
            if depth > settings.PARAM_MAXDEPTH:
                raise ValueError("Param define nesting too deep!!!, can not parse it")

            inst_variables = param.__dict__
            redundant_attrs = []
            for config_key, config_value in config.items():
                # redundant attr
                if config_key not in inst_variables:
                    if not update_from_raw_conf and config_key.startswith("_"):
                        setattr(param, config_key, config_value)
                    else:
                        setattr(param, config_key, config_value)
                        # redundant_attrs.append(config_key)
                    continue

                full_config_key = f"{prefix}{config_key}"

                if update_from_raw_conf:
                    # add user feeded params
                    user_feeded_params_set.add(full_config_key)

                    # update user feeded deprecated param set
                    if full_config_key in deprecated_params_set:
                        feeded_deprecated_params_set.add(full_config_key)

                # supported attr
                attr = getattr(param, config_key)
                if type(attr).__name__ in dir(builtins) or attr is None:
                    setattr(param, config_key, config_value)

                else:
                    # recursive set obj attr
                    sub_params = _recursive_update_param(
                        attr, config_value, depth + 1, prefix=f"{prefix}{config_key}."
                    )
                    setattr(param, config_key, sub_params)

            if not allow_redundant and redundant_attrs:
                raise ValueError(
                    f"cpn `{getattr(self, '_name', type(self))}` has redundant parameters: `{[redundant_attrs]}`"
                )

            return param

        return _recursive_update_param(param=self, config=conf, depth=0, prefix="")

    def extract_not_builtin(self):
        def _get_not_builtin_types(obj):
            ret_dict = {}
            for variable in obj.__dict__:
                attr = getattr(obj, variable)
                if attr and type(attr).__name__ not in dir(builtins):
                    ret_dict[variable] = _get_not_builtin_types(attr)

            return ret_dict

        return _get_not_builtin_types(self)

    def validate(self):
        self.builtin_types = dir(builtins)
        self.func = {
            "ge": self._greater_equal_than,
            "le": self._less_equal_than,
            "in": self._in,
            "not_in": self._not_in,
            "range": self._range,
        }
        home_dir = os.path.abspath(os.path.dirname(os.path.realpath(__file__)))
        param_validation_path_prefix = home_dir + "/param_validation/"

        param_name = type(self).__name__
        param_validation_path = "/".join(
            [param_validation_path_prefix, param_name + ".json"]
        )

        validation_json = None

        try:
            with open(param_validation_path, "r") as fin:
                validation_json = json.loads(fin.read())
        except BaseException:
            return

        self._validate_param(self, validation_json)

    def _validate_param(self, param_obj, validation_json):
        default_section = type(param_obj).__name__
        var_list = param_obj.__dict__

        for variable in var_list:
            attr = getattr(param_obj, variable)

            if type(attr).__name__ in self.builtin_types or attr is None:
                if variable not in validation_json:
                    continue

                validation_dict = validation_json[default_section][variable]
                value = getattr(param_obj, variable)
                value_legal = False

                for op_type in validation_dict:
                    if self.func[op_type](value, validation_dict[op_type]):
                        value_legal = True
                        break

                if not value_legal:
                    raise ValueError(
                        "Plase check runtime conf, {} = {} does not match user-parameter restriction".format(
                            variable, value
                        )
                    )

            elif variable in validation_json:
                self._validate_param(attr, validation_json)

    @staticmethod
    def check_string(param, descr):
        if type(param).__name__ not in ["str"]:
            raise ValueError(
                descr + " {} not supported, should be string type".format(param)
            )

    @staticmethod
    def check_empty(param, descr):
        if not param:
            raise ValueError(
                descr + " does not support empty value."
            )

    @staticmethod
    def check_positive_integer(param, descr):
        if type(param).__name__ not in ["int", "long"] or param <= 0:
            raise ValueError(
                descr + " {} not supported, should be positive integer".format(param)
            )

    @staticmethod
    def check_positive_number(param, descr):
        if type(param).__name__ not in ["float", "int", "long"] or param <= 0:
            raise ValueError(
                descr + " {} not supported, should be positive numeric".format(param)
            )

    @staticmethod
    def check_nonnegative_number(param, descr):
        if type(param).__name__ not in ["float", "int", "long"] or param < 0:
            raise ValueError(
                descr
                + " {} not supported, should be non-negative numeric".format(param)
            )

    @staticmethod
    def check_decimal_float(param, descr):
        if type(param).__name__ not in ["float", "int"] or param < 0 or param > 1:
            raise ValueError(
                descr
                + " {} not supported, should be a float number in range [0, 1]".format(
                    param
                )
            )

    @staticmethod
    def check_boolean(param, descr):
        if type(param).__name__ != "bool":
            raise ValueError(
                descr + " {} not supported, should be bool type".format(param)
            )

    @staticmethod
    def check_open_unit_interval(param, descr):
        if type(param).__name__ not in ["float"] or param <= 0 or param >= 1:
            raise ValueError(
                descr + " should be a numeric number between 0 and 1 exclusively"
            )

    @staticmethod
    def check_valid_value(param, descr, valid_values):
        if param not in valid_values:
            raise ValueError(
                descr
                + " {} is not supported, it should be in {}".format(param, valid_values)
            )

    @staticmethod
    def check_defined_type(param, descr, types):
        if type(param).__name__ not in types:
            raise ValueError(
                descr + " {} not supported, should be one of {}".format(param, types)
            )

    @staticmethod
    def check_and_change_lower(param, valid_list, descr=""):
        if type(param).__name__ != "str":
            raise ValueError(
                descr
                + " {} not supported, should be one of {}".format(param, valid_list)
            )

        lower_param = param.lower()
        if lower_param in valid_list:
            return lower_param
        else:
            raise ValueError(
                descr
                + " {} not supported, should be one of {}".format(param, valid_list)
            )

    @staticmethod
    def _greater_equal_than(value, limit):
        return value >= limit - settings.FLOAT_ZERO

    @staticmethod
    def _less_equal_than(value, limit):
        return value <= limit + settings.FLOAT_ZERO

    @staticmethod
    def _range(value, ranges):
        in_range = False
        for left_limit, right_limit in ranges:
            if (
                    left_limit - settings.FLOAT_ZERO
                    <= value
                    <= right_limit + settings.FLOAT_ZERO
            ):
                in_range = True
                break

        return in_range

    @staticmethod
    def _in(value, right_value_list):
        return value in right_value_list

    @staticmethod
    def _not_in(value, wrong_value_list):
        return value not in wrong_value_list

    def _warn_deprecated_param(self, param_name, descr):
        if self._deprecated_params_set.get(param_name):
            logging.warning(
                f"{descr} {param_name} is deprecated and ignored in this version."
            )

    def _warn_to_deprecate_param(self, param_name, descr, new_param):
        if self._deprecated_params_set.get(param_name):
            logging.warning(
                f"{descr} {param_name} will be deprecated in future release; "
                f"please use {new_param} instead."
            )
            return True
        return False


class ComponentBase(ABC):
    component_name: str
    thread_limiter = trio.CapacityLimiter(int(os.environ.get('MAX_CONCURRENT_CHATS', 10)))
    variable_ref_patt = r"\{* *\{([a-zA-Z:0-9]+@[A-Za-z:0-9_.-]+|sys\.[a-z_]+)\} *\}*"

    def __str__(self):
        """
        {
            "component_name": "Begin",
            "params": {}
        }
        """
        return """{{
            "component_name": "{}",
            "params": {}
        }}""".format(self.component_name,
                     self._param
        )

    def __init__(self, canvas, id, param: ComponentParamBase):
        from agent.canvas import Canvas  # Local import to avoid cyclic dependency
        assert isinstance(canvas, Canvas), "canvas must be an instance of Canvas"
        self._canvas = canvas
        self._id = id
        self._param = param
        self._param.check()

    def invoke(self, **kwargs) -> dict[str, Any]:
        self.set_output("_created_time", time.perf_counter())
        try:
            self._invoke(**kwargs)
        except Exception as e:
            if self.get_exception_default_value():
                self.set_exception_default_value()
            else:
                self.set_output("_ERROR", str(e))
            logging.exception(e)
        self._param.debug_inputs = {}
        self.set_output("_elapsed_time", time.perf_counter() - self.output("_created_time"))
        return self.output()

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        raise NotImplementedError()

    def output(self, var_nm: str=None) -> Union[dict[str, Any], Any]:
        if var_nm:
            return self._param.outputs.get(var_nm, {}).get("value", "")
        return {k: o.get("value") for k,o in self._param.outputs.items()}

    def set_output(self, key: str, value: Any):
        if key not in self._param.outputs:
            self._param.outputs[key] = {"value": None, "type": str(type(value))}
        self._param.outputs[key]["value"] = value

    def error(self):
        return self._param.outputs.get("_ERROR", {}).get("value")

    def reset(self):
        for k in self._param.outputs.keys():
            self._param.outputs[k]["value"] = None
        for k in self._param.inputs.keys():
            self._param.inputs[k]["value"] = None
        self._param.debug_inputs = {}

    def get_input(self, key: str=None) -> Union[Any, dict[str, Any]]:
        if key:
            return self._param.inputs.get(key, {}).get("value")

        res = {}
        for var, o in self.get_input_elements().items():
            v = self.get_param(var)
            if v is None:
                continue
            if isinstance(v, str) and self._canvas.is_reff(v):
                self.set_input_value(var, self._canvas.get_variable_value(v))
            else:
                self.set_input_value(var, v)
            res[var] = self.get_input_value(var)
        return res

    def get_input_values(self) -> Union[Any, dict[str, Any]]:
        if self._param.debug_inputs:
            return self._param.debug_inputs

        return {var: self.get_input_value(var) for var, o in self.get_input_elements().items()}

    def get_input_elements_from_text(self, txt: str) -> dict[str, dict[str, str]]:
        res = {}
        for r in re.finditer(self.variable_ref_patt, txt, flags=re.IGNORECASE|re.DOTALL):
            exp = r.group(1)
            cpn_id, var_nm = exp.split("@") if exp.find("@")>0 else ("", exp)
            res[exp] = {
                "name": (self._canvas.get_component_name(cpn_id) +f"@{var_nm}") if cpn_id else exp,
                "value": self._canvas.get_variable_value(exp),
                "_retrival": self._canvas.get_variable_value(f"{cpn_id}@_references") if cpn_id else None,
                "_cpn_id": cpn_id
            }
        return res

    def get_input_elements(self) -> dict[str, Any]:
        return self._param.inputs

    def get_input_form(self) -> dict[str, dict]:
        return self._param.get_input_form()

    def set_input_value(self, key: str, value: Any) -> None:
        if key not in self._param.inputs:
            self._param.inputs[key] = {"value": None}
        self._param.inputs[key]["value"] = value

    def get_input_value(self, key: str) -> Any:
        if key not in self._param.inputs:
            return None
        return self._param.inputs[key].get("value")

    def get_component_name(self, cpn_id) -> str:
        return self._canvas.get_component(cpn_id)["obj"].component_name.lower()

    def get_param(self, name):
        if hasattr(self._param, name):
            return getattr(self._param, name)

    def debug(self, **kwargs):
        return self._invoke(**kwargs)

    def get_parent(self) -> Union[object, None]:
        pid = self._canvas.get_component(self._id).get("parent_id")
        if not pid:
            return
        return self._canvas.get_component(pid)["obj"]

    def get_upstream(self) -> List[str]:
        cpn_nms = self._canvas.get_component(self._id)['upstream']
        return cpn_nms

    @staticmethod
    def string_format(content: str, kv: dict[str, str]) -> str:
        for n, v in kv.items():
            def repl(_match, val=v):
                return str(val) if val is not None else ""
            content = re.sub(
                r"\{%s\}" % re.escape(n),
                repl,
                content
            )
        return content

    def exception_handler(self):
        if not self._param.exception_method:
            return
        return {
            "goto": self._param.exception_goto,
            "default_value": self._param.exception_default_value
        }

    def get_exception_default_value(self):
        if self._param.exception_method != "comment":
            return ""
        return self._param.exception_default_value

    def set_exception_default_value(self):
        self.set_output("result", self.get_exception_default_value())

    @abstractmethod
    def thoughts(self) -> str:
        ...
