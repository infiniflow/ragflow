import React from 'react'
import { NavLink, useNavigate } from 'react-router-dom'

const TopNav = () => {
  const navigate = useNavigate()

  return (
    <div className="bg-white shadow-md">
      <div className="max-w-7xl mx-auto px-4">
        <div className="flex items-center h-16">
          <div className="flex-shrink-0 w-48">
            <h1 className="text-2xl font-bold text-gray-800">Logen AI</h1>
          </div>
          <nav className="flex-1 flex justify-center space-x-8">
            <NavLink
              to="/knowledge"
              className={({ isActive }) =>
                `px-4 py-2 rounded-md text-base font-medium ${
                  isActive
                    ? 'bg-blue-50 text-blue-600'
                    : 'text-gray-600 hover:bg-gray-50'
                }`
              }
            >
              Knowledge Base
            </NavLink>
            <NavLink
              to="/chat"
              className={({ isActive }) =>
                `px-4 py-2 rounded-md text-base font-medium ${
                  isActive
                    ? 'bg-blue-50 text-blue-600'
                    : 'text-gray-600 hover:bg-gray-50'
                }`
              }
            >
              Chat
            </NavLink>
            <NavLink
              to="/agent"
              className={({ isActive }) =>
                `px-4 py-2 rounded-md text-base font-medium ${
                  isActive
                    ? 'bg-blue-50 text-blue-600'
                    : 'text-gray-600 hover:bg-gray-50'
                }`
              }
            >
              Agent
            </NavLink>
          </nav>
          <div className="w-48 flex justify-end space-x-2">
            <button
              onClick={() => navigate('/settings')}
              className="p-2 text-gray-600 hover:text-gray-900 rounded-full hover:bg-gray-100"
              title="Settings"
            >
              <svg
                className="w-6 h-6"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                xmlns="http://www.w3.org/2000/svg"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="2"
                  d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                />
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth="2"
                  d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                />
              </svg>
            </button>
            <button
              onClick={() => navigate('/account')}
              className="p-2 text-gray-600 hover:text-gray-900 rounded-full hover:bg-gray-100"
              title="Account"
            >
              <svg
                className="w-6 h-6"
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
          </div>
        </div>
      </div>
    </div>
  )
}

export default TopNav 