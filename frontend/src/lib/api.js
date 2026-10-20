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
