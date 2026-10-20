import { defineStore } from 'pinia'
import { api } from '../lib/api'

export const useCommentsStore = defineStore('comments', {
  state: () => ({ itemsByNoteId: {} }),
  actions: {
    async fetchForNote(noteId) {
      const result = await api.get(`/api/notes/${noteId}/comments`)
      this.itemsByNoteId[noteId] = result.items || []
    },
    async create(noteId, content) {
      const comment = await api.post(`/api/notes/${noteId}/comments`, { content })
      this.itemsByNoteId[noteId] = [...(this.itemsByNoteId[noteId] || []), comment]
      return comment
    },
    async update(noteId, commentId, content) {
      const updated = await api.put(`/api/comments/${commentId}`, { content })
      this.itemsByNoteId[noteId] = (this.itemsByNoteId[noteId] || []).map((c) =>
        c.id === commentId ? updated : c
      )
      return updated
    },
    async remove(noteId, commentId) {
      await api.delete(`/api/comments/${commentId}`)
      this.itemsByNoteId[noteId] = (this.itemsByNoteId[noteId] || []).filter(
        (c) => c.id !== commentId
      )
    },
  },
})
