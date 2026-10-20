import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from './auth'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({ api: { get: vi.fn() } }))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('useAuthStore', () => {
  it('loads the current user from /api/me', async () => {
    api.get.mockResolvedValue({ id: 1, email: 'a@example.com', display_name: 'Alice', role: 'admin' })
    const store = useAuthStore()

    await store.load()

    expect(api.get).toHaveBeenCalledWith('/api/me')
    expect(store.user.display_name).toBe('Alice')
    expect(store.loaded).toBe(true)
    expect(store.isAdmin).toBe(true)
  })

  it('canEdit is true for the owner, false for others, true for admin', async () => {
    api.get.mockResolvedValue({ id: 5, email: 'u@example.com', display_name: 'User', role: 'user' })
    const store = useAuthStore()
    await store.load()

    expect(store.canEdit(5)).toBe(true)
    expect(store.canEdit(999)).toBe(false)
  })

  it('canEdit is false before load() resolves', () => {
    const store = useAuthStore()
    expect(store.canEdit(5)).toBe(false)
  })

  it('canEdit is true for admin even if not the owner', async () => {
    api.get.mockResolvedValue({ id: 10, email: 'admin@example.com', display_name: 'Admin', role: 'admin' })
    const store = useAuthStore()
    await store.load()

    expect(store.canEdit(999)).toBe(true)
  })
})
