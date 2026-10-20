import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import { createRouter, createWebHistory } from 'vue-router'
import NotesView from './NotesView.vue'
import { api } from '../lib/api'
import { useAuthStore } from '../stores/auth'

vi.mock('../lib/api', () => ({
  api: {
    get: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    post: vi.fn().mockResolvedValue({ html: '<p>rendered</p>' }),
    put: vi.fn(),
    delete: vi.fn(),
  },
  ApiError: class ApiError extends Error {
    constructor(status, message) {
      super(message)
      this.status = status
    }
  },
}))

beforeEach(() => {
  vi.clearAllMocks()
})

async function mountAt(path) {
  const router = createRouter({
    history: createWebHistory(),
    routes: [
      { path: '/notes', name: 'notes', component: NotesView },
      { path: '/notes/new', name: 'note-new', component: NotesView },
      { path: '/notes/:id', name: 'note-detail', component: NotesView },
    ],
  })
  router.push(path)
  await router.isReady()

  const pinia = createTestingPinia({ stubActions: false })
  const wrapper = mount(NotesView, { global: { plugins: [pinia, router] } })
  return { wrapper, router, pinia }
}

async function flushAll(wrapper) {
  await wrapper.vm.$nextTick()
  await Promise.resolve()
  await wrapper.vm.$nextTick()
  await Promise.resolve()
  await wrapper.vm.$nextTick()
}

describe('NotesView', () => {
  it('shows a prompt to select a note when nothing is selected', async () => {
    const { wrapper } = await mountAt('/notes')
    expect(wrapper.text()).toContain('Select a note')
  })

  it('shows the new-note editor on /notes/new', async () => {
    const { wrapper } = await mountAt('/notes/new')
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.find('input[placeholder="Title"]').exists()).toBe(true)
  })

  it('resets NoteDetail edit state (and does not carry over its draft) when switching notes', async () => {
    api.get.mockImplementation((path) => {
      if (path === '/api/notes/1') {
        return Promise.resolve({ id: 1, title: 'Note One', content: 'One body', author_id: 1 })
      }
      if (path === '/api/notes/2') {
        return Promise.resolve({ id: 2, title: 'Note Two', content: 'Two body', author_id: 1 })
      }
      if (path === '/api/notes/1/comments' || path === '/api/notes/2/comments') {
        return Promise.resolve({ items: [] })
      }
      return Promise.resolve({ items: [], total: 0 })
    })

    const { wrapper, router, pinia } = await mountAt('/notes/1')
    const auth = useAuthStore(pinia)
    auth.user = { id: 1, role: 'user' }
    auth.canEdit = (authorId) => authorId === 1
    await flushAll(wrapper)

    await wrapper.findAll('button').find((b) => b.text() === 'Edit').trigger('click')
    await flushAll(wrapper)

    const editInput = wrapper.find('input.border.w-full.p-1.mb-2')
    expect(editInput.exists()).toBe(true)
    await editInput.setValue('Edited title for note one')
    expect(editInput.element.value).toBe('Edited title for note one')

    await router.push('/notes/2')
    await flushAll(wrapper)

    // Switching notes must destroy/recreate NoteDetail: no leftover edit mode or stale draft.
    expect(wrapper.find('input.border.w-full.p-1.mb-2').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Save')
    expect(wrapper.text()).toContain('Note Two')
    expect(wrapper.text()).toContain('Edit')
  })
})
