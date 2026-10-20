import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import { createRouter, createWebHistory } from 'vue-router'
import NotesView from './NotesView.vue'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn().mockResolvedValue({ items: [], total: 0 }), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
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
  return mount(NotesView, { global: { plugins: [pinia, router] } })
}

describe('NotesView', () => {
  it('shows a prompt to select a note when nothing is selected', async () => {
    const wrapper = await mountAt('/notes')
    expect(wrapper.text()).toContain('Select a note')
  })

  it('shows the new-note editor on /notes/new', async () => {
    const wrapper = await mountAt('/notes/new')
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.find('input[placeholder="Title"]').exists()).toBe(true)
  })
})
