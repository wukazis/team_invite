import { type FormEvent, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Api, ApiError, type TeamAccountStatus } from '../lib/api'
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
  const [teamAccounts, setTeamAccounts] = useState<TeamAccountStatus[]>([])
  const [selectedAccountId, setSelectedAccountId] = useState<number | null>(null)
  const [loadingAccounts, setLoadingAccounts] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    // 加载车的状态
    Api.getTeamAccountsStatus()
      .then((res) => {
        setTeamAccounts(res.accounts || [])
        if (res.accounts?.length > 0) {
          setSelectedAccountId(res.accounts[0].id)
        }
      })
      .catch(() => setTeamAccounts([]))
      .finally(() => setLoadingAccounts(false))
  }, [])

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

  const getAvailableSeats = (acc: TeamAccountStatus) => {
    return acc.seatsEntitled - acc.seatsInUse - acc.pendingInvites
  }

  if (!userToken) {
    return (
      <main className="page invite-page">
        <section className="card">
          <p>用 linuxdo 授权登录以继续</p>
        </section>
      </main>
    )
  }

  return (
    <main className="page invite-page">
      {/* 车的状态卡片 - 仪表盘形式 */}
      <section className="card team-accounts-card">
        <h2>🚗 车位状态</h2>
        {loadingAccounts ? (
          <p className="muted">加载中...</p>
        ) : teamAccounts.length === 0 ? (
          <p className="muted">暂无可用车位</p>
        ) : (
          <div className="team-accounts-dashboard">
            {teamAccounts.map((acc) => {
              const available = getAvailableSeats(acc)
              const isFull = available <= 0
              const isSelected = selectedAccountId === acc.id
              const usedPercent = (acc.seatsInUse / acc.seatsEntitled) * 100
              const pendingPercent = (acc.pendingInvites / acc.seatsEntitled) * 100
              const availablePercent = (available / acc.seatsEntitled) * 100
              return (
                <div
                  key={acc.id}
                  className={`dashboard-row ${isSelected ? 'selected' : ''} ${isFull ? 'full' : ''}`}
                  onClick={() => !isFull && setSelectedAccountId(acc.id)}
                >
                  <div className="dashboard-name">{acc.name}</div>
                  <div className="dashboard-gauges">
                    <div className="gauge">
                      <svg viewBox="0 0 36 36">
                        <path className="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <path className="gauge-fill used" strokeDasharray={`${usedPercent}, 100`} d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <text x="18" y="20.5" className="gauge-text">{acc.seatsInUse}</text>
                      </svg>
                      <span className="gauge-label">已用</span>
                    </div>
                    <div className="gauge">
                      <svg viewBox="0 0 36 36">
                        <path className="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <path className="gauge-fill pending" strokeDasharray={`${pendingPercent}, 100`} d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <text x="18" y="20.5" className="gauge-text">{acc.pendingInvites}</text>
                      </svg>
                      <span className="gauge-label">待处理</span>
                    </div>
                    <div className="gauge">
                      <svg viewBox="0 0 36 36">
                        <path className="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <path className="gauge-fill total" strokeDasharray="100, 100" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <text x="18" y="20.5" className="gauge-text">{acc.seatsEntitled}</text>
                      </svg>
                      <span className="gauge-label">总席位</span>
                    </div>
                    <div className="gauge">
                      <svg viewBox="0 0 36 36">
                        <path className="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <path className={`gauge-fill ${isFull ? 'empty' : 'available'}`} strokeDasharray={`${availablePercent}, 100`} d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831" />
                        <text x="18" y="20.5" className="gauge-text">{available}</text>
                      </svg>
                      <span className="gauge-label">剩余</span>
                    </div>
                    {acc.activeUntil && (
                      <div className="dashboard-expire">
                        <span className="expire-label">到期</span>
                        <span className="expire-date">{new Date(acc.activeUntil).toLocaleDateString()}</span>
                      </div>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </section>

      {/* 邀请码提交卡片 */}
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
            {selectedAccountId && teamAccounts.length > 0 && (
              <p className="selected-account">
                将加入: <strong>{teamAccounts.find(a => a.id === selectedAccountId)?.name}</strong>
              </p>
            )}
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
