import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'

type WorkflowTemplate = {
  id: string
  name: string
  description: string
  icon: string
}

type Agent = {
  id: string
  name: string
  description: string
  createdAt: string
  status: 'active' | 'draft' | 'archived'
}

const FlowDesigner = () => {
  const navigate = useNavigate()
  const [searchQuery, setSearchQuery] = useState('')
  const [showTemplates, setShowTemplates] = useState(false)

  // Sample workflow templates
  const workflowTemplates: WorkflowTemplate[] = [
    { id: '1', name: 'Loan Underwriting', 
      description: 'Automate loan application review and approval process', 
      icon: '📝' },
    { id: '2', name: 'Individual Tax Filing', 
      description: 'Streamline personal tax preparation and filing', 
      icon: '💰' },
    { id: '3', name: 'Corporate Tax Filing', 
      description: 'Manage corporate tax compliance and documentation', 
      icon: '🏢' },
    { id: '4', name: 'Commercial Loan', 
      description: 'Process and evaluate commercial loan applications', 
      icon: '🏦' },
    { id: '5', name: 'Rental Property', 
      description: 'Manage rental property documentation and applications', 
      icon: '🏠' },
    { id: '6', name: 'Property Closing', 
      description: 'Streamline real estate closing procedures', 
      icon: '🔑' },
    { id: '7', name: 'Customer Service', 
      description: 'Automate customer inquiry handling and resolution', 
      icon: '👥' },
    { id: '8', name: 'Blank Template', 
      description: 'Start from scratch with a blank workflow template', 
      icon: '➕' },
  ]

  // Sample agents
  const agents: Agent[] = [
    { id: '1', name: 'Customer Onboarding', description: 'Handles new customer document processing and verification', createdAt: '2024-03-15', status: 'active' },
    { id: '2', name: 'Loan Assessment', description: 'Evaluates loan applications against criteria', createdAt: '2024-03-10', status: 'active' },
    { id: '3', name: 'Document Validator', description: 'Validates identity and financial documents', createdAt: '2024-02-28', status: 'draft' },
    { id: '4', name: 'Tax Calculator', description: 'Calculates tax obligations based on financial data', createdAt: '2024-02-15', status: 'active' },
  ]

  const selectTemplate = (template: WorkflowTemplate) => {
    // Navigate to the canvas page with the template information
    navigate('/flow-canvas', { state: { templateName: template.name } })
    setShowTemplates(false)
  }

  const handleEditWorkflow = (agent: Agent, e: React.MouseEvent) => {
    e.stopPropagation()
    navigate('/flow-canvas', { state: { templateName: agent.name, isEditing: true } })
  }

  return (
    <div>
      {/* Search and Create Section */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex-1 max-w-2xl mx-4">
          <div className="relative">
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search workflows..."
              className="w-full px-4 py-2 pl-10 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <svg
              className="absolute left-3 top-2.5 h-5 w-5 text-gray-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
              />
            </svg>
          </div>
        </div>
        <button
          onClick={() => setShowTemplates(true)}
          className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 font-medium flex items-center"
        >
          <svg
            className="w-5 h-5 mr-2"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="2"
              d="M12 6v6m0 0v6m0-6h6m-6 0H6"
            />
          </svg>
          Create Workflow
        </button>
      </div>

      {/* Workflow Templates Modal */}
      {showTemplates && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-10">
          <div className="bg-white rounded-lg p-6 w-[800px] max-h-[80vh] overflow-y-auto">
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-xl font-semibold text-gray-900">Select Workflow Template</h2>
              <button
                onClick={() => setShowTemplates(false)}
                className="text-gray-500 hover:text-gray-700"
              >
                <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {workflowTemplates.map(template => (
                <div
                  key={template.id}
                  onClick={() => selectTemplate(template)}
                  className="border rounded-lg p-4 hover:bg-blue-50 cursor-pointer transition-colors"
                >
                  <div className="flex items-center mb-2">
                    <span className="text-2xl mr-2">{template.icon}</span>
                    <h3 className="text-lg font-semibold text-gray-900">{template.name}</h3>
                  </div>
                  <p className="text-sm text-gray-600">{template.description}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Agents Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {agents
          .filter(agent => 
            agent.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
            agent.description.toLowerCase().includes(searchQuery.toLowerCase())
          )
          .map(agent => (
            <div
              key={agent.id}
              className="bg-white rounded-lg shadow-sm hover:shadow-md transition-shadow p-6"
            >
              <div className="flex flex-col">
                <div className="mb-4">
                  <h3 className="text-lg font-semibold text-gray-900 mb-2">
                    {agent.name}
                  </h3>
                  <p className="text-sm text-gray-600 mb-3">{agent.description}</p>
                  <p className="text-sm text-gray-500">
                    Created on {new Date(agent.createdAt).toLocaleDateString()}
                  </p>
                  <span className={`inline-block mt-2 px-2 py-1 text-xs rounded-full ${
                    agent.status === 'active' 
                      ? 'bg-green-100 text-green-800'
                      : agent.status === 'draft'
                      ? 'bg-yellow-100 text-yellow-800'
                      : 'bg-gray-100 text-gray-800'
                  }`}>
                    {agent.status.charAt(0).toUpperCase() + agent.status.slice(1)}
                  </span>
                </div>
                <div className="mt-auto">
                  <button
                    onClick={(e) => handleEditWorkflow(agent, e)}
                    className="w-full px-3 py-2 bg-gray-100 text-gray-800 rounded-md hover:bg-gray-200 font-medium flex items-center justify-center"
                  >
                    <svg
                      className="w-4 h-4 mr-2"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth="2"
                        d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"
                      />
                    </svg>
                    Edit Workflow
                  </button>
                </div>
              </div>
            </div>
          ))}
      </div>
    </div>
  )
}

export default FlowDesigner 