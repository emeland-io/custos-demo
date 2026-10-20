import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useNotesStore } from './notes'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('useNotesStore', () => {
  it('fetchList populates items and total', async () => {
    api.get.mockResolvedValue({ items: [{ id: 1, title: 'Note' }], total: 1 })
    const store = useNotesStore()

    await store.fetchList()

    expect(store.items).toHaveLength(1)
    expect(store.total).toBe(1)
    expect(api.get).toHaveBeenCalledWith(expect.stringContaining('/api/notes?'))
  })

  it('fetchList includes search and author_id when set', async () => {
    api.get.mockResolvedValue({ items: [], total: 0 })
    const store = useNotesStore()
    store.search = 'grocery'
    store.authorId = 7

    await store.fetchList()

    const calledPath = api.get.mock.calls[0][0]
    expect(calledPath).toContain('search=grocery')
    expect(calledPath).toContain('author_id=7')
  })

  it('remove deletes and drops the note from items', async () => {
    api.delete.mockResolvedValue(null)
    const store = useNotesStore()
    store.items = [{ id: 1, title: 'A' }, { id: 2, title: 'B' }]

    await store.remove(1)

    expect(api.delete).toHaveBeenCalledWith('/api/notes/1')
    expect(store.items.map((n) => n.id)).toEqual([2])
  })

  it('create posts the payload and returns the created note', async () => {
    api.post.mockResolvedValue({ id: 3, title: 'New' })
    const store = useNotesStore()

    const created = await store.create({ title: 'New', content: 'Body' })

    expect(api.post).toHaveBeenCalledWith('/api/notes', { title: 'New', content: 'Body' })
    expect(created.id).toBe(3)
  })
})
