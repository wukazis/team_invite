import { type FormEvent, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Api, ApiError, type TeamAccountStatus } from '../lib/api'
import { useAuth } from '../context/AuthContext'
import './Invite.css'

declare global {
  interface Window {
    turnstile?: {
      render: (element: HTMLElement, options: Record<string, any>) => any
      reset?: (id: any) => void
      remove?: (id: any) => void
    }
  }
}

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
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('')
  const [turnstileToken, setTurnstileToken] = useState<string | null>(null)
  const widgetRef = useRef<HTMLDivElement | null>(null)
  const widgetIdRef = useRef<any>(null)
  const navigate = useNavigate()
  const resolveTimerRef = useRef<number | null>(null)
  const [lockedTeamAccountId, setLockedTeamAccountId] = useState<number | null>(null)
  const [loadingAccounts, setLoadingAccounts] = useState(true)

  useEffect(() => {
    // 加载车的状态
    Api.getTeamAccountsStatus()
      .then((res) => {
        const accounts = res.accounts || []
        setTeamAccounts(accounts)
        const selectable = accounts.find((acc) => acc.seatsInUse + acc.pendingInvites < 5)
        setSelectedAccountId(selectable ? selectable.id : accounts[0]?.id ?? null)
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

  useEffect(() => {
    Api.getTurnstileSiteKey()
      .then((res) => setTurnstileSiteKey(res.siteKey || ''))
      .catch(() => setTurnstileSiteKey(''))
  }, [])

  useEffect(() => {
    if (!turnstileSiteKey) return
    const ensureScript = () =>
      new Promise<void>((resolve, reject) => {
        if (window.turnstile) {
          resolve()
          return
        }
        const existing = document.querySelector('script[data-turnstile]')
        if (existing) {
          existing.addEventListener('load', () => resolve(), { once: true })
          existing.addEventListener('error', () => reject(new Error('turnstile script load failed')), { once: true })
          return
        }
        const script = document.createElement('script')
        script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js'
        script.async = true
        script.defer = true
        script.setAttribute('data-turnstile', '1')
        script.onload = () => resolve()
        script.onerror = () => reject(new Error('turnstile script load failed'))
        document.body.appendChild(script)
      })

    ensureScript()
      .then(() => {
        if (!window.turnstile || !widgetRef.current) return
        widgetIdRef.current = window.turnstile.render(widgetRef.current, {
          sitekey: turnstileSiteKey,
          theme: 'light',
          callback: (token: string) => setTurnstileToken(token),
          'error-callback': () => setTurnstileToken(null),
          'expired-callback': () => setTurnstileToken(null),
        })
      })
      .catch(() => {
        setTurnstileToken(null)
      })

    return () => {
      if (widgetIdRef.current && window.turnstile?.remove) {
        window.turnstile.remove(widgetIdRef.current)
      }
    }
  }, [turnstileSiteKey])

  useEffect(() => {
    if (inviteCode) return
    const trimmed = manualCode.trim()
    if (!trimmed) {
      setLockedTeamAccountId(null)
    }
    if (resolveTimerRef.current) {
      window.clearTimeout(resolveTimerRef.current)
    }
    if (!trimmed) return
    resolveTimerRef.current = window.setTimeout(() => {
      Api.resolveInviteCode(trimmed)
        .then((res) => {
          if (res.teamAccountId) {
            setLockedTeamAccountId(res.teamAccountId)
            const exists = teamAccounts.find((a) => a.id === res.teamAccountId)
            if (exists) {
              setSelectedAccountId(res.teamAccountId)
            }
          } else {
            setLockedTeamAccountId(null)
          }
        })
        .catch(() => {})
    }, 300)
    return () => {
      if (resolveTimerRef.current) {
        window.clearTimeout(resolveTimerRef.current)
      }
    }
  }, [manualCode, inviteCode, teamAccounts])

  const getAvailableSeats = (acc: TeamAccountStatus) => {
    return acc.seatsEntitled - acc.seatsInUse - acc.pendingInvites
  }

  const handleSubmit = async (evt: FormEvent) => {
    evt.preventDefault()
    setStatus('submitting')
    setError(null)
    setSuccessMessage(null)
    if (!selectedAccountId) {
      setError('请选择要加入的车位')
      setStatus('idle')
      return
    }
    if (turnstileSiteKey && !turnstileToken) {
      setError('请完成人机验证')
      setStatus('idle')
      return
    }
    try {
      const payloadCode = inviteCode ?? manualCode.trim()
      await Api.submitInvite(email, selectedAccountId, payloadCode, turnstileToken || undefined)
      setUsedEmail(email)
      setStatus('completed')
      setSuccessMessage('发送成功 ✓')
      // 刷新车状态
      Api.getTeamAccountsStatus()
        .then((res) => {
          const accounts = res.accounts || []
          setTeamAccounts(accounts)
          const selectable = accounts.find((acc) => acc.seatsInUse + acc.pendingInvites < 5)
          setSelectedAccountId(selectable ? selectable.id : accounts[0]?.id ?? null)
        })
        .catch(() => {})
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
              const isBusy = acc.seatsInUse + acc.pendingInvites >= 5
              const isFull = isBusy
              const isSelected = selectedAccountId === acc.id
              const usedPercent = acc.seatsEntitled > 0 ? (acc.seatsInUse / acc.seatsEntitled) * 100 : 0
              const pendingPercent = acc.seatsEntitled > 0 ? (acc.pendingInvites / acc.seatsEntitled) * 100 : 0
              const availablePercent = acc.seatsEntitled > 0 ? (available / acc.seatsEntitled) * 100 : 0
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
            <label>
              <span>将加入</span>
              <select
                className="fancy-select"
                value={selectedAccountId ?? ''}
                onChange={(e) => {
                  if (lockedTeamAccountId) return
                  setSelectedAccountId(Number(e.target.value) || null)
                }}
                required
                disabled={Boolean(lockedTeamAccountId)}
              >
                <option value="" disabled>
                  {teamAccounts.length === 0 ? '暂无车位' : '请选择车位'}
                </option>
                {teamAccounts.map((acc) => {
                  const busy = acc.seatsInUse + acc.pendingInvites >= 5
                  return (
                    <option key={acc.id} value={acc.id} disabled={busy && !lockedTeamAccountId}>
                      {acc.name} ({acc.seatsInUse + acc.pendingInvites}/{acc.seatsEntitled}) {busy ? '(不可选)' : ''}
                    </option>
                  )
                })}
              </select>
              {lockedTeamAccountId && <p className="muted">该邀请码已绑定车位，已自动选择</p>}
            </label>
            {turnstileSiteKey && (
              <div className="turnstile-block">
                <div ref={widgetRef} />
              </div>
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
