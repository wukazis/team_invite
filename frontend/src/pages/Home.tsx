import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Api } from '../lib/api'
import { useAuth } from '../context/AuthContext'
import './Home.css'

type UserState = {
  id: number
  username: string
  displayName: string
  trustLevel: number
  avatarTemplate?: string
  inviteStatus: number
}

type InviteInfo = {
  code?: string
  used?: boolean
  usedEmail?: string
  usedAt?: string
  createdAt?: string
}

const INVITE_STATUS = {
  NONE: 0,
  PENDING: 1,
  COMPLETED: 2,
} as const

export function HomePage() {
  const { userToken } = useAuth()
  const [user, setUser] = useState<UserState | null>(null)
  const [invite, setInvite] = useState<InviteInfo | null>(null)
  const navigate = useNavigate()

  const inviteStatus = user?.inviteStatus ?? INVITE_STATUS.NONE
  const invitePending = inviteStatus === INVITE_STATUS.PENDING && Boolean(invite?.code)
  const inviteCompleted = inviteStatus === INVITE_STATUS.COMPLETED && Boolean(invite)

  useEffect(() => {
    if (!userToken) {
      setUser(null)
      setInvite(null)
      return
    }
    Api.getUserState()
      .then((res) => {
        setUser(res.user)
        setInvite(res.invite ?? null)
      })
      .catch(() => {
        setUser(null)
      })
  }, [userToken])

  const statusText = useMemo(() => {
    if (!userToken) return '请先登录'
    if (!user) return '正在同步账户信息...'
    if (inviteStatus === INVITE_STATUS.COMPLETED) return '您已完成邀请码提交'
    if (inviteStatus === INVITE_STATUS.PENDING) return '您已获取邀请码，请前往填写邮箱完成领取'
    return '您尚未获得邀请码，请联系管理员或使用邀请码'
  }, [userToken, user, inviteStatus])

  return (
    <main className="page home-page">
      <section className="hero card">
        <div className="hero__meta">
          <p className="eyebrow">Team Invite</p>
          <h1>邀请码领取</h1>
          <p className="subtitle">使用邀请码填写邮箱领取邀请。</p>
        </div>
      </section>

      <section className="card spin-panel">
        <div className="spin-panel__status">
          <p className="status">{statusText}</p>
        </div>
        {userToken && user && inviteStatus === INVITE_STATUS.NONE && (
          <button className="btn btn-primary" onClick={() => navigate('/invite')}>
            使用邀请码
          </button>
        )}
      </section>

      {invitePending && invite?.code && (
        <section className="card invite-card">
          <div>
            <h3>已获取专属邀请码</h3>
            <p>填写邮箱即可完成领取</p>
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
            <p>以下为已用邀请的记录。</p>
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
