/**
 * Main Application Component
 * 
 * This component serves as the root of the application, setting up:
 * 1. Authentication context and state management
 * 2. Routing configuration with protected routes
 * 3. Persistent authentication using localStorage
 * 
 * The component wraps the entire application in the AuthContext provider
 * to make authentication state and functions available throughout the app.
 */

import React, { useState, useEffect } from 'react'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import MainLayout from './layouts/MainLayout'
import Login from './pages/Login'
import ForgotPassword from './pages/ForgotPassword'
import ResetPassword from './pages/ResetPassword'
import AuthContext from './contexts/AuthContext'

function App() {
  // Authentication state
  const [user, setUser] = useState<any>(null)           // Current user data
  const [isAuthenticated, setIsAuthenticated] = useState(false) // Authentication status
  const [loading, setLoading] = useState(true)          // Loading state during initial auth check

  /**
   * Check for existing authentication on component mount
   * 
   * This effect runs once when the application loads to:
   * 1. Check if the user has a valid token in localStorage
   * 2. Restore the authentication state from localStorage if available
   * 3. Update the loading state once the check is complete
   */
  useEffect(() => {
    // Retrieve authentication data from localStorage
    const token = localStorage.getItem('token')
    const userData = localStorage.getItem('user')
    
    // If both token and user data exist, restore authentication state
    if (token && userData) {
      setUser(JSON.parse(userData))
      setIsAuthenticated(true)
    }
    
    // Mark loading as complete
    setLoading(false)
  }, [])

  /**
   * Login function
   * 
   * Handles user login by:
   * 1. Storing authentication token in localStorage
   * 2. Storing user data in localStorage
   * 3. Updating the authentication state
   * 
   * @param {string} token - JWT authentication token
   * @param {object} userData - User profile information
   */
  const login = (token: string, userData: any) => {
    localStorage.setItem('token', token)
    localStorage.setItem('user', JSON.stringify(userData))
    setUser(userData)
    setIsAuthenticated(true)
  }

  /**
   * Logout function
   * 
   * Handles user logout by:
   * 1. Removing authentication data from localStorage
   * 2. Resetting the authentication state
   */
  const logout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('user')
    setUser(null)
    setIsAuthenticated(false)
  }

  // Display loading indicator while checking authentication
  if (loading) {
    return <div>Loading...</div>
  }

  return (
    // Provide authentication context to all child components
    <AuthContext.Provider value={{ user, isAuthenticated, login, logout }}>
      <Router>
        <Routes>
          {/* Public routes - accessible without authentication */}
          {/* Redirect to home if already authenticated */}
          <Route path="/login" element={!isAuthenticated ? <Login /> : <Navigate to="/" />} />
          <Route path="/forgot-password" element={<ForgotPassword />} />
          <Route path="/reset-password" element={<ResetPassword />} />
          
          {/* Protected routes - require authentication */}
          {/* Redirect to login if not authenticated */}
          <Route
            path="/*"
            element={isAuthenticated ? <MainLayout /> : <Navigate to="/login" />}
          />
        </Routes>
      </Router>
    </AuthContext.Provider>
  )
}

export default App