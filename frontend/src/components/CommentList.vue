<script setup>
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useCommentsStore } from '../stores/comments'

const props = defineProps({ noteId: { type: Number, required: true } })
const auth = useAuthStore()
const comments = useCommentsStore()

const newContent = ref('')
const editingId = ref(null)
const editContent = ref('')

function items() {
  return comments.itemsByNoteId[props.noteId] || []
}

async function submit() {
  if (!newContent.value.trim()) return
  await comments.create(props.noteId, newContent.value)
  newContent.value = ''
}

function startEdit(comment) {
  editingId.value = comment.id
  editContent.value = comment.content
}

async function saveEdit() {
  await comments.update(props.noteId, editingId.value, editContent.value)
  editingId.value = null
}

async function remove(comment) {
  await comments.remove(props.noteId, comment.id)
}
</script>

<template>
  <div>
    <ul>
      <li v-for="comment in items()" :key="comment.id" class="border-b py-2">
        <template v-if="editingId === comment.id">
          <textarea v-model="editContent" class="w-full border p-1" />
          <button type="button" @click="saveEdit">Save</button>
        </template>
        <template v-else>
          <p>{{ comment.content }}</p>
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
    <textarea v-model="newContent" class="w-full border p-1 mt-2" placeholder="Add a comment..." />
    <button type="button" @click="submit">Comment</button>
  </div>
</template>
