import { describe, expect, it, vi } from 'vitest'
import { createSessionStore } from './session'

const pair = { access_token: 'a1', refresh_token: 'r1' }

describe('session store', () => {
  it('keeps the access token in memory only and the refresh token in the given storage', () => {
    const storage = window.sessionStorage
    const s = createSessionStore(storage)
    s.set(pair)
    expect(s.getAccess()).toBe('a1')
    expect(s.getRefresh()).toBe('r1')
    expect(storage.getItem('studio-suite.refresh')).toBe('r1')
    expect(JSON.stringify({ ...storage })).not.toContain('a1') // the access token is never written
    // a "reload": a new store over the same storage has the refresh token but no access token
    const reloaded = createSessionStore(storage)
    expect(reloaded.getAccess()).toBeNull()
    expect(reloaded.getRefresh()).toBe('r1')
  })

  it('clear() forgets both and notifies subscribers until they unsubscribe', () => {
    const s = createSessionStore(window.sessionStorage)
    const listener = vi.fn()
    const off = s.subscribe(listener)
    s.set(pair)
    s.clear()
    expect(s.getAccess()).toBeNull()
    expect(s.getRefresh()).toBeNull()
    expect(listener).toHaveBeenCalledTimes(1)
    off()
    s.clear()
    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('survives a storage that throws (blocked cookies, private mode)', () => {
    const broken = {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
      removeItem: () => {
        throw new Error('blocked')
      },
    } as unknown as Storage
    const s = createSessionStore(broken)
    expect(() => s.set(pair)).not.toThrow()
    expect(s.getAccess()).toBe('a1') // works for this page load
    expect(s.getRefresh()).toBeNull()
    expect(() => s.clear()).not.toThrow()
  })

  it('works without any storage', () => {
    const s = createSessionStore(null)
    s.set(pair)
    expect(s.getAccess()).toBe('a1')
    expect(s.getRefresh()).toBeNull()
    s.clear()
  })
})
