import { createRouter, createWebHistory } from 'vue-router'
import NotesView from '../views/NotesView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/notes' },
    { path: '/notes', name: 'notes', component: NotesView },
    { path: '/notes/new', name: 'note-new', component: NotesView },
    { path: '/notes/:id', name: 'note-detail', component: NotesView },
  ],
})

export default router
