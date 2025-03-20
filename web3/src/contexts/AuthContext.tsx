/**
 * Authentication Context
 * 
 * This module defines a React Context for managing authentication state throughout the application.
 * It provides a centralized store for:
 * - Current user information
 * - Authentication status
 * - Login and logout functions
 * 
 * Components can consume this context to:
 * - Access the current authentication state
 * - Perform authentication actions (login/logout)
 * - Check if a user is authenticated before rendering protected content
 */

import { createContext } from 'react'

/**
 * User Interface
 * 
 * Defines the structure of user data stored in authentication state.
 * 
 * @property {number} id - Unique user identifier
 * @property {string} email - User's email address
 * @property {string} firstName - User's first name
 * @property {string} lastName - User's last name
 */
interface User {
  id: number
  email: string
  firstName: string
  lastName: string
}

/**
 * Authentication Context Interface
 * 
 * Defines the shape of the authentication context that will be
 * available to components throughout the application.
 * 
 * @property {User|null} user - Current authenticated user or null if not authenticated
 * @property {boolean} isAuthenticated - Whether a user is currently authenticated
 * @property {Function} login - Function to handle user login with token and user data
 * @property {Function} logout - Function to handle user logout
 */
interface AuthContextProps {
  user: User | null
  isAuthenticated: boolean
  login: (token: string, user: User) => void
  logout: () => void
}

/**
 * Create Authentication Context
 * 
 * Creates a new React Context with default values.
 * The actual values will be provided by the AuthContext.Provider
 * in the App component.
 */
const AuthContext = createContext<AuthContextProps>({
  user: null,
  isAuthenticated: false,
  login: () => {},  // Default no-op function
  logout: () => {}  // Default no-op function
})

export default AuthContext 