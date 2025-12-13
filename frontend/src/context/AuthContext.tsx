import { createContext, useContext, useState, useEffect, type ReactNode } from 'react'
import { Storage } from '../lib/storage'

type AuthContextValue = {
  userToken: string | null
  adminToken: string | null
  setUserToken: (token: string | null) => void
  setAdminToken: (token: string | null) => void
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [userToken, setUserTokenState] = useState<string | null>(() => Storage.getUserToken())
  const [adminToken, setAdminTokenState] = useState<string | null>(() => Storage.getAdminToken())

  useEffect(() => {
    Storage.setUserToken(userToken)
  }, [userToken])

  useEffect(() => {
    Storage.setAdminToken(adminToken)
  }, [adminToken])

  const logout = () => {
    setUserTokenState(null)
    setAdminTokenState(null)
  }

  return (
    <AuthContext.Provider
      value={{
        userToken,
        adminToken,
        setUserToken: setUserTokenState,
        setAdminToken: setAdminTokenState,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
