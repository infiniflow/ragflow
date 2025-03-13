import React, { useState } from 'react'

const Chat = () => {
  const [message, setMessage] = useState('')
  const [chatHistory, setCharHistory] = useState<Array<{ role: 'user' | 'assistant', content: string }>>([])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!message.trim()) return

    setCharHistory(prev => [...prev, { role: 'user', content: message }])
    
    // Simulate response
    setTimeout(() => {
      setCharHistory(prev => [...prev, { 
        role: 'assistant', 
        content: 'This is a chat response simulation.' 
      }])
    }, 1000)

    setMessage('')
  }

  return (
    <div className="max-w-4xl mx-auto">
      <div className="bg-white rounded-lg shadow p-6">
        <div className="space-y-4 mb-6 min-h-[400px]">
          {chatHistory.map((msg, index) => (
            <div
              key={index}
              className={`p-4 rounded-lg ${
                msg.role === 'user'
                  ? 'bg-blue-50 ml-12'
                  : 'bg-gray-50 mr-12'
              }`}
            >
              {msg.content}
            </div>
          ))}
        </div>
        
        <form onSubmit={handleSubmit} className="mt-4">
          <div className="flex gap-2">
            <input
              type="text"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              placeholder="Type your message..."
              className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <button
              type="submit"
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
            >
              Send
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default Chat 