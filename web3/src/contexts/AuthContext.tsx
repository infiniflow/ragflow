import { createContext } from 'react'

interface User {
  id: number
  email: string
  firstName: string
  lastName: string
}

interface AuthContextProps {
  user: User | null
  isAuthenticated: boolean
  login: (token: string, user: User) => void
  logout: () => void
}

const AuthContext = createContext<AuthContextProps>({
  user: null,
  isAuthenticated: false,
  login: () => {},
  logout: () => {}
})

export default AuthContext 