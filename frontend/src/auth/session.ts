import type { TokenPair } from '../api/types'

// Token storage (see docs/adr/0008-token-storage.md):
//  - the ACCESS token (15 min) lives only in memory: gone on reload, never written anywhere;
//  - the REFRESH token lives in sessionStorage: survives a reload of the tab, dies with the tab and
//    is not shared across tabs (a refresh token is single use, so two tabs sharing one would race
//    and trip the server's reuse detection, revoking every session of the user).

const REFRESH_KEY = 'studio-suite.refresh'

export interface SessionStore {
  getAccess(): string | null
  getRefresh(): string | null
  set(pair: Pick<TokenPair, 'access_token' | 'refresh_token'>): void
  clear(): void
  /** Called after `clear()` — whoever ended the session (logout, refresh failure, reuse detection). */
  subscribe(listener: () => void): () => void
}

export function createSessionStore(storage: Storage | null = safeSessionStorage()): SessionStore {
  let access: string | null = null
  const listeners = new Set<() => void>()
  return {
    getAccess: () => access,
    getRefresh() {
      try {
        return storage?.getItem(REFRESH_KEY) ?? null
      } catch {
        return null
      }
    },
    set(pair) {
      access = pair.access_token
      try {
        storage?.setItem(REFRESH_KEY, pair.refresh_token)
      } catch {
        /* storage blocked: the session just will not survive a reload */
      }
    },
    clear() {
      access = null
      try {
        storage?.removeItem(REFRESH_KEY)
      } catch {
        /* nothing to remove */
      }
      listeners.forEach((l) => l())
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
  }
}

function safeSessionStorage(): Storage | null {
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

export const session = createSessionStore()
