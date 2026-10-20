import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useCommentsStore } from './comments'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('useCommentsStore', () => {
  it('fetchForNote populates itemsByNoteId', async () => {
    api.get.mockResolvedValue({ items: [{ id: 1, content: 'Hi' }] })
    const store = useCommentsStore()

    await store.fetchForNote(10)

    expect(api.get).toHaveBeenCalledWith('/api/notes/10/comments')
    expect(store.itemsByNoteId[10]).toHaveLength(1)
  })

  it('create appends the new comment for that note', async () => {
    api.post.mockResolvedValue({ id: 2, content: 'New comment' })
    const store = useCommentsStore()
    store.itemsByNoteId[10] = []

    await store.create(10, 'New comment')

    expect(api.post).toHaveBeenCalledWith('/api/notes/10/comments', { content: 'New comment' })
    expect(store.itemsByNoteId[10]).toHaveLength(1)
  })

  it('update replaces the matching comment', async () => {
    api.put.mockResolvedValue({ id: 2, content: 'Edited' })
    const store = useCommentsStore()
    store.itemsByNoteId[10] = [{ id: 2, content: 'Original' }]

    await store.update(10, 2, 'Edited')

    expect(api.put).toHaveBeenCalledWith('/api/comments/2', { content: 'Edited' })
    expect(store.itemsByNoteId[10][0].content).toBe('Edited')
  })

  it('remove drops the matching comment', async () => {
    api.delete.mockResolvedValue(null)
    const store = useCommentsStore()
    store.itemsByNoteId[10] = [{ id: 2, content: 'Bye' }]

    await store.remove(10, 2)

    expect(api.delete).toHaveBeenCalledWith('/api/comments/2')
    expect(store.itemsByNoteId[10]).toHaveLength(0)
  })
})
