import { useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'

export function AuthCallbackPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { setUserToken } = useAuth()

  useEffect(() => {
    const params = new URLSearchParams(location.search)
    const token = params.get('token')
    if (token) {
      setUserToken(token)
      navigate('/', { replace: true })
    } else {
      navigate('/login', { replace: true })
    }
  }, [location.search, navigate, setUserToken])

  return (
    <main className="page">
      <p>正在处理授权，请稍候...</p>
    </main>
  )
}
