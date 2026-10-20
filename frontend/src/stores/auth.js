import { defineStore } from 'pinia'
import { api } from '../lib/api'

export const useAuthStore = defineStore('auth', {
  state: () => ({ user: null, loaded: false }),
  getters: {
    isAdmin: (state) => state.user?.role === 'admin',
  },
  actions: {
    async load() {
      this.user = await api.get('/api/me')
      this.loaded = true
    },
    canEdit(authorId) {
      return this.user != null && (this.user.id === authorId || this.user.role === 'admin')
    },
  },
})
