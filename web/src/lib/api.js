const DEFAULT_SERVER_PORT = '8080'
const AUTH_TOKEN_KEY = 'remote_agent_auth_token'

export function getAuthToken() {
  if (typeof window === 'undefined') return ''
  return localStorage.getItem(AUTH_TOKEN_KEY) || ''
}

export function setAuthToken(token) {
  if (typeof window === 'undefined') return
  if (token) localStorage.setItem(AUTH_TOKEN_KEY, token)
  else localStorage.removeItem(AUTH_TOKEN_KEY)
}

export function clearAuthToken() {
  setAuthToken('')
}

export function getApiBaseUrl() {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL.replace(/\/$/, '')
  }

  if (typeof window === 'undefined') {
    return `http://localhost:${DEFAULT_SERVER_PORT}`
  }

  if (window.location.port === '3000' || window.location.port === '5173') {
    return `http://${window.location.hostname}:${DEFAULT_SERVER_PORT}`
  }

  return window.location.origin
}

export function apiUrl(path) {
  const base = getApiBaseUrl()
  return `${base}${path.startsWith('/') ? path : `/${path}`}`
}

export async function apiFetch(path, options = {}) {
  const headers = new Headers(options.headers || {})
  const token = getAuthToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (options.body && !headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  return fetch(apiUrl(path), {
    ...options,
    headers,
  })
}
