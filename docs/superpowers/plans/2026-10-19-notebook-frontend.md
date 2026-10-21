# Notebook App — Vue Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Vue 3 SPA: a note list pane with search/filter/
pagination, a note detail pane with rendered Markdown and a flat
comment thread, a GitHub-style write/preview Markdown editor shared
between note creation/editing and comment entry, and session-aware API
access that redirects to the backend's OIDC login on a 401.

**Architecture:** `src/lib/api.js` is the single fetch wrapper every
store goes through (handles `credentials: 'include'` and 401
redirects). Pinia stores (`auth`, `notes`, `comments`) hold all server
state; components are thin and read/write only through stores or,
where a one-off call doesn't warrant store state (fetching a single
note, rendering a preview), through `api` directly. Ownership/role
gating in the UI uses `useAuthStore().canEdit(authorId)`, mirroring the
backend's `isOwnerOrAdmin` check from Plan 1 — the backend is still the
authority; the frontend check only decides whether to show the
button.

**Tech Stack:** Vue 3 (Composition API, `<script setup>`), Vite, Pinia,
vue-router 4, Tailwind CSS, Vitest + `@vue/test-utils` for component
tests.

**Spec:** `docs/superpowers/specs/2026-10-19-notebook-app.md`

## Handoff Note (read before starting)

This plan assumes Plans 1 and 2
(`docs/superpowers/plans/2026-10-19-notebook-backend-foundations.md`,
`docs/superpowers/plans/2026-10-19-notebook-oidc-auth.md`) were
implemented exactly as written: the JSON shapes returned by
`GET/POST /api/notes`, `GET/PUT/DELETE /api/notes/{id}`,
`GET/POST /api/notes/{noteId}/comments`, `PUT/DELETE /api/comments/{id}`,
`POST /api/render-preview`, and that unauthenticated `/api/*` requests
return 401 JSON while `GET /auth/login` performs the OIDC redirect.
Task 1 below also adds a `GET /api/me` endpoint that neither Plan 1 nor
Plan 2 included — it was discovered as a gap only once the frontend
needed a way to know the current user's identity/role for UI gating.
**Before this plan's second task starts, the reviewer who signed off on
Plan 2's final whole-branch review must confirm Task 1's addition
doesn't conflict with anything Plan 2 actually built** (route naming,
response envelope conventions), and more generally re-read this whole
plan against the real Plan 1/2 codebase and fix any drift before
proceeding.

## Global Constraints

- All frontend HTTP calls go through `src/lib/api.js` — no component or
  store calls `fetch` directly.
- The SPA never stores or reads the session cookie itself (it's
  HttpOnly); the only signal of auth state is a successful vs. 401
  response from the API.
- Markdown is always rendered by the backend's `/api/render-preview`
  endpoint — no client-side Markdown parser is added, so stored and
  previewed output can never drift from each other.
- Package manager is npm (`package-lock.json` committed), matching the
  CI approach already decided for the backend.
- Component tests use `@vue/test-utils` with a real (not shallow)
  mount wherever feasible, and mock only `src/lib/api.js` — never the
  stores being tested.

## Review Focus

- `GET /api/me` called with no session (401) — `authStore.load()` must
  leave `user: null` without throwing an unhandled error that breaks
  app boot.
- A note list search that returns zero results — `NoteList` must render
  an empty list, not a broken pagination control (e.g. "Next" disabled
  state must not divide by zero or show negative ranges).
- Switching the editor to the Preview tab with empty content — must
  show empty rendered output, not an error, and must not call the API
  with a stale previous value.
- A comment/note edit attempted by a user who is not the owner and not
  admin — the Edit/Delete buttons must not render at all (the backend
  already 403s, but the UI must not even offer the action).
- Navigating directly to `/notes/:id` for a note that was deleted (404
  from the backend) — `NoteDetail` must show a clear "not found" state,
  not an indefinite blank pane or an unhandled promise rejection.

---

## File Structure

```
internal/api/me_handler.go                  (backend addition)
frontend/package.json
frontend/vite.config.js
frontend/tailwind.config.js
frontend/postcss.config.js
frontend/index.html
frontend/src/main.js
frontend/src/index.css
frontend/src/App.vue
frontend/src/App.spec.js
frontend/src/router/index.js
frontend/src/lib/api.js
frontend/src/lib/api.spec.js
frontend/src/stores/auth.js
frontend/src/stores/auth.spec.js
frontend/src/stores/notes.js
frontend/src/stores/notes.spec.js
frontend/src/stores/comments.js
frontend/src/stores/comments.spec.js
frontend/src/components/MarkdownEditor.vue
frontend/src/components/MarkdownEditor.spec.js
frontend/src/components/NoteList.vue
frontend/src/components/NoteList.spec.js
frontend/src/components/CommentList.vue
frontend/src/components/NoteDetail.vue
frontend/src/components/NoteDetail.spec.js
frontend/src/views/NotesView.vue
frontend/src/views/NotesView.spec.js
```

---

### Task 1: Backend `GET /api/me` endpoint

**Files:**
- Create: `internal/api/me_handler.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/me_handler_test.go`

**Interfaces:**
- Consumes: `CurrentUser` (Plan 1), `StubAuthMiddleware` (Plan 1, test-only).
- Produces: `api.MeHandler(w http.ResponseWriter, r *http.Request)`, mounted at `GET /api/me` under the session-protected subrouter; response body `{"id": int64, "email": string, "display_name": string, "role": string}`.

- [ ] **Step 1: Write the failing test**

```go
// internal/api/me_handler_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"notebook/internal/models"
)

func TestMeHandler_ReturnsCurrentUser(t *testing.T) {
	user := &models.User{ID: 7, Email: "a@example.com", DisplayName: "Alice", Role: models.RoleAdmin}

	router := mux.NewRouter()
	router.Use(StubAuthMiddleware(user))
	router.HandleFunc("/api/me", MeHandler).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"id":7`, `"email":"a@example.com"`, `"display_name":"Alice"`, `"role":"admin"`} {
		if !contains(body, want) {
			t.Fatalf("expected body to contain %q, got %s", want, body)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestMeHandler -v`
Expected: FAIL — `MeHandler` undefined.

- [ ] **Step 3: Implement the handler and wire the route**

```go
// internal/api/me_handler.go
package api

import "net/http"

func MeHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r.Context())
	if !ok || user == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
		"role":         string(user.Role),
	})
}
```

In `internal/api/router.go`, add inside the `/api` subrouter block
(after `api.Use(auth.SessionMiddleware)`):

```go
api.HandleFunc("/me", MeHandler).Methods(http.MethodGet)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/... -run TestMeHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/me_handler.go internal/api/me_handler_test.go internal/api/router.go
git commit -m "feat: add GET /api/me endpoint for frontend session identity"
```

---

### Task 2: Frontend scaffold (Vite + Vue 3 + Tailwind + Vitest)

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/vite.config.js`
- Create: `frontend/tailwind.config.js`
- Create: `frontend/postcss.config.js`
- Create: `frontend/index.html`
- Create: `frontend/src/main.js`
- Create: `frontend/src/index.css`
- Create: `frontend/src/App.vue`
- Create: `frontend/src/router/index.js`
- Create: `frontend/src/views/NotesView.vue` (placeholder content, filled out in Task 9)
- Test: `frontend/src/App.spec.js`

**Interfaces:**
- Produces: a working `npm run build`, `npm run test`, and `npm run dev` in `frontend/`; `router` default export from `src/router/index.js` with routes `notes`, `note-new`, `note-detail`.

- [ ] **Step 1: Create `package.json`**

```json
{
  "name": "notebook-frontend",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview",
    "test": "vitest run"
  },
  "dependencies": {
    "pinia": "^2.2.4",
    "vue": "^3.5.12",
    "vue-router": "^4.4.5"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.1.4",
    "@vue/test-utils": "^2.4.6",
    "autoprefixer": "^10.4.20",
    "jsdom": "^25.0.1",
    "postcss": "^8.4.47",
    "tailwindcss": "^3.4.14",
    "vite": "^5.4.10",
    "vitest": "^2.1.4"
  }
}
```

- [ ] **Step 2: Install dependencies**

```bash
cd frontend && npm install
```

- [ ] **Step 3: Add build/test configuration**

```js
// frontend/vite.config.js
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    globals: true,
  },
})
```

```js
// frontend/tailwind.config.js
export default {
  content: ['./index.html', './src/**/*.{vue,js}'],
  theme: { extend: {} },
  plugins: [],
}
```

```js
// frontend/postcss.config.js
export default {
  plugins: { tailwindcss: {}, autoprefixer: {} },
}
```

```html
<!-- frontend/index.html -->
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <title>Notebook</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.js"></script>
  </body>
</html>
```

```css
/* frontend/src/index.css */
@tailwind base;
@tailwind components;
@tailwind utilities;
```

- [ ] **Step 4: Write the app shell, router, and entry point**

```js
// frontend/src/router/index.js
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
```

```vue
<!-- frontend/src/views/NotesView.vue — placeholder, replaced in Task 9 -->
<template>
  <div class="p-4">Notes view placeholder.</div>
</template>
```

```js
// frontend/src/main.js
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './index.css'

createApp(App).use(createPinia()).use(router).mount('#app')
```

```vue
<!-- frontend/src/App.vue — extended in Task 9 to load the auth store -->
<script setup></script>

<template>
  <div class="min-h-screen bg-gray-50">
    <router-view />
  </div>
</template>
```

- [ ] **Step 5: Write the failing smoke test**

```js
// frontend/src/App.spec.js
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'

describe('App', () => {
  it('mounts without error given a ready router', async () => {
    const router = createRouter({
      history: createWebHistory(),
      routes: [{ path: '/', component: { template: '<div>home</div>' } }],
    })
    router.push('/')
    await router.isReady()

    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.exists()).toBe(true)
  })
})
```

- [ ] **Step 6: Run the test, then the build, to verify both succeed**

Run: `cd frontend && npm run test`
Expected: PASS

Run: `cd frontend && npm run build`
Expected: succeeds, producing `frontend/dist/`.

- [ ] **Step 7: Commit**

```bash
git add frontend
git commit -m "feat: scaffold Vite/Vue3/Tailwind/Pinia/vue-router frontend"
```

---

### Task 3: API client module

**Files:**
- Create: `frontend/src/lib/api.js`
- Test: `frontend/src/lib/api.spec.js`

**Interfaces:**
- Produces: `export class ApiError extends Error` (has `.status`); `export async function apiFetch(path, options)`; `export const api = { get, post, put, delete }`.

- [ ] **Step 1: Write the failing tests**

```js
// frontend/src/lib/api.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiFetch, ApiError } from './api'

function mockFetchOnce(body, init = {}) {
  global.fetch = vi.fn().mockResolvedValue(
    new Response(body === undefined ? null : JSON.stringify(body), init)
  )
}

describe('apiFetch', () => {
  beforeEach(() => {
    delete window.location
    window.location = { assign: vi.fn() }
  })

  it('returns parsed JSON on success', async () => {
    mockFetchOnce({ ok: true }, { status: 200 })
    const result = await apiFetch('/api/notes')
    expect(result).toEqual({ ok: true })
  })

  it('returns null for 204 responses', async () => {
    mockFetchOnce(undefined, { status: 204 })
    const result = await apiFetch('/api/notes/1')
    expect(result).toBeNull()
  })

  it('redirects to /auth/login and throws on 401', async () => {
    mockFetchOnce({ error: 'authentication required' }, { status: 401 })
    await expect(apiFetch('/api/notes')).rejects.toThrow(ApiError)
    expect(window.location.assign).toHaveBeenCalledWith('/auth/login')
  })

  it('throws an ApiError carrying the server message on a non-ok response', async () => {
    mockFetchOnce({ error: 'title and content are required' }, { status: 400 })
    await expect(apiFetch('/api/notes', { method: 'POST' })).rejects.toThrow(
      'title and content are required'
    )
  })

  it('sends credentials and JSON content-type by default', async () => {
    mockFetchOnce({ ok: true }, { status: 200 })
    await apiFetch('/api/notes')
    const [, options] = global.fetch.mock.calls[0]
    expect(options.credentials).toBe('include')
    expect(options.headers['Content-Type']).toBe('application/json')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/lib/api.spec.js`
Expected: FAIL — module `./api` does not exist.

- [ ] **Step 3: Implement the API client**

```js
// frontend/src/lib/api.js
export class ApiError extends Error {
  constructor(status, message) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export async function apiFetch(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
  })

  if (response.status === 401) {
    window.location.assign('/auth/login')
    throw new ApiError(401, 'authentication required')
  }

  if (!response.ok) {
    let message = `request failed with status ${response.status}`
    try {
      const body = await response.json()
      if (body && body.error) message = body.error
    } catch {
      // response had no JSON body; keep the default message
    }
    throw new ApiError(response.status, message)
  }

  if (response.status === 204) return null
  return response.json()
}

export const api = {
  get: (path) => apiFetch(path),
  post: (path, body) => apiFetch(path, { method: 'POST', body: JSON.stringify(body) }),
  put: (path, body) => apiFetch(path, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (path) => apiFetch(path, { method: 'DELETE' }),
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/lib/api.spec.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/api.js frontend/src/lib/api.spec.js
git commit -m "feat: add API client with credentialed fetch and 401 redirect handling"
```

---

### Task 4: Auth store and Notes store

**Files:**
- Create: `frontend/src/stores/auth.js`
- Create: `frontend/src/stores/notes.js`
- Test: `frontend/src/stores/auth.spec.js`
- Test: `frontend/src/stores/notes.spec.js`

**Interfaces:**
- Consumes: `api` (Task 3).
- Produces: `useAuthStore()` with state `{user, loaded}`, getter `isAdmin`, actions `load()`, `canEdit(authorId)`; `useNotesStore()` with state `{items, total, search, authorId, offset, limit}`, actions `fetchList()`, `create(payload)`, `update(id, payload)`, `remove(id)`.

- [ ] **Step 1: Write the failing auth store test**

```js
// frontend/src/stores/auth.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from './auth'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({ api: { get: vi.fn() } }))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('useAuthStore', () => {
  it('loads the current user from /api/me', async () => {
    api.get.mockResolvedValue({ id: 1, email: 'a@example.com', display_name: 'Alice', role: 'admin' })
    const store = useAuthStore()

    await store.load()

    expect(api.get).toHaveBeenCalledWith('/api/me')
    expect(store.user.display_name).toBe('Alice')
    expect(store.loaded).toBe(true)
    expect(store.isAdmin).toBe(true)
  })

  it('canEdit is true for the owner, false for others, true for admin', async () => {
    api.get.mockResolvedValue({ id: 5, email: 'u@example.com', display_name: 'User', role: 'user' })
    const store = useAuthStore()
    await store.load()

    expect(store.canEdit(5)).toBe(true)
    expect(store.canEdit(999)).toBe(false)
  })

  it('canEdit is false before load() resolves', () => {
    const store = useAuthStore()
    expect(store.canEdit(5)).toBe(false)
  })
})
```

- [ ] **Step 2: Write the failing notes store test**

```js
// frontend/src/stores/notes.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useNotesStore } from './notes'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('useNotesStore', () => {
  it('fetchList populates items and total', async () => {
    api.get.mockResolvedValue({ items: [{ id: 1, title: 'Note' }], total: 1 })
    const store = useNotesStore()

    await store.fetchList()

    expect(store.items).toHaveLength(1)
    expect(store.total).toBe(1)
    expect(api.get).toHaveBeenCalledWith(expect.stringContaining('/api/notes?'))
  })

  it('fetchList includes search and author_id when set', async () => {
    api.get.mockResolvedValue({ items: [], total: 0 })
    const store = useNotesStore()
    store.search = 'grocery'
    store.authorId = 7

    await store.fetchList()

    const calledPath = api.get.mock.calls[0][0]
    expect(calledPath).toContain('search=grocery')
    expect(calledPath).toContain('author_id=7')
  })

  it('remove deletes and drops the note from items', async () => {
    api.delete.mockResolvedValue(null)
    const store = useNotesStore()
    store.items = [{ id: 1, title: 'A' }, { id: 2, title: 'B' }]

    await store.remove(1)

    expect(api.delete).toHaveBeenCalledWith('/api/notes/1')
    expect(store.items.map((n) => n.id)).toEqual([2])
  })

  it('create posts the payload and returns the created note', async () => {
    api.post.mockResolvedValue({ id: 3, title: 'New' })
    const store = useNotesStore()

    const created = await store.create({ title: 'New', content: 'Body' })

    expect(api.post).toHaveBeenCalledWith('/api/notes', { title: 'New', content: 'Body' })
    expect(created.id).toBe(3)
  })
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd frontend && npx vitest run src/stores/auth.spec.js src/stores/notes.spec.js`
Expected: FAIL — modules `./auth`, `./notes` do not exist.

- [ ] **Step 4: Implement the auth store**

```js
// frontend/src/stores/auth.js
import { defineStore } from 'pinia'
import { api } from '../lib/api'

export const useAuthStore = defineStore('auth', {
  state: () => ({ user: null, loaded: false }),
  getters: {
    isAdmin: (state) => state.user?.role === 'admin',
  },
  actions: {
    async load() {
      this.user = await api.get('/api/me')
      this.loaded = true
    },
    canEdit(authorId) {
      return this.user != null && (this.user.id === authorId || this.user.role === 'admin')
    },
  },
})
```

- [ ] **Step 5: Implement the notes store**

```js
// frontend/src/stores/notes.js
import { defineStore } from 'pinia'
import { api } from '../lib/api'

export const useNotesStore = defineStore('notes', {
  state: () => ({
    items: [],
    total: 0,
    search: '',
    authorId: null,
    offset: 0,
    limit: 20,
  }),
  actions: {
    async fetchList() {
      const params = new URLSearchParams()
      if (this.search) params.set('search', this.search)
      if (this.authorId) params.set('author_id', String(this.authorId))
      params.set('offset', String(this.offset))
      params.set('limit', String(this.limit))

      const result = await api.get(`/api/notes?${params.toString()}`)
      this.items = result.items || []
      this.total = result.total || 0
    },
    create(payload) {
      return api.post('/api/notes', payload)
    },
    update(id, payload) {
      return api.put(`/api/notes/${id}`, payload)
    },
    async remove(id) {
      await api.delete(`/api/notes/${id}`)
      this.items = this.items.filter((n) => n.id !== id)
    },
  },
})
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd frontend && npx vitest run src/stores/auth.spec.js src/stores/notes.spec.js`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add frontend/src/stores/auth.js frontend/src/stores/auth.spec.js frontend/src/stores/notes.js frontend/src/stores/notes.spec.js
git commit -m "feat: add auth and notes Pinia stores"
```

---

### Task 5: Comments store

**Files:**
- Create: `frontend/src/stores/comments.js`
- Test: `frontend/src/stores/comments.spec.js`

**Interfaces:**
- Consumes: `api` (Task 3).
- Produces: `useCommentsStore()` with state `{itemsByNoteId}`, actions `fetchForNote(noteId)`, `create(noteId, content)`, `update(noteId, commentId, content)`, `remove(noteId, commentId)`.

- [ ] **Step 1: Write the failing test**

```js
// frontend/src/stores/comments.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useCommentsStore } from './comments'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('useCommentsStore', () => {
  it('fetchForNote populates itemsByNoteId', async () => {
    api.get.mockResolvedValue({ items: [{ id: 1, content: 'Hi' }] })
    const store = useCommentsStore()

    await store.fetchForNote(10)

    expect(api.get).toHaveBeenCalledWith('/api/notes/10/comments')
    expect(store.itemsByNoteId[10]).toHaveLength(1)
  })

  it('create appends the new comment for that note', async () => {
    api.post.mockResolvedValue({ id: 2, content: 'New comment' })
    const store = useCommentsStore()
    store.itemsByNoteId[10] = []

    await store.create(10, 'New comment')

    expect(api.post).toHaveBeenCalledWith('/api/notes/10/comments', { content: 'New comment' })
    expect(store.itemsByNoteId[10]).toHaveLength(1)
  })

  it('update replaces the matching comment', async () => {
    api.put.mockResolvedValue({ id: 2, content: 'Edited' })
    const store = useCommentsStore()
    store.itemsByNoteId[10] = [{ id: 2, content: 'Original' }]

    await store.update(10, 2, 'Edited')

    expect(api.put).toHaveBeenCalledWith('/api/comments/2', { content: 'Edited' })
    expect(store.itemsByNoteId[10][0].content).toBe('Edited')
  })

  it('remove drops the matching comment', async () => {
    api.delete.mockResolvedValue(null)
    const store = useCommentsStore()
    store.itemsByNoteId[10] = [{ id: 2, content: 'Bye' }]

    await store.remove(10, 2)

    expect(api.delete).toHaveBeenCalledWith('/api/comments/2')
    expect(store.itemsByNoteId[10]).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/stores/comments.spec.js`
Expected: FAIL — module `./comments` does not exist.

- [ ] **Step 3: Implement the comments store**

```js
// frontend/src/stores/comments.js
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/stores/comments.spec.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/stores/comments.js frontend/src/stores/comments.spec.js
git commit -m "feat: add comments Pinia store"
```

---

### Task 6: Markdown editor (write/preview tabs)

**Files:**
- Create: `frontend/src/components/MarkdownEditor.vue`
- Test: `frontend/src/components/MarkdownEditor.spec.js`

**Interfaces:**
- Consumes: `api` (Task 3).
- Produces: `MarkdownEditor` component, props `{modelValue: String}`, emits `update:modelValue`.

- [ ] **Step 1: Write the failing test**

```js
// frontend/src/components/MarkdownEditor.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownEditor from './MarkdownEditor.vue'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({ api: { post: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('MarkdownEditor', () => {
  it('emits update:modelValue when typing in the write tab', async () => {
    const wrapper = mount(MarkdownEditor, { props: { modelValue: '' } })
    const textarea = wrapper.find('textarea')

    await textarea.setValue('# Hello')

    expect(wrapper.emitted('update:modelValue')[0]).toEqual(['# Hello'])
  })

  it('renders backend-rendered HTML when switching to the preview tab', async () => {
    api.post.mockResolvedValue({ html: '<h1>Hello</h1>' })
    const wrapper = mount(MarkdownEditor, { props: { modelValue: '# Hello' } })

    await wrapper.findAll('button').find((b) => b.text() === 'Preview').trigger('click')
    await wrapper.vm.$nextTick()
    await Promise.resolve()
    await wrapper.vm.$nextTick()

    expect(api.post).toHaveBeenCalledWith('/api/render-preview', { content: '# Hello' })
    expect(wrapper.html()).toContain('<h1>Hello</h1>')
  })

  it('shows an error message if preview rendering fails', async () => {
    api.post.mockRejectedValue(new Error('server exploded'))
    const wrapper = mount(MarkdownEditor, { props: { modelValue: 'oops' } })

    await wrapper.findAll('button').find((b) => b.text() === 'Preview').trigger('click')
    await wrapper.vm.$nextTick()
    await Promise.resolve()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('server exploded')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/components/MarkdownEditor.spec.js`
Expected: FAIL — component file does not exist.

- [ ] **Step 3: Implement the component**

```vue
<!-- frontend/src/components/MarkdownEditor.vue -->
<script setup>
import { ref, watch } from 'vue'
import { api } from '../lib/api'

const props = defineProps({ modelValue: { type: String, default: '' } })
const emit = defineEmits(['update:modelValue'])

const tab = ref('write')
const previewHtml = ref('')
const previewError = ref('')

async function renderPreview() {
  previewError.value = ''
  try {
    const result = await api.post('/api/render-preview', { content: props.modelValue })
    previewHtml.value = result.html
  } catch (err) {
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/components/MarkdownEditor.spec.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/MarkdownEditor.vue frontend/src/components/MarkdownEditor.spec.js
git commit -m "feat: add Markdown write/preview editor component"
```

---

### Task 7: Note list component

**Files:**
- Create: `frontend/src/components/NoteList.vue`
- Test: `frontend/src/components/NoteList.spec.js`

**Interfaces:**
- Consumes: `useNotesStore` (Task 4).
- Produces: `NoteList` component, emits `select(id)`.

- [ ] **Step 1: Write the failing test**

```js
// frontend/src/components/NoteList.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import NoteList from './NoteList.vue'
import { useNotesStore } from '../stores/notes'

beforeEach(() => {
  vi.clearAllMocks()
})

describe('NoteList', () => {
  it('fetches the list on mount and renders items', async () => {
    const pinia = createTestingPinia({ stubActions: false })
    const wrapper = mount(NoteList, { global: { plugins: [pinia] } })
    const store = useNotesStore()
    store.fetchList = vi.fn().mockImplementation(async () => {
      store.items = [{ id: 1, title: 'First note', author_id: 1, updated_at: '2026-01-01' }]
      store.total = 1
    })

    // Re-mount so onMounted picks up the mocked action.
    const wrapper2 = mount(NoteList, { global: { plugins: [pinia] } })
    await wrapper2.vm.$nextTick()
    await Promise.resolve()
    await wrapper2.vm.$nextTick()

    expect(store.fetchList).toHaveBeenCalled()
    expect(wrapper2.text()).toContain('First note')
    wrapper.unmount()
  })

  it('emits select with the clicked note id', async () => {
    const pinia = createTestingPinia({ stubActions: true })
    const wrapper = mount(NoteList, { global: { plugins: [pinia] } })
    const store = useNotesStore()
    store.items = [{ id: 42, title: 'Clickable', author_id: 1, updated_at: '2026-01-01' }]
    await wrapper.vm.$nextTick()

    await wrapper.find('li').trigger('click')

    expect(wrapper.emitted('select')[0]).toEqual([42])
  })

  it('disables Previous at offset 0 and Next when all results are shown', async () => {
    const pinia = createTestingPinia({ stubActions: true })
    const wrapper = mount(NoteList, { global: { plugins: [pinia] } })
    const store = useNotesStore()
    store.items = [{ id: 1, title: 'Only note', author_id: 1, updated_at: '2026-01-01' }]
    store.total = 1
    store.offset = 0
    await wrapper.vm.$nextTick()

    const buttons = wrapper.findAll('button')
    const prev = buttons.find((b) => b.text() === 'Previous')
    const next = buttons.find((b) => b.text() === 'Next')
    expect(prev.attributes('disabled')).toBeDefined()
    expect(next.attributes('disabled')).toBeDefined()
  })
})
```

- [ ] **Step 2: Add the testing-pinia dev dependency**

```bash
cd frontend && npm install -D @pinia/testing@^0.1.7
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/components/NoteList.spec.js`
Expected: FAIL — component file does not exist.

- [ ] **Step 4: Implement the component**

```vue
<!-- frontend/src/components/NoteList.vue -->
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/components/NoteList.spec.js`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/NoteList.vue frontend/src/components/NoteList.spec.js frontend/package.json frontend/package-lock.json
git commit -m "feat: add note list component with search and pagination"
```

---

### Task 8: Note detail and comment thread components

**Files:**
- Create: `frontend/src/components/CommentList.vue`
- Create: `frontend/src/components/NoteDetail.vue`
- Test: `frontend/src/components/NoteDetail.spec.js`

**Interfaces:**
- Consumes: `api` (Task 3), `useAuthStore` (Task 4), `useCommentsStore` (Task 5), `MarkdownEditor` (Task 6).
- Produces: `NoteDetail` component, prop `{noteId: Number}`, emits `updated(note)`, `deleted(id)`; `CommentList` component, prop `{noteId: Number}`.

- [ ] **Step 1: Write the failing test**

```js
// frontend/src/components/NoteDetail.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import NoteDetail from './NoteDetail.vue'
import { useAuthStore } from '../stores/auth'
import { useCommentsStore } from '../stores/comments'
import { api } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

beforeEach(() => {
  vi.clearAllMocks()
})

function mountDetail(noteId = 1) {
  const pinia = createTestingPinia({ stubActions: false })
  const wrapper = mount(NoteDetail, { props: { noteId }, global: { plugins: [pinia] } })
  return { wrapper, auth: useAuthStore(), comments: useCommentsStore() }
}

describe('NoteDetail', () => {
  it('loads the note, renders it, and fetches comments', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: '# Hi', author_id: 9 })
    api.post.mockResolvedValue({ html: '<h1>Hi</h1>' })
    const { wrapper, comments } = mountDetail(1)
    comments.fetchForNote = vi.fn().mockResolvedValue(undefined)

    await flushAll(wrapper)

    expect(api.get).toHaveBeenCalledWith('/api/notes/1')
    expect(api.post).toHaveBeenCalledWith('/api/render-preview', { content: '# Hi' })
    expect(wrapper.html()).toContain('<h1>Hi</h1>')
  })

  it('shows Edit/Delete only when the current user can edit', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: 'Body', author_id: 9 })
    api.post.mockResolvedValue({ html: '<p>Body</p>' })
    const { wrapper, auth } = mountDetail(1)
    auth.user = { id: 9, role: 'user' }
    auth.canEdit = (authorId) => authorId === 9

    await flushAll(wrapper)

    expect(wrapper.text()).toContain('Edit')
    expect(wrapper.text()).toContain('Delete')
  })

  it('hides Edit/Delete for a non-owner, non-admin viewer', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: 'Body', author_id: 9 })
    api.post.mockResolvedValue({ html: '<p>Body</p>' })
    const { wrapper, auth } = mountDetail(1)
    auth.user = { id: 123, role: 'user' }
    auth.canEdit = (authorId) => authorId === 123

    await flushAll(wrapper)

    expect(wrapper.text()).not.toContain('Edit')
    expect(wrapper.text()).not.toContain('Delete')
  })

  it('emits deleted after a successful delete', async () => {
    api.get.mockResolvedValue({ id: 1, title: 'My Note', content: 'Body', author_id: 9 })
    api.post.mockResolvedValue({ html: '<p>Body</p>' })
    api.delete.mockResolvedValue(null)
    const { wrapper, auth } = mountDetail(1)
    auth.user = { id: 9, role: 'user' }
    auth.canEdit = () => true

    await flushAll(wrapper)
    await wrapper.findAll('button').find((b) => b.text() === 'Delete').trigger('click')
    await flushAll(wrapper)

    expect(api.delete).toHaveBeenCalledWith('/api/notes/1')
    expect(wrapper.emitted('deleted')[0]).toEqual([1])
  })
})

async function flushAll(wrapper) {
  await wrapper.vm.$nextTick()
  await Promise.resolve()
  await wrapper.vm.$nextTick()
  await Promise.resolve()
  await wrapper.vm.$nextTick()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/components/NoteDetail.spec.js`
Expected: FAIL — component files do not exist.

- [ ] **Step 3: Implement `CommentList.vue`**

```vue
<!-- frontend/src/components/CommentList.vue -->
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
```

- [ ] **Step 4: Implement `NoteDetail.vue`**

```vue
<!-- frontend/src/components/NoteDetail.vue -->
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/components/NoteDetail.spec.js`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/CommentList.vue frontend/src/components/NoteDetail.vue frontend/src/components/NoteDetail.spec.js
git commit -m "feat: add note detail view with ownership-gated edit/delete and comment thread"
```

---

### Task 9: Assemble the notes view, app shell, and router

**Files:**
- Modify: `frontend/src/views/NotesView.vue`
- Modify: `frontend/src/App.vue`
- Test: `frontend/src/views/NotesView.spec.js`

**Interfaces:**
- Consumes: `NoteList`, `NoteDetail`, `MarkdownEditor` (Tasks 6–8), `useNotesStore`, `useAuthStore`.

- [ ] **Step 1: Write the failing test**

```js
// frontend/src/views/NotesView.spec.js
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import { createRouter, createWebHistory } from 'vue-router'
import NotesView from './NotesView.vue'

vi.mock('../lib/api', () => ({
  api: { get: vi.fn().mockResolvedValue({ items: [], total: 0 }), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

beforeEach(() => {
  vi.clearAllMocks()
})

async function mountAt(path) {
  const router = createRouter({
    history: createWebHistory(),
    routes: [
      { path: '/notes', name: 'notes', component: NotesView },
      { path: '/notes/new', name: 'note-new', component: NotesView },
      { path: '/notes/:id', name: 'note-detail', component: NotesView },
    ],
  })
  router.push(path)
  await router.isReady()

  const pinia = createTestingPinia({ stubActions: false })
  return mount(NotesView, { global: { plugins: [pinia, router] } })
}

describe('NotesView', () => {
  it('shows a prompt to select a note when nothing is selected', async () => {
    const wrapper = await mountAt('/notes')
    expect(wrapper.text()).toContain('Select a note')
  })

  it('shows the new-note editor on /notes/new', async () => {
    const wrapper = await mountAt('/notes/new')
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.find('input[placeholder="Title"]').exists()).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/views/NotesView.spec.js`
Expected: FAIL — placeholder `NotesView.vue` doesn't match expected markup.

- [ ] **Step 3: Implement the real `NotesView.vue`**

```vue
<!-- frontend/src/views/NotesView.vue -->
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
```

- [ ] **Step 4: Implement the final `App.vue`**

```vue
<!-- frontend/src/App.vue -->
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
```

- [ ] **Step 5: Run the full frontend test suite and the build**

Run: `cd frontend && npm run test`
Expected: PASS (all specs)

Run: `cd frontend && npm run build`
Expected: succeeds

- [ ] **Step 6: Manual verification**

Run: `cd frontend && npm run dev`, then open the printed local URL in a
browser. Note that without the backend (Plan 1+2) and a real OIDC
provider running, `/api/me` will 401 and redirect to `/auth/login`,
which will fail to resolve — this is expected at this stage. Full
browser-driven verification against a real running stack is Plan 6's
Playwright suite; this manual step only confirms the dev server boots
and renders the shell without a console error before that point.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/views/NotesView.vue frontend/src/views/NotesView.spec.js frontend/src/App.vue
git commit -m "feat: assemble notes list/detail/editor view and app shell"
```

---

## Out of scope for this plan (covered by later plans)

- Serving the built frontend from the Go binary (Plan 4).
- Helm/Kubernetes values for API base URL, if ever needed beyond
  same-origin serving (Plan 5).
- Full browser e2e verification of the login redirect and the complete
  user journey (Plan 6).

## Handoff to Plan 4

Plan 4 (`docs/superpowers/plans/2026-10-19-notebook-container-build.md`)
assumes the frontend builds to `frontend/dist/` via `npm run build` and
that `index.html` references assets with root-relative paths (Vite's
default), since it will be served by the Go binary from the same
origin as `/api/*` and `/auth/*` — no separate frontend origin or CORS
configuration exists anywhere in this plan. Once this plan's tasks are
complete and its final whole-branch review has passed, **the reviewer
who signed off on that review must read Plan 4 against what was
actually built** (build output location, `package.json` script names)
and correct any drift before Plan 4 execution begins.
