import React, { useState } from 'react'

type KnowledgeBase = {
  id: string
  name: string
  permission: 'me' | 'team'
}

type ChatSession = {
  id: string
  title: string
  lastMessage: string
  date: string
  messages: Array<{ role: 'user' | 'assistant', content: string }>
}

const Chat = () => {
  const [message, setMessage] = useState('')
  const [chatHistory, setChatHistory] = useState<Array<{ role: 'user' | 'assistant', content: string }>>([])
  const [selectedKbs, setSelectedKbs] = useState<string[]>([])
  const [currentSession, setCurrentSession] = useState<string | null>(null)

  // Sample data - should be shared with Knowledge component
  const knowledgeBases: KnowledgeBase[] = [
    { id: '1', name: 'Product Documentation', permission: 'team' },
    { id: '2', name: 'API Guidelines', permission: 'me' },
    { id: '3', name: 'User Manuals', permission: 'team' },
  ]

  // Sample chat sessions
  const chatSessions: ChatSession[] = [
    {
      id: '1',
      title: 'Product Questions',
      lastMessage: 'How do I integrate the API?',
      date: '2 hours ago',
      messages: [
        { role: 'user', content: 'What are the main features of the product?' },
        { role: 'assistant', content: 'The main features include API integration, user management, and data analytics.' },
        { role: 'user', content: 'How do I integrate the API?' },
        { role: 'assistant', content: 'You can integrate the API using our SDK or direct REST calls. Documentation is available in the API Guidelines.' }
      ]
    },
    {
      id: '2',
      title: 'Troubleshooting',
      lastMessage: 'Why am I getting error 404?',
      date: 'Yesterday',
      messages: [
        { role: 'user', content: 'I\'m having trouble accessing the dashboard.' },
        { role: 'assistant', content: 'Let\'s troubleshoot. Are you logged in with the correct credentials?' },
        { role: 'user', content: 'Yes, but I still get an error.' },
        { role: 'user', content: 'Why am I getting error 404?' },
        { role: 'assistant', content: 'Error 404 typically means the page wasn\'t found. Check if the URL is correct and you have permissions to access it.' }
      ]
    },
    {
      id: '3',
      title: 'Feature Request',
      lastMessage: 'Can you add dark mode?',
      date: '3 days ago',
      messages: [
        { role: 'user', content: 'I have some feature suggestions.' },
        { role: 'assistant', content: 'Great! I\'d be happy to hear them.' },
        { role: 'user', content: 'Can you add dark mode?' },
        { role: 'assistant', content: 'Thanks for the suggestion! I\'ll pass this along to our product team for consideration.' }
      ]
    }
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

    const newMessage = { role: 'user' as const, content: message }
    setChatHistory(prev => [...prev, newMessage])
    
    // Simulate response
    setTimeout(() => {
      const response = { 
        role: 'assistant' as const, 
        content: `Response using knowledge bases: ${selectedKbs.map(id => 
          knowledgeBases.find(kb => kb.id === id)?.name
        ).join(', ')}` 
      }
      setChatHistory(prev => [...prev, response])
    }, 1000)

    setMessage('')
  }

  const loadChatSession = (sessionId: string) => {
    const session = chatSessions.find(s => s.id === sessionId)
    if (session) {
      setChatHistory(session.messages)
      setCurrentSession(sessionId)
    }
  }

  return (
    <div className="flex h-[calc(100vh-150px)]">
      {/* Knowledge Base Selection Sidebar */}
      <div className="w-64 bg-white shadow-md p-4 mr-4 rounded-lg overflow-y-auto">
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
      <div className="flex-1 flex">
        <div className="flex-1 bg-white rounded-lg shadow p-6 mr-4 flex flex-col">
          <div className="flex-1 overflow-y-auto mb-4">
            <div className="space-y-4">
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
          </div>
          
          <form onSubmit={handleSubmit} className="mt-auto">
            <div className="flex gap-2">
              <input
                type="text"
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                placeholder={selectedKbs.length === 0 
                  ? "Please select at least one knowledge base..."
                  : "Type your message..."}
                disabled={selectedKbs.length === 0}
                className="flex-1 p-3 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100 disabled:text-gray-500"
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

        {/* Chat History Sidebar */}
        <div className="w-80 bg-white shadow-md rounded-lg overflow-hidden">
          <div className="p-4 border-b">
            <h2 className="text-lg font-semibold text-gray-800">Chat History</h2>
          </div>
          <div className="overflow-y-auto" style={{ maxHeight: 'calc(100% - 60px)' }}>
            {chatSessions.map(session => (
              <div
                key={session.id}
                onClick={() => loadChatSession(session.id)}
                className={`p-4 border-b cursor-pointer hover:bg-gray-50 transition-colors ${
                  currentSession === session.id ? 'bg-blue-50' : ''
                }`}
              >
                <div className="flex justify-between items-start mb-1">
                  <h3 className="font-medium text-gray-900">{session.title}</h3>
                  <span className="text-xs text-gray-500">{session.date}</span>
                </div>
                <p className="text-sm text-gray-600 truncate">{session.lastMessage}</p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

export default Chat 