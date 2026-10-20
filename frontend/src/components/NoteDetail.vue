<script setup>
import { onMounted, ref, watch } from 'vue'
import { api, ApiError } from '../lib/api'
import { useAuthStore } from '../stores/auth'
import { useCommentsStore } from '../stores/comments'
import CommentList from './CommentList.vue'
import MarkdownEditor from './MarkdownEditor.vue'

const props = defineProps({ noteId: { type: Number, required: true } })
const emit = defineEmits(['updated', 'deleted'])

const auth = useAuthStore()
const comments = useCommentsStore()

const note = ref(null)
const notFound = ref(false)
const renderedHtml = ref('')
const editing = ref(false)
const editTitle = ref('')
const editContent = ref('')

async function load() {
  notFound.value = false
  note.value = null
  try {
    note.value = await api.get(`/api/notes/${props.noteId}`)
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      notFound.value = true
      return
    }
    throw err
  }
  const rendered = await api.post('/api/render-preview', { content: note.value.content })
  renderedHtml.value = rendered.html
  await comments.fetchForNote(props.noteId)
}

function startEdit() {
  editTitle.value = note.value.title
  editContent.value = note.value.content
  editing.value = true
}

async function saveEdit() {
  const updated = await api.put(`/api/notes/${props.noteId}`, {
    title: editTitle.value,
    content: editContent.value,
  })
  note.value = updated
  const rendered = await api.post('/api/render-preview', { content: updated.content })
  renderedHtml.value = rendered.html
  editing.value = false
  emit('updated', updated)
}

async function remove() {
  await api.delete(`/api/notes/${props.noteId}`)
  emit('deleted', props.noteId)
}

onMounted(load)
watch(() => props.noteId, load)
</script>

<template>
  <div v-if="notFound" class="text-gray-500 p-4">This note was not found.</div>
  <div v-else-if="note">
    <div v-if="!editing">
      <h2 class="text-xl font-bold">{{ note.title }}</h2>
      <div v-html="renderedHtml" />
      <button v-if="auth.canEdit(note.author_id)" type="button" @click="startEdit">Edit</button>
      <button v-if="auth.canEdit(note.author_id)" type="button" @click="remove">Delete</button>
    </div>
    <div v-else>
      <input v-model="editTitle" class="border w-full p-1 mb-2" />
      <MarkdownEditor v-model="editContent" />
      <button type="button" @click="saveEdit">Save</button>
      <button type="button" @click="editing = false">Cancel</button>
    </div>
    <CommentList :note-id="noteId" />
  </div>
</template>
