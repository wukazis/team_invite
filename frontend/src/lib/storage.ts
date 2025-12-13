const USER_TOKEN_KEY = 'ti_user_token'
const ADMIN_TOKEN_KEY = 'ti_admin_token'

const safeLocalStorage = () => {
  if (typeof window === 'undefined') return null
  return window.localStorage
}

export const Storage = {
  getUserToken(): string | null {
    const store = safeLocalStorage()
    return store ? store.getItem(USER_TOKEN_KEY) : null
  },
  setUserToken(token: string | null) {
    const store = safeLocalStorage()
    if (!store) return
    if (token) {
      store.setItem(USER_TOKEN_KEY, token)
    } else {
      store.removeItem(USER_TOKEN_KEY)
    }
  },
  getAdminToken(): string | null {
    const store = safeLocalStorage()
    return store ? store.getItem(ADMIN_TOKEN_KEY) : null
  },
  setAdminToken(token: string | null) {
    const store = safeLocalStorage()
    if (!store) return
    if (token) {
      store.setItem(ADMIN_TOKEN_KEY, token)
    } else {
      store.removeItem(ADMIN_TOKEN_KEY)
    }
  },
}
