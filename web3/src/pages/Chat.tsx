import React, { useState } from 'react'

type KnowledgeBase = {
  id: string
  name: string
  permission: 'me' | 'team'
}

const Chat = () => {
  const [message, setMessage] = useState('')
  const [chatHistory, setCharHistory] = useState<Array<{ role: 'user' | 'assistant', content: string }>>([])
  const [selectedKbs, setSelectedKbs] = useState<string[]>([])

  // Sample data - should be shared with Knowledge component
  const knowledgeBases: KnowledgeBase[] = [
    { id: '1', name: 'Product Documentation', permission: 'team' },
    { id: '2', name: 'API Guidelines', permission: 'me' },
    { id: '3', name: 'User Manuals', permission: 'team' },
  ]

  const handleSelectAll = () => {
    if (selectedKbs.length === knowledgeBases.length) {
      setSelectedKbs([])
    } else {
      setSelectedKbs(knowledgeBases.map(kb => kb.id))
    }
  }

  const handleToggleKb = (kbId: string) => {
    setSelectedKbs(prev => 
      prev.includes(kbId) 
        ? prev.filter(id => id !== kbId)
        : [...prev, kbId]
    )
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!message.trim() || selectedKbs.length === 0) return

    setCharHistory(prev => [...prev, { role: 'user', content: message }])
    
    // Simulate response
    setTimeout(() => {
      setCharHistory(prev => [...prev, { 
        role: 'assistant', 
        content: `Response using knowledge bases: ${selectedKbs.map(id => 
          knowledgeBases.find(kb => kb.id === id)?.name
        ).join(', ')}` 
      }])
    }, 1000)

    setMessage('')
  }

  return (
    <div className="flex h-full">
      {/* Knowledge Base Selection Sidebar */}
      <div className="w-64 bg-white shadow-md p-4 mr-4 rounded-lg">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-800">Knowledge Bases</h2>
          <button
            onClick={handleSelectAll}
            className="text-sm text-blue-600 hover:text-blue-700"
          >
            {selectedKbs.length === knowledgeBases.length ? 'Deselect All' : 'Select All'}
          </button>
        </div>
        <div className="space-y-2">
          {knowledgeBases.map(kb => (
            <div
              key={kb.id}
              className="flex items-center p-2 hover:bg-gray-50 rounded-md"
            >
              <input
                type="checkbox"
                checked={selectedKbs.includes(kb.id)}
                onChange={() => handleToggleKb(kb.id)}
                className="h-4 w-4 text-blue-600 rounded border-gray-300 focus:ring-blue-500"
              />
              <label className="ml-2 text-sm text-gray-700 cursor-pointer flex-1">
                {kb.name}
              </label>
              <span className={`text-xs px-2 py-1 rounded-full ${
                kb.permission === 'team' 
                  ? 'bg-green-100 text-green-800'
                  : 'bg-blue-100 text-blue-800'
              }`}>
                {kb.permission === 'team' ? 'Team' : 'Private'}
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Chat Interface */}
      <div className="flex-1">
        <div className="bg-white rounded-lg shadow p-6 h-full flex flex-col">
          <div className="flex-1 space-y-4 mb-6 overflow-y-auto">
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
                placeholder={selectedKbs.length === 0 
                  ? "Please select at least one knowledge base..."
                  : "Type your message..."}
                disabled={selectedKbs.length === 0}
                className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100 disabled:text-gray-500"
              />
              <button
                type="submit"
                disabled={selectedKbs.length === 0}
                className={`px-4 py-2 rounded-lg ${
                  selectedKbs.length === 0
                    ? 'bg-gray-300 text-gray-500 cursor-not-allowed'
                    : 'bg-blue-600 text-white hover:bg-blue-700'
                }`}
              >
                Send
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}

export default Chat 