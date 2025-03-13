import React, { useState } from 'react'
import { useLocation } from 'react-router-dom'

const Header = () => {
  const [isDropdownOpen, setIsDropdownOpen] = useState(false)
  const location = useLocation()

  const getHeaderTitle = () => {
    switch (location.pathname) {
      case '/':
      case '/knowledge':
        return 'Knowledge Base'
      case '/chat':
        return 'Chat'
      case '/agent':
        return 'Agent'
      default:
        return 'Knowledge Base'
    }
  }

  return (
    <header className="bg-white shadow-sm">
      <div className="flex justify-between items-center px-4 py-3">
        <div className="text-xl font-semibold text-gray-800">{getHeaderTitle()}</div>
        <div className="relative">
          <button
            onClick={() => setIsDropdownOpen(!isDropdownOpen)}
            className="flex items-center space-x-2 text-gray-700 hover:text-gray-900 focus:outline-none"
          >
            <svg
              className="w-8 h-8 text-gray-600"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              xmlns="http://www.w3.org/2000/svg"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth="2"
                d="M5.121 17.804A13.937 13.937 0 0112 16c2.5 0 4.847.655 6.879 1.804M15 10a3 3 0 11-6 0 3 3 0 016 0zm6 2a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
          </button>

          {isDropdownOpen && (
            <div className="absolute right-0 mt-2 w-48 bg-white rounded-md shadow-lg py-1 z-10 border">
              <div className="px-4 py-2 border-b">
                <p className="text-sm font-medium text-gray-900">John Doe</p>
                <p className="text-sm text-gray-500">john@example.com</p>
              </div>
              <div className="px-4 py-2 border-b">
                <p className="text-xs font-medium text-gray-500">Account Tier</p>
                <p className="text-sm font-medium text-blue-600">Premium</p>
              </div>
              <a
                href="#"
                className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                onClick={(e) => {
                  e.preventDefault()
                  // Add password change logic here
                }}
              >
                Change Password
              </a>
              <a
                href="#"
                className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                onClick={(e) => {
                  e.preventDefault()
                  // Add account settings logic here
                }}
              >
                Account Settings
              </a>
              <a
                href="#"
                className="block px-4 py-2 text-sm text-red-600 hover:bg-gray-100"
                onClick={(e) => {
                  e.preventDefault()
                  // Add logout logic here
                }}
              >
                Sign Out
              </a>
            </div>
          )}
        </div>
      </div>
    </header>
  )
}

export default Header