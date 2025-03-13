import React from 'react'
import { Routes, Route } from 'react-router-dom'
import Sidebar from './Sidebar'
import Header from './Header'
import Knowledge from '../pages/Knowledge'

const MainLayout = () => {
  return (
    <div className="flex h-screen bg-gray-100">
      <Sidebar />
      <div className="flex flex-col flex-1">
        <Header />
        <main className="flex-1 overflow-y-auto p-4">
          <Routes>
            <Route path="/" element={<Knowledge />} />
            <Route path="/knowledge" element={<Knowledge />} />
          </Routes>
        </main>
      </div>
    </div>
  )
}

export default MainLayout