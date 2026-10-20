import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import NoteDetail from './NoteDetail.vue'
import { useAuthStore } from '../stores/auth'
import { useCommentsStore } from '../stores/comments'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

beforeEach(() => {
  vi.clearAllMocks()
})

function mountDetail(noteId = 1) {
  const pinia = createTestingPinia({ stubActions: false })
  const wrapper = mount(NoteDetail, { props: { noteId }, global: { plugins: [pinia] } })
  return { wrapper, auth: useAuthStore(), comments: useCommentsStore() }
}

describe('NoteDetail', () => {
  it('loads the note, renders it, and fetches comments', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: '# Hi', author_id: 9 })
    api.post.mockResolvedValue({ html: '<h1>Hi</h1>' })
    const { wrapper, comments } = mountDetail(1)
    comments.fetchForNote = vi.fn().mockResolvedValue(undefined)

    await flushAll(wrapper)

    expect(api.get).toHaveBeenCalledWith('/api/notes/1')
    expect(api.post).toHaveBeenCalledWith('/api/render-preview', { content: '# Hi' })
    expect(wrapper.html()).toContain('<h1>Hi</h1>')
  })

  it('shows Edit/Delete only when the current user can edit', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: 'Body', author_id: 9 })
    api.post.mockResolvedValue({ html: '<p>Body</p>' })
    const { wrapper, auth } = mountDetail(1)
    auth.user = { id: 9, role: 'user' }
    auth.canEdit = (authorId) => authorId === 9

    await flushAll(wrapper)

    expect(wrapper.text()).toContain('Edit')
    expect(wrapper.text()).toContain('Delete')
  })

  it('hides Edit/Delete for a non-owner, non-admin viewer', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: 'Body', author_id: 9 })
    api.post.mockResolvedValue({ html: '<p>Body</p>' })
    const { wrapper, auth } = mountDetail(1)
    auth.user = { id: 123, role: 'user' }
    auth.canEdit = (authorId) => authorId === 123

    await flushAll(wrapper)

    expect(wrapper.text()).not.toContain('Edit')
    expect(wrapper.text()).not.toContain('Delete')
  })

  it('emits deleted after a successful delete', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: 'Body', author_id: 9 })
    api.post.mockResolvedValue({ html: '<p>Body</p>' })
    api.delete.mockResolvedValue(null)
    const { wrapper, auth } = mountDetail(1)
    auth.user = { id: 9, role: 'user' }
    auth.canEdit = () => true

    await flushAll(wrapper)
    await wrapper.findAll('button').find((b) => b.text() === 'Delete').trigger('click')
    await flushAll(wrapper)

    expect(api.delete).toHaveBeenCalledWith('/api/notes/1')
    expect(wrapper.emitted('deleted')[0]).toEqual([1])
  })
})

async function flushAll(wrapper) {
  await wrapper.vm.$nextTick()
  await Promise.resolve()
  await wrapper.vm.$nextTick()
  await Promise.resolve()
  await wrapper.vm.$nextTick()
}
