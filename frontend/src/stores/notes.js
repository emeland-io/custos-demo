import { defineStore } from 'pinia'
import { api } from '../lib/api'

export const useNotesStore = defineStore('notes', {
  state: () => ({
    items: [],
    total: 0,
    search: '',
    authorId: null,
    offset: 0,
    limit: 20,
  }),
  actions: {
    async fetchList() {
      const params = new URLSearchParams()
      if (this.search) params.set('search', this.search)
      if (this.authorId) params.set('author_id', String(this.authorId))
      params.set('offset', String(this.offset))
      params.set('limit', String(this.limit))

      const result = await api.get(`/api/notes?${params.toString()}`)
      this.items = result.items || []
      this.total = result.total || 0
    },
    create(payload) {
      return api.post('/api/notes', payload)
    },
    update(id, payload) {
      return api.put(`/api/notes/${id}`, payload)
    },
    async remove(id) {
      await api.delete(`/api/notes/${id}`)
      this.items = this.items.filter((n) => n.id !== id)
    },
  },
})
