# 插件

这个文件夹包含了RAGFlow的插件机制。

RAGFlow将会从`embedded_plugins`子文件夹中递归加载所有的插件。

## 支持的插件类型

目前，唯一支持的插件类型是`llm_tools`。

- `llm_tools`：用于供LLM进行调用的工具。

## 如何添加一个插件

添加一个LLM工具插件是很简单的：创建一个插件文件，向其中放一个继承自`LLMToolPlugin`的类，再实现它的`get_metadata`和`invoke`方法即可。

- `get_metadata`方法：这个方法返回一个`LLMToolMetadata`对象，其中包含了对这个工具的描述。
这些描述信息将被提供给LLM进行调用，和RAGFlow的Web前端用作展示。

- `invoke`方法：这个方法接受LLM生成的参数，并且返回一个`str`对象，其中包含了这个工具的执行结果。
这个工具的所有执行逻辑都应当放到这个方法里。

当你启动RAGFlow时，你会在日志中看见你的插件被加载了：

```
2025-05-15 19:29:08,959 INFO     34670 Recursively importing plugins from path `/some-path/ragflow/plugin/embedded_plugins`
2025-05-15 19:29:08,960 INFO     34670 Loaded llm_tools plugin BadCalculatorPlugin version 1.0.0
```

也可能会报错，这时就需要根据报错对你的插件进行修复。

### 示例

我们将会添加一个会给出错误答案的计算器工具，来演示添加插件的过程。

首先，在`embedded_plugins/llm_tools`文件夹下创建一个插件文件`bad_calculator.py`。

接下来，我们创建一个`BadCalculatorPlugin`类，继承基类`LLMToolPlugin`：

```python
class BadCalculatorPlugin(LLMToolPlugin):
    _version_ = "1.0.0"
```

`_version_`字段是必填的，用于指定这个插件的版本号。

我们的计算器拥有两个输入字段`a`和`b`，所以我们添加如下的`invoke`方法到`BadCalculatorPlugin`类中：

```python
def invoke(self, a: int, b: int) -> str:
    return str(a + b + 100)
```

`invoke`方法将会被LLM所调用。这个方法可以有许多参数，但它必须返回一个`str`。

最后，我们需要添加一个`get_metadata`方法，来告诉LLM怎样使用我们的`bad_calculator`工具：

```python
@classmethod
def get_metadata(cls) -> LLMToolMetadata:
    return {
        # 这个工具的名称，会提供给LLM
        "name": "bad_calculator",
        # 这个工具的展示名称，会提供给RAGFlow的Web前端
        "displayName": "$t:bad_calculator.name",
        # 这个工具的用法描述，会提供给LLM
        "description": "A tool to calculate the sum of two numbers (will give wrong answer)",
        # 这个工具的描述，会提供给RAGFlow的Web前端
        "displayDescription": "$t:bad_calculator.description",
        # 这个工具的参数
        "parameters": {
            # 第一个参数 - a
            "a": {
                # 参数类型，选项为：number, string, 或者LLM可以识别的任何类型
                "type": "number",
                # 这个参数的描述，会提供给LLM
                "description": "The first number",
                # 这个参数的描述，会提供给RAGFlow的Web前端
                "displayDescription": "$t:bad_calculator.params.a",
                # 这个参数是否是必填的
                "required": True
            },
            # 第二个参数 - b
            "b": {
                "type": "number",
                "description": "The second number",
                "displayDescription": "$t:bad_calculator.params.b",
                "required": True
            }
        }
```

`get_metadata`方法是一个`classmethod`。它会把这个工具的描述提供给LLM。

以`display`开头的字段可以使用一种特殊写法`$t:xxx`，这种写法将使用RAGFlow的国际化机制，从`llmTools`这个分类中获取文字。如果你不使用这种写法，那么前端将会显示此处的原始内容。

现在，我们的工具已经做好了，你可以在`生成回答`组件中选择这个工具来尝试一下。

