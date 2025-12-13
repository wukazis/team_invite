import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Api, ApiError } from '../lib/api'
import { useAuth } from '../context/AuthContext'
import './Home.css'

type UserState = {
  id: number
  username: string
  displayName: string
  trustLevel: number
  avatarTemplate?: string
  chances: number
  inviteStatus: number
}

type InviteInfo = {
  code?: string
  used?: boolean
  usedEmail?: string
  usedAt?: string
  createdAt?: string
}

type SpinResult = {
  prize: { type: string; name: string }
  invite?: { code: string }
  quota: number
  spinStatus: string
}

const INVITE_STATUS = {
  NONE: 0,
  PENDING: 1,
  COMPLETED: 2,
} as const

export function HomePage() {
  const { userToken } = useAuth()
  const [user, setUser] = useState<UserState | null>(null)
  const [quota, setQuota] = useState<number>(0)
  const [quotaSchedule, setQuotaSchedule] = useState<{
    applyAt: string
    target: number
    message?: string
  } | null>(null)
  const [serverOffsetMs, setServerOffsetMs] = useState(0)
  const [spinResult, setSpinResult] = useState<SpinResult | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const [spinning, setSpinning] = useState(false)
  const [invite, setInvite] = useState<InviteInfo | null>(null)
  const navigate = useNavigate()

  const inviteStatus = user?.inviteStatus ?? INVITE_STATUS.NONE
  const invitePending = inviteStatus === INVITE_STATUS.PENDING && Boolean(invite?.code)
  const inviteCompleted = inviteStatus === INVITE_STATUS.COMPLETED && Boolean(invite)
  const inviteLocked = invitePending || inviteCompleted

  useEffect(() => {
    let timer: number
    const fetchQuota = async () => {
      try {
        const res = await Api.getQuotaPublic()
        setQuota(res.quota)
        setQuotaSchedule(res.schedule ?? null)
        if (res.serverTime) {
          const serverMs = Date.parse(res.serverTime)
          if (!Number.isNaN(serverMs)) {
            setServerOffsetMs(serverMs - Date.now())
          }
        }
      } catch {
        // ignore
      }
    }
    fetchQuota()
    timer = window.setInterval(fetchQuota, 1000)
    return () => {
      window.clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    if (!userToken) {
      setUser(null)
      setInvite(null)
      return
    }
    Api.getUserState()
      .then((res) => {
        setUser(res.user)
        setQuota(res.quota)
        setInvite(res.invite ?? null)
      })
      .catch(() => {
        setUser(null)
      })
  }, [userToken])

  const canSpin = useMemo(() => {
    return Boolean(userToken && user && quota > 0 && user.chances > 0 && !spinning && !inviteLocked)
  }, [userToken, user, quota, spinning, inviteLocked])

  const statusText = useMemo(() => {
    if (!userToken) return '请先登录以开始抽奖'
    if (!user) return '正在同步账户信息...'
    if (inviteStatus === INVITE_STATUS.COMPLETED) return '您已完成邀请码提交，无法继续抽奖'
    if (inviteStatus === INVITE_STATUS.PENDING) return '您已获取邀请码，请前往填写邮箱完成领取'
    if (quota <= 0) return '名额已满，请等待管理员补充名额'
    if (user.chances <= 0) return '今日次数已用完'
    return '准备好迎接今日的幸运了吗？'
  }, [userToken, user, quota, inviteStatus])

  const spinButtonText = useMemo(() => {
    if (inviteStatus === INVITE_STATUS.PENDING) return '已获得邀请码'
    if (inviteStatus === INVITE_STATUS.COMPLETED) return '邀请已完成'
    if (quota <= 0) return '请等待管理员补充名额'
    return spinning ? '等待抽奖...' : '立即抽奖'
  }, [inviteStatus, quota, spinning])

  const scheduleText = useMemo(() => {
    if (!quotaSchedule?.applyAt) return null
    const applyAtMs = Date.parse(quotaSchedule.applyAt)
    if (Number.isNaN(applyAtMs)) return null
    const nowMs = Date.now() + serverOffsetMs
    const remainingMs = Math.max(0, applyAtMs - nowMs)
    if (remainingMs <= 0) return null
    const totalSeconds = Math.ceil(remainingMs / 1000)
    const hours = Math.floor(totalSeconds / 3600)
    const minutes = Math.floor((totalSeconds % 3600) / 60)
    const seconds = totalSeconds % 60
    const hh = hours > 0 ? `${hours}:` : ''
    const mm = `${hours > 0 ? String(minutes).padStart(2, '0') : String(minutes)}`
    const ss = String(seconds).padStart(2, '0')
    const base = `预计 ${hh}${mm}:${ss} 后补充到 ${quotaSchedule.target}`
    return quotaSchedule.message ? `${base}（${quotaSchedule.message}）` : base
  }, [quotaSchedule, serverOffsetMs])

  const handleSpin = async () => {
    if (!canSpin) return
    setSpinning(true)
    setErrorMsg(null)
    setSpinResult(null)
    try {
      const spinId = crypto.randomUUID()
      const res = await Api.spin(spinId)
      setSpinResult(res)
      setQuota(res.quota)
      setInvite(res.invite ?? null)
      setUser((prev) => {
        if (!prev) return prev
        const nextChance =
          prev.chances -
          1 +
          (res.prize.type === 'retry' ? 1 : 0) // retry 会返还一次
        return { ...prev, chances: Math.max(0, nextChance) }
      })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          setErrorMsg('名额或机会不足，请稍后再试')
        } else if (err.status === 401) {
          setErrorMsg('登录已失效，请重新登录')
        } else {
          setErrorMsg(err.message || '抽奖失败，请稍后再试')
        }
      } else {
        setErrorMsg('抽奖失败，请稍后再试')
      }
    } finally {
      setSpinning(false)
    }
  }

  useEffect(() => {
    if (!spinResult) return
    if (spinResult.prize.type === 'win') return
    const timer = window.setTimeout(() => setSpinResult(null), 4000)
    return () => window.clearTimeout(timer)
  }, [spinResult])

  return (
    <main className="page home-page">
      <section className="hero card">
        <div className="hero__meta">
          <p className="eyebrow">抽奖大厅</p>
          <h1>开始抽奖</h1>
          <p className="subtitle">中奖后填写邮箱领取邀请。</p>
          {scheduleText && <p className="countdown">{scheduleText}</p>}
        </div>
        <div className="hero__status">
          <div className="quota-pill">
            <span>剩余名额</span>
            <strong>{quota}</strong>
          </div>
          <div className="quota-pill secondary">
            <span>剩余次数</span>
            <strong>{user?.chances ?? 0}</strong>
          </div>
        </div>
      </section>

      <section className="card spin-panel">
        <button className="btn btn-primary spin-button" disabled={!canSpin} onClick={handleSpin}>
          {spinButtonText}
        </button>
        <div className="spin-panel__status">
          <p className="status">{statusText}</p>
          {errorMsg && <p className="error">{errorMsg}</p>}
        </div>
      </section>

      {spinResult && (
        <section className="card result-card">
          {spinResult.prize.type === 'win' ? (
            <div className="result success">
              <h2>恭喜中奖!</h2>
              <div className="invite-code">{spinResult.invite?.code ?? '待下发'}</div>
              <button className="btn btn-primary" onClick={() => navigate('/invite')}>
                前往填写
              </button>
            </div>
          ) : (
            <div className="result neutral">
              <h2>{spinResult.prize.name}</h2>
              <p>{spinResult.prize.type === 'retry' ? '再试一次' : '明天再来'}</p>
            </div>
          )}
        </section>
      )}

      {invitePending && invite?.code && (
        <section className="card invite-card">
          <div>
            <h3>已获取专属邀请码</h3>
            <p>抽奖机会已锁定，填写邮箱即可完成领取</p>
          </div>
          <div className="invite-code">{invite.code}</div>
          <button className="btn btn-primary" onClick={() => navigate('/invite')}>
            去填写
          </button>
        </section>
      )}

      {inviteCompleted && invite && (
        <section className="card invite-history">
          <div className="invite-history__header">
            <h3>已完成领取</h3>
            <p>以下为已用邀请的记录，系统将不再提供抽奖入口。</p>
          </div>
          <ul className="invite-history__list">
            <li>
              <span>邀请码</span>
              <strong>{invite.code ?? '—'}</strong>
            </li>
            <li>
              <span>发送邮箱</span>
              <strong>{invite.usedEmail ?? '—'}</strong>
            </li>
            <li>
              <span>使用时间</span>
              <strong>{invite.usedAt ? new Date(invite.usedAt).toLocaleString() : '—'}</strong>
            </li>
          </ul>
        </section>
      )}
    </main>
  )
}
