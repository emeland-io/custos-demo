<script setup>
import { onMounted, ref, watch } from 'vue'
import { api, ApiError } from '../lib/api'
import { useAuthStore } from '../stores/auth'
import { useCommentsStore } from '../stores/comments'
import { useNotesStore } from '../stores/notes'
import CommentList from './CommentList.vue'
import MarkdownEditor from './MarkdownEditor.vue'

const props = defineProps({ noteId: { type: Number, required: true } })
const emit = defineEmits(['updated', 'deleted'])

const auth = useAuthStore()
const comments = useCommentsStore()
const notesStore = useNotesStore()

const note = ref(null)
const notFound = ref(false)
const loadError = ref('')
const renderedHtml = ref('')
const editing = ref(false)
const editTitle = ref('')
const editContent = ref('')
const error = ref('')

let loadSeq = 0

async function load() {
  notFound.value = false
  loadError.value = ''
  note.value = null
  const seq = ++loadSeq
  let loaded
  try {
    loaded = await api.get(`/api/notes/${props.noteId}`)
  } catch (err) {
    if (seq !== loadSeq) return
    if (err instanceof ApiError && err.status === 404) {
      notFound.value = true
      return
    }
    loadError.value = err.message
    return
  }
  if (seq !== loadSeq) return
  note.value = loaded
  try {
    const rendered = await api.post('/api/render-preview', { content: note.value.content })
    if (seq !== loadSeq) return
    renderedHtml.value = rendered.html
    await comments.fetchForNote(props.noteId)
  } catch (err) {
    if (seq !== loadSeq) return
    loadError.value = err.message
  }
}

function startEdit() {
  editTitle.value = note.value.title
  editContent.value = note.value.content
  editing.value = true
  error.value = ''
}

async function saveEdit() {
  error.value = ''
  try {
    const updated = await api.put(`/api/notes/${props.noteId}`, {
      title: editTitle.value,
      content: editContent.value,
    })
    note.value = updated
    const rendered = await api.post('/api/render-preview', { content: updated.content })
    renderedHtml.value = rendered.html
    editing.value = false
    emit('updated', updated)
  } catch (err) {
    error.value = err.message
  }
}

async function remove() {
  error.value = ''
  try {
    await notesStore.remove(props.noteId)
    emit('deleted', props.noteId)
  } catch (err) {
    error.value = err.message
  }
}

onMounted(load)
watch(() => props.noteId, load)
</script>

<template>
  <div v-if="notFound" class="text-gray-500 p-4">This note was not found.</div>
  <div v-else-if="loadError" class="text-red-600 p-4">{{ loadError }}</div>
  <div v-else-if="note">
    <p v-if="error" class="text-red-600">{{ error }}</p>
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
