<script setup>
import { onMounted } from 'vue'
import { useNotesStore } from '../stores/notes'

const emit = defineEmits(['select'])
const notes = useNotesStore()

onMounted(() => notes.fetchList())

function onSearchInput(event) {
  notes.search = event.target.value
  notes.offset = 0
  notes.fetchList()
}

function nextPage() {
  notes.offset += notes.limit
  notes.fetchList()
}

function prevPage() {
  notes.offset = Math.max(0, notes.offset - notes.limit)
  notes.fetchList()
}
</script>

<template>
  <div>
    <input
      type="text"
      placeholder="Search notes..."
      class="border px-2 py-1 w-full mb-2"
      @input="onSearchInput"
    />
    <ul>
      <li
        v-for="note in notes.items"
        :key="note.id"
        class="cursor-pointer p-2 border-b hover:bg-gray-100"
        @click="emit('select', note.id)"
      >
        <div class="font-semibold">{{ note.title }}</div>
        <div class="text-sm text-gray-500">updated {{ note.updated_at }}</div>
      </li>
    </ul>
    <p v-if="notes.items.length === 0" class="text-gray-500 p-2">No notes found.</p>
    <div class="flex justify-between mt-2">
      <button type="button" :disabled="notes.offset === 0" @click="prevPage">Previous</button>
      <button
        type="button"
        :disabled="notes.offset + notes.limit >= notes.total"
        @click="nextPage"
      >
        Next
      </button>
    </div>
  </div>
</template>
