<script setup>
import { ref, watch } from 'vue'
import { api } from '../lib/api'
import { useAuthStore } from '../stores/auth'
import { useCommentsStore } from '../stores/comments'
import MarkdownEditor from './MarkdownEditor.vue'

const props = defineProps({ noteId: { type: Number, required: true } })
const auth = useAuthStore()
const comments = useCommentsStore()

const newContent = ref('')
const editingId = ref(null)
const editContent = ref('')
const error = ref('')
const renderedByCommentId = ref({})

function items() {
  return comments.itemsByNoteId[props.noteId] || []
}

async function renderComments(list) {
  for (const comment of list) {
    if (renderedByCommentId.value[comment.id] !== undefined) continue
    try {
      const rendered = await api.post('/api/render-preview', { content: comment.content })
      renderedByCommentId.value = { ...renderedByCommentId.value, [comment.id]: rendered.html }
    } catch (err) {
      renderedByCommentId.value = { ...renderedByCommentId.value, [comment.id]: '' }
    }
  }
}

watch(() => items(), (list) => renderComments(list), { immediate: true })

async function submit() {
  if (!newContent.value.trim()) return
  error.value = ''
  try {
    await comments.create(props.noteId, newContent.value)
    newContent.value = ''
  } catch (err) {
    error.value = err.message
  }
}

function startEdit(comment) {
  editingId.value = comment.id
  editContent.value = comment.content
  error.value = ''
}

async function saveEdit() {
  error.value = ''
  try {
    await comments.update(props.noteId, editingId.value, editContent.value)
    editingId.value = null
  } catch (err) {
    error.value = err.message
  }
}

async function remove(comment) {
  error.value = ''
  try {
    await comments.remove(props.noteId, comment.id)
    const next = { ...renderedByCommentId.value }
    delete next[comment.id]
    renderedByCommentId.value = next
  } catch (err) {
    error.value = err.message
  }
}
</script>

<template>
  <div>
    <p v-if="error" class="text-red-600">{{ error }}</p>
    <ul>
      <li v-for="comment in items()" :key="comment.id" class="border-b py-2">
        <template v-if="editingId === comment.id">
          <MarkdownEditor v-model="editContent" />
          <button type="button" @click="saveEdit">Save</button>
        </template>
        <template v-else>
          <div v-html="renderedByCommentId[comment.id]" />
          <div class="text-xs text-gray-500">{{ comment.updated_at }}</div>
          <button v-if="auth.canEdit(comment.author_id)" type="button" @click="startEdit(comment)">
            Edit
          </button>
          <button v-if="auth.canEdit(comment.author_id)" type="button" @click="remove(comment)">
            Delete
          </button>
        </template>
      </li>
    </ul>
    <MarkdownEditor v-model="newContent" />
    <button type="button" @click="submit">Comment</button>
  </div>
</template>
