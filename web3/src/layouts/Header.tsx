import React from 'react'

const Header = () => {
  return (
    <header className="bg-white shadow-sm">
      <div className="flex justify-between items-center px-4 py-3">
        <div className="text-xl font-semibold text-gray-800">Knowledge Base</div>
        <div className="flex items-center space-x-4">
          {/* Add any header actions here */}
        </div>
      </div>
    </header>
  )
}

export default Header