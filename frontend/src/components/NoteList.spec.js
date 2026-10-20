import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import NoteList from './NoteList.vue'
import { useNotesStore } from '../stores/notes'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('NoteList', () => {
  it('fetches the list on mount and renders items', async () => {
    const pinia = createTestingPinia({ stubActions: false })
    const store = useNotesStore()
    store.fetchList = vi.fn().mockImplementation(async () => {
      store.items = [{ id: 1, title: 'First note', author_id: 1, updated_at: '2026-01-01' }]
      store.total = 1
    })

    // Mount with fetchList already mocked so onMounted doesn't trigger real API call
    const wrapper = mount(NoteList, { global: { plugins: [pinia] } })
    await wrapper.vm.$nextTick()
    await Promise.resolve()
    await wrapper.vm.$nextTick()

    expect(store.fetchList).toHaveBeenCalled()
    expect(wrapper.text()).toContain('First note')
  })

  it('emits select with the clicked note id', async () => {
    const pinia = createTestingPinia({ stubActions: true })
    const wrapper = mount(NoteList, { global: { plugins: [pinia] } })
    const store = useNotesStore()
    store.items = [{ id: 42, title: 'Clickable', author_id: 1, updated_at: '2026-01-01' }]
    await wrapper.vm.$nextTick()

    await wrapper.find('li').trigger('click')

    expect(wrapper.emitted('select')[0]).toEqual([42])
  })

  it('disables Previous at offset 0 and Next when all results are shown', async () => {
    const pinia = createTestingPinia({ stubActions: true })
    const wrapper = mount(NoteList, { global: { plugins: [pinia] } })
    const store = useNotesStore()
    store.items = [{ id: 1, title: 'Only note', author_id: 1, updated_at: '2026-01-01' }]
    store.total = 1
    store.offset = 0
    await wrapper.vm.$nextTick()

    const buttons = wrapper.findAll('button')
    const prev = buttons.find((b) => b.text() === 'Previous')
    const next = buttons.find((b) => b.text() === 'Next')
    expect(prev.attributes('disabled')).toBeDefined()
    expect(next.attributes('disabled')).toBeDefined()
  })

  it('renders empty state when no notes are found', async () => {
    const pinia = createTestingPinia({ stubActions: true })
    const wrapper = mount(NoteList, { global: { plugins: [pinia] } })
    const store = useNotesStore()
    store.items = []
    store.total = 0
    store.offset = 0
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('No notes found.')
    const buttons = wrapper.findAll('button')
    const next = buttons.find((b) => b.text() === 'Next')
    expect(next.attributes('disabled')).toBeDefined()
  })
})
