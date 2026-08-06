import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiFetch, configureApiClient } from './client'

function mockResponse(options: {
  ok: boolean
  status: number
  statusText?: string
  contentType?: string
  body?: string
}): Response {
  const headers = new Headers()
  if (options.contentType !== undefined) {
    headers.set('content-type', options.contentType)
  }
  return {
    ok: options.ok,
    status: options.status,
    statusText: options.statusText ?? '',
    headers,
    text: () => Promise.resolve(options.body ?? ''),
    json: () => Promise.resolve(options.body !== undefined ? JSON.parse(options.body) : undefined),
  } as unknown as Response
}

describe('apiFetch', () => {
  let fetchSpy: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('always sends credentials: include', async () => {
    fetchSpy.mockResolvedValue(
      mockResponse({ ok: true, status: 204 }),
    )
    await apiFetch('/api/session', { method: 'DELETE' })
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/session',
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('returns undefined for 204 No Content', async () => {
    fetchSpy.mockResolvedValue(mockResponse({ ok: true, status: 204 }))
    const result = await apiFetch('/api/session', { method: 'DELETE' })
    expect(result).toBeUndefined()
  })

  it('returns undefined for 201 with empty body', async () => {
    fetchSpy.mockResolvedValue(mockResponse({ ok: true, status: 201, body: '' }))
    const result = await apiFetch('/api/setup', { method: 'POST' })
    expect(result).toBeUndefined()
  })

  it('parses a JSON success body', async () => {
    fetchSpy.mockResolvedValue(
      mockResponse({ ok: true, status: 200, contentType: 'application/json', body: '{"site_name":"foo"}' }),
    )
    const result = await apiFetch<{ site_name: string }>('/api/settings')
    expect(result).toEqual({ site_name: 'foo' })
  })

  it('normalizes a JSON error body into ApiError', async () => {
    fetchSpy.mockResolvedValue(
      mockResponse({
        ok: false,
        status: 400,
        contentType: 'application/json',
        body: '{"error":"invalid slug"}',
      }),
    )
    await expect(apiFetch('/api/posts', { method: 'POST' })).rejects.toMatchObject({
      status: 400,
      message: 'invalid slug',
    })
  })

  it('normalizes a plain-text error body into ApiError and trims trailing newline', async () => {
    fetchSpy.mockResolvedValue(
      mockResponse({
        ok: false,
        status: 401,
        contentType: 'text/plain; charset=utf-8',
        body: 'unauthorized\n',
      }),
    )
    let caught: unknown
    try {
      await apiFetch('/api/posts')
    } catch (err) {
      caught = err
    }
    expect(caught).toBeInstanceOf(ApiError)
    expect((caught as ApiError).status).toBe(401)
    expect((caught as ApiError).message).toBe('unauthorized')
  })

  it('on 401, clears the queryClient and calls onUnauthorized', async () => {
    fetchSpy.mockResolvedValue(
      mockResponse({ ok: false, status: 401, contentType: 'text/plain', body: 'unauthorized\n' }),
    )
    const clear = vi.fn()
    const onUnauthorized = vi.fn()
    configureApiClient({
      queryClient: { clear } as unknown as import('@tanstack/react-query').QueryClient,
      onUnauthorized,
    })

    await expect(apiFetch('/api/posts')).rejects.toBeInstanceOf(ApiError)
    expect(clear).toHaveBeenCalledTimes(1)
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
  })

  it('on 401 with skipUnauthorizedRedirect, does not clear the queryClient or call onUnauthorized', async () => {
    // POST /api/session (bad password) and POST /api/setup (bad token) also
    // return 401, but that's not a "session expired" event -- the calling
    // form shows its own inline error. Regression test for the bug found
    // during T7 review: login()'s 401 was silently firing the global
    // logout-redirect handler underneath the form's own error handling.
    fetchSpy.mockResolvedValue(
      mockResponse({ ok: false, status: 401, contentType: 'application/json', body: '{"error":"unauthorized"}' }),
    )
    const clear = vi.fn()
    const onUnauthorized = vi.fn()
    configureApiClient({
      queryClient: { clear } as unknown as import('@tanstack/react-query').QueryClient,
      onUnauthorized,
    })

    await expect(
      apiFetch('/api/session', { method: 'POST', skipUnauthorizedRedirect: true }),
    ).rejects.toBeInstanceOf(ApiError)
    expect(clear).not.toHaveBeenCalled()
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it('does not trigger onUnauthorized for non-401 errors', async () => {
    fetchSpy.mockResolvedValue(
      mockResponse({ ok: false, status: 404, contentType: 'application/json', body: '{"error":"post not found"}' }),
    )
    const clear = vi.fn()
    const onUnauthorized = vi.fn()
    configureApiClient({
      queryClient: { clear } as unknown as import('@tanstack/react-query').QueryClient,
      onUnauthorized,
    })

    await expect(apiFetch('/api/posts/x')).rejects.toBeInstanceOf(ApiError)
    expect(clear).not.toHaveBeenCalled()
    expect(onUnauthorized).not.toHaveBeenCalled()
  })
})
