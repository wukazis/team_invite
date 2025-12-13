import { useState } from 'react'
import { Api } from '../lib/api'
import './Login.css'

export function LoginPage() {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const startOAuth = async () => {
    setError(null)
    setLoading(true)
    try {
      const { authUrl } = await Api.login()
      window.location.href = authUrl
    } catch (err) {
      setError('无法获取授权链接，请稍后重试')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="page login-page">
      <section className="card login-card">
        <h1>登录</h1>
        <p className="muted">使用 Linux.do 授权后即可开始抽奖。</p>
        {error && <p className="error">{error}</p>}
        <button className="btn btn-primary full" onClick={startOAuth} disabled={loading}>
          {loading ? '跳转中...' : '使用 Linux.do 登录'}
        </button>
        <small className="muted">仅用于抽奖统计与邀请发送。</small>
      </section>
    </main>
  )
}
