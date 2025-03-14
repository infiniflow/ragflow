import React from 'react'
import { Routes, Route } from 'react-router-dom'
import TopNav from './TopNav'
import Greeting from '../components/Greeting'
import Knowledge from '../pages/Knowledge'
import Chat from '../pages/Chat'
import Agent from '../pages/Agent'
import AccountSettings from '../pages/AccountSettings'
import Settings from '../pages/Settings'

const MainLayout = () => {
  return (
    <div className="flex flex-col h-screen bg-gray-100">
      <TopNav />
      <main className="flex-1 overflow-y-auto">
        <div className="max-w-7xl mx-auto px-4 py-4">
          <Greeting />
          <div className="mt-4">
            <Routes>
              <Route path="/" element={<Knowledge />} />
              <Route path="/knowledge" element={<Knowledge />} />
              <Route path="/chat" element={<Chat />} />
              <Route path="/agent" element={<Agent />} />
              <Route path="/account" element={<AccountSettings />} />
              <Route path="/settings" element={<Settings />} />
            </Routes>
          </div>
        </div>
      </main>
    </div>
  )
}

export default MainLayout