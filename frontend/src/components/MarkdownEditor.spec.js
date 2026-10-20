import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownEditor from './MarkdownEditor.vue'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({ api: { post: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('MarkdownEditor', () => {
  it('emits update:modelValue when typing in the write tab', async () => {
    const wrapper = mount(MarkdownEditor, { props: { modelValue: '' } })
    const textarea = wrapper.find('textarea')

    await textarea.setValue('# Hello')

    expect(wrapper.emitted('update:modelValue')[0]).toEqual(['# Hello'])
  })

  it('renders backend-rendered HTML when switching to the preview tab', async () => {
    api.post.mockResolvedValue({ html: '<h1>Hello</h1>' })
    const wrapper = mount(MarkdownEditor, { props: { modelValue: '# Hello' } })

    await wrapper.findAll('button').find((b) => b.text() === 'Preview').trigger('click')
    await wrapper.vm.$nextTick()
    await Promise.resolve()
    await wrapper.vm.$nextTick()

    expect(api.post).toHaveBeenCalledWith('/api/render-preview', { content: '# Hello' })
    expect(wrapper.html()).toContain('<h1>Hello</h1>')
  })

  it('shows an error message if preview rendering fails', async () => {
    api.post.mockRejectedValue(new Error('server exploded'))
    const wrapper = mount(MarkdownEditor, { props: { modelValue: 'oops' } })

    await wrapper.findAll('button').find((b) => b.text() === 'Preview').trigger('click')
    await wrapper.vm.$nextTick()
    await Promise.resolve()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('server exploded')
  })
})
