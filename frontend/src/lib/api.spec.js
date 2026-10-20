import { describe, it, expect, vi, beforeEach } from 'vitest'
import { apiFetch, ApiError, api } from './api'

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

  it('api.get calls apiFetch with GET method', async () => {
    mockFetchOnce({ data: 'test' }, { status: 200 })
    const result = await api.get('/api/notes')
    const [, options] = global.fetch.mock.calls[0]
    expect(options.method).toBeUndefined() // GET is default
    expect(result).toEqual({ data: 'test' })
  })

  it('api.post calls apiFetch with POST method and stringified body', async () => {
    mockFetchOnce({ id: 1 }, { status: 200 })
    const body = { title: 'Test Note', content: 'Test' }
    await api.post('/api/notes', body)
    const [, options] = global.fetch.mock.calls[0]
    expect(options.method).toBe('POST')
    expect(options.body).toBe(JSON.stringify(body))
  })

  it('api.put calls apiFetch with PUT method and stringified body', async () => {
    mockFetchOnce({ id: 1, title: 'Updated' }, { status: 200 })
    const body = { title: 'Updated' }
    await api.put('/api/notes/1', body)
    const [, options] = global.fetch.mock.calls[0]
    expect(options.method).toBe('PUT')
    expect(options.body).toBe(JSON.stringify(body))
  })

  it('api.delete calls apiFetch with DELETE method', async () => {
    mockFetchOnce(undefined, { status: 204 })
    await api.delete('/api/notes/1')
    const [, options] = global.fetch.mock.calls[0]
    expect(options.method).toBe('DELETE')
  })

  it('throws ApiError with fallback message on non-ok response with non-JSON body', async () => {
    global.fetch = vi.fn().mockResolvedValue(
      new Response('Internal Server Error', { status: 500 })
    )
    await expect(apiFetch('/api/notes')).rejects.toThrow(
      'request failed with status 500'
    )
  })
})
