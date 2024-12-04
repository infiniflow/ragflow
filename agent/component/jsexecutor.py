from abc import ABC
import json
import logging
from agent.component.base import ComponentBase, ComponentParamBase
import execjs

class JSExecutorParam(ComponentParamBase):
    """
    定义JS执行器组件参数
    """
    def __init__(self):
        super().__init__()
        self.script = ""  # JavaScript脚本
        self.input_names = ["input0"]  # 默认一个输入变量

    def check(self):
        # 检查input_names不能为空
        if not self.input_names:
            self.input_names = ["input0"]

class JSExecutor(ComponentBase, ABC):
    component_name = "JSExecutor"
    
    def _run(self, history, **kwargs):
        try:
            # 获取所有上游输入
            inputs = self.get_inputs()
            
            # 如果没有脚本且有输入,直接返回第一个输入
            if not self._param.script and inputs:
                # 获取第一个输入的内容
                first_input = inputs[0].get("content", "") if inputs else ""
                return JSExecutor.be_output(first_input)
                
            # 准备JavaScript执行环境
            script_template = """
                function process(inputs, names) {
                    // 将输入绑定到对应的变量名
                    let vars = {};
                    for(let i = 0; i < inputs.length; i++) {
                        if(i < names.length) {
                            vars[names[i]] = inputs[i];
                        } else {
                            vars['input' + i] = inputs[i];
                        }
                    }
                    
                    // 用户脚本
                    %s
                    
                    // 如果没有return语句,默认返回第一个输入
                    return vars[names[0]] || inputs[0];
                }
            """ % (self._param.script or "return vars[names[0]] || inputs[0];")
            
            # 执行脚本
            ctx = execjs.compile(script_template)
            result = ctx.call("process", 
                            [i.get("content", "") for i in inputs], 
                            self._param.input_names)
            
            return JSExecutor.be_output(result)
            
        except Exception as e:
            logging.error(f"JavaScript execution error: {str(e)}")
            return JSExecutor.be_output(f"Error: {str(e)}")