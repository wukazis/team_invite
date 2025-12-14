import { type FormEvent, useEffect, useState } from 'react'
import { Api, ApiError, type TeamAccountStatus } from '../../lib/api'
import { useAuth } from '../../context/AuthContext'
import './AdminDashboard.css'

type Overview = {
  total: number
  win: number
  retry: number
  lose: number
  unknown: number
}

type HourlyRow = {
  hourLabel: string
  win: number
  retry: number
  lose: number
}

type PrizeItem = {
  type: string
  name: string
  probability: number
}

type UserRow = {
  id: number
  username: string
  trustLevel: number
  chances: number
  inviteStatus: number
  createdAt: string
  updatedAt: string
}

type SpinRow = {
  id: number
  userId: number
  username: string
  prize: string
  status: string
  detail?: string
  spinId: string
  createdAt: string
}

const ENV_DISPLAY_ORDER: Array<{ key: string; fullWidth?: boolean }> = [
  { key: 'SECRET_KEY' },
  { key: 'ADMIN_PASSWORD' },
  { key: 'AUTHORIZATION_TOKEN' },
  { key: 'ACCOUNT_ID' },
  { key: 'INVITE_ACCOUNTS', fullWidth: true },
  { key: 'INVITE_STRATEGY' },
  { key: 'INVITE_ACTIVE_ACCOUNT_ID' },
  { key: 'JWT_SECRET' },
  { key: 'POSTGRES_URL' },
  { key: 'CF_TURNSTILE_SECRET_KEY' },
  { key: 'CF_TURNSTILE_SITE_KEY' },
  { key: 'LINUXDO_CLIENT_ID' },
  { key: 'LINUXDO_CLIENT_SECRET' },
  { key: 'LINUXDO_REDIRECT_URI', fullWidth: false },
]

export function AdminDashboardPage() {
  const { logout } = useAuth()
  const USER_PAGE_SIZE = 50
  const [envValues, setEnvValues] = useState<Record<string, string>>({})
  const [envLoading, setEnvLoading] = useState(true)
  const [envMessage, setEnvMessage] = useState<string | null>(null)
  const [quotaInput, setQuotaInput] = useState('')
  const [scheduleMinutes, setScheduleMinutes] = useState(30)
  const [scheduleTarget, setScheduleTarget] = useState('')
  const [stats, setStats] = useState<HourlyRow[]>([])
  const [overview, setOverview] = useState<Overview | null>(null)
  const [statMessage, setStatMessage] = useState<string | null>(null)
  const [prizeConfig, setPrizeConfig] = useState<string>('[]')
  const [prizeMessage, setPrizeMessage] = useState<string | null>(null)
  const [resetMessage, setResetMessage] = useState<string | null>(null)
  const [users, setUsers] = useState<UserRow[]>([])
  const [userTotal, setUserTotal] = useState(0)
  const [userMessage, setUserMessage] = useState<string | null>(null)
  const [spins, setSpins] = useState<SpinRow[]>([])
  const [spinTotal, setSpinTotal] = useState(0)
  const [inviteCodes, setInviteCodes] = useState<InviteCodeRow[]>([])
  const [inviteTotal, setInviteTotal] = useState(0)
  const [inviteMessage, setInviteMessage] = useState<string | null>(null)
  const [inviteCountInput, setInviteCountInput] = useState('1')
  const [editingInvite, setEditingInvite] = useState<InviteCodeRow | null>(null)
  const [userOffset, setUserOffset] = useState(0)
  const [inviteOffset, setInviteOffset] = useState(0)
  const [activeSection, setActiveSection] = useState('env')
  // Team Accounts
  const [teamAccounts, setTeamAccounts] = useState<TeamAccountStatus[]>([])
  const [teamAccountMessage, setTeamAccountMessage] = useState<string | null>(null)
  const [editingAccount, setEditingAccount] = useState<Partial<TeamAccountStatus> | null>(null)

  useEffect(() => {
    Promise.all([loadEnv(), loadStats(), loadPrizeConfig(), loadUsers(0), loadSpins(), loadInviteCodes(), loadTeamAccounts()]).finally(() => setEnvLoading(false))
  }, [])

  const loadTeamAccounts = async () => {
    try {
      const res = await Api.adminListTeamAccounts()
      setTeamAccounts(res.accounts || [])
    } catch {
      setTeamAccountMessage('加载车账号失败')
    }
  }

  const handleSaveTeamAccount = async (evt: FormEvent) => {
    evt.preventDefault()
    if (!editingAccount) return
    try {
      if (editingAccount.id) {
        await Api.adminUpdateTeamAccount(editingAccount.id, {
          name: editingAccount.name || '',
          accountId: editingAccount.accountId || '',
          authToken: editingAccount.authToken || '',
          maxSeats: editingAccount.maxSeats || 50,
          enabled: editingAccount.enabled ?? true,
        })
        setTeamAccountMessage('更新成功')
      } else {
        await Api.adminCreateTeamAccount({
          name: editingAccount.name || '',
          accountId: editingAccount.accountId || '',
          authToken: editingAccount.authToken || '',
          maxSeats: editingAccount.maxSeats || 50,
        })
        setTeamAccountMessage('创建成功')
      }
      setEditingAccount(null)
      loadTeamAccounts()
    } catch (err) {
      setTeamAccountMessage(err instanceof ApiError ? err.message : '操作失败')
    }
  }

  const handleDeleteTeamAccount = async (acc: TeamAccountStatus) => {
    if (!window.confirm(`确定删除车账号 "${acc.name}" 吗？`)) return
    try {
      await Api.adminDeleteTeamAccount(acc.id)
      setTeamAccountMessage('删除成功')
      loadTeamAccounts()
    } catch (err) {
      setTeamAccountMessage(err instanceof ApiError ? err.message : '删除失败')
    }
  }

  const loadEnv = async () => {
    try {
      const res = await Api.adminFetchEnv()
      setEnvValues(res.env || {})
    } catch (err) {
      setEnvMessage(err instanceof ApiError ? err.message : '加载配置失败')
    }
  }

  const loadStats = async () => {
    try {
      const res = await Api.adminStats()
      setOverview(res.overview || null)
      setStats(res.stats || [])
    } catch (err) {
      setStatMessage(err instanceof ApiError ? err.message : '统计获取失败')
    }
  }

  const loadPrizeConfig = async () => {
    try {
      const res = await Api.adminGetPrizeConfig()
      setPrizeConfig(JSON.stringify(res.items, null, 2))
    } catch {
      setPrizeConfig('[]')
    }
  }

  const handleEnvSave = async (evt: FormEvent) => {
    evt.preventDefault()
    try {
      await Api.adminUpdateEnv(envValues)
      setEnvMessage('保存成功')
    } catch (err) {
      setEnvMessage(err instanceof ApiError ? err.message : '保存失败')
    }
  }

  const handleQuotaUpdate = async (evt: FormEvent) => {
    evt.preventDefault()
    try {
      const value = Number(quotaInput)
      await Api.adminUpdateQuota(value)
      setStatMessage(`已更新名额为 ${value}`)
      setQuotaInput('')
      loadStats()
    } catch (err) {
      setStatMessage(err instanceof ApiError ? err.message : '更新失败')
    }
  }

  const handleSchedule = async (evt: FormEvent) => {
    evt.preventDefault()
    try {
      await Api.adminScheduleQuota({ target: Number(scheduleTarget), delayMinutes: scheduleMinutes })
      setStatMessage('已设置定时名额')
      setScheduleTarget('')
    } catch (err) {
      setStatMessage(err instanceof ApiError ? err.message : '设置失败')
    }
  }

  const handlePrizeSave = async (evt: FormEvent) => {
    evt.preventDefault()
    try {
      const parsed: PrizeItem[] = JSON.parse(prizeConfig)
      await Api.updatePrizeConfig(parsed)
      setPrizeMessage('中奖配置已更新')
    } catch (err) {
      setPrizeMessage(err instanceof ApiError ? err.message : '配置格式错误或保存失败')
    }
  }

  const handleReset = async () => {
    try {
      const res = await Api.adminResetUsers()
      setResetMessage(`已重置 ${res.resetChances} 位用户抽奖次数`)
    } catch (err) {
      setResetMessage(err instanceof ApiError ? err.message : '操作失败')
    }
  }

  const loadUsers = async (offset = userOffset) => {
    try {
      const res = await Api.adminFetchUsers(USER_PAGE_SIZE, offset)
      setUsers(res.users || [])
      setUserTotal(res.total || 0)
      setUserOffset(offset)
    } catch (err) {
      setUserMessage(err instanceof ApiError ? err.message : '加载用户失败')
    }
  }

  const loadSpins = async () => {
    try {
      const res = await Api.adminFetchSpins()
      setSpins(res.records || [])
      setSpinTotal(res.total || 0)
    } catch {
      /* ignore */
    }
  }

  const loadInviteCodes = async (offset = inviteOffset) => {
    try {
      const res = await Api.adminFetchInviteCodes(20, offset)
      setInviteCodes(res.codes || [])
      setInviteTotal(res.total || 0)
      setInviteOffset(offset)
      setInviteMessage(null)
    } catch {
      setInviteMessage('加载邀请码失败')
    }
  }

  const handleGenerateInvites = async (evt: FormEvent) => {
    evt.preventDefault()
    const count = Math.min(10, Math.max(1, Number(inviteCountInput) || 1))
    try {
      const res = await Api.adminCreateInviteCodes(count)
      const codes = res.codes?.map((item) => item.code).join(', ')
      setInviteMessage(codes ? `已生成：${codes}` : '已生成新的邀请码')
      setInviteCountInput('1')
      loadInviteCodes(0)
    } catch (err) {
      setInviteMessage(err instanceof ApiError ? err.message : '生成失败')
    }
  }

  const handleDeleteInvite = async (code: InviteCodeRow) => {
    if (!window.confirm(`确定删除邀请码 ${code.code} 吗？`)) {
      return
    }
    try {
      await Api.adminDeleteInviteCode(code.id)
      setInviteMessage('邀请码已删除')
      loadInviteCodes(inviteOffset)
    } catch (err) {
      setInviteMessage(err instanceof ApiError ? err.message : '删除失败')
    }
  }

  const handleAssignInvite = async (code: InviteCodeRow) => {
    const userInput = window.prompt('请输入要绑定的用户ID', '')
    if (!userInput) return
    const userId = Number(userInput)
    if (!Number.isInteger(userId) || userId <= 0) {
      setInviteMessage('用户ID无效')
      return
    }
    try {
      await Api.adminAssignInviteCode(code.id, { userId })
      setInviteMessage(`邀请码已绑定到用户 ${userId}`)
      loadInviteCodes(inviteOffset)
    } catch (err) {
      setInviteMessage(err instanceof ApiError ? err.message : '绑定失败')
    }
  }

  const updateUserRow = async (user: UserRow, changes: { chances?: number; inviteStatus?: number }) => {
    try {
      await Api.adminUpdateUser(user.id, changes)
      setUsers((prev) =>
        prev.map((item) => (item.id === user.id ? { ...item, ...changes } : item)),
      )
      setUserMessage('用户已更新')
      loadUsers()
    } catch (err) {
      setUserMessage(err instanceof ApiError ? err.message : '更新失败')
    }
  }

  const inviteStatusOptions = [
    { value: 0, label: '未中奖' },
    { value: 1, label: '待填写' },
    { value: 2, label: '已完成' },
  ]

  if (envLoading) {
    return (
      <main className="page admin-page">
        <p>加载中...</p>
      </main>
    )
  }
  const envSection = (
    <section className="card admin-card">
      <h2>环境变量 (.env)</h2>
      {envMessage && <p className="info">{envMessage}</p>}
      <form onSubmit={handleEnvSave}>
        <div className="env-grid">
          {ENV_DISPLAY_ORDER.map(({ key, fullWidth }) => (
            <label key={key} className={fullWidth ? 'full-width' : undefined}>
              <span>{key}</span>
              <input value={envValues[key] ?? ''} onChange={(e) => setEnvValues((prev) => ({ ...prev, [key]: e.target.value }))} />
            </label>
          ))}
        </div>
        <div className="env-actions">
          <button className="btn btn-primary small" type="submit">
            保存 .env
          </button>
        </div>
      </form>
    </section>
  )

  const userSection = (
    <section className="card admin-card">
      <h2>用户管理</h2>
      <p className="info">共 {userTotal} 位用户</p>
      {userMessage && <p className="info">{userMessage}</p>}
      <div className="admin-table users">
        <div className="admin-table__head">
          <span>ID</span>
          <span>用户名</span>
          <span>等级</span>
          <span>次数</span>
          <span>状态</span>
          <span>操作</span>
        </div>
        {users.map((user) => (
          <div className="admin-table__row" key={user.id}>
            <span>{user.id}</span>
            <span>{user.username}</span>
            <span>Lv.{user.trustLevel}</span>
            <span>
              <input
                type="number"
                min={0}
                value={user.chances}
                onChange={(e) =>
                  setUsers((prev) =>
                    prev.map((item) => (item.id === user.id ? { ...item, chances: Number(e.target.value) } : item)),
                  )
                }
              />
            </span>
            <span>
              <select
                value={user.inviteStatus}
                onChange={(e) =>
                  setUsers((prev) =>
                    prev.map((item) => (item.id === user.id ? { ...item, inviteStatus: Number(e.target.value) } : item)),
                  )
                }
              >
                {inviteStatusOptions.map((opt) => (
                  <option value={opt.value} key={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </span>
            <span>
              <button
                className="btn btn-muted small"
                type="button"
                onClick={() =>
                  updateUserRow(user, { chances: user.chances, inviteStatus: user.inviteStatus })
                }
              >
                保存
              </button>
            </span>
          </div>
        ))}
      </div>
      <div className="table-actions">
        <span>
          显示 {userTotal === 0 ? 0 : userOffset + 1}-{Math.min(userOffset + USER_PAGE_SIZE, userTotal)} / {userTotal}
        </span>
        <div className="table-actions__buttons">
          <button className="btn btn-muted small" type="button" disabled={userOffset === 0} onClick={() => loadUsers(Math.max(0, userOffset - USER_PAGE_SIZE))}>
            上一页
          </button>
          <button
            className="btn btn-muted small"
            type="button"
            disabled={userOffset + USER_PAGE_SIZE >= userTotal}
            onClick={() => loadUsers(userOffset + USER_PAGE_SIZE)}
          >
            下一页
          </button>
        </div>
      </div>
    </section>
  )

  const spinsSection = (
    <section className="card admin-card">
      <h2>最新抽奖记录</h2>
      <p className="info">共 {spinTotal} 条记录</p>
      <div className="admin-table spins">
        <div className="admin-table__head">
          <span>时间</span>
          <span>用户</span>
          <span>奖项</span>
          <span>状态</span>
        </div>
        {spins.map((record) => (
          <div className="admin-table__row" key={record.id}>
            <span>{new Date(record.createdAt).toLocaleString()}</span>
            <span>
              {record.username}#{record.userId}
            </span>
            <span>{record.prize}</span>
            <span>{record.status}</span>
          </div>
        ))}
      </div>
    </section>
  )

  const inviteSection = (
    <section className="card admin-card">
      <h2>邀请码管理</h2>
      <p className="info">
        共 {inviteTotal} 条
        <button className="btn btn-muted small" type="button" onClick={() => loadInviteCodes(inviteOffset)}>
          刷新
        </button>
      </p>
      {inviteMessage && <p className="info">{inviteMessage}</p>}
      <form className="invite-actions" onSubmit={handleGenerateInvites}>
        <input
          type="number"
          min={1}
          max={10}
          value={inviteCountInput}
          onChange={(e) => setInviteCountInput(e.target.value)}
          placeholder="生成数量 (1-10)"
        />
        <button className="btn btn-primary small" type="submit">
          生成邀请码
        </button>
      </form>
      <div className="admin-table invite-codes">
        <div className="admin-table__head">
          <span>编码</span>
          <span>用户ID</span>
          <span>状态</span>
          <span>邮箱</span>
          <span>创建时间</span>
          <span>操作</span>
        </div>
          {inviteCodes.map((code) => (
            <div className="admin-table__row" key={code.id}>
              <span>{code.code}</span>
              <span>{code.userId ?? '-'}</span>
            <span>{code.used ? (code.usedEmail ? '已使用 ✓' : '已使用') : '未使用'}</span>
            <span>{code.usedEmail ?? '-'}</span>
              <span>{new Date(code.createdAt).toLocaleString()}</span>
              <span className="invite-actions__row">
                {!code.userId && !code.used && (
                  <button className="btn btn-muted small" type="button" onClick={() => handleAssignInvite(code)}>
                    绑定用户
                  </button>
                )}
                <button className="btn btn-muted small" type="button" onClick={() => setEditingInvite(code)}>
                  编辑
                </button>
              <button className="btn btn-muted small" type="button" onClick={() => handleDeleteInvite(code)}>
                删除
              </button>
            </span>
          </div>
        ))}
      </div>
      <div className="table-actions">
        <div className="table-actions__buttons">
          <button className="btn btn-muted small" type="button" disabled={inviteOffset === 0} onClick={() => loadInviteCodes(Math.max(0, inviteOffset - 20))}>
            上一页
          </button>
          <button
            className="btn btn-muted small"
            type="button"
            disabled={inviteOffset + 20 >= inviteTotal}
            onClick={() => loadInviteCodes(inviteOffset + 20)}
          >
            下一页
          </button>
        </div>
      </div>

      {editingInvite && (
        <div className="invite-editor">
          <h3>编辑邀请码 {editingInvite.code}</h3>
          <div className="form-row">
            <label>
              <span>状态</span>
              <select
                value={editingInvite.used ? 'used' : 'unused'}
                onChange={(e) =>
                  setEditingInvite((prev) =>
                    prev ? { ...prev, used: e.target.value === 'used' } : prev,
                  )
                }
              >
                <option value="used">已使用</option>
                <option value="unused">未使用</option>
              </select>
            </label>
            <label>
              <span>邮箱</span>
              <input
                type="email"
                value={editingInvite.usedEmail ?? ''}
                onChange={(e) =>
                  setEditingInvite((prev) => (prev ? { ...prev, usedEmail: e.target.value } : prev))
                }
                placeholder="user@example.com"
                disabled={!editingInvite.used}
              />
            </label>
          </div>
          <div className="env-actions">
            <button className="btn btn-muted small" type="button" onClick={() => setEditingInvite(null)}>
              取消
            </button>
            <button
              className="btn btn-primary small"
              type="button"
              onClick={async () => {
                if (!editingInvite) return
                try {
                  await Api.adminUpdateInviteCode(editingInvite.id, {
                    used: editingInvite.used,
                    usedEmail: editingInvite.used ? (editingInvite.usedEmail || undefined) : undefined,
                  })
                  setInviteMessage('邀请码已更新')
                  setEditingInvite(null)
                  loadInviteCodes(inviteOffset)
                } catch (err) {
                  setInviteMessage(err instanceof ApiError ? err.message : '更新失败')
                }
              }}
            >
              保存
            </button>
          </div>
        </div>
      )}
    </section>
  )

  const quotaSection = (
    <section className="card admin-card">
      <h2>剩余名额与定时</h2>
      <form onSubmit={handleQuotaUpdate} className="form-row">
        <input type="number" min={0} value={quotaInput} onChange={(e) => setQuotaInput(e.target.value)} placeholder="立即设置名额" required />
        <button className="btn btn-primary">立即更新</button>
      </form>
      <form onSubmit={handleSchedule} className="form-row">
        <input type="number" min={0} value={scheduleTarget} onChange={(e) => setScheduleTarget(e.target.value)} placeholder="定时名额" required />
        <input type="number" min={1} value={scheduleMinutes} onChange={(e) => setScheduleMinutes(Number(e.target.value))} placeholder="分钟后生效" />
        <button className="btn btn-muted">创建定时</button>
      </form>
      {statMessage && <p className="info">{statMessage}</p>}
    </section>
  )

  const statsSection = (
    <section className="card admin-card">
      <h2>24 小时抽奖概览</h2>
      {overview && (
        <div className="overview">
          <div>
            <span>总计</span>
            <strong>{overview.total}</strong>
          </div>
          <div>
            <span>未中奖</span>
            <strong>{overview.lose}</strong>
          </div>
          <div>
            <span>再来一次</span>
            <strong>{overview.retry}</strong>
          </div>
          <div>
            <span>中奖</span>
            <strong>{overview.win}</strong>
          </div>
        </div>
      )}
      <div className="table">
        <div className="table__head">
          <span>时间</span>
          <span>未中奖</span>
          <span>再来一次</span>
          <span>中奖</span>
        </div>
        {stats.map((row) => (
          <div className="table__row" key={row.hourLabel}>
            <span>{row.hourLabel}</span>
            <span>{row.lose}</span>
            <span>{row.retry}</span>
            <span>{row.win}</span>
          </div>
        ))}
      </div>
    </section>
  )

  const prizeSection = (
    <section className="card admin-card">
      <h2>中奖概率配置 (JSON)</h2>
      <form onSubmit={handlePrizeSave} className="prize-form">
        <textarea value={prizeConfig} onChange={(e) => setPrizeConfig(e.target.value)} rows={8} />
        {prizeMessage && <p className="info">{prizeMessage}</p>}
        <button className="btn btn-primary">保存配置</button>
      </form>
    </section>
  )

  const maintenanceSection = (
    <section className="card admin-card">
      <h2>维护工具</h2>
      <button className="btn btn-muted" onClick={handleReset}>
        重置未中奖用户抽奖次数
      </button>
      {resetMessage && <p className="info">{resetMessage}</p>}
    </section>
  )

  const teamAccountsSection = (
    <section className="card admin-card">
      <h2>🚗 车账号管理</h2>
      <p className="info">
        共 {teamAccounts.length} 个账号
        <button className="btn btn-muted small" type="button" onClick={loadTeamAccounts}>刷新</button>
        <button className="btn btn-primary small" type="button" onClick={() => setEditingAccount({ enabled: true, maxSeats: 50 })}>添加账号</button>
      </p>
      {teamAccountMessage && <p className="info">{teamAccountMessage}</p>}
      <div className="admin-table team-accounts">
        <div className="admin-table__head">
          <span>名称</span>
          <span>已用/总席位</span>
          <span>待处理</span>
          <span>到期时间</span>
          <span>状态</span>
          <span>操作</span>
        </div>
        {teamAccounts.map((acc) => (
          <div className="admin-table__row" key={acc.id}>
            <span>{acc.name}</span>
            <span>{acc.seatsInUse}/{acc.seatsEntitled}</span>
            <span>{acc.pendingInvites}</span>
            <span>{acc.activeUntil ? new Date(acc.activeUntil).toLocaleDateString() : '-'}</span>
            <span>{acc.enabled ? '启用' : '禁用'}</span>
            <span>
              <button className="btn btn-muted small" type="button" onClick={() => setEditingAccount(acc)}>编辑</button>
              <button className="btn btn-muted small" type="button" onClick={() => handleDeleteTeamAccount(acc)}>删除</button>
            </span>
          </div>
        ))}
      </div>
      {editingAccount && (
        <div className="invite-editor">
          <h3>{editingAccount.id ? '编辑车账号' : '添加车账号'}</h3>
          <form onSubmit={handleSaveTeamAccount}>
            <div className="form-row">
              <label>
                <span>名称</span>
                <input type="text" value={editingAccount.name || ''} onChange={(e) => setEditingAccount(prev => prev ? {...prev, name: e.target.value} : prev)} required />
              </label>
              <label>
                <span>Account ID</span>
                <input type="text" value={editingAccount.accountId || ''} onChange={(e) => setEditingAccount(prev => prev ? {...prev, accountId: e.target.value} : prev)} required />
              </label>
            </div>
            <div className="form-row">
              <label>
                <span>Auth Token</span>
                <input type="text" value={editingAccount.authToken || ''} onChange={(e) => setEditingAccount(prev => prev ? {...prev, authToken: e.target.value} : prev)} required />
              </label>
              <label>
                <span>最大席位</span>
                <input type="number" value={editingAccount.maxSeats || 50} onChange={(e) => setEditingAccount(prev => prev ? {...prev, maxSeats: Number(e.target.value)} : prev)} />
              </label>
            </div>
            {editingAccount.id && (
              <div className="form-row">
                <label>
                  <span>启用</span>
                  <select value={editingAccount.enabled ? 'yes' : 'no'} onChange={(e) => setEditingAccount(prev => prev ? {...prev, enabled: e.target.value === 'yes'} : prev)}>
                    <option value="yes">启用</option>
                    <option value="no">禁用</option>
                  </select>
                </label>
              </div>
            )}
            <div className="env-actions">
              <button className="btn btn-muted small" type="button" onClick={() => setEditingAccount(null)}>取消</button>
              <button className="btn btn-primary small" type="submit">保存</button>
            </div>
          </form>
        </div>
      )}
    </section>
  )

  const sections = [
    { id: 'team-accounts', label: '🚗 车账号', content: teamAccountsSection },
    { id: 'env', label: '环境变量', content: envSection },
    { id: 'users', label: '用户管理', content: userSection },
    { id: 'spins', label: '抽奖记录', content: spinsSection },
    { id: 'invites', label: '邀请码管理', content: inviteSection },
    { id: 'quota', label: '名额 / 定时', content: quotaSection },
    { id: 'stats', label: '抽奖统计', content: statsSection },
    { id: 'prize', label: '概率配置', content: prizeSection },
    { id: 'tools', label: '维护工具', content: maintenanceSection },
  ]

  const resolvedSectionId = sections.some((section) => section.id === activeSection)
    ? activeSection
    : sections[0].id
  const currentSection = sections.find((section) => section.id === resolvedSectionId)

  return (
    <main className="page admin-page">
      <header className="admin-header">
        <h1>后台管理</h1>
        <button className="btn btn-muted small" onClick={logout}>
          退出
        </button>
      </header>
      <nav className="admin-tabs">
        {sections.map((section) => (
          <button
            type="button"
            key={section.id}
            className={section.id === resolvedSectionId ? 'admin-tab active' : 'admin-tab'}
            onClick={() => setActiveSection(section.id)}
          >
            {section.label}
          </button>
        ))}
      </nav>
      <div className="admin-content">{currentSection?.content}</div>
    </main>
  )
}
type InviteCodeRow = {
  id: number
  code: string
  used: boolean
  usedEmail?: string | null
  userId: number | null
  createdAt: string
  usedAt?: string | null
}
