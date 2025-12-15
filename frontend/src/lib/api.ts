import { Storage } from './storage'

const API_BASE = import.meta.env.VITE_API_BASE ?? ''

export class ApiError extends Error {
  status: number
  payload: unknown

  constructor(message: string, status: number, payload: unknown) {
    super(message)
    this.status = status
    this.payload = payload
  }
}

type TokenKind = 'user' | 'admin' | null

const tokenMap: Record<Exclude<TokenKind, null>, () => string | null> = {
  user: () => Storage.getUserToken(),
  admin: () => Storage.getAdminToken(),
}

function resolveToken(kind: TokenKind) {
  if (!kind) return null
  const getter = tokenMap[kind]
  return getter ? getter() : null
}

export async function apiRequest<T>(
  path: string,
  options: RequestInit = {},
  tokenKind: TokenKind = 'user',
): Promise<T> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 15000)
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json')
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const token = resolveToken(tokenKind)
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers,
      signal: controller.signal,
      credentials: 'include',
    })
    const contentType = res.headers.get('Content-Type')
    const payload = contentType?.includes('application/json')
      ? await res.json().catch(() => ({}))
      : await res.text()
    if (!res.ok) {
      throw new ApiError((payload as any)?.error ?? res.statusText, res.status, payload)
    }
    return payload as T
  } finally {
    clearTimeout(timeout)
  }
}

export const Api = {
  login(): Promise<{ authUrl: string; state: string }> {
    return apiRequest('/api/oauth/login', { method: 'GET' }, null)
  },
  getUserState() {
    return apiRequest<{
      user: {
        id: number
        username: string
        displayName: string
        trustLevel: number
        avatarTemplate?: string
        chances: number
        inviteStatus: number
      }
      quota: number
      invite?: { code?: string; used?: boolean; usedEmail?: string; usedAt?: string; createdAt?: string }
    }>('/api/state')
  },
  getQuotaPublic() {
    return apiRequest<{
      quota: number
      updatedAt: string
      serverTime?: string
      schedule?: { applyAt: string; target: number; message?: string; author?: string; createdAt?: string } | null
    }>('/api/quota/public', { method: 'GET' }, null)
  },
  spin(spinId: string) {
    return apiRequest<{
      prize: { type: string; name: string }
      invite?: { code: string; used?: boolean; usedEmail?: string; usedAt?: string; createdAt?: string }
      quota: number
      spinStatus: string
    }>('/api/spins', {
      method: 'POST',
      body: JSON.stringify({ spinId }),
    })
  },
  submitInvite(email: string, teamAccountId: number, code?: string, turnstileToken?: string) {
    return apiRequest<{ status: string }>('/api/invite', {
      method: 'POST',
      body: JSON.stringify({ email, code, teamAccountId, turnstileToken }),
    })
  },
  getInvite() {
    return apiRequest<{ invite: { code: string; used: boolean; usedEmail?: string } | null }>('/api/invite')
  },
  resolveInviteCode(code: string) {
    return apiRequest<{ teamAccountId?: number | null; teamAccountName?: string }>(
      `/api/invite/resolve?code=${encodeURIComponent(code)}`,
    )
  },
  adminLogin(payload: { password: string; code: string }) {
    return apiRequest<{ token: string }>('/api/admin/login', {
      method: 'POST',
      body: JSON.stringify(payload),
    }, null)
  },
  adminFetchEnv() {
    return apiRequest<{ env: Record<string, string> }>('/api/admin/env', undefined, 'admin')
  },
  adminUpdateEnv(env: Record<string, string | number>) {
    return apiRequest<{ updated: number }>('/api/admin/env', {
      method: 'PUT',
      body: JSON.stringify(env),
    }, 'admin')
  },
  adminUpdateQuota(value: number) {
    return apiRequest<{ quota: number }>('/api/admin/quota', {
      method: 'POST',
      body: JSON.stringify({ value }),
    }, 'admin')
  },
  adminScheduleQuota(payload: { target: number; delayMinutes?: number; applyAt?: string; message?: string }) {
    return apiRequest<{ scheduled: unknown }>('/api/admin/quota/schedule', {
      method: 'POST',
      body: JSON.stringify(payload),
    }, 'admin')
  },
  adminResetUsers() {
    return apiRequest<{ resetChances: number }>('/api/admin/users/reset', { method: 'POST' }, 'admin')
  },
  adminStats() {
    return apiRequest<{ overview: { total: number; win: number; retry: number; lose: number; unknown: number }; stats: Array<{ hourLabel: string; win: number; retry: number; lose: number }> }>('/api/admin/stats/hourly', { method: 'GET' }, 'admin')
  },
  adminGetPrizeConfig() {
    return apiRequest<{ items: Array<{ type: string; name: string; probability: number }> }>('/api/admin/prize-config', { method: 'GET' }, 'admin')
  },
  updatePrizeConfig(payload: Array<{ type: string; name: string; probability: number }>) {
    return apiRequest<{ updated: number }>('/api/admin/prize-config', {
      method: 'PUT',
      body: JSON.stringify(payload),
    }, 'admin')
  },
  adminFetchUsers(limit = 50, offset = 0) {
    return apiRequest<{ users: Array<any>; total: number }>(`/api/admin/users?limit=${limit}&offset=${offset}`, { method: 'GET' }, 'admin')
  },
  adminUpdateUser(id: number, payload: { chances?: number; inviteStatus?: number }) {
    return apiRequest<{ status: string }>(`/api/admin/users/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }, 'admin')
  },
  adminFetchSpins(limit = 50, offset = 0) {
    return apiRequest<{ records: Array<any>; total: number }>(`/api/admin/spins?limit=${limit}&offset=${offset}`, { method: 'GET' }, 'admin')
  },
  adminFetchInviteCodes(limit = 50, offset = 0) {
    return apiRequest<{ codes: Array<any>; total: number }>(`/api/admin/invite-codes?limit=${limit}&offset=${offset}`, { method: 'GET' }, 'admin')
  },
  adminCreateInviteCodes(count: number, teamAccountId?: number) {
    return apiRequest<{ codes: Array<any> }>('/api/admin/invite-codes', {
      method: 'POST',
      body: JSON.stringify({ count, teamAccountId }),
    }, 'admin')
  },
  adminUpdateInviteCode(id: number, payload: { used: boolean; usedEmail?: string; teamAccountId?: number }) {
    return apiRequest<{ status: string }>(`/api/admin/invite-codes/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }, 'admin')
  },
  adminDeleteInviteCode(id: number) {
    return apiRequest<{ status: string }>(`/api/admin/invite-codes/${id}`, { method: 'DELETE' }, 'admin')
  },
  adminAssignInviteCode(id: number, payload: { userId: number }) {
    return apiRequest<{ status: string }>(`/api/admin/invite-codes/${id}/assign`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }, 'admin')
  },
  // Team Accounts
  getTeamAccountsStatus() {
    return apiRequest<{ accounts: TeamAccountStatus[] }>('/api/team-accounts/status')
  },
  adminListTeamAccounts() {
    return apiRequest<{ accounts: TeamAccountStatus[] }>('/api/admin/team-accounts', undefined, 'admin')
  },
  adminCreateTeamAccount(payload: { name: string; accountId: string; authToken: string; maxSeats?: number }) {
    return apiRequest<{ account: TeamAccountStatus }>('/api/admin/team-accounts', {
      method: 'POST',
      body: JSON.stringify(payload),
    }, 'admin')
  },
  adminUpdateTeamAccount(id: number, payload: { name: string; accountId: string; authToken: string; maxSeats?: number; enabled?: boolean }) {
    return apiRequest<{ status: string }>(`/api/admin/team-accounts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }, 'admin')
  },
  adminDeleteTeamAccount(id: number) {
    return apiRequest<{ status: string }>(`/api/admin/team-accounts/${id}`, { method: 'DELETE' }, 'admin')
  },
  getTurnstileSiteKey() {
    return apiRequest<{ siteKey: string }>('/api/turnstile/site-key', { method: 'GET' }, null)
  },
}

export type TeamAccountStatus = {
  id: number
  name: string
  accountId?: string
  authToken?: string
  maxSeats: number
  enabled: boolean
  createdAt: string
  seatsInUse: number
  seatsEntitled: number
  pendingInvites: number
  planType: string
  activeUntil: string
}
