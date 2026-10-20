import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'

describe('App', () => {
  it('mounts without error given a ready router', async () => {
    const router = createRouter({
      history: createWebHistory(),
      routes: [{ path: '/', component: { template: '<div>home</div>' } }],
    })
    router.push('/')
    await router.isReady()

    const pinia = createTestingPinia()
    const wrapper = mount(App, { global: { plugins: [pinia, router] } })
    expect(wrapper.exists()).toBe(true)
  })
})
