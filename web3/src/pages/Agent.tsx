import React, { useState } from 'react'

const Agent = () => {
  const [prompt, setPrompt] = useState('')
  const [agentResponses, setAgentResponses] = useState<Array<{ role: 'user' | 'agent', content: string }>>([])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!prompt.trim()) return

    setAgentResponses(prev => [...prev, { role: 'user', content: prompt }])
    
    // Simulate agent response
    setTimeout(() => {
      setAgentResponses(prev => [...prev, { 
        role: 'agent', 
        content: 'This is an agent response simulation.' 
      }])
    }, 1000)

    setPrompt('')
  }

  return (
    <div className="max-w-4xl mx-auto">
      <div className="bg-white rounded-lg shadow p-6">
        <div className="space-y-4 mb-6 min-h-[400px]">
          {agentResponses.map((response, index) => (
            <div
              key={index}
              className={`p-4 rounded-lg ${
                response.role === 'user'
                  ? 'bg-blue-50 ml-12'
                  : 'bg-green-50 mr-12'
              }`}
            >
              {response.content}
            </div>
          ))}
        </div>
        
        <form onSubmit={handleSubmit} className="mt-4">
          <div className="flex gap-2">
            <input
              type="text"
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="Enter your prompt..."
              className="flex-1 p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <button
              type="submit"
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
            >
              Execute
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default Agent 