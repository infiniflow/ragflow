# RAGFlow WebSocket Examples

This directory contains example implementations for using RAGFlow's WebSocket API for real-time streaming responses.

## 📁 Files

- **`python_client.py`** - Python WebSocket client with interactive mode
- **`index.html`** - Web-based demo with interactive UI

## 🚀 Quick Start

### Python Client

#### Prerequisites

```bash
pip install websocket-client
```

#### Single Question Mode

```bash
python python_client.py \
    --url ws://localhost/v1/ws/chat \
    --token ragflow-your-api-token \
    --chat-id your-chat-id \
    --question "What is RAGFlow?"
```

#### Interactive Mode

```bash
python python_client.py \
    --url ws://localhost/v1/ws/chat \
    --token ragflow-your-api-token \
    --chat-id your-chat-id \
    --interactive
```

#### Continue Existing Session

```bash
python python_client.py \
    --url ws://localhost/v1/ws/chat \
    --token ragflow-your-api-token \
    --chat-id your-chat-id \
    --session-id existing-session-id \
    --question "Follow-up question?"
```

### Web Demo

1. Open `index.html` in your web browser
2. Enter your RAGFlow server URL, API token, and chat ID
3. Click "Connect"
4. Start chatting!

The web demo features:
- Real-time streaming responses
- Session persistence
- Error handling
- Auto-reconnection support
- Settings saved in localStorage

## 📖 Usage Examples

### Python Client Features

**Interactive conversation:**
```bash
python python_client.py --url ws://localhost/v1/ws/chat \
                       --token your-token \
                       --chat-id your-chat-id \
                       --interactive

# Then type questions interactively
👤 You: What is machine learning?
🤖 Answer: Machine learning is a subset of artificial intelligence...
✓ Stream completed

👤 You: Can you give examples?
🤖 Answer: Sure! Here are some examples...
```

**Debug mode:**
```bash
python python_client.py --url ws://localhost/v1/ws/chat \
                       --token your-token \
                       --chat-id your-chat-id \
                       --question "Hello" \
                       --debug
```

### Web Demo Features

**Auto-save settings:**
The web demo automatically saves your connection settings to localStorage, so you don't need to enter them every time.

**Session continuity:**
The demo maintains the session ID, allowing multi-turn conversations without reconnecting.

**Visual feedback:**
- Connection status indicator
- Streaming animation
- Error messages
- Message timestamps

## 🔧 Configuration

### Environment Variables

You can also use environment variables with the Python client:

```bash
export RAGFLOW_WS_URL="ws://localhost/v1/ws/chat"
export RAGFLOW_API_TOKEN="ragflow-your-token"
export RAGFLOW_CHAT_ID="your-chat-id"

python python_client.py --question "Hello"
```

### SSL/TLS

For secure connections, use `wss://` instead of `ws://`:

```bash
python python_client.py --url wss://your-ragflow-host/v1/ws/chat ...
```

## 📚 Documentation

For complete documentation, see:
- [WebSocket API Guide](../../docs/guides/websocket_api.md)
- [RAGFlow API Documentation](https://ragflow.io/docs/api)

## 🐛 Troubleshooting

### Connection Refused

**Problem:** `WebSocket error: Connection refused`

**Solution:**
1. Verify RAGFlow server is running
2. Check the WebSocket URL is correct
3. Ensure no firewall is blocking the connection

### Authentication Failed

**Problem:** `Error 401: Authentication required`

**Solution:**
1. Verify your API token is correct
2. Check token hasn't expired
3. Ensure proper token format: `ragflow-xxxxx`

### Invalid Chat ID

**Problem:** `Error 404: Dialog not found`

**Solution:**
1. Verify the chat ID exists
2. Check you have access to the dialog
3. Ensure you're using the correct tenant

### SSL Certificate Error

**Problem:** Certificate verification failed with `wss://`

**Solution:**

For Python client, disable SSL verification (development only):
```python
# In websocket.WebSocketApp
ws.run_forever(sslopt={"cert_reqs": ssl.CERT_NONE})
```

For production, use valid SSL certificates.

## 🎯 Best Practices

1. **Reuse connections**: Don't create new WebSocket for each message
2. **Handle reconnection**: Implement exponential backoff for reconnection
3. **Validate inputs**: Check all parameters before sending
4. **Error handling**: Always handle connection errors gracefully
5. **Clean up**: Close WebSocket when done

## 📝 License

Copyright 2024 The InfiniFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0.

## 🤝 Support

For issues or questions:
- GitHub Issues: https://github.com/infiniflow/ragflow/issues
- Documentation: https://ragflow.io/docs
- Community: Join our Discord/Slack

## 🌟 Contributing

We welcome contributions! Please see our [Contributing Guide](../../docs/contribution/README.md) for details.

