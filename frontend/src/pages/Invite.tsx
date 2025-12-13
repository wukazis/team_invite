import { type FormEvent, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Api, ApiError } from '../lib/api'
import { useAuth } from '../context/AuthContext'
import './Invite.css'

export function InvitePage() {
  const { userToken } = useAuth()
  const [inviteCode, setInviteCode] = useState<string | null>(null)
  const [manualCode, setManualCode] = useState('')
  const [usedEmail, setUsedEmail] = useState<string | null>(null)
  const [email, setEmail] = useState('')
  const [status, setStatus] = useState<'idle' | 'submitting' | 'completed'>('idle')
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)
  const navigate = useNavigate()

  useEffect(() => {
    if (!userToken) return
    Api.getInvite()
      .then((res) => {
        if (res.invite) {
          setInviteCode(res.invite.code)
          if (res.invite.used && res.invite.usedEmail) {
            setUsedEmail(res.invite.usedEmail)
            setStatus('completed')
          }
        } else {
          setInviteCode(null)
        }
      })
      .catch(() => {
        setInviteCode(null)
      })
  }, [userToken])

  const handleSubmit = async (evt: FormEvent) => {
    evt.preventDefault()
    setStatus('submitting')
    setError(null)
    setSuccessMessage(null)
    try {
      const payloadCode = inviteCode ?? manualCode.trim()
      await Api.submitInvite(email, payloadCode)
      setUsedEmail(email)
      setStatus('completed')
      setSuccessMessage('发送成功 ✓')
    } catch (err) {
      const message = err instanceof ApiError ? err.message : '提交失败，请稍后再试'
      setError(message)
      setStatus('idle')
    }
  }

  if (!userToken) {
    return (
      <main className="page invite-page">
        <section className="card">
          <p>请先登录以查看邀请码。</p>
          <button className="btn btn-primary" onClick={() => navigate('/login')}>
            去登录
          </button>
        </section>
      </main>
    )
  }

  return (
    <main className="page invite-page">
      <section className="card invite-card">
        <h1>邀请码提交</h1>
        {inviteCode && (
          <div className="invite-code-block">
            <div className="invite-code">{inviteCode}</div>
            <p className="muted">这是你的中奖邀请码</p>
          </div>
        )}
        {!inviteCode && <p className="muted">填写后台提供的邀请码和邮箱即可领取邀请。</p>}
        {usedEmail ? (
          <div className="success-box">
            <h2>已发送到</h2>
            <p className="success-text">{usedEmail}</p>
            <button className="btn btn-muted full" type="button" onClick={() => navigate('/')}>
              返回抽奖大厅
            </button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="invite-form">
            {!inviteCode && (
              <>
                <label htmlFor="manual-code">邀请码</label>
                <input
                  type="text"
                  id="manual-code"
                  value={manualCode}
                  onChange={(e) => setManualCode(e.target.value)}
                  placeholder="请输入后台提供的邀请码"
                  required={!inviteCode}
                />
              </>
            )}
            <label htmlFor="email">邮箱</label>
            <input
              type="email"
              id="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
            />
            {error && <p className="error">{error}</p>}
            {successMessage && <p className="success-text">{successMessage}</p>}
            <button className="btn btn-primary full" disabled={status === 'submitting'}>
              {status === 'submitting' ? '发送中...' : '发送邀请'}
            </button>
          </form>
        )}
      </section>
    </main>
  )
}
