import { type ReactElement } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { HomePage } from './pages/Home'
import { LoginPage } from './pages/Login'
import { InvitePage } from './pages/Invite'
import { AuthCallbackPage } from './pages/AuthCallback'
import { AdminLoginPage } from './pages/admin/AdminLogin'
import { AdminDashboardPage } from './pages/admin/AdminDashboard'
import { NavBar } from './components/NavBar'
import { useAuth } from './context/AuthContext'

function ProtectedRoute({ children }: { children: ReactElement }) {
  const { adminToken } = useAuth()
  if (!adminToken) {
    return <Navigate to="/admin/login" replace />
  }
  return children
}

export default function App() {
  return (
    <div className="app-shell">
      <NavBar />
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/invite" element={<InvitePage />} />
        <Route path="/auth/callback" element={<AuthCallbackPage />} />
        <Route path="/admin/login" element={<AdminLoginPage />} />
        <Route
          path="/admin"
          element={
            <ProtectedRoute>
              <AdminDashboardPage />
            </ProtectedRoute>
          }
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </div>
  )
}
