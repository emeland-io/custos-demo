<script setup>
import { ref, watch } from 'vue'
import { api } from '../lib/api'

const props = defineProps({ modelValue: { type: String, default: '' } })
const emit = defineEmits(['update:modelValue'])

const tab = ref('write')
const previewHtml = ref('')
const previewError = ref('')

let previewSeq = 0

async function renderPreview() {
  previewError.value = ''
  const seq = ++previewSeq
  try {
    const result = await api.post('/api/render-preview', { content: props.modelValue })
    if (seq !== previewSeq) return
    previewHtml.value = result.html
  } catch (err) {
    if (seq !== previewSeq) return
    previewError.value = err.message
  }
}

watch(tab, (value) => {
  if (value === 'preview') renderPreview()
})
</script>

<template>
  <div class="border rounded">
    <div class="flex border-b">
      <button
        type="button"
        class="px-4 py-2"
        :class="tab === 'write' ? 'font-semibold border-b-2 border-blue-600' : 'text-gray-500'"
        @click="tab = 'write'"
      >
        Write
      </button>
      <button
        type="button"
        class="px-4 py-2"
        :class="tab === 'preview' ? 'font-semibold border-b-2 border-blue-600' : 'text-gray-500'"
        @click="tab = 'preview'"
      >
        Preview
      </button>
    </div>
    <textarea
      v-if="tab === 'write'"
      class="w-full h-64 p-2 font-mono"
      :value="modelValue"
      @input="emit('update:modelValue', $event.target.value)"
    />
    <div v-else class="p-2 min-h-64">
      <p v-if="previewError" class="text-red-600">{{ previewError }}</p>
      <div v-else v-html="previewHtml" />
    </div>
  </div>
</template>
