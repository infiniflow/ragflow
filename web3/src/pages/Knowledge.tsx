import React, { useState } from 'react'

type ModalStep = 'closed' | 'name' | 'details' | 'edit'
type KnowledgeBase = {
  id: string
  name: string
  permission: 'me' | 'team'
  createdAt: string
}

const Knowledge = () => {
  const [modalStep, setModalStep] = useState<ModalStep>('closed')
  const [kbName, setKbName] = useState('')
  const [permission, setPermission] = useState<'me' | 'team'>('me')
  const [files, setFiles] = useState<File[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedKb, setSelectedKb] = useState<KnowledgeBase | null>(null)
  const [showDropdown, setShowDropdown] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editPermission, setEditPermission] = useState<'me' | 'team'>('me')

  // Sample data - replace with actual data
  const knowledgeBases: KnowledgeBase[] = [
    { id: '1', name: 'Product Documentation', permission: 'team', createdAt: '2024-03-15' },
    { id: '2', name: 'API Guidelines', permission: 'me', createdAt: '2024-03-14' },
    { id: '3', name: 'User Manuals', permission: 'team', createdAt: '2024-03-13' },
  ]

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const droppedFiles = Array.from(e.dataTransfer.files)
    setFiles(prev => [...prev, ...droppedFiles])
  }

  const handleCancel = () => {
    setModalStep('closed')
    setKbName('')
    setPermission('me')
    setFiles([])
  }

  const handleNext = () => {
    if (!kbName.trim()) return
    setModalStep('details')
  }

  const handleEditClick = (kb: KnowledgeBase) => {
    setSelectedKb(kb)
    setEditName(kb.name)
    setEditPermission(kb.permission)
    setModalStep('edit')
    setShowDropdown(null)
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
              placeholder="Search knowledge bases..."
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
          onClick={() => setModalStep('name')}
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
          Create Knowledge Base
        </button>
      </div>

      {/* Knowledge Base Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {knowledgeBases
          .filter(kb => 
            kb.name.toLowerCase().includes(searchQuery.toLowerCase())
          )
          .map(kb => (
            <div
              key={kb.id}
              className="bg-white rounded-lg shadow-sm hover:shadow-md transition-shadow p-6"
            >
              <div className="flex items-start justify-between">
                <div>
                  <h3 className="text-lg font-semibold text-gray-900 mb-2">
                    {kb.name}
                  </h3>
                  <p className="text-sm text-gray-500">
                    Created on {new Date(kb.createdAt).toLocaleDateString()}
                  </p>
                  <span className={`inline-block mt-2 px-2 py-1 text-xs rounded-full ${
                    kb.permission === 'team' 
                      ? 'bg-green-100 text-green-800'
                      : 'bg-blue-100 text-blue-800'
                  }`}>
                    {kb.permission === 'team' ? 'Team' : 'Private'}
                  </span>
                </div>
                <div className="relative">
                  <button 
                    className="text-gray-400 hover:text-gray-600"
                    onClick={() => setShowDropdown(showDropdown === kb.id ? null : kb.id)}
                  >
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z" />
                    </svg>
                  </button>
                  {showDropdown === kb.id && (
                    <div className="absolute right-0 mt-2 w-48 bg-white rounded-md shadow-lg py-1 z-10 border">
                      <button
                        onClick={() => handleEditClick(kb)}
                        className="block w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                      >
                        Edit Knowledge Base
                      </button>
                      <button
                        onClick={() => {
                          setSelectedKb(kb)
                          setModalStep('details')
                          setShowDropdown(null)
                        }}
                        className="block w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                      >
                        Upload Files
                      </button>
                    </div>
                  )}
                </div>
              </div>
            </div>
          ))}
      </div>

      {/* Name Input Modal */}
      {modalStep === 'name' && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center">
          <div className="bg-white rounded-lg p-6 w-96">
            <h2 className="text-xl font-semibold mb-4">Create Knowledge Base</h2>
            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Knowledge Base Name *
              </label>
              <input
                type="text"
                value={kbName}
                onChange={(e) => setKbName(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="Enter name"
                autoFocus
              />
            </div>
            <div className="flex justify-end space-x-3">
              <button
                onClick={handleCancel}
                className="px-4 py-2 text-gray-700 hover:bg-gray-100 rounded-md"
              >
                Cancel
              </button>
              <button
                onClick={handleNext}
                disabled={!kbName.trim()}
                className={`px-4 py-2 rounded-md ${
                  kbName.trim()
                    ? 'bg-blue-600 text-white hover:bg-blue-700'
                    : 'bg-gray-300 text-gray-500 cursor-not-allowed'
                }`}
              >
                OK
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Details Modal */}
      {modalStep === 'details' && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center">
          <div className="bg-white rounded-lg p-6 w-[600px]">
            <h2 className="text-xl font-semibold mb-4">Knowledge Base Setup</h2>
            
            {/* Knowledge Base Name Display */}
            <div className="mb-6">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Knowledge Base Name
              </label>
              <div className="text-gray-900">{kbName}</div>
            </div>

            {/* Permission Settings */}
            <div className="mb-6">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Permission
              </label>
              <div className="flex space-x-4">
                <button
                  onClick={() => setPermission('me')}
                  className={`px-4 py-2 rounded-md ${
                    permission === 'me'
                      ? 'bg-blue-600 text-white'
                      : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                  }`}
                >
                  Just Me
                </button>
                <button
                  onClick={() => setPermission('team')}
                  className={`px-4 py-2 rounded-md ${
                    permission === 'team'
                      ? 'bg-blue-600 text-white'
                      : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                  }`}
                >
                  Team
                </button>
              </div>
            </div>

            {/* File Upload Area */}
            <div className="mb-6">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Upload Files
              </label>
              <div
                onDragOver={(e) => e.preventDefault()}
                onDrop={handleDrop}
                className="border-2 border-dashed border-gray-300 rounded-lg p-8 text-center hover:border-blue-500 transition-colors"
              >
                <div className="text-gray-600">
                  Drop your files/folders here
                </div>
                {files.length > 0 && (
                  <div className="mt-4">
                    <div className="text-sm font-medium text-gray-700 mb-2">
                      Uploaded Files:
                    </div>
                    <ul className="text-sm text-gray-600">
                      {files.map((file, index) => (
                        <li key={index}>{file.name}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            </div>

            {/* Action Buttons */}
            <div className="flex justify-end space-x-3">
              <button
                onClick={handleCancel}
                className="px-4 py-2 text-gray-700 hover:bg-gray-100 rounded-md"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  // Handle creation logic here
                  handleCancel()
                }}
                className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {modalStep === 'edit' && selectedKb && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center">
          <div className="bg-white rounded-lg p-6 w-[600px]">
            <h2 className="text-xl font-semibold mb-4">Edit Knowledge Base</h2>
            
            {/* Knowledge Base Name Edit */}
            <div className="mb-6">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Knowledge Base Name
              </label>
              <input
                type="text"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="Enter name"
              />
            </div>

            {/* Permission Settings */}
            <div className="mb-6">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Permission
              </label>
              <div className="flex space-x-4">
                <button
                  onClick={() => setEditPermission('me')}
                  className={`px-4 py-2 rounded-md ${
                    editPermission === 'me'
                      ? 'bg-blue-600 text-white'
                      : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                  }`}
                >
                  Just Me
                </button>
                <button
                  onClick={() => setEditPermission('team')}
                  className={`px-4 py-2 rounded-md ${
                    editPermission === 'team'
                      ? 'bg-blue-600 text-white'
                      : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                  }`}
                >
                  Team
                </button>
              </div>
            </div>

            {/* Action Buttons */}
            <div className="flex justify-end space-x-3">
              <button
                onClick={() => {
                  setModalStep('closed')
                  setSelectedKb(null)
                }}
                className="px-4 py-2 text-gray-700 hover:bg-gray-100 rounded-md"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  // Handle update logic here
                  setModalStep('closed')
                  setSelectedKb(null)
                }}
                className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
              >
                Save Changes
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default Knowledge 