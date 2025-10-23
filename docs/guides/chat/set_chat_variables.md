---
sidebar_position: 4
slug: /set_chat_variables
---

# Set variables

Set variables to be used together with the system prompt for your LLM.

---

When configuring the system prompt for a chat model, variables play an important role in enhancing flexibility and reusability. With variables, you can dynamically adjust the system prompt to be sent to your model. In the context of RAGFlow, if you have defined variables in **Chat setting**, except for the system's reserved variable `{knowledge}`, you are required to pass in values for them from RAGFlow's [HTTP API](../../references/http_api_reference.md#converse-with-chat-assistant) or through its [Python SDK](../../references/python_api_reference.md#converse-with-chat-assistant).

:::danger IMPORTANT
In RAGFlow, variables are closely linked with the system prompt. When you add a variable in the **Variable** section, include it in the system prompt. Conversely, when deleting a variable, ensure it is removed from the system prompt; otherwise, an error would occur.
:::

## Where to set variables

![set_variables](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/chat_variables.jpg)

## 1. Manage variables

In the **Variable** section, you add, remove, or update variables.

### `{knowledge}` - a reserved variable

`{knowledge}` is the system's reserved variable, representing the chunks retrieved from the dataset(s) specified by **Knowledge bases** under the **Assistant settings** tab. If your chat assistant is associated with certain datasets, you can keep it as is.

:::info NOTE
It currently makes no difference whether  `{knowledge}` is set as optional or mandatory, but please note this design will be updated in due course.
:::

From v0.17.0 onward, you can start an AI chat without specifying datasets. In this case, we recommend removing the `{knowledge}` variable to prevent unnecessary reference and keeping the **Empty response** field empty to avoid errors.

### Custom variables

Besides `{knowledge}`, you can also define your own variables to pair with the system prompt. To use these custom variables, you must pass in their values through RAGFlow's official APIs. The **Optional** toggle determines whether these variables are required in the corresponding APIs:

- **Disabled** (Default): The variable is mandatory and must be provided.
- **Enabled**: The variable is optional and can be omitted if not needed.

## 2. Update system prompt

After you add or remove variables in the **Variable** section, ensure your changes are reflected in the system prompt to avoid inconsistencies or errors. Here's an example:

```
You are an intelligent assistant. Please answer the question by summarizing chunks from the specified dataset(s)...

Your answers should follow a professional and {style} style.

...

Here is the dataset:
{knowledge}
The above is the dataset.
```

:::tip NOTE
If you have removed `{knowledge}`, ensure that you thoroughly review and update the entire system prompt to achieve optimal results.
:::

## APIs

The *only* way to pass in values for the custom variables defined in the **Chat Configuration** dialogue is to call RAGFlow's [HTTP API](../../references/http_api_reference.md#converse-with-chat-assistant) or through its [Python SDK](../../references/python_api_reference.md#converse-with-chat-assistant).

### HTTP API

See [Converse with chat assistant](../../references/http_api_reference.md#converse-with-chat-assistant). Here's an example:

```json {9}
curl --request POST \
     --url http://{address}/api/v1/chats/{chat_id}/completions \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer <YOUR_API_KEY>' \
     --data-binary '
     {
          "question": "xxxxxxxxx",
          "stream": true,
          "style":"hilarious"
     }'
```

### Python API

See [Converse with chat assistant](../../references/python_api_reference.md#converse-with-chat-assistant). Here's an example:

```python {18}
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag_object.list_chats(name="Miss R")
assistant = assistant[0]
session = assistant.create_session()    

print("\n==================== Miss R =====================\n")
print("Hello. What can I do for you?")

while True:
    question = input("\n==================== User =====================\n> ")
    style = input("Please enter your preferred style (e.g., formal, informal, hilarious): ")
    
    print("\n==================== Miss R =====================\n")
    
    cont = ""
    for ans in session.ask(question, stream=True, style=style):
        print(ans.content[len(cont):], end='', flush=True)
        cont = ans.content
```

