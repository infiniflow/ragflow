# WebSocket API for Streaming Responses

## Overview

RAGFlow now supports WebSocket connections for real-time streaming responses. This feature is particularly useful for platforms like **WeChat Mini Programs** that require persistent bidirectional connections for interactive chat experiences.

## Why WebSocket?

Traditional HTTP-based Server-Sent Events (SSE) work well for most web applications, but some platforms have specific requirements:

- **WeChat Mini Programs** require WebSocket for real-time communication
- **Mobile apps** benefit from persistent connections with lower latency
- **Interactive applications** need bidirectional communication
- **Network efficiency** - reuse connections instead of creating new ones for each message

## Connection URL

```
ws://your-ragflow-host/v1/ws/chat
wss://your-ragflow-host/v1/ws/chat  (for SSL/TLS)
```

## Authentication

WebSocket connections support multiple authentication methods:

### 1. API Token (Recommended for Integrations)

Pass your API token in the Authorization header or as a query parameter:

**Header-based (preferred):**
```javascript
const ws = new WebSocket('ws://host/v1/ws/chat', {
    headers: {
        'Authorization': 'Bearer ragflow-your-api-token'
    }
});
```

**Query parameter (fallback for clients that can't set headers):**
```javascript
const ws = new WebSocket('ws://host/v1/ws/chat?token=ragflow-your-api-token');
```

### 2. User Session (For Web Applications)

If you're already logged in via the web interface, you can use your session JWT:

```javascript
const ws = new WebSocket('ws://host/v1/ws/chat', {
    headers: {
        'Authorization': 'your-jwt-token'
    }
});
```

## Message Format

### Client → Server (Request)

Send a JSON message to start a chat completion:

```json
{
    "type": "chat",
    "chat_id": "your-dialog-id",
    "question": "What is RAGFlow?",
    "stream": true,
    "session_id": "optional-session-id",
    "kb_ids": ["optional-kb-id"],
    "doc_ids": "optional-doc-ids"
}
```

**Fields:**
- `type` (string, required): Message type, currently only `"chat"` is supported
- `chat_id` (string, required): Your dialog/chat ID
- `question` (string, required): User's question or message
- `stream` (boolean, optional): Enable streaming responses (default: `true`)
- `session_id` (string, optional): Conversation session ID. If not provided, a new session is created
- `kb_ids` (array, optional): Knowledge base IDs to query for RAG
- `doc_ids` (string, optional): Comma-separated document IDs to prioritize
- `files` (array, optional): File IDs attached to this message

### Server → Client (Response)

The server sends multiple messages for a streaming response:

**Streaming chunk:**
```json
{
    "code": 0,
    "message": "",
    "data": {
        "answer": "RAGFlow is an open-source",
        "reference": {},
        "id": "message-id",
        "session_id": "session-id"
    }
}
```

**Completion marker:**
```json
{
    "code": 0,
    "message": "",
    "data": true
}
```

**Error message:**
```json
{
    "code": 500,
    "message": "Error description",
    "data": {
        "answer": "**ERROR**: Error details",
        "reference": []
    }
}
```

## Example Clients

### JavaScript (Web Browser / Node.js)

```javascript
// Create WebSocket connection
const ws = new WebSocket('ws://localhost/v1/ws/chat?token=ragflow-your-token');

// Connection opened
ws.addEventListener('open', function (event) {
    console.log('Connected to RAGFlow WebSocket');
    
    // Send a chat message
    ws.send(JSON.stringify({
        type: 'chat',
        chat_id: 'your-chat-id',
        question: 'What is artificial intelligence?',
        stream: true
    }));
});

// Listen for messages
ws.addEventListener('message', function (event) {
    const response = JSON.parse(event.data);
    
    // Check for completion
    if (response.data === true) {
        console.log('Stream completed');
        return;
    }
    
    // Check for errors
    if (response.code !== 0) {
        console.error('Error:', response.message);
        return;
    }
    
    // Display incremental answer
    console.log('Received chunk:', response.data.answer);
    
    // You can append to UI here
    // document.getElementById('answer').innerText += response.data.answer;
});

// Handle errors
ws.addEventListener('error', function (event) {
    console.error('WebSocket error:', event);
});

// Handle connection close
ws.addEventListener('close', function (event) {
    console.log('WebSocket closed:', event.code, event.reason);
});
```

### WeChat Mini Program

```javascript
// WeChat Mini Program WebSocket example
const app = getApp();

Page({
    data: {
        answer: '',
        socket: null
    },
    
    onLoad: function() {
        // Connect to WebSocket
        const socket = wx.connectSocket({
            url: 'wss://your-ragflow-host/v1/ws/chat?token=ragflow-your-token',
            success: () => {
                console.log('WebSocket connected');
            }
        });
        
        // Connection opened
        socket.onOpen(() => {
            console.log('WebSocket connection established');
            this.setData({ socket: socket });
            
            // Send chat message
            socket.send({
                data: JSON.stringify({
                    type: 'chat',
                    chat_id: 'your-chat-id',
                    question: '你好，什么是RAGFlow?',
                    stream: true
                })
            });
        });
        
        // Receive messages
        socket.onMessage((res) => {
            const response = JSON.parse(res.data);
            
            // Check for completion
            if (response.data === true) {
                console.log('Stream completed');
                return;
            }
            
            // Check for errors
            if (response.code !== 0) {
                wx.showToast({
                    title: response.message,
                    icon: 'none'
                });
                return;
            }
            
            // Append incremental answer
            this.setData({
                answer: this.data.answer + response.data.answer
            });
        });
        
        // Handle errors
        socket.onError((error) => {
            console.error('WebSocket error:', error);
            wx.showToast({
                title: 'Connection error',
                icon: 'none'
            });
        });
        
        // Handle close
        socket.onClose(() => {
            console.log('WebSocket connection closed');
        });
    },
    
    onUnload: function() {
        // Close WebSocket when leaving page
        if (this.data.socket) {
            this.data.socket.close();
        }
    }
});
```

### Python

```python
import websocket
import json
import threading

class RAGFlowWebSocketClient:
    def __init__(self, url, token):
        self.url = f"{url}?token={token}"
        self.ws = None
        
    def on_message(self, ws, message):
        """Handle incoming messages"""
        response = json.loads(message)
        
        # Check for completion
        if response['data'] is True:
            print('\nStream completed')
            return
        
        # Check for errors
        if response['code'] != 0:
            print(f"Error: {response['message']}")
            return
        
        # Print incremental answer
        print(response['data']['answer'], end='', flush=True)
    
    def on_error(self, ws, error):
        """Handle errors"""
        print(f"Error: {error}")
    
    def on_close(self, ws, close_status_code, close_msg):
        """Handle connection close"""
        print(f"\nConnection closed: {close_status_code} - {close_msg}")
    
    def on_open(self, ws):
        """Handle connection open"""
        print("Connected to RAGFlow")
        
        # Send chat message in a separate thread
        def send_message():
            message = {
                'type': 'chat',
                'chat_id': 'your-chat-id',
                'question': 'What is machine learning?',
                'stream': True
            }
            ws.send(json.dumps(message))
        
        threading.Thread(target=send_message).start()
    
    def connect(self):
        """Establish WebSocket connection"""
        self.ws = websocket.WebSocketApp(
            self.url,
            on_open=self.on_open,
            on_message=self.on_message,
            on_error=self.on_error,
            on_close=self.on_close
        )
        
        # Run forever (blocking)
        self.ws.run_forever()

# Usage
if __name__ == '__main__':
    client = RAGFlowWebSocketClient(
        url='ws://localhost/v1/ws/chat',
        token='ragflow-your-api-token'
    )
    client.connect()
```

### Go

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "github.com/gorilla/websocket"
)

type ChatRequest struct {
    Type     string `json:"type"`
    ChatID   string `json:"chat_id"`
    Question string `json:"question"`
    Stream   bool   `json:"stream"`
}

type ChatResponse struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data"`
}

func main() {
    // Connect to WebSocket
    url := "ws://localhost/v1/ws/chat?token=ragflow-your-token"
    conn, _, err := websocket.DefaultDialer.Dial(url, nil)
    if err != nil {
        log.Fatal("dial:", err)
    }
    defer conn.Close()
    
    // Send chat request
    request := ChatRequest{
        Type:     "chat",
        ChatID:   "your-chat-id",
        Question: "What is deep learning?",
        Stream:   true,
    }
    
    if err := conn.WriteJSON(request); err != nil {
        log.Fatal("write:", err)
    }
    
    // Read responses
    for {
        var response ChatResponse
        if err := conn.ReadJSON(&response); err != nil {
            log.Println("read:", err)
            return
        }
        
        // Check for completion
        if data, ok := response.Data.(bool); ok && data {
            fmt.Println("\nStream completed")
            break
        }
        
        // Check for errors
        if response.Code != 0 {
            log.Printf("Error: %s\n", response.Message)
            break
        }
        
        // Print incremental answer
        if dataMap, ok := response.Data.(map[string]interface{}); ok {
            if answer, ok := dataMap["answer"].(string); ok {
                fmt.Print(answer)
            }
        }
    }
}
```

## Connection Management

### Persistent Connections

WebSocket connections are persistent and can handle multiple request/response cycles without reconnecting:

```javascript
const ws = new WebSocket('ws://host/v1/ws/chat?token=your-token');

ws.onopen = () => {
    // Send first question
    ws.send(JSON.stringify({
        type: 'chat',
        chat_id: 'chat-id',
        question: 'First question?'
    }));
    
    // After receiving the complete response, you can send another question
    // without reconnecting
};

let responseCount = 0;
ws.onmessage = (event) => {
    const response = JSON.parse(event.data);
    
    if (response.data === true) {
        responseCount++;
        
        // Send next question
        if (responseCount === 1) {
            ws.send(JSON.stringify({
                type: 'chat',
                chat_id: 'chat-id',
                session_id: 'same-session-id', // Continue conversation
                question: 'Follow-up question?'
            }));
        }
    }
};
```

### Error Handling

Always implement proper error handling:

```javascript
ws.onerror = (error) => {
    console.error('WebSocket error:', error);
    // Implement reconnection logic if needed
};

ws.onclose = (event) => {
    if (event.code !== 1000) {
        // Abnormal closure - implement reconnection
        console.log('Reconnecting in 3 seconds...');
        setTimeout(() => {
            // Reconnect logic here
        }, 3000);
    }
};
```

### Close Codes

Common WebSocket close codes:

- `1000` - Normal closure
- `1008` - Policy violation (authentication failed)
- `1011` - Internal server error
- `1006` - Abnormal closure (connection lost)

## Session Management

### Creating a New Session

Don't provide a `session_id` in your first message:

```json
{
    "type": "chat",
    "chat_id": "your-chat-id",
    "question": "First question"
}
```

The server will create a new session and return the `session_id` in the response.

### Continuing a Session

Use the `session_id` from previous responses:

```json
{
    "type": "chat",
    "chat_id": "your-chat-id",
    "session_id": "returned-session-id",
    "question": "Follow-up question"
}
```

## Health Check

Test WebSocket connectivity without authentication:

```javascript
const ws = new WebSocket('ws://host/v1/ws/health');

ws.onopen = () => {
    ws.send('ping');
};

ws.onmessage = (event) => {
    console.log('Health check:', JSON.parse(event.data));
};
```

## Troubleshooting

### Connection Refused

- Check if RAGFlow server is running
- Verify the correct host and port
- Ensure WebSocket support is enabled

### Authentication Failed

- Verify your API token is correct
- Check if the token has expired
- Ensure proper authorization format: `Bearer <token>`

### No Response

- Verify the `chat_id` exists and you have access
- Check if the dialog has knowledge bases configured
- Review server logs for errors

### Connection Drops

- Implement reconnection logic
- Use heartbeat/ping messages to keep connection alive
- Check network stability

## Performance Tips

1. **Reuse connections**: Don't create new WebSocket for each message
2. **Implement backoff**: Wait before reconnecting after errors
3. **Buffer messages**: Queue messages if connection is temporarily down
4. **Clean up**: Always close WebSocket when done
5. **Monitor latency**: Track round-trip times for optimization

## Security Considerations

1. **Always use WSS (WebSocket Secure)** in production
2. **Never expose API tokens** in client-side code
3. **Implement rate limiting** on client side
4. **Validate all inputs** before sending
5. **Handle sensitive data** according to your security policies

## Migration from SSE

If you're currently using Server-Sent Events (SSE), here's how to migrate:

**SSE (Old):**
```javascript
const eventSource = new EventSource('/v1/conversation/completion');
eventSource.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log(data);
};
```

**WebSocket (New):**
```javascript
const ws = new WebSocket('ws://host/v1/ws/chat?token=your-token');
ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log(data);
};
ws.send(JSON.stringify({
    type: 'chat',
    chat_id: 'your-chat-id',
    question: 'Your question'
}));
```

## Additional Resources

- [WebSocket API Standard](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
- [WeChat Mini Program WebSocket](https://developers.weixin.qq.com/miniprogram/dev/api/network/websocket/wx.connectSocket.html)
- [RAGFlow API Documentation](../references/api_reference.md)

## Support

For issues or questions:
- GitHub Issues: https://github.com/infiniflow/ragflow/issues
- Documentation: https://ragflow.io/docs
- Community: Join our Discord/Slack channel

