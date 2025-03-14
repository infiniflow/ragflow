import React, { useState } from 'react'

type ModalStep = 'closed' | 'name' | 'details'

const Knowledge = () => {
  const [modalStep, setModalStep] = useState<ModalStep>('closed')
  const [kbName, setKbName] = useState('')
  const [permission, setPermission] = useState<'me' | 'team'>('me')
  const [files, setFiles] = useState<File[]>([])

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

  return (
    <div className="max-w-4xl mx-auto">
      <button
        onClick={() => setModalStep('name')}
        className="px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 font-medium"
      >
        Create Knowledge Base
      </button>

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
    </div>
  )
}

export default Knowledge 