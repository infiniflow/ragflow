import React from 'react'
import { NavLink } from 'react-router-dom'

const Sidebar = () => {
  return (
    <div className="w-64 bg-white shadow-md">
      <div className="p-4">
        <h1 className="text-2xl font-bold text-gray-800">Logen AI</h1>
      </div>
      <nav className="mt-4 flex flex-col space-y-1">
        <NavLink
          to="/knowledge"
          className={({ isActive }) =>
            `flex items-center px-4 py-2 ${
              isActive ? 'bg-blue-50 text-blue-600' : 'text-gray-600 hover:bg-gray-50'
            }`
          }
        >
          <span className="ml-2">Knowledge Base</span>
        </NavLink>
        <NavLink
          to="/chat"
          className={({ isActive }) =>
            `flex items-center px-4 py-2 ${
              isActive ? 'bg-blue-50 text-blue-600' : 'text-gray-600 hover:bg-gray-50'
            }`
          }
        >
          <span className="ml-2">Chat</span>
        </NavLink>
        <NavLink
          to="/agent"
          className={({ isActive }) =>
            `flex items-center px-4 py-2 ${
              isActive ? 'bg-blue-50 text-blue-600' : 'text-gray-600 hover:bg-gray-50'
            }`
          }
        >
          <span className="ml-2">Agent</span>
        </NavLink>
      </nav>
    </div>
  )
}

export default Sidebar