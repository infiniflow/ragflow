```markdown
# RAGFlow Chat Integration Plugin (`ragflow_chat`)

## Overview

This folder contains the source code for the `ragflow_chat` plugin, which extends the core functionality of the RAGFlow API to support conversational interactions using Retrieval-Augmented Generation (RAG). It is designed to facilitate chatbot-style dialogues by combining a knowledge base with external language model API calls.

## Features

- **Conversational Interaction**: Supports user input in natural language and responds with answers based on RAG.
- **Knowledge Base Integration**: Leverages a RAG-based approach to search and retrieve relevant information from predefined knowledge bases.
- **Session Management**: Each conversation is tracked with a session ID, allowing for multi-turn dialogue handling.
- **API Key Authorization**: Requires an API key to access RAGFlow's services.

## Folder Structure

```bash
ragflow/
  └── api/
      └── ragflow_chat/
          ├── ragflow_chat.py         # Main plugin logic.
          ├── config.json             # Configuration file for API keys and endpoints.
          ├── requirements.txt        # Dependencies for the plugin.
          └── README.md               # Documentation for the plugin.
```

## Installation

1. **Install Dependencies**: 
   Make sure all dependencies are installed by running:
   ```bash
   pip install -r requirements.txt
   ```

2. **Configure API Keys**: 
   Edit the `config.json` file to include your RAGFlow and external API keys:
   ```json
   {
       "api_key": "YOUR_RAGFLOW_API_KEY",
       "host_address": "127.0.0.1:280"
   }
   ```

3. **Run the Plugin**:
   You can execute the plugin within your RAGFlow environment by calling the `ragflow_chat.py` script:
   ```bash
   python ragflow_chat.py
   ```

## Usage

### Create a New Conversation

You can initiate a new conversation with the following API request:

```bash
POST /api/ragflow_chat/new_conversation
```

**Request Example**:
```json
{
    "user_id": "unique_user_id"
}
```

### Send a Message

To send a message to an ongoing conversation, use the following request:

```bash
POST /api/ragflow_chat/send_message
```

**Request Example**:
```json
{
    "conversation_id": "existing_conversation_id",
    "message": {
        "role": "user",
        "content": "What is the capital of France?"
    }
}
```

**Response Example**:
```json
{
    "data": {
        "answer": "The capital of France is Paris.",
        "reference": []
    },
    "retcode": 0,
    "retmsg": "success"
}
```

### Configuration

All necessary configurations for the API key, endpoints, and session management are stored in the `config.json` file. Ensure that your API keys are up to date and that the host address points to the correct RAGFlow instance.

## Contribution

We welcome contributions! Please feel free to open issues or submit pull requests. When submitting code, ensure all changes are thoroughly tested.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
```

### 主要部分解释：
- **Overview**：介绍插件的核心功能，说明这是一个基于RAG（检索增强生成）的聊天插件。
- **Folder Structure**：展示项目的文件结构，帮助用户理解各文件的作用。
- **Installation**：安装步骤，包括如何安装依赖和配置API Key。
- **Usage**：提供简单的API用法示例，说明如何开始会话和发送消息。
- **Contribution**：邀请其他开发者贡献代码或提出问题。
- **License**：列出项目的许可信息，建议使用MIT License或其他常见的开源许可证。

这种README文件清晰简洁，能够帮助开发者快速上手你的插件项目。
