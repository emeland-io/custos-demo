<script setup>
import { onMounted } from 'vue'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()
onMounted(() => auth.load())
</script>

<template>
  <div class="min-h-screen bg-gray-50">
    <header class="p-4 border-b bg-white flex justify-between">
      <h1 class="font-bold text-lg">Notebook</h1>
      <span v-if="auth.user">{{ auth.user.display_name }} ({{ auth.user.role }})</span>
    </header>
    <router-view />
  </div>
</template>
