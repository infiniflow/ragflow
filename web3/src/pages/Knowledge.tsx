import React from 'react'
import { useState } from 'react'

const Knowledge = () => {
  const [query, setQuery] = useState('')
  const [messages, setMessages] = useState<Array<{ role: 'user' | 'assistant', content: string }>>([])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!query.trim()) return

    // Add user message
    setMessages(prev => [...prev, { role: 'user', content: query }])
    
    // TODO: Implement actual API call here
    // Simulating API response
    setTimeout(() => {
      setMessages(prev => [...prev, { 
        role: 'assistant', 
        content: 'This is a sample response. Replace with actual API integration.' 
      }])
    }, 1000)

    setQuery('')
  }

  return (
    <div className="max-w-4xl mx-auto">
      <div className="bg-white rounded-lg shadow p-6">
        <div className="space-y-4 mb-6">
          {messages.map((message, index) => (
            <div
              key={index}
              className={`p-4 rounded-lg ${
                message.role === 'user'
                  ? 'bg-blue-50 ml-12'
                  : 'bg-gray-50 mr-12'
              }`}
            >
              {message.content}
            </div>
          ))}
        </div>
        
        <form onSubmit={handleSubmit} className="mt-4">
          <div className="flex gap-2">
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Ask a question..."
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

export default Knowledge 