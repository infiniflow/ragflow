import React, { useState } from 'react'

const AccountSettings = () => {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')

  return (
    <div className="max-w-3xl mx-auto p-6">
      <h1 className="text-2xl font-bold text-gray-900 mb-8">Account Settings</h1>
      
      {/* Profile Section */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold text-gray-800 mb-4">Profile</h2>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">Username</label>
            <input
              type="text"
              value="John Doe"
              disabled
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md bg-gray-50"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">Email</label>
            <input
              type="email"
              value="john@example.com"
              disabled
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md bg-gray-50"
            />
          </div>
        </div>
      </div>

      {/* Password Change Section */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold text-gray-800 mb-4">Change Password</h2>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">Current Password</label>
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">New Password</label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">Confirm New Password</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
            />
          </div>
          <button className="w-full px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
            Update Password
          </button>
        </div>
      </div>

      {/* Subscription Section */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="text-xl font-semibold text-gray-800 mb-4">Subscription</h2>
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <div>
              <p className="font-medium text-gray-900">Current Plan: Premium</p>
              <p className="text-sm text-gray-500">Billed monthly</p>
            </div>
            <button className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
              Manage Subscription
            </button>
          </div>
        </div>
      </div>

      {/* Account Cancellation */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-xl font-semibold text-red-600 mb-4">Cancel Account</h2>
        <p className="text-gray-600 mb-4">
          Once you cancel your account, all of your data will be permanently deleted.
          This action cannot be undone.
        </p>
        <button className="px-4 py-2 bg-red-600 text-white rounded-md hover:bg-red-700">
          Cancel Account
        </button>
      </div>
    </div>
  )
}

export default AccountSettings 