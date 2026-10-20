<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import NoteList from '../components/NoteList.vue'
import NoteDetail from '../components/NoteDetail.vue'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import { useNotesStore } from '../stores/notes'

const route = useRoute()
const router = useRouter()
const notes = useNotesStore()

const selectedId = computed(() => (route.params.id ? Number(route.params.id) : null))
const isNew = computed(() => route.name === 'note-new')

const newTitle = ref('')
const newContent = ref('')

function select(id) {
  router.push({ name: 'note-detail', params: { id } })
}

async function createNote() {
  const created = await notes.create({ title: newTitle.value, content: newContent.value })
  newTitle.value = ''
  newContent.value = ''
  router.push({ name: 'note-detail', params: { id: created.id } })
}

function onDeleted() {
  router.push({ name: 'notes' })
}
</script>

<template>
  <div class="grid grid-cols-3 gap-4 p-4">
    <div class="col-span-1">
      <button
        type="button"
        class="mb-2 px-2 py-1 border rounded"
        @click="router.push({ name: 'note-new' })"
      >
        New note
      </button>
      <NoteList @select="select" />
    </div>
    <div class="col-span-2">
      <div v-if="isNew">
        <input v-model="newTitle" class="border w-full p-1 mb-2" placeholder="Title" />
        <MarkdownEditor v-model="newContent" />
        <button type="button" class="mt-2 px-2 py-1 border rounded" @click="createNote">
          Create
        </button>
      </div>
      <NoteDetail v-else-if="selectedId" :note-id="selectedId" @deleted="onDeleted" />
      <p v-else class="text-gray-500">Select a note from the list.</p>
    </div>
  </div>
</template>
