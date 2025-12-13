import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Api, ApiError } from '../../lib/api'
import { useAuth } from '../../context/AuthContext'
import '../Login.css'

export function AdminLoginPage() {
  const { setAdminToken } = useAuth()
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const handleSubmit = async (evt: FormEvent) => {
    evt.preventDefault()
    setLoading(true)
    setError(null)
    try {
      const res = await Api.adminLogin({ password, code })
      setAdminToken(res.token)
      navigate('/admin')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="page login-page">
      <section className="card login-card">
        <h1>管理员登录</h1>
        <form onSubmit={handleSubmit} className="admin-login-form">
          <input value={password} onChange={(e) => setPassword(e.target.value)} placeholder="后台密码" type="password" required />
          <input value={code} onChange={(e) => setCode(e.target.value)} placeholder="TOTP 动态码" required />
          {error && <p className="error">{error}</p>}
          <button className="btn btn-primary" disabled={loading}>
            {loading ? '登录中...' : '登录后台'}
          </button>
        </form>
      </section>
    </main>
  )
}
