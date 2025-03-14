import React, { useState } from 'react'

const Settings = () => {
  const [theme, setTheme] = useState('light')
  const [language, setLanguage] = useState('en')

  return (
    <div className="max-w-3xl mx-auto">
      <h1 className="text-2xl font-bold text-gray-900 mb-8">Settings</h1>
      
      {/* Theme Settings */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold text-gray-800 mb-4">Theme</h2>
        <div className="space-y-4">
          <div className="flex items-center space-x-4">
            <button
              onClick={() => setTheme('light')}
              className={`px-4 py-2 rounded-md ${
                theme === 'light'
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
              }`}
            >
              Light
            </button>
            <button
              onClick={() => setTheme('dark')}
              className={`px-4 py-2 rounded-md ${
                theme === 'dark'
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
              }`}
            >
              Dark
            </button>
            <button
              onClick={() => setTheme('system')}
              className={`px-4 py-2 rounded-md ${
                theme === 'system'
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
              }`}
            >
              System
            </button>
          </div>
        </div>
      </div>

      {/* Language Settings */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-semibold text-gray-800 mb-4">Language</h2>
        <div className="space-y-4">
          <select
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
            className="block w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            <option value="en">English</option>
            <option value="es">Español</option>
            <option value="fr">Français</option>
            <option value="de">Deutsch</option>
            <option value="zh">中文</option>
            <option value="ja">日本語</option>
            <option value="ko">한국어</option>
          </select>
          <button className="w-full px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
            Save Changes
          </button>
        </div>
      </div>
    </div>
  )
}

export default Settings 